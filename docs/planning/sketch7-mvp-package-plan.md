# Sketch7 MVP and Package Plan

**Status:** Proposed | **Date:** 2026-05-30

## Why sketch7 exists

`sketch6` proved several important ideas:

- durable histories can be reduced into runtime views,
- agent cognitive state and workflow state are distinct but related,
- prompt assembly should carry provenance,
- tool calls are the right mutation boundary,
- and a local inference/tool loop is enough to exercise the model.

It also showed where complexity began to swell faster than capability.

The main problem is not that the core runtime is too large.
The problem is that too many adjacent abstractions grew around it at once:

- duplicate state reduction paths,
- duplicated or triplicated binding representation,
- a growing chunk taxonomy,
- demo provider behavior coupled to prompt prose,
- and a registry/ingest stack that is useful long-term but too large for the MVP.

`sketch7` should not be a refactor of `sketch6`.
It should be a fresh implementation that preserves the validated core ideas while collapsing premature structure.

## MVP goal

Build the smallest runtime that can truthfully support:

1. starting an interactive chat session in a repo,
2. loading model, agent, and workflow definitions from YAML,
3. maintaining separate durable agent-session and workflow-instance histories,
4. reconstructing current cognitive state, workflow state, and binding state from durable records,
5. assembling an inference request from explicit projections with provenance,
6. sending that request through a real provider boundary,
7. executing tool calls as the only mutation path,
8. recording enough provenance to explain and recover the run after restart,
9. hot-reloading behavioral YAML changes while the system is live,
10. and resuming a bound session deterministically after process restart.

This MVP is **not** a scheduler, marketplace, or permissions system.
It is the minimal execution runtime on which those systems can later stand.

## Product statement

Maelstrom is a coding CLI where conversation, planning, and implementation all run through durable agent/workflow execution.

The MVP should let a developer:

- talk to an agent about a feature or bug,
- let that conversation flow into a workflow-backed execution path,
- interrupt the agent while it works and provide new input or redirection,
- have the agent inspect, edit, test, and verify code in the repo,
- update agent or workflow behavior live through YAML,
- and recover the entire run after restart.

The differentiator is not just “chat with tools.”
The differentiator is:

- explicit cognition,
- explicit workflow progression,
- hot-reloadable behavior,
- and recoverable execution.

## First useful user story

The first useful user story for sketch7 should be stated plainly:

> I want to talk with an agent about a feature or bug, have it turn that conversation into a tight execution workflow, and then carry that work through implementation, testing, and review inside the CLI.

An even shorter version is:

> I describe what I want built, and Maelstrom turns the conversation into a recoverable workflow that implements it step by step.

This should also remain true when the session is interrupted mid-execution for clarification, correction, or redirection.

If sketch7 cannot satisfy this story, the architecture is missing the product.

## MVP non-goals

For sketch7, explicitly do **not** build:

- workflow scheduling or assignment policy,
- multi-agent dispatch policy,
- financial accounting,
- full permissions/security policy engine,
- template preprocessing beyond late env expansion if needed,
- speculative chunk budgeting and policy DSLs,
- generalized action systems embedded in statecharts,
- or a fully general registry/revision architecture.

Also explicitly out of MVP:

- KPI-driven meta-agent optimization loops,
- autonomous YAML self-improvement and promotion pipelines,
- financial accounting and scoring,
- broad timer ecosystems,
- and a proliferation of workflow families before the first anchor workflow is excellent.

If a feature does not materially help us validate the core inference/runtime model, it should wait.

## Core principles

### 1. Durable history is authoritative

Runtime state is derived.

On restart, the system should reconstruct current execution state from:

- authored definitions,
- durable session history,
- durable workflow history,
- and deterministic reduction logic.

### 2. Session and workflow histories remain separate

This is intentional and should continue.

- **session history** is for agent/session-local execution and auditing,
- **workflow history** is for durable work progression and cross-agent views.

They may eventually live in separate tables.
The architecture should respect that now.

### 3. Binding is a first-class runtime relationship

Binding is not a workflow chart state and not a cognitive state.

