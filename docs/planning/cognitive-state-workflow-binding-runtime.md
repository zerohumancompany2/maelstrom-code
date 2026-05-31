# Cognitive State, Workflow State, and Binding Runtime

**Status:** Proposed | **Date:** 2026-05-30

## Problem

We now have a clearer ownership model for:

- model definitions,
- agent definitions,
- and hydration into runtime agent state.

What remains less clear is the runtime model that sits above agent execution when agents interact with durable workflows.

In particular, we need a clean way to express and relate:

- **agent cognitive state**,
- **workflow state**,
- **binding** between an active agent and a workflow,
- **projection of those runtime views into the context map**,
- and **tool availability** as a function of both agent and workflow state.

These concepts are related, but they are not the same thing.

Without a clear model, we risk collapsing:

- work progression into agent behavior,
- internal agent mode into workflow phase,
- binding relationship state into one of the charts,
- durable workflow history into per-agent session history,
- and context projection into ad hoc hardcoded chunk behavior.

## Goal

Define a runtime model in which:

- **agent cognitive state** and **workflow state** use the same authored statechart substrate,
- but remain distinct in ownership, durability, and projection into the inference bundle,
- **binding** remains a first-class runtime relationship rather than being treated as a chart state,
- **state and binding projection** happen through named chunk declarations resolved during hydration,
- and **effective tool access** is computed from agent, cognitive-state, and workflow-state policy.

Conceptually, we want to support flows such as:

1. an agent sees an available task,
2. the agent binds to a workflow,
3. the workflow is projected into context,
4. the agent cycles through cognitive states while working the workflow,
5. the workflow advances through durable states,
6. the agent unbinds,
7. another agent later binds and continues or reviews the same workflow.

## Core distinction

## Workflow state answers

**What stage is the work item in?**

Examples:

- available
- triage
- implementing
- awaiting_review
- revising
- done

This is the durable progression of the work item.

## Cognitive state answers

**How is the currently bound agent operating while working the item?**

Examples:

- observe
- orient
- decide
- act

This is the current behavioral/control mode of the active agent.

## Rule of thumb

If a state must survive handoff between agents, it is probably a **workflow state**.

If a state only describes how the currently bound agent is operating right now, it is probably a **cognitive state**.

## Shared statechart substrate

A major simplifying idea is that cognitive state and workflow state do **not** need different underlying machinery.

They can use the same authored substrate:

- states,
- transitions,
- guards,
- and possibly actions.

The difference is not in the statechart mechanism itself.
The difference is in the semantic role of the chart.

## Recommendation

Use one general statechart definition shape in authored config, reused in two places:

- agent cognitive chart
- workflow chart

Illustrative direction:

```go
type StatechartDefinition struct {
    InitialState string
    States       []StateDefinition
    Transitions  []TransitionDefinition
}
```

Then, conceptually:

```go
type AgentDefinition struct {
    ...
    Cognitive StatechartDefinition
}

type WorkflowDefinition struct {
    ...
    Statechart StatechartDefinition
}
```

This gives us shared mechanics without shared meaning.

## Where cognitive state and workflow state differ

Even if the schema is shared, the semantics should differ in important ways.

## 1. Ownership

- cognitive chart is authored as part of the **agent definition**
- workflow chart is authored as part of the **workflow definition**

## 2. Lifetime

- cognitive state is local to the currently active agent/session/binding
- workflow state persists across bindings, sessions, and agents

## 3. Projection into inference context

- cognitive state is projected into a **cognitive chunk**
- workflow state is projected into a **workflow chunk**

These chunks appear in different places and serve different prompt purposes.

## 4. Authority and effect

- cognitive transitions affect how the current agent operates
- workflow transitions affect durable work progression

That usually means workflow transitions should be treated as mutating shared runtime state, while cognitive transitions are more local even though they should still be recorded and audited.

## Binding is not a chart state

Binding should be treated as a first-class runtime relationship.

It is not the same thing as:

- workflow state,
- cognitive state,
- or session state.

Binding answers a different question:

**Which workflow, if any, is the current agent attached to right now?**

## Recommendation

Keep binding separate from both statecharts.

Conceptually, binding is:

- an active relationship between an agent instance and a workflow instance,
- recorded through bind/unbind events,
- and used by runtime to decide what workflow context is projected into the inference bundle.

`sketch6` already has useful primitive records for bind/unbind references. That direction should continue.

## Binding as tool call

A good default is to model bind/unbind as tool calls.

This is attractive because it preserves one consistent mutation boundary:

