#!/usr/bin/env python3
import json
import os
import re
import sys
import textwrap
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path


BASE_SYSTEM_PROMPT = """You are assisting with a software engineering task.
You are working on one bounded task at a time.
Use tools only when necessary.
When the current task is complete, respond with JSON only matching the required schema.
Do not speculate about future tasks. Focus only on the current task.
"""


def system_prompt_text():
    extra = os.environ.get("MAELSTROM_EXPERIMENT_SYSTEM_PROMPT_APPEND", "").strip()
    if not extra:
        return BASE_SYSTEM_PROMPT
    return BASE_SYSTEM_PROMPT.rstrip() + "\n" + extra + "\n"


SYSTEM_PROMPT = system_prompt_text()


TASKS = [
    {
        "name": "understand_request",
        "prompt": """Understand the user's request and decide whether repository inspection is needed.

Return JSON only with this shape:
{
  "task_summary": "string",
  "repo_inspection_needed": true,
  "reason": "string"
}
""",
        "schema": {
            "task_summary": str,
            "repo_inspection_needed": bool,
            "reason": str,
        },
        "tools": [],
    },
    {
        "name": "repo_inspection",
        "prompt": """Inspect the repository enough to identify likely relevant files, concrete findings, and remaining unknowns for the user's request.

Return JSON only with this shape:
{
  "relevant_files": ["string"],
  "findings": ["string"],
  "unknowns": ["string"],
  "enough_context": true
}
""",
        "schema": {
            "relevant_files": list,
            "findings": list,
            "unknowns": list,
            "enough_context": bool,
        },
        "tools": ["list_files", "search_files", "read_file"],
    },
    {
        "name": "readiness_check",
        "prompt": """Decide whether enough information has been gathered to ask clarifying questions or proceed toward planning.

Return JSON only with this shape:
{
  "needs_clarification": true,
  "questions": ["string"],
  "ready_for_planning": false,
  "reason": "string"
}
""",
        "schema": {
            "needs_clarification": bool,
            "questions": list,
            "ready_for_planning": bool,
            "reason": str,
        },
        "tools": [],
    },
]


def tool_definitions(task):
    defs = []
    if "list_files" in task["tools"]:
        defs.append(
            {
                "type": "function",
                "function": {
                    "name": "list_files",
                    "description": "List files and directories under a path",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "path": {"type": "string"},
                        },
                        "required": [],
                    },
                },
            }
        )
    if "search_files" in task["tools"]:
        defs.append(
            {
                "type": "function",
                "function": {
                    "name": "search_files",
                    "description": "Search file contents by regex pattern",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "pattern": {"type": "string"},
                            "include_glob": {"type": "string"},
                        },
                        "required": ["pattern"],
                    },
                },
            }
        )
    if "read_file" in task["tools"]:
        defs.append(
            {
                "type": "function",
                "function": {
                    "name": "read_file",
                    "description": "Read a file or line range from a file",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "path": {"type": "string"},
                            "start_line": {"type": "integer"},
                            "end_line": {"type": "integer"},
                        },
                        "required": ["path"],
                    },
                },
            }
        )
    return defs


def all_tool_definitions():
    return tool_definitions({"tools": ["list_files", "search_files", "read_file"]})


def append_event(events, event_type, **data):
    event = {"type": event_type}
    event.update(data)
    events.append(event)


def new_metrics(mode):
    return {
        "mode": mode,
        "model_turns": 0,
        "assistant_messages": 0,
        "tool_calls": 0,
        "tool_calls_by_name": {},
        "forced_reports": 0,
        "repair_turns": 0,
        "completed": False,
        "unique_files_read": [],
        "unique_directories_listed": [],
        "search_patterns": [],
        "read_test_files": False,
        "read_non_context_go_files": False,
        "tasks_completed": [],
        "provider_usage": {"input_tokens": 0, "output_tokens": 0, "total_tokens": 0},
    }


def record_usage(metrics, response):
    usage = response.get("usage") or {}
    prompt_tokens = int(usage.get("prompt_tokens", 0) or 0)
    completion_tokens = int(usage.get("completion_tokens", 0) or 0)
    total_tokens = int(usage.get("total_tokens", 0) or 0)
    metrics["provider_usage"]["input_tokens"] += prompt_tokens
    metrics["provider_usage"]["output_tokens"] += completion_tokens
    metrics["provider_usage"]["total_tokens"] += total_tokens


def record_tool_metrics(metrics, tool_name, args):
    metrics["tool_calls"] += 1
    metrics["tool_calls_by_name"][tool_name] = (
        metrics["tool_calls_by_name"].get(tool_name, 0) + 1
    )
    if tool_name == "read_file":
        path = str(args.get("path", "")).strip()
        if path:
            if path not in metrics["unique_files_read"]:
                metrics["unique_files_read"].append(path)
            if path.endswith("_test.go"):
                metrics["read_test_files"] = True
            if path.endswith(".go") and "sketch/sketch7/context/" not in path:
                metrics["read_non_context_go_files"] = True
    elif tool_name == "list_files":
        path = str(args.get("path", ".")).strip() or "."
        if path not in metrics["unique_directories_listed"]:
            metrics["unique_directories_listed"].append(path)
    elif tool_name == "search_files":
        pattern = str(args.get("pattern", "")).strip()
        if pattern and pattern not in metrics["search_patterns"]:
            metrics["search_patterns"].append(pattern)


