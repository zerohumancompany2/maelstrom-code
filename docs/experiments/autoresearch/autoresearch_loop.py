#!/usr/bin/env python3
"""Variant-aware autoresearch loop for prompt/control experiments.

This loop evaluates bounded-task runner variants against benchmark suites and
records keep/discard decisions. It currently focuses on prompt/control-surface
experiments rather than arbitrary code mutation.

Capabilities:
- evaluate a baseline variant plus candidate variants from variants.yaml
- support iteration bound and wall-clock time bound
- save per-evaluation benchmark result bundles
- append keep/discard decisions to a TSV ledger
- keep the current best variant according to benchmark pass rate and tie-breakers

Current mutation surface is intentionally narrow:
- bounded-task system prompt append
- coverage repair message template override

The loop works by creating temporary environment overrides consumed by
bounded_task_runner.py when generating new raw runs.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

try:
    import yaml
except ImportError as exc:  # pragma: no cover
    raise SystemExit("PyYAML is required to run autoresearch_loop.py") from exc


HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
RUNNER = HERE / "run_benchmarks.py"
BOUNDED_TASK_RUNNER = ROOT / "docs" / "experiments" / "bounded_task_runner.py"
VARIANTS_PATH = HERE / "variants.yaml"
RESULTS_DIR = HERE / "results"
LEDGER_PATH = HERE / "autoresearch_results.tsv"
PROMPT = (
    "Review our implementation of context ofr inferrence in sketch/sketch7 and report how it functions. "
    "List any issues you see. Make recommendations on architectural improvements."
)


@dataclass
class Variant:
    id: str
    kind: str
    description: str
    mode: str
    overrides: dict[str, Any]


@dataclass
class LoopConfig:
    suite: str
    repo_root: Path
    hours: float | None
    max_iterations: int | None
    variant_ids: list[str]
    notes: str
    model: str | None
    trials_per_variant: int


def load_variants() -> list[Variant]:
    payload = yaml.safe_load(VARIANTS_PATH.read_text(encoding="utf-8")) or {}
    variants = []
    for item in payload.get("variants") or []:
        variants.append(
            Variant(
                id=item["id"],
                kind=item.get("kind", "bounded_task_runner_config"),
                description=item.get("description", ""),
                mode=item.get("mode", "bounded_v3"),
                overrides=item.get("overrides") or {},
            )
        )
    return variants


def ensure_ledger() -> None:
    if LEDGER_PATH.exists():
        return
    LEDGER_PATH.write_text(
        "timestamp_utc\titeration\tvariant_id\tdecision\tsuite\tmode\toverall_pass_rate\ttotal_passed\ttotal_evaluations\tresults_path\tnotes\n",
        encoding="utf-8",
    )


def append_ledger_row(
    iteration: int, variant: Variant, decision: str, payload: dict[str, Any], notes: str
) -> None:
    ensure_ledger()
    summary = payload.get("summary") or {}
    row = [
        datetime.now(timezone.utc).isoformat(),
        str(iteration),
        variant.id,
        decision,
        payload.get("metadata", {}).get("selected_suite", ""),
        variant.mode,
        str(summary.get("overall_pass_rate", "")),
        str(summary.get("total_passed", "")),
        str(summary.get("total_evaluations", "")),
        str(payload.get("saved_to", "")),
        (
            notes
            + f" north_star_pass_rate={summary.get('north_star_pass_rate', '')}"
            + f" trials={payload.get('metadata', {}).get('trials_per_variant', '')}"
        )
        .replace("\t", " ")
        .replace("\n", " "),
    ]
    with LEDGER_PATH.open("a", encoding="utf-8") as f:
        f.write("\t".join(row) + "\n")


def append_failure_ledger_row(
    iteration: int, variant: Variant, suite: str, notes: str, error: str
) -> None:
    ensure_ledger()
    row = [
        datetime.now(timezone.utc).isoformat(),
        str(iteration),
        variant.id,
        "failed",
        suite,
        variant.mode,
        "",
        "",
        "",
        "",
        (notes + " error=" + error).replace("\t", " ").replace("\n", " "),
    ]
    with LEDGER_PATH.open("a", encoding="utf-8") as f:
        f.write("\t".join(row) + "\n")


def variant_env(variant: Variant, model: str | None) -> dict[str, str]:
    env = dict(os.environ)
    if model:
        env["OPENAI_MODEL"] = model
    env["MAELSTROM_EXPERIMENT_SYSTEM_PROMPT_APPEND"] = str(
        variant.overrides.get("system_prompt_append", "")
    )
    env["MAELSTROM_EXPERIMENT_COVERAGE_REPAIR_TEMPLATE"] = str(
        variant.overrides.get("coverage_repair_message_template", "")
    )
    return env


def run_variant_experiment(config: LoopConfig, variant: Variant) -> tuple[Path, str]:
    env = variant_env(variant, config.model)
    cmd = [
        sys.executable,
        str(BOUNDED_TASK_RUNNER),
        "--mode",
        variant.mode,
        str(config.repo_root),
        PROMPT,
    ]
    proc = subprocess.run(cmd, capture_output=True, text=True, env=env)
    if proc.returncode != 0:
        raise RuntimeError(
            f"bounded_task_runner.py failed for variant {variant.id}:\nSTDOUT:\n{proc.stdout}\nSTDERR:\n{proc.stderr}"
        )
    raw_path = None
    for line in proc.stdout.splitlines():
        if line.startswith("raw data: "):
            raw_path = Path(line.split(": ", 1)[1].strip())
            break
    if raw_path is None:
        raise RuntimeError(f"Could not locate raw data path for variant {variant.id}")
    return raw_path, proc.stdout


def score_raw_run(config: LoopConfig, raw_path: Path) -> dict[str, Any]:
    output_name = f"autoresearch-{raw_path.stem}-{config.suite}.json"
    output_path = RESULTS_DIR / output_name
    cmd = [
        sys.executable,
        str(RUNNER),
        "--suite",
        config.suite,
        "--raw-run",
        str(raw_path),
        "--save",
        "--output",
        str(output_path),
    ]
    proc = subprocess.run(cmd, capture_output=True, text=True)
    if proc.returncode != 0:
        raise RuntimeError(
            f"run_benchmarks.py failed for raw run {raw_path}:\nSTDOUT:\n{proc.stdout}\nSTDERR:\n{proc.stderr}"
        )
    payload = json.loads(proc.stdout)
    payload.setdefault("metadata", {})["selected_suite"] = config.suite
    return payload


def aggregate_trial_payloads(trials: list[dict[str, Any]]) -> dict[str, Any]:
    if not trials:
        raise ValueError("No trial payloads to aggregate")

    north_star_rates = [
        float((trial.get("summary") or {}).get("north_star_pass_rate") or 0.0)
        for trial in trials
    ]
    overall_rates = [
        float((trial.get("summary") or {}).get("overall_pass_rate") or 0.0)
        for trial in trials
    ]
    guardrail_failures: dict[str, int] = {}
    saved_to = []
    benchmark_pass_rates = []
    for trial in trials:
        summary = trial.get("summary") or {}
        for benchmark_id, failures in (summary.get("guardrail_failures") or {}).items():
            guardrail_failures[benchmark_id] = guardrail_failures.get(
                benchmark_id, 0
            ) + int(failures or 0)
        if trial.get("saved_to"):
            saved_to.append(trial.get("saved_to"))
        benchmark_pass_rates.append(summary.get("benchmarks") or [])

    aggregate = {
        "metadata": dict(trials[-1].get("metadata") or {}),
        "summary": {
            "north_star_metric": "evidence_sufficiency_pass",
            "north_star_pass_rate": sum(north_star_rates) / len(north_star_rates),
            "overall_pass_rate": sum(overall_rates) / len(overall_rates),
            "trial_count": len(trials),
            "guardrail_failures": guardrail_failures,
            "benchmarks": benchmark_pass_rates[-1] if benchmark_pass_rates else [],
        },
        "trial_results": trials,
        "saved_to": saved_to,
    }
    aggregate["metadata"]["trials_per_variant"] = len(trials)
    return aggregate


def result_score(payload: dict[str, Any]) -> tuple[float, int, int]:
    summary = payload.get("summary") or {}
    return (
        float(summary.get("north_star_pass_rate") or 0.0),
        -sum(int(v) for v in (summary.get("guardrail_failures") or {}).values()),
        float(summary.get("overall_pass_rate") or 0.0),
    )


def choose_better(current: dict[str, Any] | None, candidate: dict[str, Any]) -> bool:
    if current is None:
        return True
    return result_score(candidate) > result_score(current)


def main() -> None:
    parser = argparse.ArgumentParser(description="Variant-aware autoresearch loop")
    parser.add_argument(
        "--suite", default="tier1", help="Benchmark suite to score against"
    )
    parser.add_argument(
        "--repo-root",
        default=str(ROOT),
        help="Repository root to run experiments against",
    )
    parser.add_argument(
        "--hours", type=float, default=None, help="Maximum wall-clock runtime in hours"
    )
    parser.add_argument(
        "--max-iterations",
        type=int,
        default=None,
        help="Maximum number of candidate evaluations",
    )
    parser.add_argument(
        "--variant",
        action="append",
        default=[],
        help="Variant id(s) to evaluate; defaults to all variants",
    )
    parser.add_argument("--notes", default="", help="Notes stored in the ledger")
    parser.add_argument(
        "--model", default=None, help="Optional OPENAI_MODEL override for experiments"
    )
    parser.add_argument(
        "--trials-per-variant",
        type=int,
        default=3,
        help="Number of experimental trials to run per variant before comparison",
    )
    args = parser.parse_args()

    if args.hours is None and args.max_iterations is None:
        args.max_iterations = 1

    config = LoopConfig(
        suite=args.suite,
        repo_root=Path(args.repo_root).resolve(),
        hours=args.hours,
        max_iterations=args.max_iterations,
        variant_ids=args.variant,
        notes=args.notes,
        model=args.model,
        trials_per_variant=max(1, args.trials_per_variant),
    )

    RESULTS_DIR.mkdir(parents=True, exist_ok=True)
    variants = load_variants()
    if config.variant_ids:
        allowed = set(config.variant_ids)
        variants = [v for v in variants if v.id in allowed]
    if not variants:
        raise SystemExit("No variants selected")

    start = time.time()
    deadline = start + (config.hours * 3600.0) if config.hours is not None else None

    best_payload = None
    best_variant_id = None
    history = []
    iteration = 0
    successful_iterations = 0

    for variant in variants:
        if config.max_iterations is not None and iteration >= config.max_iterations:
            break
        if deadline is not None and time.time() >= deadline:
            break

        iteration += 1
        try:
            trial_payloads = []
            runner_excerpts = []
            raw_runs = []
            for _ in range(config.trials_per_variant):
                raw_path, runner_stdout = run_variant_experiment(config, variant)
                scored_trial = score_raw_run(config, raw_path)
                trial_payloads.append(scored_trial)
                raw_runs.append(str(raw_path.relative_to(ROOT)))
                runner_excerpts.append(runner_stdout.splitlines()[-8:])
            scored = aggregate_trial_payloads(trial_payloads)
            keep = choose_better(best_payload, scored)
            decision = "keep" if keep else "discard"
            if keep:
                best_payload = scored
                best_variant_id = variant.id
            append_ledger_row(iteration, variant, decision, scored, config.notes)
            history.append(
                {
                    "iteration": iteration,
                    "variant_id": variant.id,
                    "description": variant.description,
                    "raw_runs": raw_runs,
                    "results_path": scored.get("saved_to"),
                    "decision": decision,
                    "summary": scored.get("summary"),
                    "runner_excerpt": runner_excerpts,
                    "status": "ok",
                }
            )
            successful_iterations += 1
        except Exception as exc:
            append_failure_ledger_row(
                iteration=iteration,
                variant=variant,
                suite=config.suite,
                notes=config.notes,
                error=str(exc),
            )
            history.append(
                {
                    "iteration": iteration,
                    "variant_id": variant.id,
                    "description": variant.description,
                    "decision": "failed",
                    "status": "failed",
                    "error": str(exc),
                }
            )

    output = {
        "metadata": {
            "generated_at": datetime.now(timezone.utc).isoformat(),
            "loop": str(Path(__file__).relative_to(ROOT)),
            "suite": config.suite,
            "repo_root": str(config.repo_root),
            "hours": config.hours,
            "max_iterations": config.max_iterations,
            "notes": config.notes,
            "ledger": str(LEDGER_PATH.relative_to(ROOT)),
            "variants_file": str(VARIANTS_PATH.relative_to(ROOT)),
        },
        "best_variant_id": best_variant_id,
        "successful_iterations": successful_iterations,
        "failed_iterations": iteration - successful_iterations,
        "history": history,
    }
    print(json.dumps(output, indent=2))
    if successful_iterations == 0:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
