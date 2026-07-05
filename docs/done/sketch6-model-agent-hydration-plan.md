# Sketch6 Model/Agent Hydration Implementation Plan

**Status:** Proposed | **Date:** 2026-05-30

## Problem

`sketch6` currently proves several useful ideas:

- generic YAML ingestion into typed specs,
- registries backed by revisions,
- a late binding step before execution,
- an assembly pipeline that composes prompt chunks,
- and a runner loop that executes provider/tool interactions.

But the current sketch still collapses authored configuration and runtime state together.

Today, the agent shape in `sketch6` still directly owns fields such as:

- provider name,
- provider-specific model name,
- temperature,
- and context limit.

That creates the same ownership confusion described in `docs/planning/model-and-agent-yaml.md`:

- provider concerns leak into agent YAML,
- model defaults and limits are treated like agent behavior,
- the system prompt is synthesized at runtime rather than authored as context,
- and hydration is effectively just env substitution rather than model + agent + runtime resolution.

For `sketch6`, we want a cleaner sandbox that explicitly demonstrates the intended ownership and hydration model before we port anything into the main implementation.

## Goal

Refactor `sketch6` into a self-contained experiment where:

- **model YAML** owns logical model defaults, limits, capabilities, and provider candidates,
- **agent YAML** owns model selection, behavior, tools, and context definition,
- **registries** store authored definitions,
- **hydration** resolves definitions into executable runtime state,
- **runtime** selects a concrete provider/model reference,
- and **assembly** is built from authored context chunk declarations.

Conceptually, the sketch should move toward this pipeline:

```txt
Model YAML -> ModelDefinition -> model registry
Agent YAML -> AgentDefinition -> agent registry

AgentDefinition + ModelDefinition + runtime deps
    -> hydration
    -> RuntimeAgent
    -> runner/provider execution
```

## Non-goals

For this sketch refactor, we are **not** trying to:

- preserve backward compatibility with the old `sketch6` agent YAML shape,
- design the final production migration path for the main codebase,
- finalize every future cognitive/workflow field,
- implement full provider capability negotiation,
- or overbuild file watching / persistence polish.

The sketch should optimize for architectural clarity, not compatibility.

## Design principles

### 1. Separate authored definitions from runtime state

`sketch6` should stop using one struct as both authored config and executable runtime state.

We want an explicit split between:

- authored definitions,
- hydrated runtime state,
- and execution-time components.

### 2. Make ownership obvious in types

If a field is authored by humans in YAML, it should live in a definition type.

If a field is selected or resolved at runtime, it should live in a runtime type.

This is especially important for:

- provider selection,
- provider-specific model refs,
- effective inference settings,
- and context chunk implementations.

### 3. Keep registries definition-shaped

Registries should store authored or normalized definitions, not runtime objects.

Hydration should happen after lookup, not inside the registry itself.

### 4. Make hydration explicit

The sketch should have a named hydration step that:

- resolves an agent’s logical model,
- validates cross-object constraints,
- chooses a provider candidate,
- merges defaults and overrides,
- resolves tools,
- and translates context definitions into executable chunk plans.

### 5. Keep the runner simple

The runner should consume already-hydrated runtime state.

It should not need to know how to:

- look up model definitions,
- choose a provider,
- interpret YAML ownership,
- or validate authored configuration.

## Target architecture

## Authoring layer

Authoring layer objects are decoded from YAML and represent user-authored intent.

Planned authored types:

- `model.Definition`
- `agent.Definition`

These should be declarative and portable.

## Registry layer

Registries store authored definitions by key.

Planned registries:

- model registry: `model name -> model.Definition`
- agent registry: `agent name -> agent.Definition`

The existing generic ingest pipeline in `sketch6/registry` is worth keeping.

## Hydration layer

Hydration resolves authored definitions into executable runtime state.

Planned responsibilities:

- agent -> logical model lookup,
- validation across agent/model definitions,
- provider candidate selection,
- default/override merge,
- tool resolution checks,
- and context chunk hydration.

## Runtime layer