def needs_more_coverage(task_name, metrics):
    if task_name != "repo_inspection":
        return False
    return not metrics["read_test_files"] or not metrics["read_non_context_go_files"]


def coverage_repair_message(metrics):
    override = os.environ.get(
        "MAELSTROM_EXPERIMENT_COVERAGE_REPAIR_TEMPLATE", ""
    ).strip()
    if override:
        return override
    missing = []
    if not metrics["read_test_files"]:
        missing.append("at least one relevant test file")
    if not metrics["read_non_context_go_files"]:
        missing.append("at least one adjacent non-context Go file")
    if not missing:
        return ""
    return (
        "Before reporting, gather slightly broader evidence. Inspect "
        + " and ".join(missing)
        + ", then report. Use only files that appear relevant to the current task."
    )


def coverage_satisfied(task_name, metrics):
    if task_name != "repo_inspection":
        return True
    return metrics["read_test_files"] and metrics["read_non_context_go_files"]


def maybe_relevant_hints(root, tool_name, args):
    hints = []

    def add(path, reason):
        if len(hints) >= 3:
            return
        entry = {"path": path, "reason": reason}
        if entry not in hints:
            hints.append(entry)

    if tool_name == "read_file":
        path = str(args.get("path", "")).strip()
        if path:
            target = (root / path).resolve()
            if target.exists() and target.is_file():
                parent = target.parent
                name = target.name
                stem = target.stem
                if name.endswith(".go") and not name.endswith("_test.go"):
                    test_name = stem + "_test.go"
                    test_path = parent / test_name
                    if test_path.exists():
                        add(
                            test_path.relative_to(root).as_posix(), "matching test file"
                        )
                sibling_names = sorted(p.name for p in parent.iterdir() if p.is_file())
                for sibling in sibling_names:
                    if sibling == name:
                        continue
                    sibling_path = (parent / sibling).relative_to(root).as_posix()
                    if sibling.endswith("_test.go") and len(hints) < 3:
                        add(sibling_path, "test in same directory")
                    elif sibling.endswith(".go") and len(hints) < 3:
                        add(sibling_path, "same-directory implementation file")
                rel = target.relative_to(root).as_posix()
                if rel.startswith("sketch/sketch7/context/"):
                    for candidate in [
                        "sketch/sketch7/main.go",
                        "sketch/sketch7/logs/session.go",
                        "sketch/sketch7/runner/loop.go",
                    ]:
                        candidate_path = root / candidate
                        if candidate_path.exists() and len(hints) < 3:
                            add(candidate, "adjacent integration file")

    elif tool_name == "list_files":
        path = str(args.get("path", ".")).strip() or "."
        target = (root / path).resolve()
        if target.exists() and target.is_dir():
            files = sorted(p for p in target.iterdir() if p.is_file())
            for p in files:
                rel = p.relative_to(root).as_posix()
                if p.name.endswith("_test.go"):
                    add(rel, "test file in listed directory")
            for p in files:
                rel = p.relative_to(root).as_posix()
                if p.name.endswith(".go") and not p.name.endswith("_test.go"):
                    add(rel, "implementation file in listed directory")
            rel_dir = target.relative_to(root).as_posix() if target != root else "."
            if rel_dir == "sketch/sketch7/context":
                for candidate in [
                    "sketch/sketch7/main.go",
                    "sketch/sketch7/logs/session.go",
                ]:
                    candidate_path = root / candidate
                    if candidate_path.exists() and len(hints) < 3:
                        add(candidate, "adjacent file outside context")

    return hints


def append_hints(result, hints):
    if not hints:
        return result
    lines = ["", "Possibly relevant to your current task:"]
    for hint in hints[:3]:
        lines.append(f"- {hint['path']} ({hint['reason']})")
    return result + "\n" + "\n".join(lines)


def ensure_raw_dir(root):
    raw_dir = root / "docs" / "experiments" / "raw_data"
    raw_dir.mkdir(parents=True, exist_ok=True)
    return raw_dir


def slugify(text):
    text = text.lower().strip()
    text = re.sub(r"[^a-z0-9]+", "-", text)
    text = text.strip("-")
    return text[:60] or "run"


def write_raw_run(root, mode, prompt, model, events, result, metrics=None):
    raw_dir = ensure_raw_dir(root)
    now = datetime.now(timezone.utc)
    stamp = now.strftime("%Y-%m-%dT%H-%M-%SZ")
    path = raw_dir / f"{stamp}-{mode}-{slugify(prompt)}.json"
    payload = {
        "metadata": {
            "timestamp_utc": now.isoformat(),
            "mode": mode,
            "model": model,
            "repo_root": str(root),
            "prompt": prompt,
        },
        "events": events,
        "metrics": metrics,
        "result": result,
    }
    path.write_text(json.dumps(payload, indent=2))
    return path


