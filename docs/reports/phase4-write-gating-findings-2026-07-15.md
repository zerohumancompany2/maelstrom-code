# Phase 4 Write-Gating Findings

Date: 2026-07-15

## Scope

This report closes Phase 4 of
`docs/planning/maelstrom-code-completion-plan.md`: enable autonomous write
work through explicit gating, disposable execution roots, safe validation
commands, and a separate write-enabled eval tier.

## What shipped

- The normal CLI registry is read-only. It never registers `replace_text` or
  `run_command`; there is no unsandboxed write override.
- An eval case opts into writes with `sandbox: true`. Each non-staged run gets
  a fresh temporary repository copy; all stages of a staged run share one
  copy. Cleanup occurs only after post-run file assertions.
- Sandbox copies exclude `.git`, Maelstrom/eval state, caches, and caller
  exclusions. Symlinks, sockets, devices, and pipes are not copied.
- `replace_text` is available only to sandboxed eval cases. Absolute paths,
  `..` escapes, final symlink targets, and symlink-resolved paths outside its
  root are rejected.
- `run_command` is available only when a sandboxed case also provides a
  non-empty exact command allowlist. Commands run directly without a shell;
  callers cannot append flags or arguments to an allowlisted command, and
  both lexical and symlink-resolved workdirs are clamped to the sandbox.
- File-content eval checks (`required_file_contains`) run against the
  effective sandbox before teardown and reject lexical/symlink path escapes.
- Write evals can require successful executions of specific tools
  (`required_tools_executed`). This prevents a plausible final answer from
  passing without an actual edit and validation command.
- `sandbox-write-microtasks.yaml` is a separate 2-task × 3-model write tier:
  one single-file fix and one small multi-file change. The committed fixtures
  remain intentionally broken; each run must edit and test only its copy.

## Safety review

The first GPT review found two should-fix issues:

1. a sandboxed case with an empty command allowlist still received the legacy
   shell command tool;
2. lexical root checks were blind to symlinks.

Both were fixed before the tier was accepted. A follow-up review found no
remaining blocker or should-fix. Tests cover source-copy isolation, staged
sandbox sharing, cleanup, default exclusions, symlink skipping, absolute and
relative path escapes, symlinked write targets/workdirs/file assertions,
no-shell command execution, exact allowlisting, CLI write-tool absence, and
post-run disk checks.

## Live battery

Files (gitignored):

- `.maelstrom/evals/sandbox-write-microtasks.jsonl` — initial workflow-heavy
  scoring;
- `.maelstrom/evals/sandbox-write-microtasks-r2.jsonl` — confirmed four runs
  edited and tested successfully but were rejected because the cognitive
  chart completed before workflow finalization;
- `.maelstrom/evals/sandbox-write-microtasks-r3.jsonl` — final behavior-gated
  scoring.

Final r3 result: **3/6 pass (0.50)**.

| model | single-file | multi-file | attributable result |
|---|---:|---:|---|
| minimax-m2.5 | pass | pass | 2/2: read, edit, exact `go test`, disk assertions |
| qwen3-coder-30b | fail | pass | single-file stopped after listing; behavior gates caught no edit/command |
| glm-4.7-flash | provider error | fail | both edited and tested; one provider error, one extra invalid workflow output |

The raw pass rate understates write capability: 5/6 r3 runs performed the
requested edit and validation command successfully; strict completion and
workflow-output checks rejected two of those. That distinction is visible in
summary/tool/file checks without inspecting session logs.

Most importantly, after three paid live batteries:

- `git diff -- sketch/sketch7/evals/testdata/write-microtasks` remained empty;
- every successful mutation was confined to a disposable copy;
- hallucinated completion (qwen single-file in r3, qwen multi-file in r2) was
  rejected by successful-tool and on-disk assertions;
- exact allowlisted `go test` commands executed successfully across all three
  models.

## Phase 4 acceptance

- Provider sees only effective enabled tools: **met** (existing runtime gate;
  registry additionally omits writes outside sandbox cases).
- Empty policy intersections mean no tools: **met** (existing runtime tests).
- `maxToolCalls` and wall time enforced: **met** (existing runtime bounds;
  coding agent/workflow declare both).
- Malformed/missing finalization buckets rejected: **met** (existing
  finalization tests and live attribution).
- Reports summarize tool violations/finalization failures: **met**.
- Execution in disposable worktree/equivalent sandbox: **met** (temporary
  copy; no `.git`; isolated roots; teardown; source untouched live).
- Separate read-edit-validate write tier: **met**.

## Follow-ups, not Phase 4 blockers

1. A temporary copy isolates repository mutations but is not an OS/container
   sandbox. Exact no-shell commands sharply constrain execution, but stronger
   process/network isolation can be added if threat assumptions become
   adversarial rather than accidental/model-error safety.
2. Cognitive completion can end a workflow-bound write session before its
   workflow artifact finalizes. The write tier correctly scores concrete
   behavior independently; Phase 5 should still address the binding-scope
   verb gap identified by the workflow stress test.
3. Minimax was the only 2/2 model in this small battery. Use more repeats
   before selecting a production default coding chart/model.