It should be reduced explicitly from durable bind/unbind records and projected into runtime as its own concern.

### 4. One reduction path per concern

Do not maintain parallel reduction systems.

For each concern there should be one clear reducer:

- cognitive state reducer,
- workflow state reducer,
- binding reducer.

The `charts` module should not also become a second reducer layer.

### 5. Charts are only for statechart mechanics

The statechart module should do one job:

- given a current state and trigger, decide whether the transition is legal and what next state results.

That module should not own prompt projection, runtime views, or broad snapshot logic.

### 6. Tool calls are the mutation boundary

State changes, binding changes, and domain actions should happen through explicit tool execution.

This keeps mutation:

- auditable,
- testable,
- restart-friendly,
- and future-compatible with permissions and financial accounting.

### 7. Prompt assembly must stay provenance-aware

This is one of the best validated ideas from sketch6 and should survive into sketch7.

We need to know:

- which records fed a prompt projection,
- which projection produced which text,
- and what runtime view was assembled when the provider was called.

This should now be extended to explicit context-map sections as well.

Once arbitrary context sections can influence inference, they should not be treated as invisible implementation detail forever.
We will eventually want a durable record of:

- which context sections existed,
- which ones were included in a given inference payload,
- what content or content-hash they carried,
- where they came from,
- and which final inference envelope was actually sent across the wire.

This matters for:

- debugging,
- replay and recovery,
- trust,
- and auditing hidden behavior steering.

### 8. Chat is part of the MVP entrypoint

Chat is not a bonus layer on top of the runtime.
It is the primary entry surface for the product vision.

The user should be able to begin with conversation, not with workflow authoring alone.
That conversation must live inside the same durable runtime model and be able to flow into workflow-backed execution.

Chat should also be available as a mid-flight intervention surface while the agent is working.

### 9. Sessions must be interruptible and resumable

An agent working in the CLI must be interruptible.

The user should be able to interrupt execution, provide new input, and then allow the agent to resume without losing workflow state or runtime context.

This behavior should be modeled explicitly and durably, not as an invisible in-memory convenience.

At minimum, sketch7 should support:

- an interrupt event,
- a durable return to interactive user input,
- follow-up user messages,
- and an explicit resume path back into execution.

### 10. Ontology must compress execution

The ontology should stay aligned with the problem space, but only where it makes the runtime smaller, clearer, and more recoverable.

Good ontology:

- reduces special cases,
- clarifies ownership,
- improves replay/recovery,
- and makes hot-reload behavior easier to reason about.

Bad ontology:

- invents more nouns than operations,
- duplicates authoritative facts,
- or forces simple tasks through unnecessary ceremony.

## MVP product view

The MVP runtime should support the following story:

1. A developer starts Maelstrom in a repo and opens a chat session.
2. A model definition exists.
3. An agent definition exists with a cognitive chart and context projections.
4. A workflow definition exists with a workflow chart.
5. A workflow instance exists with its own durable history.
6. An agent session exists with its own durable history.
7. The user describes a task through chat.
8. The session either begins in or binds into a workflow-backed execution path through an explicit record-producing path.
9. Runtime reconstructs:
   - current cognitive state,
   - current workflow state,
   - current binding,
   - effective visible/enabled tools.
10. Prompt assembly projects those facts plus recent history into a provider request.
11. Provider returns assistant output and/or tool requests.
12. Tools execute and emit durable records into the correct histories.
13. Runtime loops until quiescent or waits for the next user turn.
14. The user may interrupt execution and return the session to interactive input.
15. The user provides clarification, correction, or redirection.
16. The agent explicitly resumes execution from the interrupted context.
17. The user edits agent or workflow YAML while the system is live.
18. The next turn or loop picks up the new behavior safely.
19. On restart, the same runtime view can be rebuilt from durable records.

If sketch7 does this cleanly, we have the right MVP spine.

## Anchor MVP workflow

Sketch7 should begin with one anchor workflow rather than many specialized workflow families.

Recommended anchor workflow:

`conversation_to_execution`

This workflow should cover the full MVP arc from chat into implementation.

It should also tolerate interruption and resumption without requiring a second top-level runtime model.