- explicit,
- auditable,
- recorded in history,
- and runtime-validatable.

For example:

- `bind_workflow(workflowID)`
- `unbind_workflow()`
- `transition_state(chart="agent", trigger="orient")`
- `transition_state(chart="workflow", trigger="submit_review")`

This keeps both chart transitions and relationship changes within the same explicit runtime/tool discipline.

## Transition model

## Recommendation

Use the same general transition primitive for both charts.

For example:

- `transition_state(chart="agent", trigger="observe_to_orient")`
- `transition_state(chart="workflow", trigger="start_implementation")`

This gives us:

- consistent mutation semantics,
- auditable transition history,
- clear runtime validation of legal transitions,
- and no hidden state mutation path outside tool execution.

## Guards and actions

## Guards

Guards fit naturally for both charts.

Examples:

- workflow transition to `awaiting_review` only allowed if implementation artifact exists
- cognitive transition from `act` to `observe` only allowed after an action record has been emitted

Guards should likely be part of the authored statechart model early.

## Actions

Actions require more care.

There are at least two possible meanings:

1. **statechart-native side effects** such as emitting records or bookkeeping updates
2. **tool-like behavior** such as performing repo operations or mutating workflow state

### Recommendation

Start conservatively:

- allow guards first,
- keep authored actions minimal or absent at first,
- and continue to prefer explicit tool calls as the main mutation primitive.

This avoids hidden side effects and keeps runtime behavior auditable.

## Tool policy model

Tool access should be treated as the result of layered policy.

## Recommendation

Compute effective tool access in this order:

1. **Agent base tool universe**
2. **Cognitive-state filter**
3. **Workflow-state filter**
4. **Effective enabled tools**

This means:

- the agent definition says what tools the agent may ever use,
- the current cognitive state narrows that set,
- the current workflow state narrows it further,
- and runtime exposes the final enabled set.

## Visible vs enabled tools

We should distinguish between:

- **visible tools** — tools the agent is informed exist
- **enabled tools** — tools the agent is actually allowed to call right now

This distinction is useful.

Example:

- in cognitive state `observe`, the agent may see all tools for planning purposes,
- but only be allowed to call `transition_state` and perhaps read-only tools.

Later, in `act`, a broader set may become enabled.

This same distinction may also apply at workflow-state level.

## Projection into inference context

We likely want distinct chunk types for these concerns.

## Named chunk projection

A useful simplification is to treat these runtime projections as **named chunks** in the agent’s context definition.

That means authored config declares symbolic chunk names plus a small parameter block, and hydration resolves those names to internal chunk implementations.

Conceptually:

```yaml
context:
  chunks:
    - type: cognitive_state
      chart: agent
    - type: workflow_state
      source: bound
    - type: workflow_history
      includeHandoffs: true
```

The exact field names may differ, but the pattern should remain:

- authored symbolic chunk kind
- small authored parameter set
- internal implementation resolved at hydration time

This keeps authored YAML stable and readable while preserving runtime flexibility.

## Cognitive chunk

Projects the agent’s current cognitive state into prompt context.

Possible contents:

- current cognitive state name
- instructions for that state
- allowed next transitions
- visible/enabled tool summary
- behavior nudges for that state

Example:

> Cognitive mode: Observe. Gather facts. Do not perform workflow mutations except transitions into the next cognitive mode when ready.

## Workflow chunk

Projects the active workflow into prompt context.

Possible contents:

- workflow identity
- current workflow state
- work item description
- acceptance criteria
- artifacts or references
- allowed next workflow transitions
- handoff notes and durable workflow history

## Binding chunk

Binding may either be represented explicitly in its own chunk or folded into the workflow chunk.

Conceptually, it includes information such as:

- bound workflow ID
- agent role in that workflow
- assignment / attachment reason
- current ownership / handoff status

It is acceptable to keep this folded into workflow projection initially, but it should remain conceptually distinct from workflow state itself.

## State chunks should project more than a state label

A projected state chunk should usually carry more than just the current state name.

For example, a useful projected view may include:

- current state name
- state description/instructions
- allowed next transitions
- state-scoped tool visibility/enabled policy
- important guard-related hints when appropriate

This applies to both cognitive and workflow state projection.

## Session history vs workflow history

This is one of the most important runtime design boundaries.

## The problem

When a workflow survives across agents, we must decide what history is projected when a new agent binds.

There are at least three candidates:

1. current agent session history
2. durable workflow history
3. prior agents’ session histories

Projecting full prior agent sessions directly is likely too noisy and too tightly couples one agent’s local execution history to another’s context.

## Recommendation

