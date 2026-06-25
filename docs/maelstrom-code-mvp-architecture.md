# MaelstromCode MVP Architecture

**Status:** Draft  
**Scope:** Local coding agents, statecharts, context projection, IO contracts, eval-driven reliability  

## Purpose

MaelstromCode is the lightweight Go runtime for building reliable, inspectable coding agents that can run for long periods, interact with durable workflows, and remain understandable under local-model constraints.

This MVP is not trying to solve the full zero-human company problem yet. It is focused on making coding agents useful, reliable, measurable, and composable.

The immediate architectural focus is:

1. durable agent sessions and workflow histories
2. statechart-driven cognitive and workflow behavior
3. context-map-driven projection into inference
4. explicit input/output contracts for cognitive and workflow states
5. evals that measure runtime reliability and coding-task progress
6. sane default agent charts and workflow charts for local coding work

## Product boundary

## In scope for MaelstromCode MVP

- local coding agents
- locally hosted small/capable models
- durable session history
- durable workflow history
- cognitive statecharts for agent-local behavior
- workflow statecharts for durable work-item phase
- context-map chunks and projections
- state-scoped input/output contracts
- local file/code/command tools
- runtime reliability metrics
- session reports and evals
- default coding-agent charts
- default coding workflows for common workloads and tech stacks

## Out of scope for MaelstromCode MVP

- workflows with broad external side effects
- autonomous deployment/publishing/payment/customer operations
- irreversible business-process automation
- generalized multi-agent company simulation
- relying on prose-only workflow instructions and hoping the model behaves
- external benchmark integration before internal runtime reliability is stable

Local repo side effects are in scope:

- reading files
- editing files
- running local commands/tests

External operational side effects are not.

## Architectural primitives

The MVP should stay centered on a small set of primitives:

1. **Agent definition**
2. **Workflow definition**
3. **Cognitive statechart**
4. **Workflow statechart**
5. **Context map chunks**
6. **Session history**
7. **Workflow history**
8. **Tools**
9. **State IO contracts**
10. **Eval reducers / reports**

The goal is not to add a large framework around these primitives. The goal is to make these primitives explicit enough that they can be composed reliably.

## Agent charts vs workflow charts

## Agent chart

An agent chart describes **how an agent operates locally**.

It answers:

- What cognitive modes can this agent be in?
- What local task does each cognitive state perform?
- What inputs should be projected for that state?
- What output is required from that state?
- Which tools are visible/enabled in that state?
- What completion signal means the local state task is done?

Examples of cognitive states:

- inspect
- plan
- edit
- validate
- summarize
- recover

For local coding agents, these states should be small and explicit. They should not ask the model to understand the whole workflow machine. The runtime should project the current local task.

## Workflow chart

A workflow chart describes **where a durable work item is in its lifecycle**.

It answers:

- What phase is the work item in?
- What artifacts or evidence are required in this phase?
- What output must be produced before transition?
- What transition guards apply?
- What context should be made available to the active agent?

Examples of workflow states:

- triage
- investigate
- implement
- validate
- review
- ready

Workflow state is durable. Cognitive state is agent-local and operational.

They can interact, but they should not collapse into the same concept.

## Context projection

Context projection is handled through context-map chunks.

This is considered mostly solved for MVP architecture:

1. define a context chunk
2. attach it to an agent/context map
3. hydrate it into runtime projection
4. include it in the prompt/inference payload

The architectural principle is:

> Statecharts should drive what local task and IO contract are projected, but context maps should remain the mechanism for assembling inference context.

In other words, state information should enter the model through structured, state-local context chunks rather than through ad hoc prompt prose.

## State-local task projection

The agent should not need to reason about the underlying state machine.

Instead, the runtime should project something like:

```txt
Current local task:
Inspect the relevant files and gather enough evidence to identify the likely edit target.

Required inputs available:
- task statement
- repo scope
- recent relevant tool results

Required output:
Return an inspection_step_v1 object with summary, evidence, next_action, and completion_signal.

Allowed tools:
- list_files
- search_files
- read_file
```