Illustrative workflow states:

- `chatting`
- `framing`
- `planning`
- `implementing`
- `validating`
- `done`
- `blocked`

Illustrative transitions:

- `clarify_task`
- `commit_plan`
- `start_implementation`
- `run_validation`
- `complete`
- `block`
- `resume`

The point is not to freeze these exact names.
The point is to ensure there is one canonical end-to-end path that proves the product experience.

Chat should begin as the opening surface of this workflow rather than as a wholly separate workflow universe.

Interruptions should return the session to an interactive surface without redefining workflow progression itself.

This anchor workflow can contain the seeds of:

- planning,
- implementation,
- testing,
- and review

without requiring a full workflow taxonomy on day one.

## Recommended sketch7 package plan

The package plan should stay brutally small.

```text
sketch/sketch7/
  defs/
    model.go
    agent.go
    workflow.go

  logs/
    session.go
    workflow.go
    binding.go

  statecharts/
    definition.go
    machine.go

  runtime/
    view.go
    reduce.go
    hydrate.go
    host.go

  prompt/
    segment.go
    projection.go
    assemble.go
    payload.go
    provenance.go

  provider/
    provider.go
    openrouter_adapter.go
    fake.go

  tools/
    types.go
    registry.go
    transition.go
    binding.go

  trace/
    inference.go

  catalog/
    load.go
    memory.go
```

This is intentionally smaller than sketch6.

## Package responsibilities

### `defs/`

Owns authored definitions only.

- model definition
- agent definition
- workflow definition
- shared statechart definition shape if we want one source type

Keep definition schemas honest.
If runtime does not enforce a field, do not include it yet.

#### Sketch6 carry-forward

- `model.Definition`
- most of `agent.Definition`
- most of `workflow.Definition`

#### Trim from sketch6

Agent context chunk metadata that has no runtime behavior yet:

- `Source`
- `Flexible`
- `Policy`
- `Priority`
- `BudgetPct`

### `logs/`

Owns durable history structures and record types.

Keep **session** and **workflow** logs separate, because that matches the intended persistence model and later auditing needs.

However, do not duplicate more structure than necessary.

The likely shape is:

- session log types
- workflow log types
- shared helpers for record IDs and descriptions where possible
- explicit binding records in whichever log(s) are authoritative

The key is not “one log.”
The key is “one clear source of truth per concern.”

#### Sketch6 carry-forward

- session/workflow append-only history pattern
- record description/debug helpers

#### Sketch6 mistakes not to repeat

- a third standalone binding log unless a hard operational need appears
- hidden duplication of authoritative facts across logs without a declared ownership rule

### `statecharts/`

Owns statechart mechanics only.

Responsibilities:

- statechart definition helpers,
- compiled transition lookup,
- legal transition checks,
- next-state resolution.

It should **not** own runtime snapshots or broad reduction logic.

#### Sketch6 carry-forward

- chart transition legality / `Fire` logic
- conversion from authored statecharts into executable transition maps

#### Do not carry forward

- snapshot/reduction logic living beside transition engine logic

### `runtime/`

This is the core of sketch7.

Responsibilities:

- runtime agent/workflow/binding view types,
- deterministic reducers from durable histories,
- hydration from definitions into executable runtime config,
- runtime host orchestration,
- and the local inference/tool loop.

This package should contain the smallest truthful runtime model.

It must support both:

- interactive chat turns,
- and workflow-backed execution turns.

Those should not become separate runtimes.
They should be two faces of the same runtime model.

The runtime should also make session interruption and resumption explicit.
Workflow state should continue to represent work progression, not conversational turn-taking.

If needed, sketch7 may introduce a lightweight session interaction-mode concept for things like:

- interactive,
- running,
- interrupted,
- awaiting_user,
- resumable.

This should begin as a runtime concern before it becomes a full authored chart, unless experience proves otherwise.

#### Reducers

Sketch7 should explicitly centralize:

```go
ReduceCognitiveState(...)
ReduceWorkflowState(...)
ReduceBindingState(...)
```

Even if implementations use backward scans first, the conceptual model should be reducers.

