# State Contract Schema Draft

**Status:** Proposed | **Date:** 2026-06-06

## Purpose

This document proposes the first concrete schema shape for state-scoped inputs and outputs in sketch7.

It is intentionally narrow.

The goal is not to solve every future workflow need now. The goal is to define a first pass that lets us:

1. declare state-local output requirements
2. hydrate those requirements into runtime validation
3. measure contract satisfaction mechanically

This draft should be read alongside:

- `docs/evals.md`
- `docs/planning/state-scoped-io-contracts-rollout.md`
- `docs/planning/cognitive-state-workflow-binding-runtime.md`
- `docs/planning/model-and-agent-yaml.md`

## Design constraints

The first schema should be:

- small
- strict
- mechanically validatable
- useful for a minimal single-state coding agent
- extensible to future workflow states

It should not require the model to understand the state machine itself.

Instead, the state machine should define the local task and the runtime should validate whether the local task output was produced correctly.

## First-pass design choice

For the first rollout, we should define:

- a **state input contract** that names required runtime inputs
- a **state output contract** that names a required output schema
- a **state completion contract** that identifies how the runtime knows the state-local task is complete

The output contract should come first in implementation priority.

## Proposed definition-layer shape

### `defs.StateDefinition`

Current shape:

```go
type StateDefinition struct {
    Name            string
    Description     string
    VisibleTools    []string
    EnabledTools    []string
    Prompt          string
    AllowedTriggers []string
}
```

Proposed direction:

```go
type StateDefinition struct {
    Name            string
    Description     string
    VisibleTools    []string
    EnabledTools    []string
    Prompt          string
    AllowedTriggers []string
    Inputs          *StateInputContract
    Outputs         *StateOutputContract
    Completion      *StateCompletionContract
}

type StateInputContract struct {
    Required []string
    Optional []string
}

type StateOutputContract struct {
    SchemaName     string
    RequiredFields []string
    Strict         bool
}

type StateCompletionContract struct {
    SuccessWhen []string
}
```

This is intentionally simple.

Notes:

- `Inputs.Required` and `Inputs.Optional` are symbolic names resolved during hydration
- `Outputs.SchemaName` selects a concrete runtime validator shape
- `Outputs.RequiredFields` allows some schemas to be partially reused while still letting a state require a smaller subset
- `Outputs.Strict` determines whether unknown fields are tolerated in the first pass
- `Completion.SuccessWhen` starts as a simple rule list, not a full DSL

## Proposed YAML shape

### Cognitive state example

```yaml
cognitive:
  initialState: act
  states:
    - name: act
      description: Complete one small coding step and either call a tool or finish.
      prompt: Solve the local task using the required output format.
      visibleTools: [list_files, search_files, read_file, replace_text, run_command]
      enabledTools: [list_files, search_files, read_file, replace_text, run_command]
      inputs:
        required:
          - task_statement
          - recent_history
          - allowed_tools
        optional:
          - repo_context
          - prior_tool_results
      outputs:
        schema: coding_step_v1
        requiredFields:
          - state
          - action_type
          - summary
          - completion_signal
        strict: true
      completion:
        successWhen:
          - completion_signal == true
```

### Workflow state example

```yaml
statechart:
  initialState: implementing
  states:
    - name: implementing
      description: Produce an implementation-local step result.
      prompt: Complete the next implementation step and report in the required format.
      visibleTools: [read_file, replace_text, run_command, transition_state]
      enabledTools: [read_file, replace_text, run_command]
      inputs:
        required:
          - workflow_context
          - acceptance_criteria
          - allowed_tools
        optional:
          - prior_workflow_notes
      outputs:
        schema: workflow_step_v1
        requiredFields:
          - state
          - summary
          - artifact_status
          - completion_signal
        strict: true
      completion:
        successWhen:
          - completion_signal == true
```

## First-pass output schemas

The first output schemas should be very small.

## `coding_step_v1`

Recommended first-pass shape:

```json
{
  "state": "act",
  "action_type": "tool" | "final",
  "summary": "short description of what this step is doing",
  "completion_signal": true | false,
  "tool": {
    "name": "read_file",
    "arguments": {
      "path": "foo.go"
    }
  },
  "final_response": "optional final user-facing response"
}
```

### Required behavior

- `state` must equal the current cognitive state
- `action_type` must be either `tool` or `final`
- `summary` must be present and non-empty
- `completion_signal` must be present
- if `action_type == "tool"`, then `tool` is required and `final_response` should be empty or absent
- if `action_type == "final"`, then `final_response` is required and `tool` should be empty or absent

### Why this shape

