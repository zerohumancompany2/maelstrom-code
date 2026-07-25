# State-Scoped IO Contracts and Eval-Driven Runtime Rollout

**Status:** Proposed | **Date:** 2026-06-06

## Problem

Maelstrom's current statechart direction is useful, but the runtime still treats cognitive and workflow state primarily as labels plus prompt fragments.

That is not enough for the next phase.

The eval pivot requires a model where each state can be evaluated mechanically. To do that, a state must define more than:

- its name,
- its prompt,
- and its visible/enabled tools.

Instead, each state needs to define a **tiny local task** with:

- a required set of runtime inputs,
- a required output shape,
- a completion condition,
- and a bounded tool policy.

This lets us use statecharts as decomposition machinery rather than asking the agent to reason explicitly about the underlying machine itself.

That direction is consistent with:

- the eval reframing in `docs/evals.md`
- the runtime separation goals in `docs/planning/cognitive-state-workflow-binding-runtime.md`
- the hydration goals in `docs/planning/context-definition-hydration.md`
- and the decomposition-heavy reliability lesson from the MAKER paper

## Goal

Define a rollout path where:

1. authored agent and workflow states can declare **state-scoped IO contracts**
2. ingestion and hydration normalize those contracts into runtime-usable validators and projections
3. runtime evaluates each turn against those contracts
4. evals expose granular reliability metrics
5. every metric maps to recommended changes and concrete files

## Non-goals

For the first rollout:

- do not redesign all of sketch7 orchestration at once
- do not require full multi-agent workflow execution before internal runtime evals exist
- do not force the agent to reason about chart internals directly
- do not start with broad external benchmarks before internal reliability metrics are in place

## Core design idea

A state should increasingly mean:

> a tiny local task with defined inputs, defined outputs, constrained tools, and a known done condition.

That means a state definition should eventually carry at least four kinds of information:

1. **instructions**
2. **input contract**
3. **output contract**
4. **transition/completion contract**

## Why this is preferable

This gives us several advantages:

- evals can check whether a state was completed correctly
- hydration can project exactly the right state-local inputs
- runtime can distinguish malformed structure from task failure
- control logic becomes less prompt-fragile
- we can decompose larger workflows into smaller reliable units

## Proposed authored model direction

We should evolve agent/workflow state definitions beyond prompt-only state entries.

Illustrative direction:

```yaml
cognitive:
  initialState: inspect
  states:
    - name: inspect
      description: Gather the minimum required evidence before editing.
      prompt: Read the relevant files and identify the exact edit target.
      visibleTools: [list_files, search_files, read_file]
      enabledTools: [list_files, search_files, read_file]
      inputs:
        required:
          - repo_context
          - task_statement
        optional:
          - prior_tool_results
      outputs:
        schema: inspect_result_v1
        requiredFields:
          - state
          - action_type
          - summary
          - evidence
          - completion_signal
      completion:
        when:
          - evidence_count >= 2
          - completion_signal == true
```

And similarly for workflow states later.

The exact schema should be designed during implementation, but the structure should remain:

- state-local inputs
- state-local outputs
- state-local completion rules

## Rollout phases

## Phase 0: Eval framing and planning

### Objective

Reframe evals around internal reliability metrics before broader benchmark adapters.

### Deliverables

- updated `docs/evals.md`
- this planning document

### Metrics introduced

- none directly; this phase defines the rollout and metric-to-change discipline

### Files

- `docs/evals.md`
- `docs/planning/state-scoped-io-contracts-rollout.md`

## Phase 1: Minimal eval agent profile

### Objective

Create a minimal coding agent profile for runtime evals:

- one cognitive state
- self-loop only
- no workflows required
- small tool set
- strict required output shape

### Why first

This isolates runtime reliability from workflow complexity.

### Metrics

#### Metric: minimal-agent loop completion rate

**Definition**
- percent of runs that terminate correctly under the minimal profile

**Recommended changes if weak**
- reduce tool surface
- tighten output contract
- simplify loop termination semantics

**Likely files**
- `sketch/sketch7/main.go`
- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/runner/loop_test.go`

#### Metric: minimal-agent tool call count distribution

**Definition**
- number of tool calls used per completed task and per failed task

**Recommended changes if weak**
- improve prompt/projection compactness
- reduce tool ambiguity
- improve termination signaling

**Likely files**
- `sketch/sketch7/main.go`
- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/context/*`

