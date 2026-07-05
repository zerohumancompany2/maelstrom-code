# Recovery and Restart for Agent/Workflow Runtime

**Status:** Proposed | **Date:** 2026-05-30

## Problem

As we move toward a runtime that combines:

- agent definitions,
- agent cognitive state,
- workflow definitions,
- workflow durable state,
- and explicit binding between agents and workflows,

we need a clear model for restart and recovery.

A running agent may crash or be restarted while:

- in a particular cognitive state,
- bound to a workflow,
- after having emitted workflow transitions,
- or after having produced handoff-worthy workflow records.

If recovery semantics are unclear, we risk:

- losing the active cognitive state,
- confusing binding relationship with workflow state,
- replaying the wrong history into context,
- or reconstructing contradictory runtime views after restart.

## Goal

Define a recovery model in which:

- durable facts are reconstructed from history,
- local runtime state can be rehydrated deterministically,
- agent cognitive state, workflow state, and binding are recovered as separate concerns,
- and restart logic does not require hidden mutable in-memory state to be authoritative.

## Core principle

Use durable records as the source of truth.

At runtime, in-memory state is an optimization and convenience.

On restart, the system should be able to reconstruct the effective runtime view from:

- authored definitions,
- durable histories,
- and deterministic hydration/reduction logic.

## The three recovery reductions

Recovery should explicitly treat these as separate reductions.

## 1. Agent cognitive state reduction

Question:

**What cognitive state was this agent last in?**

Source of truth:

- agent session history,
- specifically agent-state transition records.

Recovery rule:

- replay or scan backward through the relevant agent session history,
- find the latest authoritative agent cognitive-state transition,
- use its destination state as the active cognitive state.

## 2. Workflow state reduction

Question:

**What workflow state is this workflow currently in?**

Source of truth:

- workflow history,
- specifically workflow-state transition records.

Recovery rule:

- replay or scan backward through workflow history,
- find the latest authoritative workflow-state transition,
- use its destination state as the current workflow state.

## 3. Binding reduction

Question:

**Is this agent currently bound to a workflow, and if so which one?**

Source of truth:

- bind/unbind records,
- potentially present in workflow history, agent session history, binding log, or a unified event stream.

Recovery rule:

- replay or scan backward through binding records,
- determine the latest effective bind/unbind relationship,
- and reconstruct the current binding snapshot.

## Recommendation

Do not fold binding recovery into workflow-state recovery.

Binding is a relationship concern, not just another workflow property.

## Durable facts vs derived runtime views

On restart, we should reconstruct **derived runtime views**, not expect the prior in-memory runtime objects to survive.

## Durable facts

Examples:

- agent session records
- workflow transition records
- binding/unbinding records
- workflow notes/handoff records
- authored definitions for models, agents, workflows

## Derived runtime views

Examples:

- current cognitive-state snapshot for an agent
- current workflow-state snapshot for a workflow
- current binding snapshot
- effective visible/enabled tool set
- hydrated workflow view projected into context

These derived views should be reproducible from durable facts.

## No hidden authority in memory

A restart-safe design should not require memory-only fields such as:

- “current workflow state” with no backing record,
- “currently attached workflow” with no binding record,
- or “current cognitive mode” with no transition record.

Those may exist in memory while running, but durable history must remain authoritative.

## Startup/restart flow

A useful restart flow for an agent runtime could look like this.

1. Load authored agent definition.
2. Hydrate runtime agent configuration from model + agent definitions.
3. Load relevant agent session history.
4. Reduce agent cognitive state from session history.
5. Reduce current binding relationship from binding records.
6. If bound:
   - load workflow definition
   - load workflow history
   - reduce current workflow state
   - derive workflow runtime view
7. Build effective context map chunks.
8. Resume execution from the reconstructed runtime view.

This makes restart behavior explicit and deterministic.

## Backward scan vs forward replay

There are two likely implementation strategies.

## Backward scan

Useful when we only need the current value.

Examples:

- latest agent cognitive state
- latest workflow state
- latest bind/unbind event

Benefits:

- efficient for “what is current?” queries
- simple for MVP recovery logic

## Forward replay / reducer

Useful when we need richer derived snapshots or invariants.

Examples:

- binding history with handoff semantics
- reduced workflow artifact set
- more sophisticated state-derived tool policy

Benefits:

- explicit and extensible
- easier to evolve into richer runtime snapshots

## Recommendation

Start with explicit reducers as the conceptual model, even if some MVP implementations use backward scans internally.

That gives us a cleaner architecture later.