def init_raw_run(root, mode, prompt, model):
    raw_dir = ensure_raw_dir(root)
    now = datetime.now(timezone.utc)
    stamp = now.strftime("%Y-%m-%dT%H-%M-%SZ")
    path = raw_dir / f"{stamp}-{mode}-{slugify(prompt)}.json"
    payload = {
        "metadata": {
            "timestamp_utc": now.isoformat(),
            "mode": mode,
            "model": model,
            "repo_root": str(root),
            "prompt": prompt,
            "status": "running",
        },
        "events": [],
        "result": None,
    }
    path.write_text(json.dumps(payload, indent=2))
    return path


def flush_raw_run(
    path, root, mode, prompt, model, events, result, status, metrics=None
):
    payload = {
        "metadata": {
            "timestamp_utc": datetime.now(timezone.utc).isoformat(),
            "mode": mode,
            "model": model,
            "repo_root": str(root),
            "prompt": prompt,
            "status": status,
        },
        "events": events,
        "metrics": metrics,
        "result": result,
    }
    path.write_text(json.dumps(payload, indent=2))
    return path


def init_text_log(root, mode, prompt, model):
    raw_dir = ensure_raw_dir(root)
    now = datetime.now(timezone.utc)
    stamp = now.strftime("%Y-%m-%dT%H-%M-%SZ")
    path = raw_dir / f"{stamp}-{mode}-{slugify(prompt)}.log"
    header = [
        f"timestamp_utc: {now.isoformat()}",
        f"mode: {mode}",
        f"model: {model}",
        f"repo_root: {root}",
        f"prompt: {prompt}",
        "status: running",
        "",
    ]
    path.write_text("\n".join(header))
    return path


def append_text_log(path, text):
    with path.open("a", encoding="utf-8") as f:
        f.write(text)
        if not text.endswith("\n"):
            f.write("\n")


def finalize_text_log(path, status):
    append_text_log(path, f"\nstatus: {status}\n")


def call_model(model, messages, tools):
    base = os.environ.get("OPENAI_API_BASE", "").rstrip("/")
    if not base:
        raise RuntimeError("OPENAI_API_BASE is required")
    if not base.startswith(("http://", "https://")):
        base = "http://" + base.lstrip("/")
    payload = {"model": model, "messages": messages}
    if tools:
        payload["tools"] = tools
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(base + "/chat/completions", data=data, method="POST")
    req.add_header("Content-Type", "application/json")
    api_key = os.environ.get("OPENAI_API_KEY", "").strip()
    if api_key:
        req.add_header("Authorization", f"Bearer {api_key}")
    try:
        with urllib.request.urlopen(req, timeout=240) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"model request failed: {e.code} {body}")


def list_files(root, path="."):
    target = (root / path).resolve()
    if not str(target).startswith(str(root.resolve())):
        return "list_files error: path escapes repo root"
    if not target.exists():
        return f"list_files error: path not found: {path}"
    if target.is_file():
        return f"Path: {path}\n{path}"
    entries = []
    for child in sorted(target.iterdir(), key=lambda p: p.name):
        if child.name in {".git", "node_modules", "__pycache__"}:
            continue
        rel = child.relative_to(root).as_posix()
        if child.is_dir():
            rel += "/"
        entries.append(rel)
    return "Path: %s\n%s" % (
        path,
        "\n".join(entries[:80]) if entries else "No entries.",
    )


def search_files(root, pattern, include_glob=""):
    matches = []
    rx = re.compile(pattern)
    for file_path in root.rglob("*"):
        if not file_path.is_file():
            continue
        if any(
            part in {".git", "node_modules", "__pycache__"} for part in file_path.parts
        ):
            continue
        rel = file_path.relative_to(root).as_posix()
        if include_glob and not file_path.match(include_glob):
            continue
        try:
            text = file_path.read_text(errors="replace")
        except Exception:
            continue
        for i, line in enumerate(text.splitlines(), start=1):
            if rx.search(line):
                matches.append(f"{rel}:{i}: {line[:200]}")
                if len(matches) >= 30:
                    return "\n".join(matches)
    return "\n".join(matches) if matches else "No matches."


def read_file(root, path, start_line=None, end_line=None):
    target = (root / path).resolve()
    if not str(target).startswith(str(root.resolve())):
        return "read_file error: path escapes repo root"
    if not target.exists():
        return f"read_file error: path not found: {path}"
    lines = target.read_text(errors="replace").splitlines()
    start = 1 if start_line is None else max(1, int(start_line))
    end = len(lines) if end_line is None else min(len(lines), int(end_line))
    if start > end:
        return "read_file error: invalid line range"
    out = []
    for line_no in range(start, end + 1):
        out.append(f"{line_no}: {lines[line_no - 1]}")
    return "\n".join(out)


