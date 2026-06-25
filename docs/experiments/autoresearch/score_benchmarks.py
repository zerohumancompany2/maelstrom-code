#!/usr/bin/env python3
"""Score benchmark specs against saved raw experiment artifacts.

This is an initial scorer/runner skeleton for the benchmark specs in
`docs/experiments/autoresearch/benchmarks/`.

Current scope:
- loads YAML benchmark specs
- loads raw run JSON artifacts from docs/experiments/raw_data
- derives a small common metric set from current bounded_task_runner artifacts
- evaluates pass conditions for a benchmark against one or more raw runs
- prints JSON output suitable for later automation

The implementation is intentionally conservative and only supports metrics that
can be computed reliably from the current experiment artifacts.
"""

from __future__ import annotations

import argparse
import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

try:
    import yaml
except ImportError as exc:  # pragma: no cover
    raise SystemExit(
        "PyYAML is required to run this scorer. Install it in the environment first."
    ) from exc


ROOT = Path(__file__).resolve().parents[3]
BENCHMARK_DIR = ROOT / "docs" / "experiments" / "autoresearch" / "benchmarks"
RAW_DATA_DIR = ROOT / "docs" / "experiments" / "raw_data"


@dataclass
class RunContext:
    path: Path
    raw: dict[str, Any]
    metadata: dict[str, Any]
    events: list[dict[str, Any]]
    metrics_blob: dict[str, Any]
    task_artifacts: dict[str, dict[str, Any]]
    assistant_messages: list[dict[str, Any]]
    tool_calls: list[dict[str, Any]]


