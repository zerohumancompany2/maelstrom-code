# Sketch7 Core Completion Plan

**Status:** Preliminary  
**Date:** 2026-06-24  
**Scope:** Finish the outstanding items after the hidden-state task IO baseline.

## Current baseline

Sketch7 now has the core substrate:

- inference loop
- cognitive state reduction
- workflow binding and workflow state reduction
- hidden state-task context rendering
- durable session history
- durable workflow history
- state enter/exit records in session history
- cognitive bounded finalization on `maxInferenceTurns`
- assistant plaintext source records plus output contract evaluation records
- finalization retries
- tools, tool validation, and disabled-tool rejection

This is enough for controlled cognitive-state experiments. It is not yet enough for write-enabled autonomous repo work without a harness.

## Objective

Complete the remaining runtime semantics so state-local task IO works for cognitive states, workflow states, and combined cognitive/workflow outputs with authoritative bounds and tool policy enforcement.

## Non-goals

- Do not expose raw statechart internals to the model.
- Do not restore `cognitive_state` or `workflow_state` prompt chunks.
- Do not make the model responsible for choosing state transitions by name.
- Do not persist verbose hidden reasoning as a canonical artifact.
- Do not let stress-test agents mutate the repo until tool narrowing, bound enforcement, and sandboxing are in place.

## Workstream 1: Fix authoritative tool policy edge cases

### Problem

The runner computes effective allowed tools, but an empty effective list is currently ambiguous:

- no policy exists: fallback to agent tools is fine
- policy exists and intersection is empty: no tools should be allowed

`validateToolRequest` currently treats empty effective allowed tools as allow-all, so the second case can accidentally permit tools.

The provider also still usually sees all registered tool definitions, even when a state-local policy would later reject some of them.

### Plan

1. Replace `effectiveAllowedTools(view) []string` with a result object:
   ```go
   type EffectiveToolPolicy struct {
       Applied bool
       Tools   []string
   }
   ```
2. Validation rule:
   - if no policy applied and agent tools are empty: existing permissive/legacy behavior, if still desired
   - if policy applied and `Tools` is empty: reject every tool with `tool_not_enabled`
   - if policy applied and non-empty: allow only those names
3. Build provider requests with only effective enabled tool definitions.
4. Preserve finalization behavior: no tools sent, no tools executed.
5. Add regression tests for:
   - cognitive/workflow intersection is empty
   - provider receives narrowed tool definitions
   - finalization still receives no tool definitions

### Acceptance criteria

- `tool_not_enabled` is recorded for tools outside the effective policy.
- Empty policy intersection means no tools, not all tools.
- Inference envelope `IncludedToolNames` matches the tools actually exposed to the provider.

## Workstream 2: Enforce remaining state bounds

### Problem

The YAML model supports:

```yaml
bounds:
  maxInferenceTurns: 3
  maxToolCalls: 6
  maxWallTimeSeconds: 120
  maxFinalizationRetries: 1
```

Current enforcement is mainly cognitive `maxInferenceTurns` and finalization retries. `maxToolCalls` and `maxWallTimeSeconds` are parsed and rendered, but not authoritative.

### Plan

1. Add reducers/counting helpers:
   - tool calls since current cognitive state enter
   - tool calls since current workflow state enter
   - elapsed wall time since state enter, using record timestamps if available or adding timestamps if missing
2. Define bound precedence:
   - wall time hit closes tool use and requests finalization if output is declared
   - tool call hit closes tool use and requests finalization if output is declared
   - inference turn hit closes tool use and requests finalization if output is declared
   - if no output contract exists, stop with explicit bound-failed completion rather than continue silently
3. Store bound-hit reason in state exit and completion records.
4. Add tests for all bound types and both success/failure paths.

### Acceptance criteria

- All declared bounds are enforced, not just rendered.
- Bound exits are inspectable from session history.
- No bound failure can lead to continued tool execution.

## Workstream 3: Generalize finalization mode beyond cognitive-only

### Problem

Cognitive finalization exists. Workflow output contracts are rendered but not finalized/evaluated as first-class outputs. The desired output cases are:

1. cognitive output required, workflow absent or still in flight
2. cognitive and workflow outputs both required
3. cognitive still in flight, workflow output required

### Plan

1. Make `runtime.FinalizationMode` real:
   ```go
   type FinalizationMode struct {
       IsFinalizing     bool
       RequireCognitive bool
       RequireWorkflow  bool
       Reason           string
       RetryAttempt     int
   }
   ```