def execute_tool(root, name, args):
    if name == "list_files":
        return list_files(root, args.get("path", "."))
    if name == "search_files":
        return search_files(root, args.get("pattern", ""), args.get("include_glob", ""))
    if name == "read_file":
        return read_file(
            root, args.get("path", ""), args.get("start_line"), args.get("end_line")
        )
    return f"tool error: unknown tool {name}"


def parse_json_object(text):
    text = text.strip()
    try:
        return json.loads(text)
    except Exception:
        pass
    match = re.search(r"\{.*\}", text, re.DOTALL)
    if not match:
        return None
    try:
        return json.loads(match.group(0))
    except Exception:
        return None


def validate_schema(obj, schema):
    if not isinstance(obj, dict):
        return False, "result is not an object"
    for key, expected_type in schema.items():
        if key not in obj:
            return False, f"missing key: {key}"
        if not isinstance(obj[key], expected_type):
            return False, f"wrong type for key: {key}"
    return True, "ok"


def schema_text(task):
    lines = ["{"]
    items = list(task["schema"].items())
    for i, (key, expected_type) in enumerate(items):
        if expected_type is str:
            value = '"string"'
        elif expected_type is bool:
            value = "true"
        elif expected_type is list:
            value = '["string"]'
        else:
            value = '"value"'
        suffix = "," if i < len(items) - 1 else ""
        lines.append(f'  "{key}": {value}{suffix}')
    lines.append("}")
    return "\n".join(lines)


def should_skip_task(task, artifacts):
    if task["name"] != "repo_inspection":
        return False
    first = artifacts.get("understand_request") or {}
    return first.get("repo_inspection_needed") is False


def build_messages(user_prompt, task, artifacts, transcript):
    prior = json.dumps(artifacts, indent=2) if artifacts else "{}"
    user_content = textwrap.dedent(
        f"""
        User request:
        {user_prompt}

        Current task:
        {task["prompt"].strip()}

        Prior completed task artifacts:
        {prior}
        """
    ).strip()
    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": user_content},
    ]
    messages.extend(transcript)
    return messages


def build_turn_notice(turn, max_turns):
    remaining = max_turns - turn + 1
    if remaining <= 1:
        return "You have 1 turn remaining before you must report. If you already have enough information, report now."
    return f"You have {remaining} turns remaining before you must report. Use them carefully."


