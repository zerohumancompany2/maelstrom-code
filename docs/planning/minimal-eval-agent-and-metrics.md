# Minimal Eval Agent Profile and Runtime Metrics Schema

**Status:** Proposed | **Date:** 2026-06-06

## Purpose

This document defines:

1. the first minimal Maelstrom eval agent profile
2. the first runtime metrics schema that should be emitted for evals

It is meant to bridge the gap between:

- state contract design
- loop/runtime implementation
- and eval reporting

Read alongside:

- `docs/evals.md`
- `docs/planning/state-scoped-io-contracts-rollout.md`
- `docs/planning/state-contract-schema-draft.md`

## Why a minimal eval agent

Before evaluating broader workflows or benchmark adapters, we need a runtime substrate whose failures are interpretable.

That substrate should:

- use a single cognitive state
- avoid workflow complexity
- expose a small tool surface
- operate under a strict output contract
- emit clear metrics for every turn

This gives us a stable baseline for reliability work.

## Minimal eval agent profile

## Design goals

The initial profile should optimize for:

- interpretability
- low ambiguity
- small context surface
- useful coding-task capability
- strong measurability

## Proposed profile

### Agent shape

- one cognitive state: `act`
- self-loop only
- no workflow required
- no transition tool required for first pass
- no bind/unbind tools
- no interrupt/resume tools

### Suggested tool set

Required:

- `list_files`
- `search_files`
- `read_file`
- `replace_text`
- `run_command`

Optional later:

- `get_file_skeleton`

Excluded initially:

- `bind_workflow`
- `unbind_workflow`
- `transition_state`
- `interrupt_session`
- `resume_session`

### Suggested state contract

The single `act` state should use `coding_step_v1` from `docs/planning/state-contract-schema-draft.md`.

### Suggested context inputs

Required:

- `task_statement`
- `recent_history`
- `allowed_tools`

Optional:

- `repo_context`
- `prior_tool_results`

## Illustrative YAML

```yaml
apiVersion: maelstrom/v1
kind: Agent
name: minimal-eval-coder
description: Minimal single-state coding agent for runtime evals
model: your-model

tools:
  - list_files
  - search_files
  - read_file
  - replace_text
  - run_command

context:
  inputBudget: 16000
  projections:
    - type: system
      name: system
      prompt: You are a precise coding agent. Complete one local task step at a time and always emit the required output shape.
    - type: interaction
    - type: cognitive_state
    - type: messages

cognitive:
  initialState: act
  states:
    - name: act
      description: Complete one coding step and either call a tool or finish.
      prompt: Solve the current local step using the required output schema.
      visibleTools: [list_files, search_files, read_file, replace_text, run_command]
      enabledTools: [list_files, search_files, read_file, replace_text, run_command]
      inputs:
        required: [task_statement, recent_history, allowed_tools]
        optional: [repo_context, prior_tool_results]
      outputs:
        schema: coding_step_v1
        requiredFields: [state, action_type, summary, completion_signal]
        strict: true
      completion:
        successWhen: [completion_signal == true]
```

## Runtime metrics schema

The runtime metrics schema should be emitted per run and should include enough detail to attribute failures to specific runtime layers.

## Session history as the source of metric facts

The recommended design is:

- runtime emits **structured session-history facts**
- eval reducers derive turn-level and run-level metrics from those facts
- reports aggregate the reducer outputs into summaries and recommendations

So the primary source of truth should be the agent session history, not ephemeral counters alone.

### Recommended layering

```txt
loop/runtime
  -> session history records
    -> metric reducers
      -> eval summaries and reports
```

### Why this matters

This gives us:

- auditable turn-by-turn provenance
- post-hoc metric recomputation
- easier debugging of bad runs
- the ability to improve scoring without changing the original execution record

### Important rule

Session history should record **events/facts**, not already-aggregated metrics.

So we should record things like:

- output parse succeeded
- output schema invalid
- output repaired
- retry attempted
- retry recovered
- tool validation passed/failed
- tool execution succeeded/failed
- completion signaled
- stop reason selected

But we should *not* record:

- `output_schema_success_rate`
- `tool_execution_success_rate`

Those belong to reducers and reports.

### Likely implementation pressure points

