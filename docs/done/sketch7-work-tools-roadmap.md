# Sketch7 Work Tools Roadmap and Benchmark Plan

**Status:** Proposed | **Date:** 2026-05-31

## Why this document exists

Sketch7 now has a meaningful runtime foundation:

- authored definitions,
- catalog loading and reload semantics,
- runtime reduction,
- prompt assembly with provenance,
- a structured provider boundary,
- explicit state/control tools,
- and a runner that can execute multi-step flows.

That foundation is important, but it is not enough to make Maelstrom feel like a strong coding CLI.

At this stage, the main risk is no longer architecture drift alone.
The main risk is building a runtime that is internally elegant but operationally weak on real coding tasks.

This document exists to keep tool evolution grounded in:

- concrete task usefulness,
- measurable improvements,
- and external coding-agent benchmarks such as Terminal-Bench.

## Core principle

Better tools reduce the amount of prompt cleverness and runtime ceremony required for good coding performance.

The immediate next goal for sketch7 is therefore:

> build the smallest work-tool surface that makes the repo-edit-test-debug loop genuinely useful.

## Immediate objective

Add a first wave of repo/work tools that let an agent:

1. inspect code precisely,
2. make targeted edits safely,
3. execute validation commands,
4. and report results with enough structure for recovery and review.

## First-wave tool set

The recommended first-wave work tools are:

1. `read_file`
2. `replace_text`
3. `run_command`

### Why this set

This trio creates the smallest useful coding loop:

- read the relevant code,
- make a targeted change,
- run a test/build/lint command,
- and iterate.

This is enough to begin solving real tasks without prematurely adding a large tool inventory.

## Tool 1: `read_file`

### Purpose

Read a file, or a slice of a file, with enough precision to avoid stuffing entire files into context unnecessarily.

### Proposed arguments

- `path` (required)
- `start_line` (optional)
- `end_line` (optional)

### Proposed behavior

- returns file contents,
- includes line numbers in returned content,
- supports partial reads for context efficiency,
- errors clearly on missing path or invalid ranges.

### Why it matters

This is the base inspection primitive.
Without it, the agent cannot operate concretely.

### Metrics to watch

- average bytes/tokens returned per read,
- percent of reads that use ranges rather than full file,
- error rate on missing/invalid paths,
- number of redundant repeated reads before successful edits.

## Tool 2: `replace_text`

### Purpose

Perform safe, targeted text replacement instead of blunt full-file rewrites.

### Proposed arguments

- `path` (required)
- `old_text` (required)
- `new_text` (required)

### Proposed behavior

- exact text replacement,
- succeeds only when `old_text` matches exactly once,
- returns an error if the target is missing or ambiguous,
- records enough information to explain what changed.

### Why this first instead of `write_file`

`replace_text` is a better MVP mutation primitive because it is:

- safer,
- easier to audit,
- easier to reason about after interruption/restart,
- and more aligned with precise code editing.

### Metrics to watch

- percent of successful single-shot replacements,
- ambiguity/error rate,
- average replacements per task,
- percent of tasks requiring fallback or repeated replacement attempts.

## Tool 3: `run_command`

### Purpose

Execute validation commands for tests, builds, lint, and reproduction steps.

### Proposed arguments

- `command` (required)
- `workdir` (optional)
- `timeout_seconds` (optional)

### Proposed behavior

- executes from repo root by default,
- captures stdout/stderr,
- returns exit code,
- records command and result explicitly,
- should be designed with future cancellation/background support in mind.

### Why it matters

Without this, sketch7 cannot really close the implementation loop.
This is the bridge from editing to proving correctness.

### Metrics to watch

- command success/failure rate,
- average output size,
- average runtime,
- percent of tasks resolved after first validation command,
- cancellation/interruption behavior once supported.

## Near-term follow-up tool

After the first three tools, the recommended next tool is:

## `get_file_skeleton`

### Purpose

Provide high-bandwidth structural context without reading whole files.

### Why it matters

This is one of the clearest lessons from systems like Dirac:
the model performs better when it can inspect code structure cheaply and precisely.

### Proposed output

- top-level definitions,
- classes/functions/methods,
- maybe spans or stable anchors later,
- enough detail to choose what to read next.

### Metrics to watch

- reduction in full-file reads,
- reduction in total prompt size per task,
- improvement in first-pass edit accuracy,
- reduction in unnecessary command/test iterations.