The model should see a local task, not the implementation details of the statechart.

## State IO contracts

State IO contracts are the main new architectural ground.

A state IO contract defines:

1. required inputs
2. optional inputs
3. output schema
4. required output fields
5. completion condition
6. tool policy

Both cognitive states and workflow states should support IO contracts, but their meanings differ.

## Cognitive-state IO contract

A cognitive-state contract describes the local operation the active agent must perform.

Example:

```yaml
cognitive:
  initialState: inspect
  states:
    - name: inspect
      description: Gather the minimum relevant code evidence.
      prompt: Identify the files and symbols needed before editing.
      visibleTools: [list_files, search_files, read_file]
      enabledTools: [list_files, search_files, read_file]
      inputs:
        required: [task_statement, repo_scope, recent_tool_results]
        optional: [prior_evidence]
      outputs:
        schema: inspection_step_v1
        requiredFields: [state, summary, evidence, next_action, completion_signal]
        strict: true
      completion:
        successWhen:
          - completion_signal == true
```

This says:

- what the state does
- what information it requires
- which tools it may use
- what it must emit
- when the local state task is complete

## Workflow-state IO contract

A workflow-state contract describes the durable work-item phase and its requirements.

Example:

```yaml
workflow:
  initialState: validate
  states:
    - name: validate
      description: Validate the current patch against acceptance criteria.
      visibleTools: [read_file, run_command]
      enabledTools: [read_file, run_command]
      inputs:
        required: [patch_summary, acceptance_criteria, validation_commands]
        optional: [known_failures]
      outputs:
        schema: validation_result_v1
        requiredFields: [state, commands_run, passed, failures, completion_signal]
        strict: true
      completion:
        successWhen:
          - passed == true
          - completion_signal == true
```

This says what must be true for the workflow phase to be considered ready to transition.

## YAML to runtime flow

The intended flow is:

```txt
YAML definitions
  -> catalog ingestion
    -> definition structs
      -> hydration
        -> runtime agent/workflow instances
          -> context-map projection
            -> provider request
              -> session history facts
                -> reducers/reports/evals
```

## YAML definition layer

The YAML layer should remain declarative.

It should describe:

- state names
- state descriptions/prompts
- visible/enabled tools
- input contract symbols
- output contract schema names
- required fields
- completion conditions

It should not contain runtime-specific evaluator implementation details.

## Ingestion layer

The ingestion/catalog layer should:

- parse state IO contract fields
- normalize names
- reject malformed definitions where possible
- preserve authored contract data in definition structs

At this layer, contracts are still mostly symbolic.

Examples:

- `task_statement`
- `recent_tool_results`
- `coding_step_v1`
- `completion_signal == true`

## Hydration layer

Hydration should turn symbolic contract fields into runtime-usable metadata.

Hydration responsibilities:

- resolve input symbols to known context-map chunks or projection dependencies
- resolve output schema names to known validators
- attach hydrated contracts to runtime cognitive/workflow state views
- validate unknown input symbols and unknown schemas
- prepare state-local projection data

Hydration is where authored config becomes executable runtime metadata.

## Runtime instance layer

Runtime agent/workflow instances should carry the current hydrated state contract.

At runtime, a state view should be able to answer:

- what state am I in?
- what local task should be projected?
- what inputs are required?
- what output schema is required?
- what tools are enabled?
- what completion condition applies?

This keeps loop/provider code from needing to reinterpret YAML directly.

## Provider/loop boundary

The loop should validate model output at the boundary before acting on it.

For each turn:

1. build context projection from current runtime state
2. send provider request
3. receive model output
4. parse output envelope
5. validate against current state output contract
6. emit session-history fact records
7. execute tool or final response only if valid enough
8. record retry/completion/tool outcomes

The loop should produce facts, not aggregate metrics.

## Session history and evals

Session history is the source of eval facts.

Reducers derive:

- schema validity
- missing required fields
- wrong-state outputs
- tool validity
- tool execution success/failure
- retry attribution
- completion status
- model/provider attribution

Reports and evals consume reducer output.

This preserves the layering:

```txt
runtime facts
  -> reducers
    -> stats/reports
      -> eval decisions
```

## Default coding agent charts

The MVP should eventually ship sane default agent charts for common code-specific tasks.

Likely first defaults:

1. **Code inspection agent**
   - inspect -> summarize
2. **Bug investigation agent**
   - inspect -> hypothesize -> validate -> summarize
3. **Implementation agent**
   - inspect -> plan -> edit -> validate -> summarize
4. **Review agent**
   - inspect -> assess -> recommend
5. **Recovery agent**
   - parse failure/tool failure -> repair/retry

Each chart should define state-local IO contracts and tool policies.

## Default coding workflows

The MVP should also ship sane default workflows for common workloads or tech stacks.

Likely first workflows:

1. generic issue-to-patch workflow
2. Go bugfix workflow
3. Python bugfix workflow
4. docs update workflow
5. test-fix workflow

Workflows should define durable phases and acceptance criteria, not rely on prose-only instructions.

## Skills / SKILLS.md-style knowledge

SKILLS.md-style content should not become “workflow as prose.”

A better framing is:

> A skill is a reusable knowledge/config package that can contribute context chunks, tool guidance, validation commands, examples, and state-contract defaults.

A skill may provide:

- applicability conditions
- relevant files or conventions
- context-map chunks
- validation commands
- tool usage guidance
- schema snippets
- examples/few-shots
- default workflow parameters

A skill should be compiled or hydrated into state-local projection and contract data. It should not be dumped wholesale into the prompt with the hope that the model obeys it.

Conceptually:

```txt
skill pack
  -> contributes context chunks / defaults / validation commands
    -> agent or workflow state contract
      -> hydrated runtime projection
```

Skills are supporting resources, not replacements for agent charts or workflow charts.

## Local model constraints

Because MaelstromCode targets locally hosted small/capable models, the architecture should assume:

- weaker long-context performance
- weaker schema obedience
- less robust multi-objective planning
- higher sensitivity to prompt clutter
- more need for high-signal tools
- more need for small state-local tasks

Therefore, the MVP should prefer:

- small schemas
- short local tasks
- tight tool surfaces
- explicit validation
- good retry feedback
- compact context chunks
- precise tools

## Evals role

Evals are not the product, but they are the forcing function.

They should answer:

1. Which local models are capable enough?
2. Which state charts work reliably?
3. Which workflow charts produce good task outcomes?
4. Which tools are too ambiguous or error-prone?
5. Which runtime contracts fail most often?

The eval endgame is:

1. a battery of small but capable local models
2. a sane suite of internal and external evals
3. sane default coding-agent charts
4. sane default coding workflows for workloads/tech stacks

## MVP implementation order

Recommended order:

1. keep context projection based on context-map chunks
2. define state IO contract semantics precisely
3. wire output contracts through YAML -> ingestion -> hydration -> runtime views
4. validate output contracts at the loop boundary
5. wire input contracts to context-map chunk requirements
6. instantiate minimal eval coding agent
7. build internal microtask evals
8. define default coding agent charts
9. define default coding workflows
10. only then connect broader external evals

## Current status

Already underway or partially implemented:

- session history facts
- reducers
- reports
- report attribution
- eval checks over session stats
- store-backed session loading/aggregation
- state output contract fields in definitions/runtime views

Still unresolved or incomplete:

- precise IO contract semantics
- named output schema registry/validators
- input contract hydration into context-map chunks
- concrete minimal eval agent profile
- internal coding microtask runner
- default coding agent charts
- default coding workflows

## Core architectural rule

Do not encode reliability as prose-only instructions.

If something matters, represent it as one of:

- a context chunk
- an input contract
- an output contract
- a completion condition
- a tool policy
- a session-history fact
- an eval metric

Prompt text can explain the task, but runtime structures should carry the contract.
