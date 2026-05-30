# Model and Agent YAML Ownership

**Status:** Proposed | **Date:** 2026-05-28

## Problem

We need a clean authored configuration model for:

- model definitions,
- agent definitions,
- and runtime provider resolution.

Right now, ownership boundaries are only implicit, which makes it easy for fields to drift into the wrong object.

Examples of confusion we want to avoid:

- provider transport concerns leaking into model YAML
- model hard limits being treated like agent behavior fields
- agent behavior concerns being split across multiple unrelated top-level fields
- context budget being treated as if it were owned entirely by either model or agent

## Goal

Define a clear ownership model for authored YAML:

- **model YAML** owns model defaults, limits, and provider preferences
- **agent YAML** owns behavior, selected model, context definition, tool exposure, and workflow concerns
- **provider instances** are resolved at runtime based on available credentials and active models/agents

## Core ownership model

## Model owns

Model definitions describe a logical model, keyed by name in a model registry.

They own:

- logical model name
- preferred provider list / provider fallbacks
- provider-specific model references
- hard model limits
- default inference settings
- model capabilities

### Examples

- max context window
- default temperature
- default top-p
- max output tokens
- tool support
- reasoning support
- streaming support

## Agent owns

Agent definitions describe assistant behavior and orchestration, keyed by name or id in an agent registry.

They own:

- agent identity and metadata
- selected logical model name
- agent-level overrides of model defaults
- context definition
- system prompt content (via context definition)
- tool exposure by reference
- cognitive states / transitions / workflow concerns

### Examples

- which model this agent defaults to
- whether this agent runs colder/hotter than the model default
- how much of the model context window this agent chooses to spend on prompt context
- which tools this agent may use
- how the system prompt and message history are assembled

## Runtime owns

Runtime concerns should not be authored directly into model or agent YAML unless there is a strong reason.

Runtime owns:

- actual provider instance construction
- API key/env availability checks
- provider selection from model preferences/fallbacks
- hydration of agent/model definitions into executable runtime objects

## Registries

The intended lookup flow is:

```txt
Model registry: model name -> ModelDefinition
Agent registry: agent name/id -> AgentDefinition
AgentDefinition.Model -> model name lookup in model registry
```

This gives us:

- stable logical model names in agent YAML
- provider portability for agents
- runtime provider fallback without changing agent definitions

## Context budget ownership

Context budget is intentionally split across two layers.

## Model owns the hard maximum

Model definitions should own the hard context window supported by the model.

For example:

- `contextWindow: 32768`

This is a model capability / constraint.

## Agent owns the usable budget

Agent definitions should own how much of that model window the agent chooses to spend on input context.

For example:

- an agent may intentionally use less than the model maximum
- an agent may reserve more or less room for output/tool use
- an agent may choose a tighter context budget for behavioral or cost reasons

So the model says:

- **what is possible**

and the agent says:

- **what this agent intends to use**

### Rule

The agent may set a usable input budget that is lower than or equal to the model context window.

It should not be allowed to exceed the model hard maximum after hydration/validation.

## Override model

Ownership does not mean values cannot be overridden.

Recommended precedence:

```txt
Provider capability bounds
    -> Model defaults and limits
        -> Agent defaults and overrides
            -> Per-request overrides
```

### Examples

- model default temperature: `0.7`
- agent override temperature: `0.4`
- one request may temporarily override to `0.2`

Likewise:

- model context window: `32768`
- agent input budget: `24000`

## Model YAML sketch

This is a first-pass authored shape, not a final schema.

```yaml
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

## Notes on model YAML

### `name`

Logical name used by the model registry and referenced by agents.

### `providers`

Ordered provider preferences / fallback candidates.

This should remain lightweight:

- provider name
- provider-specific model reference
- maybe priority if needed later

Do **not** stuff transport config here.

### `limits`

Hard model constraints and related defaults.

### `defaults`

Model-level inference defaults, overridable by agents.

### `capabilities`

Useful for validation and runtime routing.

## Agent YAML sketch

This is a first-pass authored shape, not a final schema.

```yaml
name: weather-assistant
description: Reliable weather lookup with concise answers

model: glm-4.5-air

overrides:
  temperature: 0.4

tools:
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

cognitive:
  initialState: default
  states:
    - name: default
      description: Standard user-facing weather assistance
```

## Notes on agent YAML

### `model`

References the logical model name from the model registry.

It should not directly hold a provider-specific model ID.

### `overrides`

Agent-level overrides of model defaults.

Examples:

- temperature
- top-p
- maybe max output tokens

### `tools`

References tool names, not implementations.

Hydration/runtime resolves them from a registry.

### `context`

Owns context-building behavior, including the system prompt.

This is where `Agent.SystemPrompt` should collapse into the authored model.

### `inputBudget`

Represents the agent’s intended usable input context budget, not the model’s hard max context window.

That naming is intentional and helps avoid confusion.

## Runtime provider resolution

The runtime flow should be conceptually:

1. Load agent definition.
2. Resolve logical model via model registry.
3. Inspect model provider preferences/fallbacks.
4. Select the first viable provider based on:
   - available credentials
   - runtime policy
   - model/provider compatibility
5. Hydrate agent + model definitions into executable runtime state.

## Validation expectations

### Model validation

- provider refs are structurally valid
- limits are sane
- defaults are sane
- capabilities fields are coherent

### Agent validation

- referenced model exists
- overrides are valid for the selected model
- `context.inputBudget` does not exceed model `limits.contextWindow`
- referenced tools exist
- context chunk declarations are valid

### Runtime validation

- required provider credentials are available
- selected provider can actually serve the chosen model ref
- hydration succeeds

## Near-term migration guidance

1. Introduce model registry keyed by logical model name.
2. Keep provider refs in model definitions lightweight.
3. Move `Agent.SystemPrompt` into `Agent.Context` design.
4. Split model defaults from agent overrides explicitly.
5. Rename/shape context budget in agent config as an input budget, not a model window.
6. Keep provider selection runtime-resolved.

## Risks

| Risk | Mitigation |
|---|---|
| Provider concerns creep into model YAML | Keep provider entries minimal: name + modelRef only |
| Agent YAML duplicates too many model defaults | Use agent overrides only where behavior needs it |
| Confusion around context limits | Distinguish model `contextWindow` from agent `inputBudget` explicitly |
| Logical model names drift from provider refs | Make model registry authoritative |

## Current recommendation

Adopt this ownership rule of thumb:

- **provider** = how to call a backend
- **model** = what a model can do, what its defaults are, and where it can run
- **agent** = how an assistant behaves, what model it wants, and how it builds/uses context

This keeps authored YAML portable, keeps runtime provider selection flexible, and gives us a clean basis for hydration.