## Phase 2: State-scoped output contracts

### Objective

Require each relevant state to declare a structured output contract that runtime can validate.

For the first pass, this can be introduced first for cognitive states before workflow states.

### Design direction

Add contract-bearing fields to state definitions so hydration can produce a runtime validator.

Possible definition-layer additions:

- `inputs`
- `outputs`
- `completion`

Potential targets:

- `defs.StateDefinition`
- YAML/catalog loading for state documents
- hydration into runtime state validators

### Metrics

#### Metric: cognitive output schema success rate

**Definition**
- fraction of turns producing schema-valid state output

**Recommended changes if weak**
- add validator at loop boundary
- add rescue parsing for near-miss JSON
- improve state-local prompt generation
- add clearer required output schemas during hydration

**Likely files**
- `sketch/sketch7/defs/workflow.go`
- `sketch/sketch7/catalog/load.go`
- `sketch/sketch7/compile/hydrate.go`
- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/runner/loop.go`

#### Metric: required-field presence rate

**Definition**
- fraction of outputs containing all required fields for the current state

**Recommended changes if weak**
- tighten schema enforcement
- shrink per-state output surface
- improve retry messages to mention exactly which fields are missing

**Likely files**
- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/provider/provider.go`
- future contract validation helpers

#### Metric: rescued-output rate

**Definition**
- fraction of malformed outputs repaired into valid outputs

**Recommended changes if weak**
- implement or improve rescue parser
- distinguish repairable formatting failures from deeper task failures
- instrument red-flag categories

**Likely files**
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/runner/loop.go`
- future eval harness code

## Phase 3: State-scoped input contracts and projections

### Objective

Make state inputs explicit and hydration-driven.

A state should not merely receive the full session context. It should receive the specific projections required for its local task.

### Design direction

Hydration should resolve authored input requirements into named runtime projections.

For example:

- repo context
- recent relevant tool results
- active task statement
- current state contract summary
- allowed tools summary

This implies a tighter link between:

- authored context definitions
- authored state definitions
- projection builders
- runtime session view

### Metrics

#### Metric: missing-required-input rate

**Definition**
- fraction of turns where required state inputs were unavailable or not projected

**Recommended changes if weak**
- tighten hydration validation
- add projection dependency checks
- make state/input requirements explicit in authored config

**Likely files**
- `sketch/sketch7/defs/agent.go`
- `sketch/sketch7/defs/workflow.go`
- `sketch/sketch7/catalog/load.go`
- `sketch/sketch7/compile/hydrate.go`
- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/runner/host.go`

#### Metric: projection overfill rate

**Definition**
- fraction of turns where projected inputs exceeded the intended minimal state-local set

**Recommended changes if weak**
- shrink default projections
- move from broad session projection to state-required projection lists
- reduce context duplication