It gives us a strict control envelope without requiring a large ontology.

It is also compatible with future native tool-calling paths because the control object and the tool call can still be derived together.

## `workflow_step_v1`

Recommended initial workflow-oriented shape:

```json
{
  "state": "implementing",
  "summary": "short workflow-local update",
  "artifact_status": "none" | "in_progress" | "ready",
  "completion_signal": true | false,
  "handoff_note": "optional durable note"
}
```

This should come after `coding_step_v1`, not before.

## State input contract symbols

The input contract should initially reference a small vocabulary of symbolic inputs.

Recommended first-pass symbols:

- `task_statement`
- `recent_history`
- `allowed_tools`
- `repo_context`
- `prior_tool_results`
- `workflow_context`
- `acceptance_criteria`
- `prior_workflow_notes`

Hydration should be responsible for turning these into concrete projections or validation errors.

## Hydration model

The authored schema should remain declarative.

Hydration should:

1. resolve input symbols into runtime projection dependencies
2. resolve `SchemaName` into a concrete validator
3. attach the hydrated contract to runtime state metadata
4. fail early if the state references unknown schemas or unknown required inputs

## Proposed runtime direction

We likely need a runtime view richer than the current `CognitiveView` and `WorkflowView`.

Illustrative direction:

```go
type HydratedStateContract struct {
    Inputs     HydratedInputContract
    Outputs    HydratedOutputContract
    Completion HydratedCompletionContract
}

type HydratedInputContract struct {
    RequiredProjections []string
    OptionalProjections []string
}

type HydratedOutputContract struct {
    SchemaName string
    RequiredFields []string
    Strict bool
}

type HydratedCompletionContract struct {
    SuccessWhen []string
}
```

And then:

```go
type CognitiveView struct {
    CurrentState string
    VisibleTools []string
    EnabledTools []string
    Prompt       string
    Contract     *HydratedStateContract
}
```

Potentially the same for `WorkflowView`.

## Prompting direction

The state contract should be projected into prompt context explicitly.

The current cognitive projection is too informal for this next phase.

Current shape is effectively:

- current state name
- prompt
- visible/enabled tools

It should evolve toward including:

- required input summary
- required output schema summary
- state-local completion instruction

For example:

> Current state: act. Local task: complete one coding step. Required output fields: state, action_type, summary, completion_signal. If action_type=tool, include tool.name and tool.arguments. If action_type=final, include final_response.

## Validation boundary

The first validation boundary should be at the loop/provider boundary.

That means after model output is received, but before any tool executes, runtime should:

1. parse the output envelope
2. validate the schema for the current state
3. classify outcome as:
   - valid
   - repaired-valid
   - invalid-retriable
   - invalid-terminal
4. only then proceed with tool execution or final response handling

## Metrics this schema should unlock

### 1. Output schema success rate

**Meaning**
- how often the current state contract is satisfied without rescue

**Recommended changes if weak**
- simplify schema
- improve prompt projection
- improve retry feedback

**Likely files**
- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/runner/loop.go`

### 2. Required-field presence rate

**Meaning**
- how often required fields are present

**Recommended changes if weak**
- tighten state-local instructions
- reduce required surface area
- improve validation error messages

**Likely files**
- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/provider/provider.go`

### 3. Wrong-state emission rate

**Meaning**
- how often output `state` does not match the current runtime state

**Recommended changes if weak**
- improve state projection clarity
- add stricter contract validation
- reduce state naming ambiguity

**Likely files**
- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/runtime/reduce.go`
- `sketch/sketch7/runner/loop.go`

### 4. Tool/final branch correctness rate

**Meaning**
- how often `action_type` is coherent with the rest of the output

**Recommended changes if weak**
- tighten schema rules
- improve branch-specific retry messages
- separate terminal response handling more clearly

**Likely files**
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/runner/loop.go`

### 5. Repair rate and retry recovery rate

**Meaning**
- how often malformed but near-correct output can be rescued or retried into compliance

**Recommended changes if weak**
- improve rescue parser
- improve retry nudge messages
- track red-flag categories

**Likely files**
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/runner/loop.go`

## Implementation recommendation

Implementation order should be:

1. add definition-layer schema fields to planning and then code
2. add catalog/YAML support
3. add hydration support for state contracts
4. add prompt projection of required output contract
5. add loop-level validation before tool execution
6. add result classification and metrics capture

## Decision rule

The first-pass schema is good if it is:

- small enough that the minimal eval agent can satisfy it consistently
- strict enough that failures are meaningful
- expressive enough to distinguish tool use from final response
- simple enough that bad metrics clearly suggest concrete code changes
