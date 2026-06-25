# Sketch7 Hidden-State Task IO Plan

**Status:** Draft for refinement  
**Date:** 2026-06-24

## Purpose

This plan captures the next sketch7 direction for state-local IO without exposing statechart mechanics to the model.

The core move is:

> Runtime statecharts remain private control machinery. The model sees bounded local task prompts, available inputs, tool constraints, and required return shapes.

This replaces the current sketch7 pattern where context chunks can render raw cognitive/workflow state names, workflow progress, and suggested transitions.

## Current conclusion

We should not build a broad new IO orchestration subsystem first.

Instead, use existing sketch7 primitives:

- context chunks for model-facing task prompts and output instructions
- session history records for state lifecycle and parse/evaluation facts
- YAML state definitions for output contracts and bounds
- runner enforcement for tool/turn bounds and finalization retries

The implementation should stay small and event-sourced:

- assistant plaintext remains in `AssistantMessageRecord`
- parse/evaluation results are separate records referencing that plaintext
- context chunks record what instructions/contracts were shown to the model
- state enter/exit records record lifecycle boundaries

No derived parsed-output payload record is required for the first pass.

## Design principles

1. **Hide statechart internals.**
   - Do not render cognitive state names as control semantics.
   - Do not render workflow state names as control semantics.
   - Do not render transition names or suggested transitions.

2. **Render task-facing chunks.**
   - The model sees current work instructions, constraints, tools, and required outputs.
   - State prompts are persisted like other context chunks.

3. **Use bounded state-local execution.**
   - Bounds are declared in YAML.
   - Runtime enforces bounds.
   - When a bound is hit, runtime enters a finalization/no-tools phase.

4. **Keep plaintext as source output.**
   - The assistant response is stored once as plaintext.
   - Parse pass/failure records reference the assistant record.
   - Parse records include host/parser version information.

5. **Do not preserve the old state chunks.**
   - We are still greenfield enough to remove them entirely.
   - If needed later, recover old behavior from git history.

## Model-facing output cases

There are three distinct output cases. The renderer and validator must support all three.

### Case 1: Cognitive output required; workflow absent or still in flight

The current cognitive state requires a conclusion/checkpoint output, but either:

- no workflow is bound, or
- the bound workflow state is not asking for a workflow output yet.

The model should return only the cognitive output shape.

Example conceptual return shape:

```json
{
  "cognitive": {
    "summary": "...",
    "completion_signal": true,
    "artifacts": {}
  }
}
```

### Case 2: Cognitive output and workflow output both required

Both the current cognitive state and the current workflow state require outputs.

The model should return both outputs in one machine-readable response.

Example conceptual return shape:

```json
{
  "cognitive": {
    "summary": "...",
    "completion_signal": true,
    "artifacts": {}
  },
  "workflow": {
    "summary": "...",
    "completion_signal": true,
    "artifact_status": "ready"
  }
}
```

### Case 3: Cognitive state still in flight; workflow output required

The cognitive state does not currently require a conclusion output, but the workflow state requires a workflow output.

The model should return only the workflow output shape.

Example conceptual return shape:

```json
{
  "workflow": {
    "summary": "...",
    "completion_signal": true,
    "artifact_status": "ready"
  }
}
```

### Open decision: adjacent objects vs wrapper object

One possible prompt shape is:

```txt
{...cognitive output...},{...workflow output...}
```

But that is not a single valid JSON document. The safer first-pass recommendation is a single wrapper object with optional/required buckets:

```json
{
  "cognitive": {},
  "workflow": {}
}
```

The runtime decides which buckets are required for a given finalization request.

This keeps parsing simple and avoids making the model emit multiple top-level JSON objects.

## Context chunk direction

### Remove old chunks

Remove the old model-facing state chunks entirely:

- `cognitive_state`
- `workflow_state`

These currently risk exposing implementation details such as current state names, workflow directives, and suggested transitions.

### Add a new hidden-state task chunk

Add one new chunk/projection type:

```yaml
type: state_task
```

This chunk renders private cognitive/workflow state as public task guidance.

It should include:

- current local task prompt/instructions
- input expectations, when declared
- allowed tools
- active bounds and current usage
- finalization behavior
- required output buckets and schema names, depending on the current runtime mode

It should not include:

- raw statechart transition names
- suggested next transitions
- workflow IDs unless needed for task semantics
- instructions to operate the state machine

### Persistence

The `state_task` chunk should be a normal derived context section:

```go
Section{
    Name:        "state_task",
    Role:        "system",
    LogicalKey:  "state_task",
    SectionType: "state_task",
    SourceKind:  "derived_state_task",
}
```

Because it is non-static and has a logical key, existing `ContextSnapshotRecord` persistence should capture it automatically.

## YAML additions

State outputs mostly already exist:

```yaml
outputs:
  schema: coding_step_v1
  requiredFields:
    - summary
    - completion_signal
  strict: true
```