#### Hydration

Hydration can remain separate inside `runtime/` if that helps clarity and dev UX.

The important question is not package count but whether developers can answer:

- where do authored definitions become executable runtime config?
- where are defaults/overrides applied?
- where are runtime-valid definitions rejected?

If `runtime/hydrate.go` answers those clearly, the UX is still good.

#### Host

The host should:

- load definitions from the current in-memory catalog,
- load durable histories,
- rebuild runtime state,
- run the loop,
- and persist resulting records.

It should **not** own scheduling or assignment policy.

#### Sketch6 carry-forward

- loop structure from `runner`
- runtime view types from `runtime`
- hydration logic shape from `hydrate`
- orchestration boundary from `runtimehost`

### `prompt/`

This is the renamed/simplified `assembly/` heart.

Responsibilities:

- prompt segments,
- projections over runtime/log state,
- deterministic assembly,
- payload construction,
- provenance steps.

This package should remain one of the cleanest parts of the system.

#### Simplification direction

Do not grow a wide chunk taxonomy immediately.

Start with a smaller projection model such as:

- static/system projection,
- recent-history projection,
- chat/session projection,
- session interaction projection,
- cognitive-state projection,
- workflow-state projection,
- binding projection.

If later we want a more general declarative projection language, that can come after the core runtime is stable.

#### Sketch6 carry-forward

- `Segment`
- `PromptSegment`
- provenance concept
- assembler pattern
- payload source-record collection logic

### `context/`

This package should own the explicit inference-shaping layer between runtime state/history and provider serialization.

Its near-term responsibilities now include:

- deriving ordered context sections from agent/workflow/session state,
- deriving prompt-visible transcript messages from session history,
- applying trimming/retention policy over prompt-visible transcript blocks,
- and producing a typed inference payload that downstream prompt/provider code can serialize.

Near-term context section examples include:

- system instructions,
- interaction/cognitive/workflow/binding sections,
- repo-awareness sections,
- and later other environment/task/memory sections.

This is the minimal reintroduction of the old context-map idea, but with much tighter scope.
It should not grow back into a large speculative abstraction tower.

Near-term, the authored context definition should stay narrow.
The likely next extension is to allow at most two additional optional per-section fields:

- `refreshEveryNTurns`
- `retentionMode`

If either field is absent, it should be ignored.

That keeps the authored context shape focused on:

- section ordering,
- refresh cadence for generated sections,
- and retention/selection policy for transcript or generated-section history.

We should avoid reintroducing a wide chunk-policy DSL too early.

#### Stronger version later: durable context snapshots and inference envelopes

The current sketch7 `context/` package builds sections and transcript messages ephemerally at inference time.
That is enough for the current MVP loop, but it is not the end state.

Longer-term we likely want at least two additional durable record types in session history:

1. **context snapshot records**
   - represent a generated context section/chunk,
   - include section name/type,
   - source kind (static, derived_repo, derived_workflow, derived_memory, etc.),
   - content and/or content hash,
   - freshness/turn metadata,
   - and whether this is a new version or a refresh of an existing logical section.

2. **inference payload / envelope records**
   - represent the actual bundle sent to inference,
   - include payload ID,
   - included context snapshot refs,
   - included transcript/message refs,
   - tool schema refs or hashes,
   - model/provider ref,
   - and enough ordering/provenance to answer “what did the model actually see?”

This stronger version matters because arbitrary context sections are powerful but can also create hidden behavior steering.
If context sections are not durably recorded, later debugging and audit become much weaker.

#### Stronger version later: refreshed context sections and latest-effective semantics

One subtle design problem is how refreshed context sections should “find their way back” into session history without polluting transcript replay.

Example:

- repo-awareness section generated at turn 12,
- more conversation and tool turns happen,
- repo-awareness section refreshed again at turn 24,
- inference bundle at turn 25 should likely include only the latest effective repo-awareness section,
- while still preserving durable evidence that both versions existed.

The likely direction is:

- keep context snapshot records as first-class non-chat session records,
- treat them as their own history stream inside session history,
- and when building inference payloads, select the **latest effective snapshot** per logical section key unless a more specific policy says otherwise.