def run_task(
    model,
    root,
    user_prompt,
    task,
    artifacts,
    events,
    metrics,
    flush,
    text_log_path,
    require_coverage,
    augment_hints,
    strict_coverage,
):
    transcript = []
    max_turns = 6
    coverage_followups = 0
    max_coverage_followups = 2
    print(f"\n=== TASK {task['name']} ===")
    append_text_log(text_log_path, f"\n=== TASK {task['name']} ===")
    append_event(events, "task_start", task=task["name"], max_turns=max_turns)
    flush()
    for turn in range(1, max_turns + 1):
        transcript_with_notice = list(transcript)
        turn_notice = build_turn_notice(turn, max_turns)
        transcript_with_notice.append({"role": "user", "content": turn_notice})
        messages = build_messages(user_prompt, task, artifacts, transcript_with_notice)
        tools = tool_definitions(task)
        print(f"\n--- turn {turn} ---")
        append_text_log(text_log_path, f"\n--- turn {turn} ---")
        append_event(
            events,
            "turn_start",
            task=task["name"],
            turn=turn,
            turn_notice=turn_notice,
            tools=[tool["function"]["name"] for tool in tools],
        )
        flush()
        response = call_model(model, messages, tools)
        metrics["model_turns"] += 1
        record_usage(metrics, response)
        msg = response["choices"][0]["message"]

        if msg.get("content"):
            content = msg["content"]
            print("assistant:\n" + content)
            append_text_log(text_log_path, "assistant:\n" + content)
            transcript.append({"role": "assistant", "content": content})
            metrics["assistant_messages"] += 1
            append_event(
                events,
                "assistant_message",
                task=task["name"],
                turn=turn,
                content=content,
            )
            flush()
            parsed = parse_json_object(content)
            ok, reason = (
                validate_schema(parsed, task["schema"])
                if parsed is not None
                else (False, "could not parse JSON")
            )
            if ok:
                if require_coverage and needs_more_coverage(task["name"], metrics):
                    repair = coverage_repair_message(metrics)
                    print("runtime coverage gate: " + repair)
                    append_text_log(text_log_path, "runtime coverage gate: " + repair)
                    transcript.append({"role": "user", "content": repair})
                    append_event(
                        events,
                        "runtime_coverage_gate",
                        task=task["name"],
                        turn=turn,
                        message=repair,
                    )
                    metrics["repair_turns"] += 1
                    flush()
                    coverage_followups += 1
                    if strict_coverage and coverage_followups <= max_coverage_followups:
                        continue
                    if strict_coverage and not coverage_satisfied(
                        task["name"], metrics
                    ):
                        raise RuntimeError(
                            f"coverage requirements not met for {task['name']} after followup budget"
                        )
                    continue
                print("validated artifact:")
                print(json.dumps(parsed, indent=2))
                append_event(
                    events,
                    "task_complete",
                    task=task["name"],
                    turn=turn,
                    artifact=parsed,
                )
                metrics["tasks_completed"].append(task["name"])
                flush()
                return parsed
            repair = f"Your last response did not satisfy the required JSON contract: {reason}. Respond with JSON only matching the required schema."
            print("runtime repair: " + repair)
            append_text_log(text_log_path, "runtime repair: " + repair)
            transcript.append({"role": "user", "content": repair})
            append_event(
                events,
                "runtime_repair",
                task=task["name"],
                turn=turn,
                reason=reason,
                message=repair,
            )
            flush()

        for tool_call in msg.get("tool_calls", []):
            fn = tool_call["function"]["name"]
            raw_args = tool_call["function"].get("arguments", "{}")
            try:
                args = json.loads(raw_args) if raw_args.strip() else {}
            except Exception:
                args = {}
            print(f"tool call: {fn} {json.dumps(args)}")
            result = execute_tool(root, fn, args)
            if augment_hints:
                result = append_hints(result, maybe_relevant_hints(root, fn, args))
            print("tool result:\n" + result[:4000])
            append_text_log(text_log_path, f"tool call: {fn} {json.dumps(args)}")
            append_text_log(text_log_path, "tool result:\n" + result[:4000])
            record_tool_metrics(metrics, fn, args)
            append_event(
                events, "tool_call", task=task["name"], turn=turn, tool=fn, args=args
            )
            append_event(
                events,
                "tool_result",
                task=task["name"],
                turn=turn,
                tool=fn,
                content=result,
            )
            flush()
            transcript.append(
                {
                    "role": "assistant",
                    "content": "",
                    "tool_calls": [tool_call],
                }
            )
            transcript.append(
                {
                    "role": "tool",
                    "tool_call_id": tool_call["id"],
                    "content": result,
                }
            )
            break

    force_report = (
        "Tool use is over for this task. No further repository inspection is possible. "
        "You must now produce the final report from the evidence already gathered. "
        "If evidence is incomplete, say so in the appropriate JSON fields. "
        "Respond with JSON only matching this schema:\n"
        + schema_text(task)
        + "\n"
        + "Where possible, cite supporting evidence as file:lineno inside findings, unknowns, reason, or summary fields. "
        + "Do not call tools. Do not emit tool-call markup. Any non-JSON response is invalid."
    )
    print("runtime forced report: " + force_report)
    append_text_log(text_log_path, "runtime forced report: " + force_report)
    metrics["forced_reports"] += 1
    append_event(
        events, "runtime_forced_report", task=task["name"], message=force_report
    )
    flush()
    final_transcript = list(transcript)
    final_transcript.append({"role": "user", "content": force_report})
    messages = build_messages(user_prompt, task, artifacts, final_transcript)
    response = call_model(model, messages, [])
    metrics["model_turns"] += 1
    record_usage(metrics, response)
    msg = response["choices"][0]["message"]
    content = msg.get("content", "")
    print("assistant:\n" + content)
    append_text_log(text_log_path, "assistant:\n" + content)
    metrics["assistant_messages"] += 1
    append_event(
        events,
        "assistant_message",
        task=task["name"],
        turn="forced_report",
        content=content,
    )
    flush()
    parsed = parse_json_object(content)
    ok, reason = (
        validate_schema(parsed, task["schema"])
        if parsed is not None
        else (False, "could not parse JSON")
    )
    if ok:
        if require_coverage and needs_more_coverage(task["name"], metrics):
            repair = coverage_repair_message(metrics)
            print("runtime coverage gate: " + repair)
            append_text_log(text_log_path, "runtime coverage gate: " + repair)
            metrics["repair_turns"] += 1
            append_event(
                events,
                "runtime_coverage_gate",
                task=task["name"],
                turn="forced_report",
                message=repair,
            )
            coverage_followups += 1
            final_transcript.append({"role": "assistant", "content": content})
            final_transcript.append({"role": "user", "content": repair})
            messages = build_messages(user_prompt, task, artifacts, final_transcript)
            response = call_model(model, messages, tools)
            metrics["model_turns"] += 1
            record_usage(metrics, response)
            msg = response["choices"][0]["message"]
            if msg.get("content"):
                followup_content = msg["content"]
                print("assistant:\n" + followup_content)
                append_text_log(text_log_path, "assistant:\n" + followup_content)
                metrics["assistant_messages"] += 1
                transcript.append({"role": "assistant", "content": followup_content})
                append_event(
                    events,
                    "assistant_message",
                    task=task["name"],
                    turn="coverage_followup",
                    content=followup_content,
                )
            for tool_call in msg.get("tool_calls", []):
                fn = tool_call["function"]["name"]
                raw_args = tool_call["function"].get("arguments", "{}")
                try:
                    args = json.loads(raw_args) if raw_args.strip() else {}
                except Exception:
                    args = {}
                print(f"tool call: {fn} {json.dumps(args)}")
                result = execute_tool(root, fn, args)
                if augment_hints:
                    result = append_hints(result, maybe_relevant_hints(root, fn, args))
                print("tool result:\n" + result[:4000])
                append_text_log(text_log_path, f"tool call: {fn} {json.dumps(args)}")
                append_text_log(text_log_path, "tool result:\n" + result[:4000])
                record_tool_metrics(metrics, fn, args)
                append_event(
                    events,
                    "tool_call",
                    task=task["name"],
                    turn="coverage_followup",
                    tool=fn,
                    args=args,
                )
                append_event(
                    events,
                    "tool_result",
                    task=task["name"],
                    turn="coverage_followup",
                    tool=fn,
                    content=result,
                )
                flush()
                final_transcript.append(
                    {"role": "assistant", "content": "", "tool_calls": [tool_call]}
                )
                final_transcript.append(
                    {"role": "tool", "tool_call_id": tool_call["id"], "content": result}
                )
            if strict_coverage and not coverage_satisfied(task["name"], metrics):
                if coverage_followups > max_coverage_followups:
                    raise RuntimeError(
                        f"coverage requirements not met for {task['name']} after followup budget"
                    )
            messages = build_messages(user_prompt, task, artifacts, final_transcript)
            response = call_model(model, messages, [])
            metrics["model_turns"] += 1
            record_usage(metrics, response)
            msg = response["choices"][0]["message"]
            content = msg.get("content", "")
            print("assistant:\n" + content)
            append_text_log(text_log_path, "assistant:\n" + content)
            metrics["assistant_messages"] += 1
            append_event(
                events,
                "assistant_message",
                task=task["name"],
                turn="coverage_followup_report",
                content=content,
            )
            parsed = parse_json_object(content)
            ok, reason = (
                validate_schema(parsed, task["schema"])
                if parsed is not None
                else (False, "could not parse JSON")
            )
        if ok:
            print("validated artifact:")
            print(json.dumps(parsed, indent=2))
            append_event(
                events,
                "task_complete",
                task=task["name"],
                turn="forced_report",
                artifact=parsed,
            )
            metrics["tasks_completed"].append(task["name"])
            flush()
            return parsed

    repair = (
        "Your previous response was invalid because it was not valid JSON matching the required schema. "
        "Tool use is unavailable. Use only the evidence already gathered. "
        "Respond now with JSON only matching this schema:\n" + schema_text(task)
    )
    print("runtime repair: " + repair)
    append_text_log(text_log_path, "runtime repair: " + repair)
    metrics["repair_turns"] += 1
    append_event(
        events,
        "runtime_repair",
        task=task["name"],
        turn="forced_report",
        reason=reason,
        message=repair,
    )
    flush()
    final_transcript.append({"role": "assistant", "content": content})
    final_transcript.append({"role": "user", "content": repair})
    messages = build_messages(user_prompt, task, artifacts, final_transcript)
    response = call_model(model, messages, [])
    metrics["model_turns"] += 1
    record_usage(metrics, response)
    msg = response["choices"][0]["message"]
    content = msg.get("content", "")
    print("assistant:\n" + content)
    append_text_log(text_log_path, "assistant:\n" + content)
    metrics["assistant_messages"] += 1
    append_event(
        events,
        "assistant_message",
        task=task["name"],
        turn="forced_report_repair",
        content=content,
    )
    flush()
    parsed = parse_json_object(content)
    ok, reason = (
        validate_schema(parsed, task["schema"])
        if parsed is not None
        else (False, "could not parse JSON")
    )
    if ok:
        print("validated artifact:")
        print(json.dumps(parsed, indent=2))
        append_event(
            events,
            "task_complete",
            task=task["name"],
            turn="forced_report_repair",
            artifact=parsed,
        )
        metrics["tasks_completed"].append(task["name"])
        flush()
        return parsed
    raise RuntimeError(
        f"task failed after forced report repair: {task['name']}: {reason}"
    )