Add state bounds:

```yaml
bounds:
  maxInferenceTurns: 3
  maxToolCalls: 6
  maxWallTimeSeconds: 120
  maxFinalizationRetries: 1
```

Initial implementation can support only `maxInferenceTurns`, `maxToolCalls`, and `maxFinalizationRetries`, while preserving the struct field for `maxWallTimeSeconds` if useful.

Suggested definition-layer addition:

```go
type StateBoundsContract struct {
    MaxInferenceTurns      int
    MaxToolCalls           int
    MaxWallTimeSeconds     int
    MaxFinalizationRetries int
}
```

Add it to `defs.StateDefinition`, then hydrate/reduce it into `runtime.CognitiveView` and `runtime.WorkflowView`.

## Runtime state lifecycle records

Introduce explicit lifecycle facts for cognitive and workflow states.

### State enter record

```go
type StateEnterRecord struct {
    SessionBaseRecord
    Chart          string // cognitive | workflow
    StateName      string
    DerivedFromIDs []string
}
```

### State exit record

```go
type StateExitRecord struct {
    SessionBaseRecord
    Chart              string // cognitive | workflow
    StateName          string
    Reason             string // completed | transition | max_turns | validation_failed | etc.
    DerivedFromIDs     []string
    ParseRecordIDs     []string
    CompletionAccepted bool
}
```

The existing transition records can remain conceptually separate:

- transition record = edge fired
- enter/exit records = lifecycle boundaries

For v1, session-scoped enter/exit records are enough. Workflow-history mirrors can come later if needed.

## Parse/evaluation records

The assistant plaintext output should remain an `AssistantMessageRecord`.

After recording the assistant plaintext, runtime should parse/evaluate it and append a pass/failure record referencing that plaintext.

Extend or replace `OutputContractEvaluationRecord` with fields like:

```go
type OutputContractEvaluationRecord struct {
    SessionBaseRecord

    Chart              string // cognitive | workflow | combined
    StateName          string
    SchemaName         string
    SourceRecordID     string // assistant message record id
    HostVersion        string
    ParserVersion      string

    ParseStatus        string // valid_json | invalid_json | plain_text
    ValidationStatus   string // valid | missing_required_fields | unknown_fields | wrong_bucket | etc.
    RequiredFields     []string
    MissingFields      []string
    RawContentPreview  string
}
```

For combined outputs, either:

1. create one evaluation record per required bucket, or
2. create one combined evaluation record with bucket-level details.

The simpler first pass is probably one evaluation record per required bucket, each referencing the same assistant message.

## Finalization behavior

### Normal in-state operation

While a state is under its bounds:

- runtime renders `state_task`
- runtime exposes effective allowed tools
- model can use tools or answer normally, depending on provider output

### Bound hit

When a relevant bound is hit:

- runtime renders `state_task` in finalization mode
- runtime exposes no tools
- runtime asks for the required output bucket(s)
- model response is stored as plaintext
- runtime parses/evaluates required bucket(s)

### Finalization retries

If parse/evaluation fails:

1. append a retry/failure fact
2. render a second no-tools finalization prompt explaining the schema failure
3. allow up to the configured retry limit

The retry limit is declared in state bounds:

```yaml
bounds:
  maxFinalizationRetries: 1
```

If omitted, v1 should default to `1` retry unless we decide strict zero-retry behavior is safer for a particular eval profile.

If the configured retry limit is exhausted:

- append parse/evaluation failure record(s)
- append state exit failure record if appropriate
- return loop error or completion failure

Repair transformations beyond bounded retry are explicitly out of scope for v1.

## Bounds enforcement

### Inference turns

Define an inference turn as:

> one provider request/response bundle created while a given chart/state is active.

Implementation can count `InferenceEnvelopeRecord`s since the latest `StateEnterRecord` for that chart/state.

### Tool calls

Define a tool call as:

> one accepted tool request while a given chart/state is active.

Implementation can count `ToolCallRequestRecord`s since the latest `StateEnterRecord`, or count only successfully validated/executed calls if stricter semantics are desired.

### Wall time

Defer enforcement unless needed. Preserve YAML shape if cheap.

## Tool gating

The current effective-tool calculation should become authoritative.

If a tool is not enabled for the current cognitive/workflow state combination, runtime should reject it and should not execute it.

Current gap to fix:

- `validateToolRequest` computes `effectiveAllowedTools(view)`
- but tool validity does not fully depend on membership in that effective allowed set

V1 should add a `tool_not_enabled` validation failure reason and avoid executing the request.

## State task chunk output selection

The `state_task` renderer needs a runtime output-mode decision.

Suggested internal mode:

```go
type RequiredOutputMode struct {
    RequireCognitive bool
    RequireWorkflow  bool
    Finalizing       bool
    RetryAttempt     int
}
```

Rules:

- if cognitive bound hit and cognitive outputs declared: require cognitive bucket
- if workflow bound hit and workflow outputs declared: require workflow bucket
- if both are required: require both buckets
- if neither is required: render normal task guidance and tool constraints
- in finalization mode: expose no tools and require JSON-only output
- if finalization output is invalid: retry until `maxFinalizationRetries` is exhausted

Open question:

- Should workflow output be triggered only by workflow bounds, or can cognitive finalization also ask for workflow output when workflow completion conditions appear satisfied?

## Implementation phases

### Phase 1: Remove old state chunks and add `state_task`

Files likely touched:

- `sketch/sketch7/catalog/load.go`
- `sketch/sketch7/prompt/plan.go`
- `sketch/sketch7/context/sections.go`
- `sketch/sketch7/main.go`
- tests in `catalog`, `prompt`, `context`, `runner`

Tasks:

- delete support for `cognitive_state` and `workflow_state`
- add support for `state_task`
- update default agent projections to use `state_task`
- render task-facing content only
- verify context snapshot persistence for `state_task`

### Phase 2: Add bounds to state definitions and runtime views

Files likely touched:

- `sketch/sketch7/defs/workflow.go`
- `sketch/sketch7/catalog/load.go`
- `sketch/sketch7/runtime/view.go`
- `sketch/sketch7/runtime/reduce.go`
- tests in `catalog` and `runtime`

Tasks:

- add `bounds` YAML parsing
- carry bounds into cognitive/workflow views
- add helpers to count turns/tool calls since state enter

### Phase 3: Add state enter/exit lifecycle records

Files likely touched:

- `sketch/sketch7/logs/session.go`
- `sketch/sketch7/logs/persist.go`
- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/tools/transition.go`
- tests in `logs`, `runner`, `tools`

Tasks:

- append initial cognitive enter record when needed
- append workflow enter record when workflow binding initializes, if applicable
- append exit/enter around transitions
- keep existing transition records if still useful

### Phase 4: Finalization and parse/retry/fail

Files likely touched:

- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/context/sections.go`
- `sketch/sketch7/logs/session.go`
- `sketch/sketch7/logs/reduce.go`
- tests in `runner`, `context`, `logs`

Tasks:

- detect bound-hit finalization mode
- render finalization instructions in `state_task`
- send no tools during finalization
- store assistant plaintext before parse/eval
- append parse/evaluation records referencing assistant plaintext
- retry once on invalid output
- fail after configured finalization retries are exhausted

### Phase 5: Tighten tool enforcement

This can happen earlier if convenient.

Files likely touched:

- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/runner/loop_test.go`

Tasks:

- make effective enabled tools authoritative
- do not execute invalid/disabled tool requests
- record `tool_not_enabled`

## Test targets

Important tests to add or update:

1. `state_task` projection accepted and old state projections rejected.
2. Default agent definition uses `state_task`, not old chunks.
3. `state_task` chunk avoids transition/state-machine language.
4. `state_task` chunk includes declared bounds and output schemas.
5. `state_task` context snapshot is persisted and included in inference envelope.
6. Bounds are ingested from YAML and visible in runtime views.
7. State enter records are appended once per state entry.
8. Transitions append exit/transition/enter lifecycle facts.
9. Bound hit produces no-tools finalization request.
10. Invalid finalization output retries up to the configured bound.
11. Output validation fails after configured finalization retries are exhausted.
12. Valid finalization output appends parse/evaluation pass records.
13. Cognitive-only, workflow-only, and combined output cases validate correctly.
14. Disabled tools are rejected and not executed.

## Open questions for refinement

1. Should model-facing output be a wrapper object with `cognitive` and `workflow` buckets, or should there be separate schema prompts per required output?
2. Should one assistant response be allowed to satisfy both cognitive and workflow outputs?
3. What triggers workflow output finalization: workflow bounds only, cognitive finalization when workflow appears complete, or explicit workflow completion mode?
4. Should parse/evaluation be one record per bucket or one combined record?
5. Should workflow state enter/exit be mirrored into workflow history in v1?
6. Should `maxToolCalls` count requested, valid, or successfully executed tool calls?
7. Should old `CognitiveTransitionRecord` and `WorkflowTransitionRecord` remain after enter/exit records exist?
8. What should the first concrete cognitive output schema be?
9. What should the first concrete workflow output schema be?
10. Should finalization retry instructions be rendered as a context chunk or appended as a synthetic user message?

## Recommended first concrete slice

Start with the smallest useful change:

1. remove old state chunks
2. add `state_task` chunk
3. add bounds fields to definitions/runtime views
4. render bounds/output requirements in `state_task`
5. add tests proving the new chunk is persisted and old chunks are gone

Then add finalization/retry behavior.

This keeps the first implementation focused on model-facing shape and avoids mixing rendering, lifecycle records, and finalization enforcement in one large change.
