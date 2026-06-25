#!/usr/bin/env python3
"""Repo-local live orchestrator for autoresearch experiments.

This script acts as the scheduler for early live testing. It repeatedly:

1. runs a bounded autoresearch batch,
2. invokes the manager tick wrapper,
3. sleeps for a configured interval,
4. and repeats until stopped or a cycle bound is reached.

It is intentionally simple so it can be used directly from a shell without
cron/systemd while the experiment harness is still stabilizing.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
STATE_PATH = HERE / "manager_state.json"
LOCK_PATH = HERE / ".live_loop.lock"
LOG_DIR = HERE / "live_loop_logs"
AUTORESEARCH_LOOP = HERE / "autoresearch_loop.py"
MANAGER_TICK = HERE / "manager_tick.py"


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def load_state() -> dict[str, Any]:
    if not STATE_PATH.exists():
        raise SystemExit(f"Missing manager state file: {STATE_PATH}")
    return json.loads(STATE_PATH.read_text(encoding="utf-8"))


def save_state(state: dict[str, Any]) -> None:
    STATE_PATH.write_text(json.dumps(state, indent=2) + "\n", encoding="utf-8")


def acquire_lock(force: bool = False) -> None:
    if LOCK_PATH.exists() and not force:
        age = time.time() - LOCK_PATH.stat().st_mtime
        raise SystemExit(
            f"Live loop lock exists at {LOCK_PATH} (age={age:.0f}s). Use --force to override."
        )
    LOCK_PATH.write_text(now_iso() + "\n", encoding="utf-8")


def release_lock() -> None:
    if LOCK_PATH.exists():
        LOCK_PATH.unlink()


def write_log(prefix: str, content: str) -> Path:
    LOG_DIR.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H-%M-%SZ")
    path = LOG_DIR / f"{prefix}-{stamp}.log"
    path.write_text(content, encoding="utf-8")
    return path


def run_command(
    cmd: list[str], env: dict[str, str]
) -> tuple[subprocess.CompletedProcess[str], Path, Path]:
    proc = subprocess.run(cmd, capture_output=True, text=True, env=env)
    stdout_log = write_log("stdout", proc.stdout)
    stderr_log = write_log("stderr", proc.stderr)
    return proc, stdout_log, stderr_log


def run_cycle(
    cycle_index: int,
    suite: str,
    iterations: int,
    model: str | None,
    manager_note: str | None,
) -> dict[str, Any]:
    env = dict(os.environ)

    loop_cmd = [
        sys.executable,
        str(AUTORESEARCH_LOOP),
        "--suite",
        suite,
        "--max-iterations",
        str(iterations),
        "--notes",
        f"live_loop cycle={cycle_index}",
    ]
    if model:
        loop_cmd.extend(["--model", model])
    loop_proc, loop_stdout_log, loop_stderr_log = run_command(loop_cmd, env)
    result: dict[str, Any] = {
        "cycle": cycle_index,
        "loop": {
            "returncode": loop_proc.returncode,
            "stdout_log": str(loop_stdout_log.relative_to(ROOT)),
            "stderr_log": str(loop_stderr_log.relative_to(ROOT)),
        },
        "manager_tick": None,
    }

    if loop_proc.returncode != 0:
        result["status"] = "loop_failed"
        result["error"] = {
            "stage": "autoresearch_loop",
            "message": f"autoresearch_loop.py failed in cycle {cycle_index}",
        }
        return result

    tick_cmd = [sys.executable, str(MANAGER_TICK)]
    if manager_note:
        tick_cmd.extend(["--note", manager_note])
    tick_proc, tick_stdout_log, tick_stderr_log = run_command(tick_cmd, env)
    result["manager_tick"] = {
        "returncode": tick_proc.returncode,
        "stdout_log": str(tick_stdout_log.relative_to(ROOT)),
        "stderr_log": str(tick_stderr_log.relative_to(ROOT)),
    }
    if tick_proc.returncode != 0:
        result["status"] = "manager_tick_failed"
        result["error"] = {
            "stage": "manager_tick",
            "message": f"manager_tick.py failed in cycle {cycle_index}",
        }
        return result

    result["status"] = "ok"
    return result


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Repo-local live autoresearch scheduler"
    )
    parser.add_argument("--suite", default="tier1", help="Benchmark suite to run")
    parser.add_argument(
        "--iterations-per-cycle",
        type=int,
        default=1,
        help="Autoresearch loop max iterations per cycle",
    )
    parser.add_argument(
        "--interval-seconds",
        type=int,
        default=900,
        help="Sleep interval between cycles",
    )
    parser.add_argument(
        "--cycles",
        type=int,
        default=None,
        help="Optional maximum number of cycles before exit",
    )
    parser.add_argument(
        "--run-once",
        action="store_true",
        help="Run exactly one cycle and exit",
    )
    parser.add_argument("--model", default=None, help="Optional model override")
    parser.add_argument(
        "--manager-note",
        default="Triggered by live_loop.py after a bounded experiment batch. Prefer lightweight supervision unless clear intervention is warranted.",
        help="Extra note appended to each manager tick",
    )
    parser.add_argument(
        "--force", action="store_true", help="Override existing live loop lock"
    )
    parser.add_argument(
        "--stop-on-error",
        action="store_true",
        help="Stop after the first failed cycle instead of continuing",
    )
    args = parser.parse_args()

    if args.run_once and args.cycles is None:
        args.cycles = 1

    state = load_state()
    acquire_lock(force=args.force)
    completed_cycles = []
    cycle_index = 0

    try:
        while True:
            if args.cycles is not None and cycle_index >= args.cycles:
                break

            cycle_index += 1
            cycle_result = run_cycle(
                cycle_index=cycle_index,
                suite=args.suite,
                iterations=args.iterations_per_cycle,
                model=args.model,
                manager_note=args.manager_note,
            )
            completed_cycles.append(cycle_result)

            state = load_state()
            cycle_ok = cycle_result.get("status") == "ok"
            state["status"] = "live_loop_running" if cycle_ok else "live_loop_error"
            state["last_live_loop_at"] = now_iso()
            state["last_live_loop_cycle"] = cycle_index
            state["last_live_loop_result"] = {
                "loop_returncode": cycle_result["loop"]["returncode"],
                "manager_tick_returncode": (
                    cycle_result["manager_tick"]["returncode"]
                    if cycle_result.get("manager_tick")
                    else None
                ),
                "cycle_status": cycle_result.get("status"),
            }
            state.setdefault("notes", [])
            note = (
                f"live_loop cycle={cycle_index} status={cycle_result.get('status')} "
                f"loop_rc={cycle_result['loop']['returncode']} "
                f"manager_rc={cycle_result['manager_tick']['returncode'] if cycle_result.get('manager_tick') else 'not-run'}"
            )
            state["notes"] = list(state["notes"])[-8:] + [note]
            save_state(state)

            if not cycle_ok and args.stop_on_error:
                break

            if args.run_once:
                break
            if args.cycles is not None and cycle_index >= args.cycles:
                break
            time.sleep(args.interval_seconds)
    finally:
        release_lock()

    print(
        json.dumps(
            {
                "status": "completed",
                "cycles_run": cycle_index,
                "results": completed_cycles,
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