def run_experiment(model, root, user_prompt, mode="bounded"):
    artifacts = {}
    events = []
    metrics = new_metrics(mode)
    append_event(events, "run_start", mode=mode)
    raw_path = init_raw_run(root, mode, user_prompt, model)
    text_log_path = init_text_log(root, mode, user_prompt, model)
    require_coverage = mode in {"bounded_v2", "bounded_v3"}
    augment_hints = mode in {"bounded_v2a", "bounded_v3"}
    strict_coverage = mode == "bounded_v3"

    def flush():
        flush_raw_run(
            raw_path,
            root,
            mode,
            user_prompt,
            model,
            events,
            artifacts,
            "running",
            metrics,
        )

    flush()
    for task in TASKS:
        if should_skip_task(task, artifacts):
            print(f"\n=== TASK {task['name']} skipped ===")
            append_text_log(text_log_path, f"\n=== TASK {task['name']} skipped ===")
            append_event(events, "task_skipped", task=task["name"])
            flush()
            continue
        artifacts[task["name"]] = run_task(
            model,
            root,
            user_prompt,
            task,
            artifacts,
            events,
            metrics,
            flush,
            text_log_path,
            require_coverage,
            augment_hints,
            strict_coverage,
        )
    append_event(events, "run_complete", mode=mode)
    metrics["completed"] = True
    flush_raw_run(
        raw_path,
        root,
        mode,
        user_prompt,
        model,
        events,
        artifacts,
        "completed",
        metrics,
    )
    finalize_text_log(text_log_path, "completed")
    return artifacts, events, metrics, raw_path, text_log_path


