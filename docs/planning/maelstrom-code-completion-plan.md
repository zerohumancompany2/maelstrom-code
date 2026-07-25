# Maelstrom Code Completion Plan

**Status:** Active — Phases 1–5 complete; Phase 6 next
**Date:** 2026-07-04  
**Scope:** Complete the runtime into a lean, highly functional local-model-first coding-agent harness and prove the first production experience. The whole-cloth promotion into `cmd/maelstrom`, `internal/`, and top-level definition/eval directories completed in Phase 5.

## Decisions already made

These decisions are treated as settled for this plan.

- **Current implementation line:** the former sketch7 seed is now the sole production runtime. Pre-promotion code is archived at Git tag `sketch-era-final`.
- **Immediate optimization target:** the next major deliverable is the **eval harness + model battery**, not broad feature expansion.
- **Serving target:** the harness should target a **llama.cpp / LM Studio style OpenAI-compatible endpoint**. In practice, the first battery may run primarily against **OpenRouter** because the local serving machine is unstable under long-lived model-service sessions.
- **Write safety:** write-enabled autonomous repo work should be **strictly gated** behind readiness checks, disposable worktrees, and authoritative runtime limits.
- **Product framing:** this is primarily a **personal scratch-your-own-itch coding tool**. Open-source release is desirable but secondary.
- **Promotion timing:** promote `sketch7` to the production layout **after** the eval harness and the key runtime-fix work land.

## Why this plan exists

The repo now contains:

- a large amount of validated sketch7 runtime work,
- a meaningful body of eval and experiment notes,
- several partial plans that are still relevant,
- and a number of older planning docs that are now historical.

This document consolidates the current state of the system, the intended product boundary for the **code** variant of Maelstrom, and the implementation order needed to get to a reliable, lean coding-agent harness.

## Product intent: what “maelstrom code” actually is

Maelstrom as a larger vision is infrastructure for durable, autonomous organizations.

**Maelstrom Code** is the intentionally smaller and more practical first product line:

> a lightweight Go runtime for reliable, inspectable, long-running coding agents that work primarily through local or local-style model serving, durable histories, bounded tool loops, and explicit state-local task contracts.

This product is not trying to solve broad zero-human-company operations yet.
It is trying to solve a narrower but foundational problem:

> make repo-oriented coding agents reliable enough, inspectable enough, and cheap enough to be useful day-to-day under local-model constraints.

That means the current “code” variant should be optimized for:

- bounded cognitive-state execution,
- explicit tool gating,
- durable session/workflow history,
- structured state-local outputs,
- strong retry/repair behavior,
- eval-driven model/tool/runtime qualification,
- and a tight read/edit/validate coding loop.

## Core doctrine

The clearest through-line across the current docs, experiments, and inspirations is this:

> **The runtime owns control. The model performs a small bounded local task. Reliability is represented in runtime structures and durable facts, not in long prompt prose.**

This implies several architectural rules.

### 1. States are task gates, not roleplay

A cognitive state should specify what kind of thinking governs the next bounded inference window.
The model should not need to reason about the full state machine.
The runtime should project:

- the current local task,
- the required output shape,
- the enabled tools,
- and the relevant bounds.

### 2. Tools are the mutation boundary

Repo changes, workflow movement, binding changes, and runtime progression should happen through explicit, durable, inspectable steps.
Nothing important should depend on invisible in-memory interpretation.

### 3. Validation must sit at the loop boundary

If something matters, it must be checked when model output crosses into runtime behavior:

- output schema validity,
- required-field presence,
- allowed triggers,
- enabled-tool checks,
- bound exhaustion,
- and finalization validity.

### 4. Evals are the forcing function

The harness should not grow because a feature sounds elegant.
It should grow because measured task reliability demands it.

### 5. Context must remain small, explicit, and inspectable

The system is targeting local-model-like conditions:

- weaker long-context behavior,
- weaker schema obedience,
- weaker multi-objective planning,
- higher sensitivity to prompt clutter.

Therefore the runtime should prefer:

- short state-local tasks,
- compact context sections,
- structural read tools,
- precise repair messages,
- and narrow tool surfaces.

## Inspiration mapping

The current design direction is strongly consistent with four external influences.

### Dirac

Dirac contributes several important lessons:

- **phase control through tool availability, not prose alone**,
- **structural code-reading tools** like file skeletons and function-level reads,
- **tight token discipline**,
- **repair-first handling of malformed model behavior**,
- and **cost / efficiency as first-class eval dimensions**.