## Recommended reducer/snapshot helpers

A useful shape could be:

```go
func ReduceAgentCognitiveState(history AgentSessionHistory) AgentCognitiveSnapshot
func ReduceWorkflowState(history WorkflowHistory) WorkflowStateSnapshot
func ReduceBindingState(events []BindingEvent) BindingSnapshot
```

These functions may start simple and evolve over time.

The important part is to keep reduction logic centralized rather than scattering it through:

- chunks,
- runner,
- tools,
- and startup code.

## Recovery and context projection

Restart/recovery affects how context chunks are built.

If named chunks are used for:

- cognitive state,
- workflow state,
- workflow history,
- and binding,

then chunk assembly on restart should consume the reconstructed runtime views rather than relying on stale in-memory state.

This means, for example:

- the cognitive chunk reads the recovered current cognitive state,
- the workflow chunk reads the recovered workflow-state snapshot,
- the binding chunk reads the recovered binding snapshot,
- and the history chunk reads durable workflow records or local session history as appropriate.

## Session history vs workflow history under restart

This recovery model depends on keeping history ownership clear.

## Session history

Session history is local to the current agent session or binding context.

It should be used to reconstruct:

- cognitive-state progression,
- local message history,
- local tool calls/results,
- and local working context.

## Workflow history

Workflow history is durable across bindings and across agents.

It should be used to reconstruct:

- workflow state,
- handoff notes,
- workflow-level decisions,
- artifact references,
- and durable workflow progression.

## Recommendation

Do not treat prior agents’ raw session histories as the normal recovery source for the next agent.

Instead:

- recover the workflow from workflow history,
- recover the active agent from its own local session history,
- and use promoted workflow records for cross-agent continuity.

## Binding recovery questions

Binding recovery deserves explicit answers.

### Questions to answer in implementation

- Can an agent be bound to more than one workflow at a time?
- Can a workflow be bound to more than one active agent at a time?
- Is bind/unbind strictly nested or can rebinding happen implicitly?
- What happens if restart finds contradictory bind/unbind history?

## MVP recommendation

For MVP, assume:

- one active workflow binding per agent
- one active agent binding per workflow
- explicit bind/unbind events only
- contradiction is an error surfaced by recovery validation

That keeps recovery logic much simpler.

## Recovery validation

Recovery should include sanity checks.

Examples:

- recovered cognitive state exists in the authored agent chart
- recovered workflow state exists in the authored workflow chart
- recovered binding references an existing workflow
- recovered binding is not contradictory with durable history
- enabled tool set after recovery is coherent with recovered states

## Runtime ownership after recovery

After recovery completes:

- the agent runtime holds the active cognitive-state snapshot in memory,
- binding points to the active workflow if one exists,
- the workflow runtime view is derived on demand or cached for the bound workflow,
- and context assembly proceeds from those recovered views.

This means restart does not need to restore a giant opaque runtime blob; it only needs to reconstruct the relevant snapshots.

## Example restart scenario

1. Agent was bound to workflow `feature-123`.
2. Workflow state was `implementing`.
3. Agent cognitive state was `orient`.
4. Agent process crashes.
5. On restart:
   - load agent definition
   - hydrate runtime agent config
   - reduce local session history -> `orient`
   - reduce binding history -> bound to `feature-123`
   - load workflow definition/history
   - reduce workflow history -> `implementing`
   - derive workflow runtime view
   - rebuild context chunks
6. Agent resumes from reconstructed state.

## Open questions

### 1. What is the exact authoritative event stream for binding?

Possible sources:

- binding log only
- workflow history only
- agent session only
- dual-written records with validation
- unified event store later

### 2. How much recovery state should be cached?

We may want:

- purely on-demand reduction,
- or cached snapshots invalidated by new records.

### 3. What counts as the active session after restart?

Options include:

- resume the same session ID,
- open a new session that references the prior one,
- or use binding scope rather than session scope as the main recovery unit.

### 4. How should partial tool execution be handled?

If a process crashes after a tool request is emitted but before result recording completes, recovery may need a policy for:

- retry,
- mark unknown,
- or require idempotent tool semantics.

This can be deferred, but should be acknowledged.

## Current recommendation

Adopt this restart model:

- durable history is authoritative,
- in-memory state is derived,
- agent cognitive state, workflow state, and binding are reduced separately,
- workflow recovery uses durable workflow history rather than raw prior session carryover,
- and context projection is rebuilt from recovered snapshots on restart.

This gives us a stable basis for implementing binding-aware runtime behavior without requiring opaque runtime serialization to be the source of truth.