def load_yaml(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8") as f:
        return yaml.safe_load(f)


def load_json(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8") as f:
        return json.load(f)


def benchmark_paths() -> list[Path]:
    return sorted(BENCHMARK_DIR.glob("*.yaml"))


def raw_run_paths() -> list[Path]:
    return sorted(RAW_DATA_DIR.glob("*.json"))


def index_task_artifacts(events: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    artifacts: dict[str, dict[str, Any]] = {}
    for event in events:
        if event.get("type") == "task_complete":
            task = event.get("task")
            artifact = event.get("artifact")
            if isinstance(task, str) and isinstance(artifact, dict):
                artifacts[task] = artifact
    return artifacts


def build_run_context(path: Path) -> RunContext:
    raw = load_json(path)
    metadata = raw.get("metadata") or {}
    events = raw.get("events") or []
    metrics_blob = raw.get("metrics") or {}
    assistant_messages = [e for e in events if e.get("type") == "assistant_message"]
    tool_calls = [e for e in events if e.get("type") == "tool_call"]
    task_artifacts = index_task_artifacts(events)
    return RunContext(
        path=path,
        raw=raw,
        metadata=metadata,
        events=events,
        metrics_blob=metrics_blob,
        task_artifacts=task_artifacts,
        assistant_messages=assistant_messages,
        tool_calls=tool_calls,
    )


def normalize_path(value: str) -> str:
    return value.strip().replace("\\", "/")


def unique_files_read(ctx: RunContext) -> list[str]:
    files = ctx.metrics_blob.get("unique_files_read") or []
    normalized = [normalize_path(x) for x in files if isinstance(x, str)]
    if normalized:
        return normalized
    inferred = []
    for event in ctx.tool_calls:
        if event.get("tool") != "read_file":
            continue
        args = event.get("args") or {}
        path = normalize_path(str(args.get("path", "")))
        if path and path not in inferred:
            inferred.append(path)
    return inferred


def matching_reads(paths: list[str], expected: list[str]) -> list[str]:
    expected_set = {normalize_path(x) for x in expected}
    return [p for p in paths if p in expected_set]


def count_tool_calls(ctx: RunContext) -> int:
    value = ctx.metrics_blob.get("tool_calls")
    if value is None:
        return len(ctx.tool_calls)
    return int(value or 0)


def discovery_precision(ctx: RunContext, spec: dict[str, Any]) -> float | None:
    reads = unique_files_read(ctx)
    if not reads:
        return None
    relevant = set()
    for key in ("required_files", "useful_files", "adjacent_files", "required_tests"):
        for path in spec.get(key) or []:
            relevant.add(normalize_path(path))
    relevant_reads = sum(1 for p in reads if p in relevant)
    return relevant_reads / len(reads)


def first_read_turn(ctx: RunContext, expected_paths: list[str]) -> int | None:
    expected = {normalize_path(p) for p in expected_paths}
    for event in ctx.tool_calls:
        if event.get("tool") != "read_file":
            continue
        args = event.get("args") or {}
        path = normalize_path(str(args.get("path", "")))
        if path in expected:
            turn = event.get("turn")
            if isinstance(turn, int):
                return turn
    return None


def count_post_event_tool_calls(ctx: RunContext, event_type: str) -> int:
    trigger_index = None
    for idx, event in enumerate(ctx.events):
        if event.get("type") == event_type:
            trigger_index = idx
            break
    if trigger_index is None:
        return 0
    count = 0
    for event in ctx.events[trigger_index + 1 :]:
        if event.get("type") == "tool_call":
            count += 1
    return count


def assistant_message_after_turn(
    ctx: RunContext, turn_label: str
) -> dict[str, Any] | None:
    for event in ctx.assistant_messages:
        if event.get("turn") == turn_label:
            return event
    return None


def parse_json_object(text: str) -> dict[str, Any] | None:
    text = text.strip()
    try:
        parsed = json.loads(text)
        return parsed if isinstance(parsed, dict) else None
    except Exception:
        pass
    match = re.search(r"\{.*\}", text, re.DOTALL)
    if not match:
        return None
    try:
        parsed = json.loads(match.group(0))
        return parsed if isinstance(parsed, dict) else None
    except Exception:
        return None


def infer_model_turns(ctx: RunContext) -> int:
    value = ctx.metrics_blob.get("model_turns")
    if value is not None:
        return int(value or 0)
    count = 0
    for event in ctx.events:
        if event.get("type") == "turn_start":
            count += 1
        elif event.get("type") == "assistant_message" and event.get("turn") in {
            "forced_report",
            "coverage_followup",
            "coverage_followup_report",
            "forced_report_repair",
        }:
            count += 1
    return count


def infer_repair_turns(ctx: RunContext) -> int:
    value = ctx.metrics_blob.get("repair_turns")
    if value is not None:
        return int(value or 0)
    return sum(
        1
        for e in ctx.events
        if e.get("type") in {"runtime_repair", "runtime_coverage_gate"}
    )


def infer_forced_reports(ctx: RunContext) -> int:
    value = ctx.metrics_blob.get("forced_reports")
    if value is not None:
        return int(value or 0)
    return sum(1 for e in ctx.events if e.get("type") == "runtime_forced_report")


def infer_completed(ctx: RunContext) -> bool:
    value = ctx.metrics_blob.get("completed")
    if value is not None:
        return bool(value)
    return str(ctx.metadata.get("status", "")).lower() == "completed"


def schema_validation_metrics(ctx: RunContext, spec: dict[str, Any]) -> dict[str, Any]:
    schema = spec.get("schema") or {}
    required = schema.get("required") or []
    properties = schema.get("properties") or {}

    forced = assistant_message_after_turn(ctx, "forced_report")
    repair = assistant_message_after_turn(ctx, "forced_report_repair")

    def validate_message(event: dict[str, Any] | None) -> tuple[int, int, int, int]:
        if not event:
            return 0, len(required), 0, 0
        content = str(event.get("content", ""))
        extra_text = 0
        if not content.lstrip().startswith("{"):
            extra_text = 1
        parsed = parse_json_object(content)
        if not parsed:
            return 0, len(required), 0, extra_text
        missing = 0
        wrong_types = 0
        for key in required:
            if key not in parsed:
                missing += 1
        for key, prop in properties.items():
            if key not in parsed:
                continue
            expected_type = prop.get("type")
            value = parsed[key]
            if expected_type == "array" and not isinstance(value, list):
                wrong_types += 1
            elif expected_type == "boolean" and not isinstance(value, bool):
                wrong_types += 1
            elif expected_type == "string" and not isinstance(value, str):
                wrong_types += 1
            if expected_type == "array" and isinstance(value, list):
                item_type = (prop.get("items") or {}).get("type")
                if item_type == "string" and any(not isinstance(x, str) for x in value):
                    wrong_types += 1
        valid = int(missing == 0 and wrong_types == 0 and extra_text == 0)
        return valid, missing, wrong_types, extra_text

    valid_first_try, missing_first, wrong_first, extra_first = validate_message(forced)
    valid_repair, _, _, _ = validate_message(repair)

    return {
        "valid_first_try": valid_first_try,
        "valid_after_one_repair": int(valid_first_try or valid_repair),
        "missing_fields_count": missing_first,
        "wrong_type_count": wrong_first,
        "extra_text_present": extra_first,
    }


def control_metrics(ctx: RunContext) -> dict[str, Any]:
    forced_report = any(e.get("type") == "runtime_forced_report" for e in ctx.events)
    forced_content = assistant_message_after_turn(ctx, "forced_report")
    repair_content = assistant_message_after_turn(ctx, "forced_report_repair")
    coverage_followup_content = assistant_message_after_turn(
        ctx, "coverage_followup_report"
    )

    repaired_in_one_turn = int(repair_content is not None)
    off_protocol_after_repair = 0
    repair_seen = False
    for event in ctx.events:
        if event.get("type") == "runtime_repair":
            repair_seen = True
            continue
        if repair_seen and event.get("type") == "tool_call":
            off_protocol_after_repair += 1

    return {
        "post_boundary_tool_calls": count_post_event_tool_calls(
            ctx, "runtime_forced_report"
        ),
        "report_emitted_after_boundary": int(
            forced_report and forced_content is not None
        ),
        "repaired_in_one_turn": repaired_in_one_turn,
        "off_protocol_tool_calls_after_repair": off_protocol_after_repair,
        "repair_protocol_violations": int(off_protocol_after_repair > 0),
        "repair_followup_report_present": int(coverage_followup_content is not None),
    }


def compute_metrics(ctx: RunContext, spec: dict[str, Any]) -> dict[str, Any]:
    metrics: dict[str, Any] = {}
    reads = unique_files_read(ctx)

    required_files = spec.get("required_files") or []
    useful_files = spec.get("useful_files") or []
    adjacent_files = spec.get("adjacent_files") or []
    required_tests = spec.get("required_tests") or []

    if required_files:
        required_read = matching_reads(reads, required_files)
        metrics["required_files_read"] = sorted(required_read)
        metrics["required_recall"] = len(required_read) / len(required_files)
        frt = first_read_turn(ctx, required_files)
        if frt is not None:
            metrics["first_required_read_turn"] = frt

    if useful_files:
        useful_read = matching_reads(reads, useful_files)
        metrics["useful_files_read"] = sorted(useful_read)
        metrics["useful_recall"] = len(useful_read) / len(useful_files)

    if adjacent_files:
        adjacent_read = matching_reads(reads, adjacent_files)
        metrics["adjacent_files_read"] = sorted(adjacent_read)
        metrics["adjacent_recall"] = len(adjacent_read) / len(adjacent_files)
        fat = first_read_turn(ctx, adjacent_files)
        if fat is not None:
            metrics["turns_to_first_adjacent_read"] = fat

    if required_tests:
        tests_read = matching_reads(reads, required_tests)
        metrics["required_tests_read"] = sorted(tests_read)
        metrics["test_recall"] = len(tests_read) / len(required_tests)
        ftt = first_read_turn(ctx, required_tests)
        if ftt is not None:
            metrics["tool_calls_until_first_test_read"] = ftt

    precision = discovery_precision(ctx, spec)
    if precision is not None:
        metrics["discovery_precision"] = precision

    metrics["tool_calls"] = count_tool_calls(ctx)
    metrics["model_turns"] = infer_model_turns(ctx)
    metrics["repair_turns"] = infer_repair_turns(ctx)
    metrics["forced_reports"] = infer_forced_reports(ctx)
    metrics["completed"] = infer_completed(ctx)
    metrics["mode"] = ctx.metadata.get("mode")
    metrics["prompt"] = ctx.metadata.get("prompt")

    task_name = (spec.get("setup") or {}).get("task_name")
    if task_name and task_name in ctx.task_artifacts:
        artifact = ctx.task_artifacts[task_name]
        metrics["task_artifact"] = artifact

    if spec.get("category") == "format":
        metrics.update(schema_validation_metrics(ctx, spec))
    if spec.get("category") == "control":
        metrics.update(control_metrics(ctx))

    if spec.get("id") == "interpretation.repo_inspection_needed.context_inference":
        artifact = ctx.task_artifacts.get("understand_request") or {}
        value = artifact.get("repo_inspection_needed")
        metrics["repo_inspection_needed"] = value
        metrics["binary_correctness"] = int(value is True)

    if spec.get("sub_category") == "evidence_sufficiency":
        required_recall = metrics.get("required_recall")
        test_recall = metrics.get("test_recall")
        adjacent_recall = metrics.get("adjacent_recall")
        report_emitted = metrics.get("report_emitted_after_boundary")
        completed = metrics.get("completed")

        if required_recall is not None:
            metrics["evidence_required_coverage_pass"] = int(required_recall >= 1.0)
        if test_recall is not None:
            metrics["evidence_test_coverage_pass"] = int(test_recall >= 1.0)
        if adjacent_recall is not None:
            metrics["evidence_adjacent_coverage_pass"] = int(
                adjacent_recall >= 0.333333
            )

        if completed is not None:
            metrics["report_completed_pass"] = int(bool(completed))
        if report_emitted is not None:
            metrics["report_emitted_after_boundary_pass"] = int(report_emitted == 1)

        evidence_components = []
        for key in (
            "evidence_required_coverage_pass",
            "evidence_test_coverage_pass",
            "evidence_adjacent_coverage_pass",
            "report_completed_pass",
        ):
            value = metrics.get(key)
            if value is not None:
                evidence_components.append(int(value))

        if evidence_components:
            metrics["evidence_sufficiency_score"] = sum(evidence_components) / len(
                evidence_components
            )
            metrics["evidence_sufficiency_pass"] = int(
                all(v == 1 for v in evidence_components)
            )

    return metrics


def compare(actual: Any, op: str, expected: Any) -> bool:
    if op == "==":
        return actual == expected
    if op == ">=":
        return actual is not None and actual >= expected
    if op == "<=":
        return actual is not None and actual <= expected
    if op == ">":
        return actual is not None and actual > expected
    if op == "<":
        return actual is not None and actual < expected
    raise ValueError(f"unsupported operator: {op}")


def evaluate_clause(metrics: dict[str, Any], clause: dict[str, Any]) -> bool:
    metric = clause["metric"]
    op = clause["op"]
    expected = clause["value"]
    actual = metrics.get(metric)
    return compare(actual, op, expected)


def evaluate_pass_condition(
    spec: dict[str, Any], metrics: dict[str, Any]
) -> tuple[bool, dict[str, Any]]:
    condition = spec.get("pass_condition") or {}
    if not condition:
        return False, {"reason": "no_pass_condition_defined"}

    if "all" in condition:
        clauses = condition["all"]
        results = [evaluate_clause(metrics, clause) for clause in clauses]
        return all(results), {"logic": "all", "results": results, "clauses": clauses}

    if "any" in condition:
        branches = condition["any"]
        branch_results = []
        for branch in branches:
            if "all" in branch:
                clauses = branch["all"]
                results = [evaluate_clause(metrics, clause) for clause in clauses]
                branch_results.append(
                    {"clauses": clauses, "results": results, "passed": all(results)}
                )
            else:
                branch_results.append(
                    {"passed": False, "reason": "unsupported_any_branch"}
                )
        return any(b.get("passed") for b in branch_results), {
            "logic": "any",
            "branches": branch_results,
        }

    return False, {"reason": "unsupported_pass_condition"}


def run_single_benchmark(spec_path: Path, raw_paths: list[Path]) -> dict[str, Any]:
    spec = load_yaml(spec_path)
    evaluations = []
    for raw_path in raw_paths:
        ctx = build_run_context(raw_path)
        metrics = compute_metrics(ctx, spec)
        passed, details = evaluate_pass_condition(spec, metrics)
        evaluations.append(
            {
                "raw_run": str(raw_path.relative_to(ROOT)),
                "mode": ctx.metadata.get("mode"),
                "status": ctx.metadata.get("status"),
                "passed": passed,
                "metrics": metrics,
                "pass_details": details,
            }
        )

    return {
        "benchmark": spec.get("id"),
        "spec_path": str(spec_path.relative_to(ROOT)),
        "evaluations": evaluations,
    }


def select_raw_runs(
    all_runs: list[Path], mode: str | None, prompt_substring: str | None
) -> list[Path]:
    selected = []
    for path in all_runs:
        raw = load_json(path)
        metadata = raw.get("metadata") or {}
        if mode and metadata.get("mode") != mode:
            continue
        if prompt_substring and prompt_substring not in str(metadata.get("prompt", "")):
            continue
        selected.append(path)
    return selected


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Score benchmark specs against raw run artifacts"
    )
    parser.add_argument(
        "--benchmark", help="Benchmark id or spec filename", default=None
    )
    parser.add_argument(
        "--raw-run", action="append", default=[], help="Specific raw run JSON path(s)"
    )
    parser.add_argument("--mode", default=None, help="Filter raw runs by mode")
    parser.add_argument(
        "--prompt-substring", default=None, help="Filter raw runs by prompt substring"
    )
    parser.add_argument(
        "--list-benchmarks", action="store_true", help="List available benchmark ids"
    )
    args = parser.parse_args()

    specs = benchmark_paths()
    if args.list_benchmarks:
        for path in specs:
            spec = load_yaml(path)
            print(spec.get("id", path.name))
        return

    if args.benchmark:
        matched = []
        for path in specs:
            if path.name == args.benchmark:
                matched.append(path)
                continue
            spec = load_yaml(path)
            if spec.get("id") == args.benchmark:
                matched.append(path)
        if not matched:
            raise SystemExit(f"No benchmark matched: {args.benchmark}")
        specs = matched

    if args.raw_run:
        raw_paths = [Path(p).resolve() for p in args.raw_run]
    else:
        raw_paths = select_raw_runs(raw_run_paths(), args.mode, args.prompt_substring)

    if not raw_paths:
        raise SystemExit("No raw run artifacts selected")

    results = [run_single_benchmark(spec_path, raw_paths) for spec_path in specs]
    print(json.dumps(results, indent=2))


if __name__ == "__main__":
    main()