The main takeaways for Maelstrom Code are:

- keep prompts lean,
- make tool surfaces high-signal,
- treat structural read tools as token compression,
- and validate/repair aggressively at the tool boundary.

### Forge

Forge reinforces the need for:

- response validation,
- rescue parsing,
- retry nudges,
- required-step enforcement when needed,
- and scenario-driven model qualification across repeated runs.

For Maelstrom Code, the key takeaway is:

> the first serious eval harness should look more like a native runtime-integrated Forge battery than like an ad hoc demo loop.

### MAKER / “Solving a Million-Step LLM Task with Zero Errors”

The main lesson is not “copy the exact system.”
The lesson is:

- long-range reliability requires **extreme decomposition**,
- each step must be narrow enough to validate,
- and error correction must happen continuously rather than only at the end.

That directly supports the sketch7 direction of bounded state-local task frames plus durable evaluation facts.

### Needle

Needle is not the main near-term runtime template, but it is a useful strategic reminder that:

- very small specialized models may eventually be useful for narrow routing/gating/classification jobs,
- and the harness should keep those future roles conceptually separate from the main coding model.

For now, this is a future note, not an implementation priority.

## Verified current state of sketch7

The current sketch7 implementation is already substantial.
It is not a toy anymore.

### What is already real

Sketch7 already has:

- YAML-backed model / agent / workflow definitions,
- cognitive and workflow statechart definitions,
- durable session history,
- durable workflow history,
- deterministic reduction into runtime views,
- binding as a first-class runtime concern,
- context-section assembly with provenance,
- durable context snapshot and inference-envelope records,
- a real OpenAI-compatible provider boundary,
- a tool registry plus real repo/work tools,
- cognitive finalization with retries,
- output contract evaluation records,
- session reports and reducer-derived stats,
- hot reload integration support,
- and tests across the runtime spine.

### Build/test status

At review time:

- `go build ./...` succeeded
- `go test ./sketch/sketch7/...` succeeded

### Uncommitted “states as output gates” work

The current uncommitted sketch7 changes are directionally correct and should be treated as the working baseline.

What they successfully establish:

- states as bounded output gates rather than model-owned state-machine navigation,
- runtime-owned transition behavior,
- cognitive output validation before transition/finalization,
- provider tool narrowing,
- and paired example agents (`stateless-reader`, `ooda-reader`) for stress testing.

This is a good baseline, but it does **not** complete the runtime semantics needed for a trustworthy write-enabled agent harness.

## Workstream status against the current core-completion plan

The existing `docs/done/sketch7-core-completion-plan.md` remains the main runtime implementation queue.

### Workstream 1 — authoritative tool policy edge cases

**Status:** mostly implemented, but not finished.

Implemented:

- provider requests are narrowed to effective allowed tools,
- empty effective intersections can reject tool use in practice,
- disabled-tool rejection records exist.

Still missing:

- the explicit `EffectiveToolPolicy { Applied, Tools }` semantic split,
- a cleaner distinction between “no policy” and “policy applied but empty”.

### Workstream 2 — remaining state-bound enforcement

**Status:** partial.

Implemented:

- cognitive `maxInferenceTurns`,
- finalization retry caps.

Missing and still critical:

- authoritative `maxToolCalls` enforcement,
- authoritative `maxWallTimeSeconds` enforcement,
- bound-hit reasons in lifecycle/completion records,
- prevention of continued tool use after non-turn bounds are exceeded.

### Workstream 3 — generalized finalization mode

**Status:** not implemented.

Current sketch7 finalization is still essentially **cognitive-only**.
Workflow-only and combined cognitive/workflow finalization do not yet exist as first-class runtime behavior.

### Workstream 4 — wrapper JSON bucket validation

**Status:** not implemented.

The runtime still validates one top-level object against one cognitive output contract.
It does not yet support or require the planned wrapper shape:

```json
{
  "cognitive": {},
  "workflow": {}
}
```

### Workstream 5 — output schema expressiveness

**Status:** not implemented.

Current output contracts are too shallow:

- schema name,
- required field names,
- strict mode.

Strict validation still depends on a hard-coded allowlist rather than state-specific allowed fields.

### Workstream 6 — workflow lifecycle mirroring

**Status:** partial.

Workflow lifecycle is well represented in session history, but not sufficiently mirrored into workflow history for strong cross-agent/global workflow auditing.

### Workstream 7 — reporting and eval reducers

**Status:** partial.

