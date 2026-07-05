# Sketch6 Test Plan

**Status:** Proposed | **Date:** 2026-05-30

## Goal

Add a minimal but durable test suite for `sketch6` that catches architectural regressions early while the sketch is still evolving.

The test strategy should start from deterministic domain primitives and work upward through:

- authored definition ingestion,
- hydration,
- context projection,
- tool execution,
- provider stub behavior,
- and integrated runner scenarios.

## Principles

### 1. Start with deterministic seams

The first tests should target pure or nearly-pure logic:

- statechart compilation and transitions
- reducers/snapshots
- hydration validation
- chunk planning/projection

These are the cheapest and most stable tests.

### 2. Prefer narrow tests before scenario tests

A single large end-to-end test is useful, but only after the lower layers are covered.

If the only tests are scenario tests, failures will be hard to localize.

### 3. Use the weather flow as the first integrated scenario

The existing `sketch6` weather example is already our best small orchestration path.

It should become the first integrated regression scenario.

### 4. Keep sketch tests local to sketch6

These tests should live under `sketch/sketch6/...` and should not try to validate mainline production behavior.

They are meant to stabilize the sketch sandbox.

## Test pyramid

## Level 1: domain primitives

### Statechart tests

Target:
- `sketch/sketch6/charts`

Tests:
- agent chart built from authored cognitive transitions
- workflow chart built from authored workflow transitions
- valid trigger transitions to expected next state
- invalid trigger returns error
- unknown chart returns error

### Reducer / snapshot tests

Target:
- `sketch/sketch6/runtime`
- helper reducers added around recovery logic

Tests:
- latest cognitive state is recovered from transition history
- latest workflow state is recovered from workflow history
- latest binding state is recovered from bind/unbind records
- unrelated records do not affect recovered current state

### Tool policy tests

Target:
- cognitive/workflow state snapshots and effective tool policy helpers

Tests:
- cognitive state exposes expected visible tools
- cognitive state exposes expected enabled tools
- workflow state exposes expected visible tools
- effective tool set reflects layered policy

## Level 2: authored ingestion and hydration

### Agent registry ingestion

Target:
- `sketch/sketch6/registry/agent`

Tests:
- valid agent YAML decodes and hoists correctly
- missing model rejected
- missing cognitive initial state rejected
- invalid chunk type rejected during later validation path
- cognitive states/transitions hoisted correctly

### Workflow registry ingestion

Target:
- `sketch/sketch6/registry/workflow`

Tests:
- valid workflow YAML decodes and hoists correctly
- missing workflow name rejected
- missing/invalid statechart rejected
- workflow states/transitions hoisted correctly

### Model registry ingestion

Target:
- `sketch/sketch6/registry/model`

Tests:
- valid model YAML decodes and hoists correctly
- missing providers rejected
- malformed provider entry rejected

### Hydrator tests

Target:
- `sketch/sketch6/hydrate`

Tests:
- logical model is resolved from registry
- unknown model rejected
- unknown tool rejected
- inputBudget > contextWindow rejected
- overrides correctly replace model defaults
- hydrated runtime agent contains expected provider/model selection

## Level 3: assembly / named chunk projection

### Plan building

Target:
- `sketch/sketch6/assembly.BuildPlan`

Tests:
- `system` chunk resolves
- `messages` chunk resolves
- `cognitive_state` chunk resolves
- `workflow_state` chunk resolves
- `binding` chunk resolves
- unknown chunk type returns error

### Individual chunk tests

Target:
- chunk implementations

Tests:
- static system chunk emits expected content
- cognitive chunk emits state name, prompt, visible/enabled tools
- workflow chunk emits workflow ID/state/description/context
- binding chunk emits nothing when unbound
- binding chunk emits workflow binding info when bound
- recent history chunk respects max history limit

## Level 4: tool execution

### Transition tool

Target:
- `sketch/sketch6/tools/transition.go`

Tests:
- agent transition from initial state succeeds
- workflow transition from initial state succeeds
- invalid trigger returns tool error result
- workflow transition appends durable workflow history entry
- session transition record is still emitted

### Weather tool

Target:
- `sketch/sketch6/tools/weather.go`

Tests:
- succeeds only in `lookup_pending`
- appends durable workflow transition to `data_ready`
- returns error outside expected workflow state

## Level 5: provider stub behavior

Target:
- `sketch/sketch6/provider`

Tests:
- observe + available produces expected cognitive transition request
- orient + available advances workflow
- orient + lookup_pending advances to act
- act + lookup_pending requests weather tool
- act + data_ready transitions agent back appropriately
- terminal state combination returns assistant output
- unexpected combinations return fallback assistant output

## Level 6: runner / integration

### Minimal loop tests

Target:
- `sketch/sketch6/runner`

Tests:
- loop appends tool call request/result records
- loop terminates when no tool calls remain
- loop guard trips on stuck behavior

### First scenario test

Scenario:
- model definition
- workflow definition
- agent definition
- hydrated runtime
- workflow history
- provider stub
- weather + transition tools

Assertions:
- expected session transition sequence
- expected workflow durable transition sequence
- expected final assistant output
- expected bind/unbind records

This should become the first high-value regression test.

## Level 7: recovery / restart

Target:
- reducers and runtime reconstruction helpers

Tests:
- recover cognitive state from session history
- recover workflow state from workflow history
- recover binding state from bind/unbind records
- rebuild runtime view from recovered snapshots
- projected chunks after recovery reflect current recovered state

## Initial implementation order

1. chart/state transition tests
2. hydrator tests
3. assembly plan tests
4. transition tool tests
5. weather tool tests
6. provider stub tests
7. one integrated runner scenario
8. recovery/restart reducer tests

## Minimal first batch

If we only do the smallest useful batch first, it should be:

1. `charts` tests
2. `hydrate` tests
3. `assembly.BuildPlan` tests
4. `TransitionTool` tests
5. one runner happy-path scenario

That batch should catch most architectural breakage quickly.

## Notes

The current `sketch6` runtime is still behaviorally in flux, so the integrated scenario test should be added only after the scenario is brought to a stable terminal flow.

Until then, prioritize deterministic lower-level tests first.
