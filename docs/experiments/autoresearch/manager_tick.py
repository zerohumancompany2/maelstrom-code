#!/usr/bin/env python3
"""Invoke the OpenCode runner-manager tick using a dedicated resumed session.

This wrapper is intended to be cron-safe and repo-local. It reads manager state,
ensures overlapping invocations do not proceed simultaneously, composes the tick
prompt, and invokes `opencode run` targeting the dedicated manager session.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any


HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
STATE_PATH = HERE / "manager_state.json"
PROMPT_PATH = HERE / "manager_tick_prompt.txt"
LOCK_PATH = HERE / ".manager_tick.lock"
LOG_DIR = HERE / "manager_logs"


def opencode_session_exists(session_id: str) -> bool:
    proc = subprocess.run(
        ["opencode", "session", "list"], capture_output=True, text=True
    )
    if proc.returncode != 0:
        return False
    return session_id in proc.stdout


def load_state() -> dict[str, Any]:
    if not STATE_PATH.exists():
        raise SystemExit(f"Missing manager state file: {STATE_PATH}")
    return json.loads(STATE_PATH.read_text(encoding="utf-8"))


def save_state(state: dict[str, Any]) -> None:
    STATE_PATH.write_text(json.dumps(state, indent=2) + "\n", encoding="utf-8")


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def future_after_minutes(minutes: int) -> str:
    return (
        (datetime.now(timezone.utc) + timedelta(minutes=minutes))
        .replace(microsecond=0)
        .isoformat()
    )


def acquire_lock(force: bool = False) -> None:
    if LOCK_PATH.exists() and not force:
        age = time.time() - LOCK_PATH.stat().st_mtime
        raise SystemExit(
            f"Manager tick lock exists at {LOCK_PATH} (age={age:.0f}s). Use --force to override."
        )
    LOCK_PATH.write_text(now_iso() + "\n", encoding="utf-8")


def release_lock() -> None:
    if LOCK_PATH.exists():
        LOCK_PATH.unlink()


def build_prompt(extra_message: str | None = None) -> str:
    base = PROMPT_PATH.read_text(encoding="utf-8").strip()
    if not extra_message:
        return base
    return base + "\n\nOperator note:\n" + extra_message.strip() + "\n"


def opencode_command(
    session_id: str, prompt: str, attach: str | None, use_json: bool
) -> list[str]:
    cmd = ["opencode", "run", "--session", session_id, "--dir", str(ROOT)]
    if attach:
        cmd.extend(["--attach", attach])
    if use_json:
        cmd.extend(["--format", "json"])
    cmd.append(prompt)
    return cmd


def write_log(prefix: str, content: str) -> Path:
    LOG_DIR.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H-%M-%SZ")
    path = LOG_DIR / f"{prefix}-{stamp}.log"
    path.write_text(content, encoding="utf-8")
    return path


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Cron-safe OpenCode manager tick wrapper"
    )
    parser.add_argument(
        "--force", action="store_true", help="Override existing lock file"
    )
    parser.add_argument(
        "--attach", default=None, help="Optional opencode server URL for --attach"
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Request JSON event output from opencode run",
    )
    parser.add_argument(
        "--note", default=None, help="Extra operator note appended to the tick prompt"
    )
    parser.add_argument(
        "--init-session-id",
        default=None,
        help="Initialize or overwrite manager_session_id in manager_state.json and exit",
    )
    args = parser.parse_args()

    state = load_state()

    if args.init_session_id:
        state["manager_session_id"] = args.init_session_id.strip()
        state["status"] = "initialized"
        state["last_manager_action"] = "initialized manager session id"
        state["last_tick_status"] = "ready"
        save_state(state)
        print(
            json.dumps(
                {
                    "status": "initialized",
                    "manager_session_id": state["manager_session_id"],
                },
                indent=2,
            )
        )
        return

    session_id = str(state.get("manager_session_id", "")).strip()
    if not session_id:
        raise SystemExit(
            "manager_session_id is not set in manager_state.json. Initialize it with --init-session-id."
        )
    if not opencode_session_exists(session_id):
        raise SystemExit(
            "Configured manager_session_id was not found in local opencode sessions. "
            "Use --init-session-id with a valid session id, or run with --bootstrap-single-shot for a minimal first review."
        )

    acquire_lock(force=args.force)
    try:
        prompt = build_prompt(args.note)
        cmd = opencode_command(session_id, prompt, args.attach, args.json)
        env = dict(os.environ)
        proc = subprocess.run(cmd, capture_output=True, text=True, env=env)

        stdout_log = write_log("manager-stdout", proc.stdout)
        stderr_log = write_log("manager-stderr", proc.stderr)

        state["status"] = "tick_completed" if proc.returncode == 0 else "tick_failed"
        state["last_tick_status"] = state["status"]
        state["last_manager_action"] = "manager tick invoked via wrapper"
        state.setdefault("notes", [])
        state["notes"] = list(state["notes"])[-9:] + [
            f"tick at {now_iso()} returncode={proc.returncode} stdout_log={stdout_log.name} stderr_log={stderr_log.name}"
        ]
        save_state(state)

        payload = {
            "returncode": proc.returncode,
            "stdout_log": str(stdout_log.relative_to(ROOT)),
            "stderr_log": str(stderr_log.relative_to(ROOT)),
            "command": cmd,
        }
        print(json.dumps(payload, indent=2))
        if proc.returncode != 0:
            raise SystemExit(proc.returncode)
    finally:
        release_lock()


if __name__ == "__main__":
    main()