2. Add `DetermineFinalizationMode(view, sessionHistory, workflowHistory)`.
3. Render finalization instructions based on required buckets.
4. Disable tools whenever any required finalization bucket is active.
5. Evaluate assistant plaintext into zero, one, or two bucket evaluations:
   - cognitive evaluation record when cognitive bucket is required
   - workflow evaluation record when workflow bucket is required
6. Exit only the charts whose completion bucket validates.
7. Use one completion record for the loop result, with a stop reason that distinguishes:
   - `state_finalized`
   - `workflow_state_finalized`
   - `combined_state_finalized`
   - validation failure variants

### Acceptance criteria

- Cognitive-only, workflow-only, and combined finalization are all covered by unit tests.
- Workflow finalization records chart=`workflow` evaluations and workflow state exit.
- Combined finalization can pass one bucket and fail the other without losing attribution.

## Workstream 4: Implement wrapper JSON bucket validation

### Problem

The current validator checks one top-level object against one cognitive output contract. It does not yet validate the planned wrapper shape:

```json
{
  "cognitive": {},
  "workflow": {}
}
```

### Plan

1. Introduce a small output parser module separate from `runner/loop.go`.
2. Parse assistant plaintext once into an object.
3. Determine required buckets from finalization mode.
4. For each required bucket:
   - require the bucket key
   - require object value
   - validate required fields inside the bucket
   - apply strict unknown-field checks against bucket-level allowed fields
5. For non-finalization normal assistant outputs, either:
   - keep current shallow evaluator as observation-only, or
   - require wrapper only when a finalization bucket is active.
6. Record parser version and source assistant record ID on each evaluation record.

### Acceptance criteria

- Adjacent top-level objects are rejected as invalid JSON.
- Missing required bucket is distinguishable from missing required field inside a bucket.
- Unknown bucket is distinguishable from unknown field.
- Tests cover cognitive-only, workflow-only, combined, malformed JSON, missing bucket, missing field, unknown field.

## Workstream 5: Improve output schema expressiveness without overbuilding

### Problem

`StateOutputContract` currently supports schema name, required field names, and strict mode. That is useful but shallow.

### Plan

1. Keep the first upgrade intentionally small:
   ```yaml
   outputs:
     schema: cognitive_step_v1
     requiredFields: [summary, completion_signal]
     optionalFields: [evidence, risks, next_step]
     strict: true
   ```
2. Add optional fields to `StateOutputContract`.
3. Let strict validation permit required + optional + runtime-reserved fields.
4. Defer full JSON Schema until there are enough real outputs to justify it.

### Acceptance criteria

- Strict mode is no longer hard-coded to a tiny global field allowlist.
- Different states can have different allowed output fields.
- Existing tests still pass after migration.

## Workstream 6: Mirror workflow lifecycle where it matters

### Problem

Workflow transitions are durable in workflow history. Workflow state enter/exit records currently live in session history only.

### Plan

1. Decide whether workflow history should include state enter/exit records or only transition records.
2. If mirrored:
   - add workflow `StateEnter`/`StateExit` equivalents, or
   - add a generic workflow lifecycle record
3. Keep session records as the source for agent-local attribution.
4. Keep workflow records as cross-agent/global workflow attribution.

### Acceptance criteria

- Reports can answer: which agents advanced or finalized which workflow states?
- Cross-agent workflow history can be audited without replaying every session history.

## Workstream 7: Reporting and eval reducers

### Problem

The durable records exist, but reporting is still thin for the new lifecycle and gate semantics.

### Plan

Add reducers and report fields for:

- state enter/exit counts by chart/state/reason
- finalization attempts and retry exhaustion
- output bucket validation statuses
- tool policy violations by state
- bound hits by type
- provider-exposed tools vs requested tools
- state-task context snapshot churn

### Acceptance criteria

- A stress-test report can show whether agents respected state-local IO without manual log inspection.
- Tool leakage and context bloat can be measured.

## Suggested implementation order

1. Tool policy edge case + provider tool narrowing.
2. Wrapper parser/evaluator extraction.
3. Real finalization mode object.
4. Workflow-only and combined finalization.
5. Remaining bound enforcement.
6. State-specific allowed fields.
7. Workflow lifecycle mirror if still needed.
8. Reporting/eval reducers.

## Readiness gates before autonomous write-enabled repo work

Do not run write-enabled agents on outstanding repo tasks until these are true:

- provider sees only effective enabled tools
- empty tool-policy intersection means no tools
- max tool calls is enforced
- wall-time or host timeout is enforced
- finalization parser rejects malformed/missing buckets reliably
- session reports summarize tool violations and finalization failures
- the harness runs in a disposable worktree or temporary copy

Controlled read-only cognitive-state stress testing can start before all of this, as long as test tasks cannot mutate the repository.
