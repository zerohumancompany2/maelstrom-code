#!/usr/bin/env python3
"""Dedicated runner for autoresearch benchmark workflows.

This script provides a stable entrypoint inside `docs/experiments/autoresearch/`
for listing benchmarks, selecting raw run artifacts, scoring benchmark suites,
and optionally saving results locally.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
SCORE_SCRIPT = HERE / "score_benchmarks.py"
BENCHMARK_DIR = HERE / "benchmarks"
RAW_DATA_DIR = ROOT / "docs" / "experiments" / "raw_data"
RESULTS_DIR = HERE / "results"


SUITES: dict[str, list[str]] = {
    "north_star": [
        "discovery.context_inference.evidence_sufficiency.v1",
        "discovery.context_inference.runner_evidence_sufficiency.v1",
        "control.repo_inspection.forced_report_obedience.v1",
        "format.repo_inspection.schema_validity.v1",
        "control.repo_inspection.repair_obedience.v1",
    ],
    "tier1": [
        "discovery.context_inference.required_files.v1",
        "discovery.context_inference.test_coverage.v1",
        "discovery.context_inference.adjacent_integration.v1",
        "format.repo_inspection.schema_validity.v1",
        "control.repo_inspection.forced_report_obedience.v1",
        "control.repo_inspection.repair_obedience.v1",
    ],
    "tier2": [
        "synthesis.context_inference.mechanism_summary.v1",
        "synthesis.context_inference.issue_finding.v1",
        "workflow.guidance_vs_explicit_state.task_focus.v1",
        "workflow.tool_policy_compliance.v1",
    ],
}
SUITES["all"] = SUITES["tier1"] + SUITES["tier2"]


def list_benchmark_ids() -> list[str]:
    ids = []
    for path in sorted(BENCHMARK_DIR.glob("*.yaml")):
        text = path.read_text(encoding="utf-8")
        for line in text.splitlines():
            if line.startswith("id:"):
                ids.append(line.split(":", 1)[1].strip())
                break
    return ids


def raw_run_paths(
    mode: str | None = None, prompt_substring: str | None = None
) -> list[Path]:
    paths = sorted(RAW_DATA_DIR.glob("*.json"))
    selected = []
    for path in paths:
        raw = json.loads(path.read_text(encoding="utf-8"))
        meta = raw.get("metadata") or {}
        if mode and meta.get("mode") != mode:
            continue
        if prompt_substring and prompt_substring not in str(meta.get("prompt", "")):
            continue
        selected.append(path)
    return selected


def validate_raw_runs(paths: list[Path]) -> list[Path]:
    valid = []
    for path in paths:
        if path.exists() and path.is_file():
            valid.append(path)
    return valid


def run_score_script(
    benchmarks: list[str], raw_runs: list[Path]
) -> list[dict[str, Any]]:
    all_results = []
    for benchmark in benchmarks:
        cmd = [sys.executable, str(SCORE_SCRIPT), "--benchmark", benchmark]
        for raw_run in raw_runs:
            cmd.extend(["--raw-run", str(raw_run)])
        proc = subprocess.run(cmd, capture_output=True, text=True)
        if proc.returncode != 0:
            raise RuntimeError(
                f"score_benchmarks.py failed for {benchmark}:\nSTDOUT:\n{proc.stdout}\nSTDERR:\n{proc.stderr}"
            )
        payload = json.loads(proc.stdout)
        all_results.extend(payload)
    return all_results


def summarize_results(results: list[dict[str, Any]]) -> dict[str, Any]:
    benchmark_summaries = []
    total_evaluations = 0
    total_passed = 0
    north_star_evaluations = 0
    north_star_passed = 0
    guardrail_failures: dict[str, int] = {}
    for result in results:
        evaluations = result.get("evaluations") or []
        passed = sum(1 for e in evaluations if e.get("passed"))
        total = len(evaluations)
        total_evaluations += total
        total_passed += passed
        benchmark_id = result.get("benchmark")
        if benchmark_id and benchmark_id.endswith("evidence_sufficiency.v1"):
            north_star_evaluations += total
            north_star_passed += passed
        elif benchmark_id:
            failed = total - passed
            if failed > 0:
                guardrail_failures[benchmark_id] = failed
        benchmark_summaries.append(
            {
                "benchmark": benchmark_id,
                "passed": passed,
                "total": total,
                "pass_rate": (passed / total) if total else None,
            }
        )
    return {
        "total_benchmarks": len(results),
        "total_evaluations": total_evaluations,
        "total_passed": total_passed,
        "overall_pass_rate": (total_passed / total_evaluations)
        if total_evaluations
        else None,
        "north_star_metric": "evidence_sufficiency_pass",
        "north_star_benchmark_family": "*.evidence_sufficiency.v1",
        "north_star_passed": north_star_passed,
        "north_star_evaluations": north_star_evaluations,
        "north_star_pass_rate": (north_star_passed / north_star_evaluations)
        if north_star_evaluations
        else None,
        "guardrail_failures": guardrail_failures,
        "benchmarks": benchmark_summaries,
    }


def save_results(payload: dict[str, Any], output_path: Path | None = None) -> Path:
    RESULTS_DIR.mkdir(parents=True, exist_ok=True)
    if output_path is None:
        stamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H-%M-%SZ")
        output_path = RESULTS_DIR / f"benchmark-run-{stamp}.json"
    output_path.write_text(json.dumps(payload, indent=2), encoding="utf-8")
    return output_path


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Dedicated runner for autoresearch benchmark suites"
    )
    parser.add_argument(
        "--list-benchmarks", action="store_true", help="List benchmark ids"
    )
    parser.add_argument(
        "--list-runs", action="store_true", help="List selected raw runs"
    )
    parser.add_argument(
        "--suite",
        choices=sorted(SUITES.keys()),
        default=None,
        help="Run a named benchmark suite",
    )
    parser.add_argument(
        "--benchmark", action="append", default=[], help="Run one or more benchmark ids"
    )
    parser.add_argument("--mode", default=None, help="Filter raw runs by mode")
    parser.add_argument(
        "--prompt-substring", default=None, help="Filter raw runs by prompt substring"
    )
    parser.add_argument(
        "--raw-run",
        action="append",
        default=[],
        help="Specific raw run JSON path(s) to score",
    )
    parser.add_argument(
        "--save",
        action="store_true",
        help="Save results to docs/experiments/autoresearch/results",
    )
    parser.add_argument(
        "--output", default=None, help="Optional output path for saved JSON results"
    )
    args = parser.parse_args()

    if args.list_benchmarks:
        for benchmark_id in list_benchmark_ids():
            print(benchmark_id)
        return

    if args.raw_run:
        selected_runs = validate_raw_runs([Path(p).resolve() for p in args.raw_run])
    else:
        selected_runs = raw_run_paths(
            mode=args.mode, prompt_substring=args.prompt_substring
        )
    if args.list_runs:
        for path in selected_runs:
            print(path)
        return

    benchmarks: list[str] = []
    if args.suite:
        benchmarks.extend(SUITES[args.suite])
    benchmarks.extend(args.benchmark)
    benchmarks = list(dict.fromkeys(benchmarks))

    if not benchmarks:
        raise SystemExit("No benchmarks selected. Use --suite or --benchmark.")
    if not selected_runs:
        raise SystemExit(
            "No raw runs selected. Adjust --mode/--prompt-substring or add run artifacts."
        )

    results = run_score_script(benchmarks, selected_runs)
    summary = summarize_results(results)
    payload = {
        "metadata": {
            "generated_at": datetime.now(timezone.utc).isoformat(),
            "runner": str(Path(__file__).relative_to(ROOT)),
            "selected_benchmarks": benchmarks,
            "selected_raw_runs": [str(p.relative_to(ROOT)) for p in selected_runs],
            "mode_filter": args.mode,
            "prompt_filter": args.prompt_substring,
        },
        "summary": summary,
        "results": results,
    }

    if args.save:
        output_path = Path(args.output).resolve() if args.output else None
        saved = save_results(payload, output_path)
        payload["saved_to"] = str(saved)

    print(json.dumps(payload, indent=2))


if __name__ == "__main__":
    main()
