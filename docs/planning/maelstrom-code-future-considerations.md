# Maelstrom Code Future Considerations

**Status:** Reference note  
**Date:** 2026-07-04

## Why this note exists

During implementation, some follow-up ideas will be worth remembering but are **not** immediate execution priorities.

This file exists to capture those items without:

- bloating the active completion plan,
- turning every implementation note into a near-term requirement,
- or quietly increasing architectural weight.

## Core design guardrail: stay lean on purpose

A recurring concern for Maelstrom Code is avoiding the fate of many modern coding-agent harnesses:

- hundreds of thousands of lines of code,
- large orchestration towers,
- thick middleware stacks,
- and sprawling abstractions around what often reduces to:
  - prompt assembly,
  - tool exposure,
  - output validation,
  - durable recording,
  - and iterative execution.

The existence of very large harnesses in the 500k–1M LOC range is a useful warning.

Maelstrom Code should stay aggressively skeptical of complexity that does not clearly improve:

- reliability,
- inspectability,
- eval performance,
- safety,
- or day-to-day usability.

## Current lean doctrine

When choosing whether to add a mechanism, bias toward:

1. **small runtime primitives** over orchestration frameworks,
2. **durable records + reducers** over large in-memory control layers,
3. **tight tool contracts** over broad generic automation surfaces,
4. **simple authored schemas** over expressive DSLs unless measurement demands more,
5. **state-local bounded tasks** over elaborate planner stacks,
6. **eval-backed additions** over speculative architecture.

A good heuristic:

> if a feature cannot explain which failure mode, metric, or product behavior it improves, it probably does not belong in the core runtime yet.

## Future considerations worth preserving

### 1. Stronger wall-time attribution

The current `maxWallTimeSeconds` enforcement can be based on inference-envelope timing.
That is enough for early bounded execution, but future refinements may include:

- explicit state-enter timestamps,
- duration summaries by state,
- and stronger auditability for bound-hit reasoning.

This matters only if the current lightweight mechanism proves insufficient.

### 2. Output parser extraction

`runner/loop.go` still owns too much parsing and validation logic.
A small dedicated parsing/validation boundary should likely emerge, but it should remain:

- narrow,
- runtime-owned,
- and focused on state-output validation rather than turning into a general protocol framework.

### 3. Wrapper bucket validation

Support for structured output buckets like:

```json
{
  "cognitive": { ... },
  "workflow": { ... }
}
```

is a likely future step.

But it should be added only as far as needed to support:

- generalized finalization,
- workflow-state outputs,
- and clearer eval semantics.

Avoid overdesigning a broad message schema language.

### 4. Richer output field typing

Today’s output contracts are intentionally simple.
Future work may add:

- `optionalFields`,
- field typing,
- reserved runtime fields,
- or state-specific strict schemas.

The main guardrail is to stop well before reinventing a heavy general schema system unless repeated eval pressure proves it necessary.

### 5. Workflow lifecycle mirroring

There is likely value in stronger workflow-history lifecycle mirroring for:

- cross-session attribution,
- multi-agent workflow visibility,
- and reporting.

But this should stay in the form of a few durable records and reducers, not a separate orchestration subsystem.

### 6. Structural read and edit tools

Dirac-style lessons remain compelling:

- file skeletons,
- symbol-level reads,
- anchor-aware edits,
- batched operations.

These are potentially high-leverage because they improve token efficiency and first-pass correctness.
But they should be justified by eval pressure, not added because they are elegant.

### 7. Tiny specialist models

Needle-style tiny models may eventually be useful for narrow jobs such as:

- gating,
- classification,
- route selection,
- or cheap repair assistance.

This is a future possibility, not part of the near-term runtime plan.

### 8. Local vs hosted model execution strategy

The harness should preserve one OpenAI-compatible path so that:

- local llama.cpp / llama-swap serving,
- LM Studio-style serving,
- and OpenRouter-backed experiments

all fit the same runtime path.

This lets Maelstrom stay local-model-first in architecture without being blocked by temporary hardware instability.

### 9. Keep the CLI harness smaller than the runtime

`main.go` currently does too much.
When promotion happens, the runtime should stay conceptually smaller than the CLI or experiment harness around it.

The product should not drift into a situation where most complexity lives in glue layers that obscure the actual runtime semantics.

### 10. Eval batch retry semantics

Batch resume currently treats any recorded run as done, including runs that
ended in provider errors (timeouts, transport failures). Live OpenRouter
batches occasionally hit slow constrained-decoding backends, so a
`--retry-failed` (or error-aware resume) mode may become worth adding once
error rates are observed across real batteries. Until then, rerunning into a
fresh output file is an acceptable manual workaround.

## Anti-goals

These are not absolute forever bans, but they should be resisted unless there is strong evidence.

### Avoid prematurely adding

- a large plugin/middleware framework,
- a broad policy DSL,
- a generic agent graph engine,
- a giant prompt-template subsystem,
- a heavyweight schema language,
- a broad multi-agent coordination platform,
- or a tool surface much larger than the eval/product path requires.

## Practical rule for future additions

Before expanding the runtime, ask:

1. What exact failure or limitation are we seeing?
2. Can a smaller record/reducer/tool-contract fix it?
3. Can this be proven in evals?
4. Does this make the core runtime clearer or heavier?
5. Would a user notice the improvement in daily coding use?

If the answer set is weak, defer the addition.

## Relationship to the main plan

This note is intentionally non-authoritative.

- `docs/planning/maelstrom-code-completion-plan.md` remains the main execution plan.
- `docs/planning/sketch7-core-completion-plan.md` remains the detailed runtime completion queue.

This file is just the reminder to keep Maelstrom Code:

> small, sharp, inspectable, and earned by measurement.