### Note for future evolution

`get_file_skeleton` is a good near-term tool for sketch7 because it gives the agent a higher-bandwidth structural read surface quickly.

However, for supported languages and environments, this tool should eventually be superseded or backed by language-server-powered runtimes rather than remaining a permanently bespoke implementation.

Why:

- language servers already provide rich structural and symbol information,
- they are often more accurate and better maintained for language-specific edge cases,
- and they let us leverage existing ecosystem tooling instead of rebuilding it all inside Maelstrom.

The likely long-term direction is:

- use `get_file_skeleton` as an MVP structural read primitive,
- then replace or enrich it with language-server-backed structure/symbol services for appropriate subsets of files,
- while preserving the same high-level runtime/tool contract where possible.

This is not a current implementation task.
It is a note to future selves so the MVP does not calcify into unnecessary bespoke tooling where better existing integrations are available.

## Future tool waves

### Wave 2

- `search_files`
- `list_files`
- `read_symbol` / `get_function`
- `replace_in_range`

## Note: work tools and context maps interact

Recent sketch7 experiments showed that context-map sections can materially improve first-step tool choice and repo orientation before any work tool is called.

This suggests an important planning point:

- tool evolution should not be considered in isolation,
- because some performance improvements will come from better tool contracts,
- while others will come from better context-map sections that help the model choose the right tools earlier.

Examples of useful future coding-oriented context sections/chunks may include:

- repo awareness,
- task/acceptance-criteria summaries,
- risk or policy nudges,
- changed-file summaries,
- and lightweight test/build environment summaries.

These should be treated carefully because they are effectively arbitrary inference payload injections.
Longer-term they should be backed by durable context snapshot records and explicit inference-envelope recording so their effect on behavior remains inspectable and auditable.

For now, the intended authored policy surface for these sections should stay small:

- `refreshEveryNTurns` for generated-section refresh cadence,
- `retentionMode` for latest-effective or transcript-retention behavior.

That should be enough to support useful context-map evolution without rebuilding a large chunk-policy DSL prematurely.

#### Design note: `search_files` backend layering

`search_files` should remain its own tool contract rather than becoming an alias for `run_command`.

However, it should reuse the same subprocess execution substrate as `run_command` where appropriate.

Recommended layering:

- shared subprocess/backend execution helper,
- `run_command` as the generic arbitrary command tool,
- `search_files` as a structured search tool with normalized results.

This keeps responsibilities clear:

- `run_command` owns generic shell execution,
- `search_files` owns search-specific arguments, backend selection, and normalized output.

Recommended search backend preference order:

1. `rg`
2. `grep`
3. internal fallback

This gives sketch7 a stable agent-facing tool contract while still taking advantage of strong external runtimes where available.

#### Design note: `read_symbol` backend evolution

`read_symbol` is a high-leverage follow-up tool after `search_files` and `get_file_skeleton` because it lets the agent move from broad discovery to precise symbol-level reads.

For sketch7, a Go-first implementation backed by the Go AST is a good near-term choice.

However, like `get_file_skeleton`, this tool should not be assumed to remain permanently bespoke.
Over time it should lean on stronger language-aware backends where appropriate, such as:

- tree-sitter-based runtimes,
- or language-server-backed runtimes.

Why:

- symbol extraction and reference discovery become more accurate with mature language tooling,
- symbol identity and reference resolution are deeper problems than simple text slicing,
- and Maelstrom should leverage strong existing language tooling where it exists rather than reimplementing everything forever.

So the recommended path is:

- MVP: Go-AST-backed `read_symbol` for definition extraction,
- then optional reference support,
- then eventual migration or augmentation with tree-sitter / LSP-backed implementations for supported languages.

### Wave 3

- anchor-based edit tool
- symbol-aware / AST-aware edit tool
- richer command execution modes
- diff/review-specific tools

### Wave 4

- browser/web task tools where needed,
- multi-file batch edit tooling,
- subagent-assisted tooling,
- and richer provenance-aware automation.

## Improvements to existing tools

Existing control tools should also evolve.

### `transition_state`

Potential improvements:

- stronger validation/error messages,
- richer provenance links,
- optional policy/guard evaluation hooks,
- clearer UX distinction between control transitions and work actions.

### `bind_workflow` / `unbind_workflow`

Potential improvements:

- stronger binding ownership semantics,
- prevention of invalid double-bind states,
- workflow existence validation against catalog,
- richer reason/handoff metadata.

### `interrupt_session` / `resume_session`

Potential improvements:

- richer interruption reasons,
- support for user redirection payloads,
- resume preconditions,
- and interaction-mode-aware runner behavior.

## Benchmarking philosophy

Tool development should not just be validated by unit tests.
It should be tied to task-level performance.

We should evaluate tools on three layers:

### 1. Unit-level correctness

Examples:

- read returns correct slices,
- replacement is exact and safe,
- command results capture exit code and output,
- state/control tools emit correct records.

### 2. Integration-level task behavior

Examples:

- read → replace → command loops,
- bind → workflow transition → interrupt → resume flows,
- reload definition → next run uses changed behavior.

### 3. External benchmark alignment

We should measure the system against realistic coding-agent benchmarks.

## Terminal-Bench alignment

Terminal-Bench 2.1 is a useful target for the work-tool roadmap.

It should not distort the architecture, but it is a strong external reality check.

### Why it matters

Terminal-Bench-style tasks reward:

- useful coding/tool execution,
- correct edits,
- validation discipline,
- and efficiency.

That is exactly where sketch7 needs pressure now.

### Recommendation

As sketch7 grows, define a small benchmark matrix that tracks:

- task success rate,
- cost / token consumption,
- number of tool calls,
- number of file reads,
- number of edit attempts,
- number of validation commands,
- and time-to-first-correct-result.

## Suggested measurable targets

These are initial directional metrics, not hard final commitments.

### First-wave tool quality targets

#### `read_file`

- 100% deterministic content return for valid paths/ranges
- clean error on missing file or invalid range
- support line-ranged reads from day one

#### `replace_text`

- 100% deterministic exact-once replacement behavior
- fail closed on zero-match or multi-match
- no silent partial edits

#### `run_command`

- deterministic capture of exit code and output
- explicit timeout behavior
- future-compatible with interruption/cancellation

### Early task-loop targets

For internal integration scenarios, target:

- successful completion of simple single-file bugfix/refactor tasks,
- read → replace → test loops in a small number of tool rounds,
- and clear explainability of every state and work mutation.

### External benchmark targets

For a future benchmark pass such as Terminal-Bench 2.1, aim to improve along:

- success rate,
- token efficiency,
- and average task cost,

without sacrificing correctness.

At this stage, the goal is not leaderboard chasing.
The goal is using benchmark pressure to keep tool development honest.

## Proposed implementation order

1. `read_file`
2. `replace_text`
3. `run_command`
4. integration tests for read/edit/validate loops
5. `get_file_skeleton`
6. benchmark harness planning against Terminal-Bench-style tasks

## Recommendation

The next coding milestone for sketch7 should be:

> prove that the runtime can solve a small real coding task using `read_file`, `replace_text`, and `run_command` under the existing workflow/control architecture.

If that works, the architecture is earning its keep.
If it does not, the next round of work should improve tools before adding more runtime sophistication.

## Current checkpoint update (2026-06-01)

This milestone has effectively been achieved and then surpassed in a limited but meaningful way.

Sketch7 now has working and pressure-tested support for:

- `read_file`
- `replace_text`
- `run_command`
- `get_file_skeleton`
- `search_files`
- `list_files`
- `read_symbol`
- `find_references`

And importantly, these were not just unit-tested in isolation.
They were exercised through a real local-model loop, which drove several critical improvements:

- better `search_files` ranking,
- better provider replay shape,
- introduction of `list_files`,
- explicit inference context shaping,
- repo-awareness sections that reduced wrong-language starts,
- and early durable context/inference envelope work.

This changes the planning posture.
The next risk is no longer primarily "insufficient tool surface."
The next risk is:

> building a good tool-using coding loop without yet proving the larger workflow-backed product story.

### Updated implication for tool work

Near-term tool work should now be justified mainly by one of four pressures:

1. it improves the anchor `conversation_to_execution` workflow,
2. it improves interruption/resumption or recovery behavior,
3. it improves inference context fidelity or explainability,
4. or it is clearly required by benchmark/task pressure.

That means sketch7 should now bias toward:

- fewer, more targeted tool additions,
- stronger integration scenarios,
- and clearer runtime/product behavior,

rather than broadening the tool surface quickly.