Runner, provider, and tools should consume a normalized runtime object.

Planned runtime object:

- `runtime.Agent`

This runtime object should carry:

- logical model name,
- selected provider name,
- selected provider model ref,
- effective inference settings,
- hard limits/capabilities as needed,
- tool references,
- and hydrated context plan/chunks.

## Proposed package changes

A sketch-oriented package shape could look like this:

- `sketch/sketch6/agent/definition.go`
- `sketch/sketch6/model/definition.go`
- `sketch/sketch6/runtime/agent.go`
- `sketch/sketch6/hydrate/hydrate.go`
- `sketch/sketch6/hydrate/validate.go`
- `sketch/sketch6/hydrate/provider_select.go`
- `sketch/sketch6/registry/agent/...`
- `sketch/sketch6/registry/model/...`
- `sketch/sketch6/assembly/...`
- `sketch/sketch6/provider/...`
- `sketch/sketch6/runner/...`

### Notes

- The current generic registry/ingest code should remain.
- `agent/spec.go` should be replaced or repurposed into an authored definition type.
- `bind/bind.go` should likely become hydration-oriented rather than env-substitution-oriented.

## Proposed authored types

## Model definition

The model definition should own:

- logical model name,
- ordered provider candidates,
- hard limits,
- default inference settings,
- and capabilities.

Illustrative direction:

```go
type Definition struct {
    Name         string
    Providers    []ProviderRef
    Limits       Limits
    Defaults     Defaults
    Capabilities Capabilities
}

type ProviderRef struct {
    Name     string
    ModelRef string
}

type Limits struct {
    ContextWindow   int
    MaxOutputTokens int
}

type Defaults struct {
    Temperature float64
    TopP        float64
}

type Capabilities struct {
    Tools      bool
    Reasoning  bool
    Multimodal bool
    Streaming  bool
}
```

## Agent definition

The agent definition should own:

- identity and metadata,
- logical model name selection,
- agent-level overrides,
- tool exposure,
- and context definition.

Illustrative direction:

```go
type Definition struct {
    Name        string
    Description string
    Model       string
    Overrides   Overrides
    Tools       []string
    Context     ContextDefinition
    Cognitive   CognitiveDefinition
}

type Overrides struct {
    Temperature     *float64
    TopP            *float64
    MaxOutputTokens *int
}

type ContextDefinition struct {
    InputBudget int
    Chunks      []ChunkDefinition
}

type ChunkDefinition struct {
    Type      string
    Prompt    string
    Chart     string
    Flexible  bool
    Policy    string
    Priority  int
    BudgetPct float64
}
```

`CognitiveDefinition` can remain minimal or deferred until needed.

## Proposed runtime type

The runtime agent should hold fully-resolved execution data.

Illustrative direction:

```go
type Agent struct {
    Name         string
    Description  string
    LogicalModel string

    ProviderName string
    ProviderRef  string

    Limits       Limits
    Inference    InferenceSettings
    Capabilities Capabilities

    Tools   []string
    Context ContextPlan
}
```

The exact shape can vary, but the key rule is:

- authored fields stay in definitions,
- resolved runtime values stay in runtime types.

## YAML shapes to prove in sketch6

## Model YAML

```yaml
apiVersion: maelstrom/v1
kind: Model

name: glm-4.5-air

providers:
  - name: openrouter
    modelRef: z-ai/glm-4.5-air:free
  - name: fireworks
    modelRef: accounts/fireworks/models/glm-4p5-air

limits:
  contextWindow: 32768
  maxOutputTokens: 8192

defaults:
  temperature: 0.7
  topP: 1.0

capabilities:
  tools: true
  reasoning: true
  multimodal: false
  streaming: true
```

## Agent YAML