Use this rule:

- **session history is local**
- **workflow history is durable**
- **handoff-worthy information should be promoted into workflow-owned records**

That means:

- current agent session history is projected locally for the active agent
- durable workflow events survive across bindings
- prior agent work is only surfaced to later agents if it has been promoted into workflow history as notes, summaries, artifacts, or structured records

This keeps handoffs manageable and avoids blindly projecting raw prior sessions into the next agent’s prompt.

## Implication for attribution

If workflow history includes durable handoff notes or summaries, those records should carry attribution such as:

- agent identity
- session identity
- timestamp
- workflow identity

That gives later agents context without requiring the full replay of prior session messages.

## Workflow runtime view vs workflow executor

It is useful to distinguish between:

- a long-lived workflow runtime/executor object
- and a derived workflow runtime view

### Recommendation

We do **not** need a separate long-lived workflow executor just to make workflow state available during agent execution.

However, runtime will still need a derived workflow view when:

- an agent is bound to a workflow,
- context is assembled,
- transitions are validated,
- or recovery/restart reconstructs the active work state.

So the intended model is:

- workflow durable state is stored as definition + history
- workflow runtime view is derived on demand
- agent runtime executes transitions against workflow-owned durable state

## Runtime flow example

A useful target flow could look like this.

## Example: feature implementation workflow with OODA-style cognition

### Workflow states

- available
- implementing
- awaiting_review
- revising
- done

### Cognitive states

- observe
- orient
- decide
- act

### Flow

1. Agent sees an available task in a queue/task-board chunk.
2. Agent calls a bind tool to attach to workflow `XYZ`.
3. Workflow `XYZ` is projected into the agent’s context.
4. Workflow state may be `available`; cognitive state may begin at `observe`.
5. Agent cycles through `observe -> orient -> decide -> act` as needed.
6. While in `act`, the agent may use broader enabled tools.
7. The agent transitions workflow state to `implementing` or later `awaiting_review` as appropriate.
8. Agent emits handoff-worthy records into workflow history.
9. Agent unbinds.
10. A reviewer agent later binds to the same workflow.
11. The reviewer sees durable workflow history and current workflow state, but not necessarily the full raw session history of the prior agent.
12. Reviewer begins with its own cognitive state, perhaps `observe`, and continues the workflow.

This demonstrates how cognitive state and workflow state can interact without collapsing into one chart.

## MVP-oriented recommendation

For the near term, we should optimize for the smallest runtime model that proves the separation.

## Near-term goals

1. Define a shared authored statechart shape.
2. Add agent cognitive chart support.
3. Add workflow chart support.
4. Keep chart semantics orthogonal.
5. Treat bind/unbind as explicit runtime operations, likely tool calls.
6. Keep local session history separate from durable workflow history.
7. Add named chunk projection for cognitive state, workflow state, binding, and workflow history.
8. Add effective tool filtering from:
   - agent base tools
   - cognitive state
   - workflow state

## What does not need to be solved immediately

- sophisticated authored chart actions
- full multi-agent orchestration policies
- automatic replay of prior sessions into new bindings
- every future runtime role or scheduler concern

## Open questions

### 1. How rich should guards be?

Should guards be:

- simple named predicates,
- expressions over runtime state,
- or code-bound validators?

### 2. Do we want one cognitive chart or multiple per agent?

One chart is simpler for MVP.
Multiple orthogonal cognitive charts may become useful later, but would complicate runtime and projection.

### 3. How should workflow-authored tool policy be expressed?

Options include:

- allowed tools per state,
- denied tools per state,
- or higher-level capability categories.

### 4. What gets promoted from session history into workflow history?

We likely need explicit record types for:

- handoff summary
- implementation note
- review note
- artifact reference
- decision record

### 5. Should bind/unbind be modeled as generic runtime ops or only as tools?

The recommendation here is “tool first,” but runtime may still need underlying administrative primitives that share the same implementation.

## Current recommendation

Adopt this framing:

- **statechart substrate** is shared
- **cognitive state** is agent-owned, local, and projected as behavioral context
- **workflow state** is workflow-owned, durable, and projected as work-progress context
- **binding** is a separate runtime relationship
- **history** is split into local session history and durable workflow history
- **named chunk projection** resolves authored context chunk declarations into internal runtime chunk implementations
- **workflow runtime** is a derived view, not necessarily a standalone executor
- **tool access** is computed by layering agent, cognitive-state, and workflow-state policy

This gives us a coherent next step above `sketch6` hydration without conflating behavior, task progression, runtime attachment, and context projection.