Session reducers and report output exist.
What is missing is the next layer of reliability reporting needed for stress tests and model-battery work:

- bound-hit reasons,
- bucket validation statuses,
- provider-exposed vs requested tools,
- tool-policy violations by state,
- context snapshot churn,
- and better state-exit/finalization summaries.

## Confirmed correctness issues to fix early

These are high-signal issues because they affect trust in the runtime or in the upcoming eval data.

### 1. Inference envelope tool provenance is inaccurate

The session’s inference-envelope record currently captures agent-level payload tool names rather than the **actual narrowed tool set** sent to the provider.
That means the durable provenance can misstate what the model really saw.

### 2. Response-format schema typing is too weak

The current OpenAI-compatible response-format builder types required output fields too generically, which is especially problematic for boolean fields such as `completion_signal`.

### 3. Default CLI harness is too permissive for current readiness

The default agent wiring still includes write-capable tools even though the documented readiness gates say write-enabled autonomous work should stay disabled until the remaining core-completion items are finished.

### 4. Strict validation is still globally hard-coded

State-level strict validation should be derived from each state’s authored contract, not from a small global field allowlist in loop code.

## The implementation plan

## Phase 0 — stabilize the baseline

### Goal

Turn the current sketch7 working state into an explicit baseline before deeper runtime changes begin.

### Tasks

1. Commit the current sketch7 “states as output gates” work as the accepted baseline.
2. Keep the shipped example agents (`stateless-reader`, `ooda-reader`) and sketch7 README as part of that baseline.
3. Ensure transient local data (for example deleted session DB files) is either restored, ignored, or intentionally removed.
4. Keep the current `sketch7-core-completion-plan.md` in place as the lower-level runtime queue.

### Acceptance

- sketch7 baseline is committed and reproducible,
- paired read-only agents exist for immediate eval work,
- there is no ambiguity about whether the current state-gating work is “real” or “scratch”.

## Phase 1 — fix runtime correctness so evals are trustworthy

### Goal

Before building the model battery, make sure the runtime records the truth and enforces the bounds it claims to enforce.

### Tasks

#### 1. Fix inference-envelope tool provenance

Record the exact tool names actually sent to the provider after narrowing/finalization, not the broader payload-level tool list.

#### 2. Finish authoritative tool-policy semantics

Introduce the explicit `EffectiveToolPolicy` result object:

```go
type EffectiveToolPolicy struct {
    Applied bool
    Tools   []string
}
```

This should become the source of truth for both:

- request-time provider tool narrowing,
- and tool-call validation.

#### 3. Enforce remaining bounds

Add authoritative support for:

- `maxToolCalls`,
- `maxWallTimeSeconds`.

Required behavior:

- bounds are measured from state enter,
- tool use cannot continue after a bound hit,
- bound-hit reasons are recorded durably,
- and finalization behavior is deterministic when a bound is reached.

#### 4. Extract output parsing/validation from `runner/loop.go`

Move parsing and validation into a small dedicated module.
This will make wrapper-bucket validation easier and keep loop control logic smaller.

#### 5. Improve output schema typing

The response-format builder should produce correct types for known fields like booleans.
The runtime contract layer should not pretend every field is stringly typed.

#### 6. Add `optionalFields`

Upgrade `StateOutputContract` just enough to support:

```yaml
outputs:
  schema: cognitive_step_v1
  requiredFields: [summary, completion_signal]
  optionalFields: [evidence, risks, next_step]
  strict: true
```

Strict mode should allow:

- required fields,
- optional fields,
- runtime-reserved fields.

#### 7. Standardize stop reasons and retry categories

Adopt the vocabulary already described in `minimal-eval-agent-and-metrics.md` so that the eval harness does not need to normalize ad hoc semantics later.

### Acceptance

- envelope provenance reflects the real provider request,
- all declared state bounds are enforced,
- state-specific strict validation is real,
- stop reasons and retry categories are stable enough for eval consumption,
- and loop parsing/validation is no longer tangled into one monolithic function.

## Phase 2 — build the Go eval harness and initial model battery

### Goal

Make model/runtime/tool reliability measurable through a repeatable, resumable native harness.

### Why this is the next major deliverable

This is the single most important next step because:

- the Python experiments proved the conceptual control ideas,
- but the Go runtime is now where those ideas need to be measured and operationalized,
- and the battery results should drive which runtime/tool upgrades matter next.

### Location

The harness should live in:

- `sketch/sketch7/evals/` for the core library pieces,
- plus a sketch7 eval entrypoint / runner mode.