```yaml
apiVersion: maelstrom/v1
kind: Agent

name: weather-assistant
description: Reliable weather lookup with concise answers

model: glm-4.5-air

overrides:
  temperature: 0.4

tools:
  - transition_state
  - weather

context:
  inputBudget: 24000
  chunks:
    - type: system
      prompt: |
        You are a cheerful, accurate weather assistant.
        Always respond in Celsius.
        Be concise and friendly.
      budgetPct: 0.05
      policy: fail
      priority: 10

    - type: messages
      flexible: true
      policy: hard
      priority: 5

    - type: state
      chart: agent
      priority: 4

    - type: state
      chart: workflow
      priority: 4
```

## Registry plan

## Keep the generic ingest pipeline

`sketch6/registry.Ingestor[D,S]` is already a good foundation and should remain in place.

It already provides:

- decode,
- preprocess hooks,
- hoist into typed values,
- revision storage,
- and registry update.

That generic mechanism should be reused for model and agent definitions.

## Add a model registry

Add `sketch6/registry/model` parallel to the existing agent/workflow registries.

This should include:

- a YAML `Document` shape,
- a `Decoder`,
- a `Hoister`,
- and a `NewIngestor(...)` function.

The model hoister should validate basic structure such as:

- `apiVersion` present,
- `kind == "Model"`,
- `name` present,
- at least one provider candidate,
- and sane limit/default fields.

## Refactor the agent registry

The current agent registry hoists into a runtime-ish spec that still mixes ownership layers.

It should instead hoist into `agent.Definition`.

In particular, the agent YAML shape should stop authoring:

- provider,
- provider-specific model name,
- top-level temperature,
- and top-level context limit.

## Hydration plan

## New hydrator

Add a dedicated hydration package or hydration-oriented binder.

Illustrative direction:

```go
type Hydrator struct {
    Models registry.Registry[model.Definition]
    Tools  tools.Registry
}

func (h Hydrator) HydrateAgent(def agent.Definition) (runtime.Agent, error)
```

It may also be useful to support lookup-by-name:

```go
func (h Hydrator) HydrateAgentByName(name string) (runtime.Agent, error)
```

## Hydration responsibilities

Hydration should perform the following steps.

### 1. Resolve model

- Look up `agentDef.Model` in the model registry.
- Return a clear error if the logical model does not exist.

### 2. Validate cross-object constraints

Examples:

- `agent.context.inputBudget <= model.limits.contextWindow`
- agent overrides are structurally sane
- requested tools exist in the tool registry
- chunk definitions are structurally valid
- chunk types and policies are recognized

### 3. Select provider candidate

For the sketch, provider selection can start simple:

- iterate model provider candidates in order,
- select the first viable candidate.

Initial “viable” logic may be lightweight, for example:

- provider name recognized,
- optional credential/env presence,
- or simply first candidate for early iterations.

### 4. Merge defaults and overrides

Apply precedence such as:

```txt
provider bounds
    -> model defaults/limits
        -> agent overrides
```

This should yield effective runtime inference settings.

### 5. Hydrate context plan

Translate declarative chunk definitions into executable assembly chunks.

Examples:

- `type: system` -> static system prompt chunk
- `type: messages` -> recent history/messages chunk
- `type: state` with `chart: agent` -> state projection chunk
- `type: state` with `chart: workflow` -> state projection chunk

### 6. Produce runtime agent

Return a normalized runtime agent object suitable for the runner and provider interfaces.

## Assembly plan

## Keep the chunk interface

The current `assembly.Chunk` abstraction is a good fit and should remain.

It already gives us a clean execution seam:

- definition/hydration decides what chunks exist,
- runtime assembly just executes them.

## Replace hardcoded assembler construction

Today, `bind/bind.go` constructs a fixed chunk list.

That should be replaced with hydration-driven chunk assembly based on `agent.Context.Chunks`.

## Add chunk mappings for authored context

The first chunk types supported in the sketch should be minimal:

- `system`
- `messages`
- `state`

That is enough to prove the authored context design without overbuilding.

## De-emphasize synthetic system prompt generation

The current `SystemPromptChunk` synthesizes a prompt from runtime metadata.

For the sketch’s primary path, system prompt content should come from authored context.

If a synthetic chunk remains, it should be clearly secondary or experimental.

## Provider and runner plan