**Likely files**
- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/context/*`
- `sketch/sketch7/main.go`

## Phase 4: Tool-loop guardrails

### Objective

Add Forge-inspired loop hardening:

- validation before tool execution
- rescue parsing
- retry nudges
- canonical tool error handling
- red-flag tracking

### Metrics

#### Metric: tool call validity rate

**Definition**
- fraction of tool calls with known tool names and schema-valid arguments

**Recommended changes if weak**
- add per-tool argument validators
- improve request parsing and canonicalization
- simplify ambiguous tools

**Likely files**
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/tools/*.go`
- `sketch/sketch7/runner/loop.go`

#### Metric: tool execution success rate

**Definition**
- fraction of executed tool calls that succeed without runtime error

**Recommended changes if weak**
- improve exact-match edit semantics
- improve timeout handling
- improve filesystem and command tool error reporting

**Likely files**
- `sketch/sketch7/tools/*.go`
- `sketch/sketch7/tools/*_test.go`

#### Metric: retry recovery rate

**Definition**
- fraction of invalid turns recovered by retry

**Recommended changes if weak**
- improve retry nudges
- preserve explicit error messages in feedback channel
- classify repeated failure modes

**Likely files**
- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/provider/openai_compatible.go`
- future eval harness files

## Phase 5: Workflow-state contracts

### Objective

Extend the same contract model to workflow states after the cognitive-state path is proven.

This should keep the conceptual separation already outlined elsewhere:

- workflow state answers what stage the work item is in
- cognitive state answers how the active agent is operating right now

But both should be able to declare:

- inputs
- outputs
- completion conditions

### Metrics

#### Metric: workflow transition contract satisfaction rate

**Definition**
- fraction of workflow transitions that occur only after required outputs/prerequisites are satisfied

**Recommended changes if weak**
- add transition guards derived from output contracts
- make completion conditions explicit in authored workflow states
- improve workflow runtime validation before transition tools succeed

**Likely files**
- `sketch/sketch7/defs/workflow.go`
- `sketch/sketch7/catalog/load.go`
- `sketch/sketch7/statecharts/*`
- `sketch/sketch7/tools/transition.go`
- `sketch/sketch7/runtime/reduce.go`

#### Metric: invalid-transition attempt rate

**Definition**
- fraction of attempted transitions that violate state-local completion or prerequisite rules

**Recommended changes if weak**
- improve projected transition hints
- add clearer state-local completion feedback
- tighten transition tool error messages

**Likely files**
- `sketch/sketch7/tools/transition.go`
- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/runtime/reduce.go`

## Phase 6: Minimal coding task evals

### Objective

Run internal coding microtasks on the minimal and then contract-aware runtime.

### Metrics

#### Metric: exact edit correctness rate

**Definition**
- fraction of tasks where the intended edit is completed exactly and only where intended

**Recommended changes if weak**
- improve edit precision tools inspired by Dirac
- move away from coarse edit interfaces where needed
- add better edit provenance and validation feedback

**Likely files**
- `sketch/sketch7/tools/replace_text.go`
- future patch/edit tools
- related tests

#### Metric: read-edit-validate loop success rate

**Definition**
- fraction of small coding tasks completed correctly with read/edit/command loops

**Recommended changes if weak**
- improve tool ergonomics
- shrink context surface
- improve terminal completion semantics

**Likely files**
- `sketch/sketch7/tools/read_file.go`
- `sketch/sketch7/tools/replace_text.go`
- `sketch/sketch7/tools/run_command.go`
- `sketch/sketch7/runner/loop.go`

## Phase 7: External adapters

### Objective

Only after lower layers are stable, integrate:

1. Forge scenarios
2. Terminal-Bench subsets
3. SWE-Bench Lite / Verified
4. Dirac-inspired custom refactor tasks

### Metrics

#### Metric: external-benchmark failure attribution coverage

**Definition**
- fraction of failed benchmark runs whose failure can be mapped back to lower-level runtime metrics

**Recommended changes if weak**
- improve trace logging
- preserve lower-level metric summaries in benchmark outputs
- connect benchmark adapters to common result schema

**Likely files**
- future adapter code
- eval reporting code
- trace/result schema files

## Required schema/result discipline

## Session history as the eval fact source

For the first runtime-eval phase, **agent session history should be the primary source of eval facts**.

That means:

- runtime should append structured session-history records for eval-relevant events
- eval scoring should derive metrics from those records
- aggregated metrics should be treated as a reducer/reporting layer, not as the primary runtime truth

This is the recommended layering:

```txt
runtime execution
  -> session-history facts
    -> eval reducers / scorers
      -> aggregated metrics / reports
```

### Why this is the right default

This gives us:

- auditable traces
- replayable evaluation inputs
- post-hoc metric derivation
- clear provenance from failures back to runtime events
- alignment with Maelstrom's broader goals around durability and transparent control

### Important rule

Session history should store **facts/events**, not aggregated metrics.

Good history entries:

- output parsed successfully
- output repaired successfully
- required fields missing
- retry attempted
- retry recovered
- tool validation failed
- tool execution succeeded
- completion signaled
- stop reason emitted

Not recommended as history entries:

- `output_schema_success_rate = 0.75`
- `tool_call_validity_rate = 0.92`

Those should be computed later by eval reducers.

### Recommended session-history additions

The current `sketch/sketch7/logs/session.go` model is already close to what we need.

The next likely record types are:

- output contract evaluation record
- tool validation record
- retry attempt/result record
- completion/termination record

These should remain agent-session-scoped.

### Workflow history relationship

For the minimal eval agent and first runtime-eval layer, session history should be sufficient.

Later, workflow-level evals may combine:

- session history for agent-local execution facts
- workflow history for durable work-item progression facts

But that should come after the session-history scoring path is proven.

Every eval result should capture at least:

- run id
- agent profile id
- model id
- state name
- task id
- output validity status
- tool validity status
- tool execution outcome
- retry counts
- completion status
- stop reason
- file-level or tool-level attribution when available

This is necessary so metrics can actually drive implementation work.

## File targets most likely to matter

Across the rollout, the central files are likely to be:

- `sketch/sketch7/defs/agent.go`
- `sketch/sketch7/defs/workflow.go`
- `sketch/sketch7/catalog/load.go`
- `sketch/sketch7/compile/hydrate.go`
- `sketch/sketch7/runtime/view.go`
- `sketch/sketch7/runtime/reduce.go`
- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/runner/host.go`
- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/tools/*.go`

## Recommended implementation order

1. keep the initial eval agent minimal
2. define first-pass cognitive output contract
3. add loop-level validation and metric capture
4. add authored state output contract fields
5. hydrate those contracts into runtime validators and prompt projections
6. add state input contracts and projection dependency checks
7. add workflow-state contracts and transition guards
8. pressure the system with internal coding microtasks
9. only then expand into external benchmark adapters

## Immediate implementation sequence

For the first concrete rollout, the implementation order should be treated as:

1. **define output schema**
2. **wire output schema into ingestion, hydration, runtime validation, and prompt projection**
3. **instrument metrics at the schema boundary and use them to drive changes**

That third step is important. The point is not just to “have metrics,” but to measure whether the current state contract is being satisfied.

The first metric bundle should answer:

- did the output parse?
- did it satisfy the required schema?
- did it need rescue/repair?
- did retry recover it?
- did the run still complete successfully?

Those numbers should then drive:

- schema simplification if the contract is too hard to satisfy
- hydration/projection changes if required inputs are missing or unclear
- loop/provider changes if repair and retry behavior are weak
- tool/runtime changes if structurally valid outputs still lead to failed execution

In short:

- **define the contract**
- **enforce the contract**
- **measure contract satisfaction**

That is the operational core of this rollout.

## Decision rule

At every phase:

- if a metric is weak, it must point to concrete runtime/tool changes
- if a proposed change cannot be tied to a metric, it should not be prioritized
- if an eval failure cannot be attributed to concrete files or runtime layers, the instrumentation is insufficient

That rule should keep the rollout engineering-focused instead of drifting back into vague experimentation.

## Addendum: phased implementation plan

This addendum translates the design direction above into concrete implementation phases.

The key rule is that each phase should leave the codebase in a testable, scorable state before moving to the next one.

## Phase 0: Planning and schema definition

### Goal

Finish the design substrate so implementation can proceed with stable targets.

### Deliverables

- eval framing in `docs/evals.md`
- state contract schema draft
- minimal eval agent profile and runtime metrics schema
- this phased rollout plan

### Main files

- `docs/evals.md`
- `docs/planning/state-scoped-io-contracts-rollout.md`
- `docs/planning/state-contract-schema-draft.md`
- `docs/planning/minimal-eval-agent-and-metrics.md`

### Exit criteria

- output contract shape is defined
- session-history-as-eval-fact-source is documented
- first metric vocabulary is defined

## Phase 1: Session-history instrumentation substrate

### Goal

Make session history capable of capturing all eval-relevant runtime facts.

### Main work

- add new session record types for:
  - output contract evaluation
  - tool validation
  - retry attempt/result
  - completion/termination
- ensure records are persistable and inspectable

### Main files

- `sketch/sketch7/logs/session.go`
- `sketch/sketch7/logs/persist.go`
- `sketch/sketch7/logs/*_test.go`

### Metrics unlocked

- output parse status counts
- repair counts
- retry counts
- stop reason counts

### Exit criteria

- session history can represent all first-pass eval facts
- record schema is stable enough for reducers

## Phase 2: Definition-layer state output contracts

### Goal

Add output-contract fields to authored state definitions and YAML loading.

### Main work

- extend `defs.StateDefinition`
- extend YAML document structs
- validate unknown/malformed contract definitions at load time

### Main files

- `sketch/sketch7/defs/workflow.go`
- `sketch/sketch7/defs/agent.go`
- `sketch/sketch7/catalog/load.go`
- related tests

### Metrics unlocked

- contract-definition validity
- schema-name coverage

### Exit criteria

- agent/workflow state definitions can declare output contracts
- invalid authored contracts fail during ingestion

## Phase 3: Hydration and runtime contract wiring

### Goal

Hydrate authored output contracts into runtime-visible state metadata.

### Main work

- add hydrated contract structures
- attach contracts to runtime cognitive/workflow views
- project state-local contract requirements into prompts

### Main files

- `sketch/sketch7/compile/hydrate.go`
- `sketch/sketch7/runtime/view.go`
- `sketch/sketch7/runtime/reduce.go`
- `sketch/sketch7/runner/host.go`
- `sketch/sketch7/prompt/projection.go`

### Metrics unlocked

- wrong-state emission rate
- required contract projection coverage

### Exit criteria

- runtime knows the current state's output contract
- prompt projection includes required output schema guidance

## Phase 4: Loop-level contract validation and retry

### Goal

Validate model output against the current state contract before tool execution.

### Main work

- parse output envelope
- validate schema and required fields
- classify output as valid / repaired / retriable invalid / terminal invalid
- emit session-history records for these outcomes
- add retry behavior with explicit reasons

### Main files

- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/provider/openai_compatible.go`
- related tests

### Metrics unlocked

- output schema success rate
- required-field presence rate
- repair rate
- retry recovery rate

### Exit criteria

- invalid outputs are detected before tool execution
- retry path is implemented and recorded in session history

## Phase 5: Tool validation boundary

### Goal

Validate proposed tool calls structurally before execution.

### Main work

- validate tool name against enabled tools
- validate tool arguments against expected shapes
- emit tool-validation session records
- add retry/error behavior for invalid tool proposals

### Main files

- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/tools/*.go`
- related tests

### Metrics unlocked

- tool call validity rate
- unknown tool request rate
- argument validation failure rate

### Exit criteria

- no tool executes before structural validation succeeds
- invalid tool proposals are attributable in session history

## Phase 6: Minimal eval agent and reducer/scorer

### Goal

Create the first runnable minimal eval agent and derive metrics from session history.

### Main work

- add minimal eval agent config/profile
- implement reducers that compute run-level metrics from session records
- emit machine-readable eval result bundles

### Main files

- `sketch/sketch7/main.go`
- minimal agent config files
- new reducer/scorer code
- eval-facing tests

### Metrics unlocked

- loop completion rate
- tool execution success rate
- tool calls per success
- per-run aggregate output/schema metrics

### Exit criteria

- one minimal agent can run end-to-end
- metrics are computed from session history, not only in-memory counters

## Phase 7: State input contracts and projection dependency checks

### Goal

Add state-scoped input contracts after output contract path is stable.

### Main work

- add input contract fields to definitions
- hydrate symbolic input requirements into projections
- validate required input availability

### Main files

- `sketch/sketch7/defs/agent.go`
- `sketch/sketch7/defs/workflow.go`
- `sketch/sketch7/catalog/load.go`
- `sketch/sketch7/compile/hydrate.go`
- `sketch/sketch7/prompt/projection.go`

### Metrics unlocked

- missing-required-input rate
- projection overfill rate

### Exit criteria

- state-local inputs are explicit and validated

## Phase 8: Internal coding microtask evals

### Goal

Pressure the minimal runtime with small, deterministic coding tasks.

### Main work

- define internal eval tasks
- run minimal agent against read/edit/validate loops
- score both success and runtime reliability metrics together

### Main files

- internal eval task fixtures
- reducer/reporting code
- tool tests and integration tests

### Metrics unlocked

- exact edit correctness rate
- read-edit-validate loop success rate

### Exit criteria

- internal coding evals produce actionable reports tied to runtime facts

## Phase 9: Workflow-state contracts and external adapters

### Goal

Extend the same contract discipline upward once the minimal substrate is working.

### Main work

- add workflow-state contracts and transition guards
- add Forge-style comparative evals
- add Terminal-Bench subsets
- later add SWE-Bench / Dirac-inspired tasks

### Main files

- workflow defs/runtime/transition files
- future adapter and reporting code

### Metrics unlocked

- workflow transition contract satisfaction
- external benchmark failure attribution coverage

### Exit criteria

- external benchmark failures can be traced back to lower-level runtime metrics and session-history facts