The likely authored representation for that policy is intentionally narrow:

- `refreshEveryNTurns` to control when a generated section should be refreshed,
- `retentionMode` to control how multiple historical snapshots or transcript items should be selected.

Examples:

- `repo_context` might use `refreshEveryNTurns: 12` and `retentionMode: latest_effective`
- `messages` might use `retentionMode: coherent_tail`

This implies a distinction between:

- **conversation history** — user/assistant/tool transcript,
- **context history** — generated context sections/chunks and their refreshes,
- **inference history** — exact envelopes sent to the model.

That separation is desirable.
It lets us avoid polluting the user/assistant transcript with fake system chatter while still making context injections durable and inspectable.

The likely selection rule is:

- keep durable append-only context snapshot history,
- define a logical section key (for example `repo_context`),
- allow multiple snapshots over time for that key,
- and at payload build time pick the latest effective snapshot for the current turn unless the section policy requires multiple versions.

The order of sections in the authored context definition should define payload order.
We should not introduce a separate `sticky` field unless we discover a real need for section-priority semantics beyond ordering plus retention mode.

This does add some complexity to context sections/chunks, but it is likely the right complexity rather than accidental prompt magic.

### `provider/`

Sketch7 should model providers after the real provider boundary in `internal/providers`, not after sketch6’s stub behavior.

Responsibilities:

- provider interface for sending shaped context and receiving structured assistant/tool outputs,
- OpenRouter adapter,
- fake provider for tests.

The important lesson from `internal/providers` is that provider adapters should convert between:

- shaped context items / tool definitions,
- and provider-specific request/response objects.

They should not inspect prompt prose to decide what the model “meant.”

#### Sketch7 direction

Use a structured provider contract closer to the real one:

- input: shaped prompt/context items + options
- output: structured assistant messages and tool calls

### `tools/`

Responsibilities:

- tool definitions,
- tool registry,
- execution boundary,
- concrete runtime-mutating tools.

For MVP, we likely need at least:

- `transition_state`
- `bind_workflow`
- `unbind_workflow`
- one or two demo domain tools
- coding CLI tools for repo inspection, editing, and test execution

The key design rule is:

**tools should return durable records/effects in a disciplined way, not mutate runtime state invisibly.**

Whether we model the result as records directly or as effects that are later persisted, the central rule is explicitness.

### `trace/`

Owns inference provenance records.

This remains important for:

- replay/debugging,
- post-hoc explanation,
- restart/recovery analysis,
- and later performance/financial accounting.

### `catalog/`

This is the trimmed replacement for the larger registry stack.

For MVP, it should do only what we actually need:

- load YAML definitions,
- optionally apply late env expansion,
- hold current in-memory definitions for runtime use.

If durable revision storage is required for MVP, keep it small and explicit.

Do not reintroduce the full generic ingest architecture unless the MVP proves it necessary.

For MVP, hot-reload matters more than ingest generality.
The loader should make it easy to:

- edit YAML,
- reload definitions,
- and observe the new behavior on subsequent turns.

## Dev UX considerations

The main UX risk in collapsing packages is not technical.
It is discoverability.

Developers need clear answers to these questions:

- Where do I author schemas?
- Where does YAML get loaded?
- Where do definitions become runtime config?
- Where is current state reduced from histories?
- Where do prompt projections live?
- Where do tools mutate state?
- Where does the execution loop start?

The sketch7 package plan above keeps those answers crisp while still trimming complexity.

In particular:

- keeping `runtime/hydrate.go` is good for UX,
- keeping `prompt/` distinct is good for UX,
- keeping `statecharts/` narrowly scoped is good for UX,
- and keeping `catalog/` small but explicit is better than a generic registry tower for MVP.

## What to copy from sketch6

### Copy with little or no change

- loop structure from `runner/loop.go`
- provenance and payload ideas from `assembly/`
- runtime view structs from `runtime/`
- model definition shapes from `model/`
- much of the agent/workflow definition shape
- transition engine logic from `charts/`

### Copy only after redesign