## Provider input should use hydrated runtime selection

Provider request building should use the runtime-selected values, not authored YAML fields.

That means shifting from old fields like:

- `spec.Model.Provider`
- `spec.Model.Name`

into hydrated runtime fields like:

- `runtimeAgent.ProviderName`
- `runtimeAgent.ProviderRef`

## Runner should consume runtime agent

The runner should stop depending on authored agent config.

Conceptually:

```go
func (l Loop) Run(agent runtime.Agent, history *session.History, store *inference.Store) error
```

This keeps the runner focused on execution.

## Main entrypoint sketch flow

The sketch `main.go` should be updated to demonstrate the full new flow:

1. ingest model YAML,
2. ingest agent YAML,
3. hydrate agent against the model registry,
4. build runtime chunk plan,
5. run the existing provider/tool loop.

For early iterations, inline YAML strings in `main.go` are acceptable and even preferable, since they keep focus on architecture rather than filesystem wiring.

## File-by-file implementation order

### Phase 1: establish new types

1. Add `sketch/sketch6/model/definition.go`
2. Replace or repurpose `sketch/sketch6/agent/spec.go` into authored definition types
3. Add `sketch/sketch6/runtime/agent.go`

### Phase 2: refactor registries

4. Add `sketch/sketch6/registry/model/model.go`
5. Refactor `sketch/sketch6/registry/agent/agent.go` to hoist into `agent.Definition`

### Phase 3: add hydration

6. Add `sketch/sketch6/hydrate/hydrate.go`
7. Add validation/provider-selection helpers
8. Produce `runtime.Agent`

### Phase 4: connect assembly to authored context

9. Add or rename chunk types needed for authored context
10. Replace fixed assembler construction with hydrated chunk construction

### Phase 5: wire runtime through execution

11. Update `sketch/sketch6/provider/provider.go` to consume runtime agent state
12. Update `sketch/sketch6/runner/loop.go`
13. Update `sketch/sketch6/inference/inference.go` metadata recording if needed

### Phase 6: rewrite the example flow

14. Update `sketch/sketch6/main.go` to ingest model + agent YAML and hydrate before running

## Validation expectations

## Model validation

Examples:

- `kind` is correct
- logical model name is present
- provider entries are structurally valid
- limits are sane
- defaults are sane
- capabilities are coherent enough for sketch use

## Agent validation

Examples:

- referenced logical model exists
- overrides are valid
- `context.inputBudget` is positive
- `context.inputBudget` does not exceed model `limits.contextWindow`
- requested tools exist
- chunk types are known
- chunk-specific required fields are present

## Runtime validation

Examples:

- selected provider candidate is viable enough for the sketch
- hydration succeeds
- provider request can be built from runtime agent state

## Success criteria

The `sketch6` refactor should be considered successful when this flow works cleanly:

1. ingest a `Model` YAML into a model registry
2. ingest an `Agent` YAML into an agent registry
3. hydrate the agent against the model registry
4. select a concrete provider/modelRef
5. validate `inputBudget <= contextWindow`
6. build assembly chunks from authored `context.chunks`
7. run the existing inference/tool loop from hydrated runtime state

## Risks

| Risk | Mitigation |
|---|---|
| Half-old, half-new ownership leaves the sketch ambiguous | Remove old agent-owned provider/model default/context-limit fields decisively |
| Over-modeling cognitive/workflow concerns too early | Keep initial authored schema small: model, overrides, tools, context |
| Hydration becomes too magical | Keep provider selection, merge rules, and chunk translation explicit |
| Runner complexity increases | Hydrate into a normalized runtime object before execution |
| Assembly refactor gets blocked by schema breadth | Support only `system`, `messages`, and `state` chunk types first |

## Recommendation

Implement the first pass with the narrowest useful slice:

- authored model definitions,
- authored agent definitions,
- model registry,
- hydration,
- and only these context chunk types:
  - `system`
  - `messages`
  - `state`

That is enough to make `sketch6` a credible proof of the YAML ownership and hydration plan without prematurely solving every surrounding concern.