This keeps it near the runtime it is evaluating and makes promotion into the production layout straightforward.

### Task deck source

The first Go task deck should be seeded by **porting the existing Python benchmark taxonomy** from `docs/experiments/autoresearch`, not by inventing a totally fresh deck from scratch.

Then add a small number of new sketch7-specific gate tests on top.

### Harness capabilities

The harness should support:

1. **task deck loading** from YAML or similar declarative definitions,
2. **matrix runs** over:
   - models,
   - agents,
   - tasks,
   - and repeated runs per cell,
3. **JSONL result output** for append-only batch capture,
4. **resume / skip-completed behavior** so long runs survive crashes or interruptions,
5. **structured report generation** backed by existing reducer output,
6. **time-boxed execution / watchdog behavior** so unstable local serving does not invalidate entire batches,
7. **OpenAI-compatible endpoint support** so local llama.cpp, LM Studio, llama-swap, and OpenRouter all fit the same path.

### First eval scopes

#### Scope A — read-only reliability battery

Use the shipped read-only agents as the first serious matrix:

- `stateless-reader.yaml`
- `ooda-reader.yaml`

Target:

- output contract compliance,
- tool-policy obedience,
- bounded-state behavior,
- evidence sufficiency,
- and context efficiency.

#### Scope B — minimal coding eval agent

Implement the intentionally simple **minimal eval coder** profile described in `minimal-eval-agent-and-metrics.md`.
This should stay small and strict.

#### Scope C — state-gating microtasks

Add tasks specifically designed to stress:

- forced synthesis boundaries,
- one-more-read inertia,
- required supporting-file coverage,
- and finalization obedience.

### Metrics

The initial harness should report at least:

- output schema success rate,
- required-field presence rate,
- wrong-state / wrong-bucket emission rate,
- tool-call validity rate,
- tool execution success rate,
- retry recovery rate,
- loop completion rate,
- bound-hit rate by type,
- tool calls per successful task,
- and a higher-level **Evidence Sufficiency Success Rate**.

Where possible, distinguish:

- raw success,
- repaired success,
- terminal invalidity,
- and incompleteness / overclaim behavior.

### Model battery strategy

Because the local machine may become unstable when long-lived model services stay running, the first battery should assume:

- **OpenRouter-first or hybrid operation**,
- one OpenAI-compatible path for both local and cloud backends,
- resumable batches,
- and short initial decks.

A reasonable first battery shape is:

- one reference local-ish or cloud-accessible thinking/coder model,
- plus a small comparison set across families such as Qwen, Gemma, Mistral, and a Nemotron/NVIDIA-style model.

Exact model selection can be finalized when the harness is ready to run.

### Acceptance

- the harness can run a repeatable multi-run matrix,
- batches can resume after failure,
- results are exported as JSONL and summarized into reports,
- and at least one read-only battery produces actionable differences across agents/models.

## Phase 3 — complete finalization semantics

### Goal

Generalize the current cognitive finalization path into the fuller state-bucket model already described in planning docs.

### Tasks

1. Make `FinalizationMode` real and authoritative.
2. Support:
   - cognitive-only finalization,
   - workflow-only finalization,
   - combined cognitive + workflow finalization.
3. Implement wrapper bucket validation:

```json
{
  "cognitive": { ... },
  "workflow": { ... }
}
```

4. Distinguish clearly between:
   - missing bucket,
   - missing field inside bucket,
   - unknown bucket,
   - unknown field.
5. Mirror workflow lifecycle where needed for cross-agent/global workflow attribution.
6. Expand reducers/reporting to expose bucket-level results and finalization reasons.

### Acceptance

- combined finalization is tested,
- workflow outputs are first-class runtime outputs,
- and finalization results are inspectable from durable logs and reports.

## Phase 4 — enable safe write work through gating, not optimism

### Goal

Only after Phases 1–3 should the harness begin write-enabled autonomous repo work.

### Tasks

1. Strip or disable write-capable defaults in the CLI baseline until gating is complete.
2. Add a **disposable worktree / temporary copy harness** for write-enabled runs.
3. Introduce an allowlisted safe-command tier.
4. Reuse the eval harness for write-enabled microtasks:
   - read → edit → validate,
   - single-file and small multi-file bugfix/refactor tasks,
   - verification/reporting tasks.
5. Treat write-enabled evals as a separate tier from read-only stress testing.

### Acceptance

Write-enabled autonomous runs do not start until these are true:

- provider sees only effective enabled tools,
- empty policy intersections mean no tools,
- `maxToolCalls` is enforced,
- wall-time is enforced,
- malformed/missing finalization buckets are rejected reliably,
- session reports summarize tool violations and finalization failures,
- and execution occurs in a disposable worktree or equivalent sandbox.

## Phase 5 — promote sketch7 into the production layout

**Status: complete (2026-07-15).** The repository now has one runtime under
`cmd/maelstrom` and `internal`, module `github.com/comalice/maelstrom`.
Definitions and batteries live at top-level `agents/`, `workflows/`, `models/`,
and `evals/`. Legacy commands, the old `internal/` line, and sketch6 were
deleted after tagging the archive point as `sketch-era-final`.

### Goal

Once the runtime fixes and eval harness are real, promote sketch7 into the actual product structure in one focused migration.

### Proposed result

A production-facing layout such as:

```text
cmd/maelstrom/
internal/...
```

with the sketch7 runtime spine becoming the real code line.

### Tasks

1. Move / rename sketch7 packages into the production layout.
2. Rename the module as needed for the real product line.
3. Delete or archive the fully superseded legacy code:
   - `internal/` old line,
   - `cmd/is`, `cmd/sketch`, `cmd/sketch2` ... `cmd/sketch5`,
   - `sketch/sketch6`,
   - `dump.txt`,
   - obsolete notes.
4. Run `go mod tidy` to drop old dependencies that sketch7 no longer needs.
5. Keep historical planning/testing docs in `docs/done/` rather than leaving them in the active code path.

### Acceptance

- there is one clear runtime line,
- the repo no longer implies multiple live implementation branches,
- and new work lands in the production layout rather than under `sketch/`.

## Phase 6 — prove the product path

### Goal

Use the now-measured runtime to prove the first convincing “Maelstrom Code” product experience.

### Near-term product target

The anchor story remains:

> I describe what I want built, and Maelstrom turns the conversation into a recoverable workflow that implements it step by step.

### Tasks

1. Prove the end-to-end **conversation-to-execution** flow cleanly.
2. Tighten interrupt/resume behavior.
3. Separate core runtime behavior from experiment harness behavior in `main.go`.
4. Choose default coding-agent charts and default coding workflows using eval evidence rather than taste.
5. Add stronger tools only when the battery or product path proves they matter.

### Likely next tool upgrades after the battery

If justified by eval pressure, likely candidates include:

- better structural read tools,
- range or anchor-aware edits,
- review/diff tools,
- batched operations,
- and language-aware symbol/navigation tooling.

This is where stronger Dirac-style ideas become most valuable.

## Risks and failure modes to watch

### 1. Repair can hide raw incapacity

A repaired-valid result is useful, but it should not be confused with first-pass reliability.
All reports should distinguish raw validity from repaired validity.

### 2. Single-task overfitting is a real danger

The Python autoresearch review already showed how easy it is to drift toward benchmark-specific behavior.
The Go battery should run repeated tasks across a deck, not one golden scenario.

### 3. Local serving instability changes sequencing

Because the host machine may freeze under long-running local-model sessions, resumability and batch restart behavior are core requirements, not nice-to-haves.

### 4. Architectural beauty can outrun product value

Sketch7 already has enough architecture.
The next gains should come from:

- correctness,
- measurement,
- better default task charts,
- better tools where measured,
- and cleaner product flow.

## Relationship to other planning docs

This document is the top-level execution guide.
It should be read together with:

- `docs/done/sketch7-core-completion-plan.md` for the lower-level runtime queue,
- `docs/planning/sketch7-cognitive-state-stress-test-plan.md` for the next read-only experiment matrix,
- `docs/planning/minimal-eval-agent-and-metrics.md` for metric vocabulary and minimal eval-agent intent,
- `docs/planning/state-scoped-io-contracts-rollout.md` and `state-contract-schema-draft.md` for the contract model,
- `docs/planning/sketch7-work-tools-roadmap.md` for tool-evolution pressure,
- and `docs/planning/sketch7-mvp-package-plan.md` for the product-story and promotion context.

## Immediate next moves

Phases 1–5 are complete. Continue with Phase 6:

1. prove the conversation-to-execution product flow on the promoted binary;
2. tighten interrupt/resume behavior against that flow;
3. separate command/product concerns from experiment-only CLI concerns;
4. choose default coding agents and workflows from the recorded batteries;
5. add stronger tools only where product-path evidence requires them.