- `sketch/sketch7/logs/session.go`
- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/logs/persist.go`
- future eval scorer/reducer code

## Design goals

The schema should:

- support per-run summary metrics
- support per-turn classification
- support per-tool attribution
- support repair/retry accounting
- be stable enough for aggregation across evals

## Proposed run-level schema

```json
{
  "run_id": "uuid-or-stable-id",
  "agent_profile_id": "minimal-eval-coder",
  "model_id": "model-ref",
  "task_id": "task-identifier",
  "completed": true,
  "stop_reason": "final_response",
  "turn_count": 4,
  "tool_call_count": 3,
  "tool_call_failures": 0,
  "output_schema_failures": 1,
  "output_schema_repairs": 1,
  "retry_count": 1,
  "retry_recoveries": 1,
  "metrics": {
    "output_schema_success_rate": 0.75,
    "required_field_presence_rate": 1.0,
    "tool_call_validity_rate": 1.0,
    "tool_execution_success_rate": 1.0,
    "loop_completion_rate": 1.0
  },
  "turns": []
}
```

## Proposed turn-level schema

```json
{
  "turn_index": 2,
  "state": "act",
  "output_parse_status": "valid",
  "output_schema_status": "repaired_valid",
  "required_fields_present": true,
  "wrong_state_emission": false,
  "action_type": "tool",
  "tool_name": "read_file",
  "tool_call_valid": true,
  "tool_execution_status": "success",
  "retry_attempted": true,
  "retry_recovered": true,
  "completion_signal": false,
  "stop_reason": "continue"
}
```

## Required top-level fields

Each run result should capture at least:

- `run_id`
- `agent_profile_id`
- `model_id`
- `task_id`
- `completed`
- `stop_reason`
- `turn_count`
- `tool_call_count`
- `tool_call_failures`
- `output_schema_failures`
- `output_schema_repairs`
- `retry_count`
- `retry_recoveries`
- `metrics`
- `turns`

These fields may be emitted directly by a reducer/reporting layer even if the runtime itself only persists the lower-level session-history facts.

## Required turn-level fields

Each turn should capture at least:

- `turn_index`
- `state`
- `output_parse_status`
- `output_schema_status`
- `required_fields_present`
- `wrong_state_emission`
- `action_type`
- `tool_name` when applicable
- `tool_call_valid`
- `tool_execution_status`
- `retry_attempted`
- `retry_recovered`
- `completion_signal`
- `stop_reason`

## Metric definitions

## Reducer model

Each metric in this document should be computed from session-history records.

Examples:

- output schema success rate -> derived from output contract evaluation records
- retry recovery rate -> derived from retry records
- tool call validity rate -> derived from tool validation records
- tool execution success rate -> derived from tool request/result and execution outcome records
- loop completion rate -> derived from completion/termination records

This keeps the runtime append-only and the scoring layer flexible.

## 1. Output schema success rate

### Definition

Number of turns with schema-valid output divided by total turns.

### Why it matters

This is the first and most important metric for the state contract rollout.

### Recommended changes if weak

- simplify `coding_step_v1`
- improve state contract projection text
- improve retry instructions
- add rescue parser support for common malformed outputs

### Likely files

- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/runner/loop.go`

## 2. Required-field presence rate

### Definition

Turns with all required fields present divided by total turns.

### Why it matters

This distinguishes broad schema failure from specific missing-field failure.

### Recommended changes if weak

- shrink required field set
- improve error messages naming missing fields
- make required field summary more explicit in state prompt projection

### Likely files

- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/runner/loop.go`

## 3. Wrong-state emission rate

### Definition

Turns where output `state` does not match the runtime current state divided by total turns.

### Why it matters

This is a direct measure of whether the model is satisfying the state-local contract.

### Recommended changes if weak

- improve state contract projection
- tighten validation
- reduce state naming ambiguity

### Likely files

- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/runtime/reduce.go`
- `sketch/sketch7/runner/loop.go`

## 4. Tool call validity rate

### Definition

Valid tool calls divided by total proposed tool calls.

A valid tool call means:

- known tool name
- arguments parse successfully
- required arguments are present

### Recommended changes if weak

- add per-tool validators
- tighten parser/canonicalizer
- simplify tool interfaces

### Likely files

- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/tools/*.go`
- `sketch/sketch7/runner/loop.go`

## 5. Tool execution success rate

### Definition

Successful tool executions divided by total executed tool calls.

### Recommended changes if weak

- improve tool behavior and error surfacing
- tighten exact-match edit semantics
- improve command timeout handling

### Likely files

- `sketch/sketch7/tools/*.go`
- `sketch/sketch7/tools/*_test.go`

## 6. Repair rate

### Definition

Turns classified as `repaired_valid` divided by total turns.

### Why it matters

This tells us how much reliability is currently being borrowed from rescue logic rather than raw compliance.

### Recommended changes if weak in the wrong direction

If too low because rescue is absent:
- add rescue parser

If too high because raw compliance is poor:
- simplify schema and improve projection clarity

### Likely files

- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/runner/loop.go`

## 7. Retry recovery rate

### Definition

Recovered retries divided by retry attempts.

### Recommended changes if weak

- improve retry feedback messages
- preserve structured error reasons
- classify repeated failure modes

### Likely files

- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/provider/openai_compatible.go`

## 8. Loop completion rate

### Definition

Completed runs divided by total runs.

### Recommended changes if weak

- tighten final-response branch rules
- improve completion signaling semantics
- reduce unnecessary tool surface

### Likely files

- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/main.go`
- `sketch/sketch7/runner/loop_test.go`

## 9. Tool calls per success

### Definition

Average number of tool calls for completed runs.

### Why it matters

This is a useful efficiency indicator without yet needing broad benchmark infrastructure.

### Recommended changes if weak

- improve context compactness
- improve tool specificity
- improve stop conditions

### Likely files

- `sketch/sketch7/context/*`
- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/main.go`

## Stop reason vocabulary

The first pass should standardize stop reasons so loop failures are attributable.

Recommended first-pass values:

- `final_response`
- `tool_continue`
- `schema_invalid_exhausted`
- `tool_invalid_exhausted`
- `tool_execution_error`
- `max_steps`
- `provider_error`
- `unknown`

## Output classification vocabulary

Recommended first-pass output statuses:

### Parse status

- `valid_json`
- `repaired_json`
- `invalid_json`

### Schema status

- `valid`
- `repaired_valid`
- `invalid_retriable`
- `invalid_terminal`

### Tool execution status

- `not_applicable`
- `success`
- `validation_error`
- `execution_error`

## Reporting rule

Every aggregated eval report should include:

1. the metrics
2. the dominant failure modes
3. the recommended changes implied by those failure modes
4. the concrete files most likely to need modification

If the report does not tell us what to change, the schema is insufficient.

## Recommended implementation order

1. define the minimal agent profile in docs and config
2. define the runtime result schema
3. instrument loop code to emit turn-level and run-level results
4. aggregate results into eval summaries
5. use those summaries to prioritize schema, hydration, and tool changes

## Short answer to the rollout question

The practical rollout loop is:

1. define output schema
2. wire the schema into ingestion/hydration/runtime
3. validate and emit metrics at the schema boundary
4. use those metrics to decide what to change next

That fourth step is where evals become engineering guidance instead of just measurement.