- session/workflow history code
- hydration code
- runtime host code
- transition tool code
- provider interface shape from sketch6

### Do not copy forward

- provider behavior based on parsing prompt text
- third binding log package
- broad generic registry/ingest architecture
- speculative chunk metadata not enforced by runtime
- duplicated reduction logic in multiple packages

## MVP user-facing capabilities

The MVP should feel like a real coding CLI, not just a runtime experiment.

At minimum, the user should be able to:

1. start a chat session in a repo,
2. describe a feature, bug, or refactor target,
3. let the agent turn that discussion into a structured execution path,
4. interrupt the agent while it works,
5. provide new guidance and resume execution,
6. watch the agent inspect files, edit code, run tests, and report progress,
7. see explicit workflow and cognitive progression,
8. change YAML behavior during the run,
9. and recover the session after restart.

If the system does not make the repo-edit/test/debug loop excellent, the architecture has outrun the product.

## MVP milestones

### Milestone 1: executable spine

Deliver:

- basic definitions,
- interactive chat session support,
- separate session/workflow logs,
- statechart transition engine,
- centralized reducers,
- runtime host and local loop,
- fake provider,
- transition tool,
- interrupt/resume session behavior,
- provenance-aware prompt assembly.

Acceptance:

- can run a single chat session that binds into workflow-backed execution,
- can interrupt execution, provide input, and resume coherently,
- can restart and reconstruct current runtime view from histories.

### Milestone 2: real provider integration

Deliver:

- provider adapter modeled after `internal/providers`,
- tool-call round-tripping with structured messages,
- better execution tests.

Acceptance:

- a real provider request can be built from sketch7 prompt projections,
- provider responses can yield assistant outputs and tool requests without prompt-text parsing hacks.

### Milestone 2.5: coding CLI usefulness threshold

Deliver:

- repo inspection tools,
- file edit tools,
- test/build execution tools,
- and a review/validation gate in the anchor workflow.

Acceptance:

- the user can take a task from chat through implementation and verification inside the CLI,
- and the repo-edit/test/debug loop feels meaningfully useful, not just architecturally correct.

### Milestone 3: definition loading MVP

Deliver:

- simple YAML catalog loader,
- late env expansion where required,
- in-memory definition catalog for runtime.

Acceptance:

- runtime host can boot from authored YAML definitions,
- developer can update definitions and reload them without needing the full registry architecture.

## Open questions for follow-up planning

These should be answered before sketch7 grows too far:

1. What exact MVP binding ownership rule do we want across session/workflow histories?
2. Do bind/unbind records live in both histories, or is one authoritative and the other referential?
3. What is the minimum viable effective-tool-policy calculation for MVP?
4. Do tools return durable records directly, or a typed effect set that host persists?
5. What late env expansion behavior do we require at agent/workflow/model load time?
6. What minimum revision/provenance do we need for definitions before full store/registry returns?
7. Should chat begin in a dedicated chat workflow, or should the anchor workflow itself start in `chatting`?
8. What is the minimum viable review/validation gate that makes the CLI trustworthy?
9. What hot-reload changes are allowed mid-session without requiring migration or restart?
10. What exact durable events define interrupt, user intervention, and resume semantics?

## Recommendation

Start sketch7 by implementing the smallest executable spine first, in this order:

1. `defs/`
2. `logs/`
3. `statecharts/`
4. `runtime/reduce.go`
5. `prompt/`
6. `tools/`
7. `provider/fake.go`
8. `runtime/host.go` + loop
9. `trace/`
10. `catalog/`

That order forces the architecture to prove the core runtime before surrounding it with convenience machinery.

## Roadmap after MVP

The long-term Maelstrom vision still matters, but it should be sequenced after the MVP proves itself.

Post-MVP directions may include:

- specialized workflow families such as TDD, review, debugging, and overnight experimentation,
- meta-agents that refine agent/workflow definitions,
- KPI-driven optimization of token spend and task performance,
- definition revision and promotion pipelines,
- permission and security layers,
- timers and scheduled execution,
- and eventual financial/accounting systems.

These are valid roadmap directions.
They are not required to prove sketch7.