def run_plain_loop_experiment(model, root, user_prompt):
    transcript = []
    max_turns = 10
    events = []
    metrics = new_metrics("plain")
    append_event(events, "run_start", mode="plain")
    raw_path = init_raw_run(root, "plain", user_prompt, model)
    text_log_path = init_text_log(root, "plain", user_prompt, model)

    def flush(result):
        flush_raw_run(
            raw_path,
            root,
            "plain",
            user_prompt,
            model,
            events,
            result,
            "running",
            metrics,
        )

    flush({"final_answer": None})
    system_prompt = textwrap.dedent(
        """
        You are assisting with a software engineering task.
        Use tools when helpful.
        Investigate the repository and answer the user's request.
        When you are ready, provide a final answer.
        Where possible, cite supporting evidence as file:lineno.
        """
    ).strip()

    print("\n=== PLAIN LOOP MODE ===")
    append_text_log(text_log_path, "\n=== PLAIN LOOP MODE ===")
    for turn in range(1, max_turns + 1):
        messages = [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": user_prompt},
        ]
        messages.extend(transcript)
        print(f"\n--- turn {turn} ---")
        append_text_log(text_log_path, f"\n--- turn {turn} ---")
        append_event(
            events,
            "turn_start",
            turn=turn,
            tools=[tool["function"]["name"] for tool in all_tool_definitions()],
        )
        flush({"final_answer": None})
        response = call_model(model, messages, all_tool_definitions())
        metrics["model_turns"] += 1
        record_usage(metrics, response)
        msg = response["choices"][0]["message"]

        content = msg.get("content", "")
        tool_calls = msg.get("tool_calls", [])
        if content:
            print("assistant:\n" + content)
            append_text_log(text_log_path, "assistant:\n" + content)
            transcript.append({"role": "assistant", "content": content})
            metrics["assistant_messages"] += 1
            append_event(events, "assistant_message", turn=turn, content=content)
            flush({"final_answer": content if not tool_calls else None})
            if not tool_calls:
                append_event(events, "run_complete", mode="plain")
                metrics["completed"] = True
                flush_raw_run(
                    raw_path,
                    root,
                    "plain",
                    user_prompt,
                    model,
                    events,
                    {"final_answer": content},
                    "completed",
                )
                finalize_text_log(text_log_path, "completed")
                return content, events, metrics, raw_path, text_log_path

        handled_tool = False
        for tool_call in tool_calls:
            fn = tool_call["function"]["name"]
            raw_args = tool_call["function"].get("arguments", "{}")
            try:
                args = json.loads(raw_args) if raw_args.strip() else {}
            except Exception:
                args = {}
            print(f"tool call: {fn} {json.dumps(args)}")
            result = execute_tool(root, fn, args)
            print("tool result:\n" + result[:4000])
            append_text_log(text_log_path, f"tool call: {fn} {json.dumps(args)}")
            append_text_log(text_log_path, "tool result:\n" + result[:4000])
            record_tool_metrics(metrics, fn, args)
            append_event(events, "tool_call", turn=turn, tool=fn, args=args)
            append_event(events, "tool_result", turn=turn, tool=fn, content=result)
            flush({"final_answer": None})
            transcript.append(
                {
                    "role": "assistant",
                    "content": "",
                    "tool_calls": [tool_call],
                }
            )
            transcript.append(
                {
                    "role": "tool",
                    "tool_call_id": tool_call["id"],
                    "content": result,
                }
            )
            handled_tool = True
            break

        if not tool_calls and not content:
            append_event(events, "run_complete", mode="plain")
            metrics["completed"] = True
            flush_raw_run(
                raw_path,
                root,
                "plain",
                user_prompt,
                model,
                events,
                {"final_answer": ""},
                "completed",
            )
            finalize_text_log(text_log_path, "completed")
            return "", events, metrics, raw_path, text_log_path
        if not handled_tool and content:
            append_event(events, "run_complete", mode="plain")
            metrics["completed"] = True
            flush_raw_run(
                raw_path,
                root,
                "plain",
                user_prompt,
                model,
                events,
                {"final_answer": content},
                "completed",
            )
            finalize_text_log(text_log_path, "completed")
            return content, events, metrics, raw_path, text_log_path

    force_answer = "Provide your best final answer now. Do not call tools. Cite evidence as file:lineno where possible."
    print("runtime forced final answer: " + force_answer)
    append_text_log(text_log_path, "runtime forced final answer: " + force_answer)
    metrics["forced_reports"] += 1
    append_event(events, "runtime_forced_final_answer", message=force_answer)
    flush({"final_answer": None})
    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_prompt},
    ]
    final_transcript = list(transcript)
    final_transcript.append({"role": "user", "content": force_answer})
    messages.extend(final_transcript)
    response = call_model(model, messages, [])
    metrics["model_turns"] += 1
    record_usage(metrics, response)
    msg = response["choices"][0]["message"]
    content = msg.get("content", "")
    print("assistant:\n" + content)
    append_text_log(text_log_path, "assistant:\n" + content)
    metrics["assistant_messages"] += 1
    append_event(
        events, "assistant_message", turn="forced_final_answer", content=content
    )
    append_event(events, "run_complete", mode="plain")
    metrics["completed"] = True
    flush_raw_run(
        raw_path,
        root,
        "plain",
        user_prompt,
        model,
        events,
        {"final_answer": content},
        "completed",
    )
    finalize_text_log(text_log_path, "completed")
    return content, events, metrics, raw_path, text_log_path


def main(argv):
    if len(argv) < 2:
        print(
            "usage: bounded_task_runner.py [--mode bounded|bounded_v2|bounded_v2a|bounded_v3|plain] <repo-root> <prompt>"
        )
        return 1
    mode = "bounded"
    args = list(argv)
    if len(args) >= 2 and args[0] == "--mode":
        mode = args[1].strip()
        args = args[2:]
    if len(args) < 2:
        print(
            "usage: bounded_task_runner.py [--mode bounded|bounded_v2|bounded_v2a|bounded_v3|plain] <repo-root> <prompt>"
        )
        return 1
    root = Path(args[0]).resolve()
    prompt = " ".join(args[1:]).strip()
    if not root.exists() or not root.is_dir():
        print(f"invalid repo root: {root}")
        return 1
    if not prompt:
        print("prompt is required")
        return 1
    if mode not in {"bounded", "bounded_v2", "bounded_v2a", "bounded_v3", "plain"}:
        print(f"invalid mode: {mode}")
        return 1
    model = os.environ.get("OPENAI_MODEL", "zh-qwen36-27b-thinking")
    print(f"repo root: {root}")
    print(f"model: {model}")
    print(f"mode: {mode}")
    print(f"prompt: {prompt}")
    if mode in {"bounded", "bounded_v2", "bounded_v2a", "bounded_v3"}:
        try:
            artifacts, events, metrics, raw_path, text_log_path = run_experiment(
                model, root, prompt, mode=mode
            )
            print("\n=== FINAL ARTIFACTS ===")
            print(json.dumps(artifacts, indent=2))
            print("\n=== METRICS ===")
            print(json.dumps(metrics, indent=2))
            print(f"\nraw data: {raw_path}")
            print(f"text log: {text_log_path}")
        except Exception as exc:
            raw_dir = ensure_raw_dir(root)
            existing = sorted(raw_dir.glob(f"*-{mode}-{slugify(prompt)}.json"))
            raw_path = existing[-1] if existing else None
            text_log_path = raw_path.with_suffix(".log") if raw_path else None
            failure_payload = {}
            failure_metrics = None
            if raw_path and raw_path.exists():
                try:
                    current = json.loads(raw_path.read_text(encoding="utf-8"))
                    failure_payload = current.get("result") or {}
                    failure_metrics = current.get("metrics") or {}
                except Exception:
                    failure_payload = {}
                    failure_metrics = None
            failure_payload = dict(failure_payload)
            failure_payload["error"] = str(exc)
            if raw_path:
                flush_raw_run(
                    raw_path,
                    root,
                    mode,
                    prompt,
                    model,
                    current.get("events")
                    if "current" in locals() and isinstance(current, dict)
                    else [],
                    failure_payload,
                    "failed",
                    failure_metrics,
                )
            if text_log_path and text_log_path.exists():
                finalize_text_log(text_log_path, "failed")
            print(f"\nrun failed: {exc}", file=sys.stderr)
            if raw_path:
                print(f"raw data: {raw_path}", file=sys.stderr)
            if text_log_path:
                print(f"text log: {text_log_path}", file=sys.stderr)
            return 1
    else:
        answer, events, metrics, raw_path, text_log_path = run_plain_loop_experiment(
            model, root, prompt
        )
        print("\n=== FINAL ANSWER ===")
        print(answer)
        print("\n=== METRICS ===")
        print(json.dumps(metrics, indent=2))
        print(f"\nraw data: {raw_path}")
        print(f"text log: {text_log_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
