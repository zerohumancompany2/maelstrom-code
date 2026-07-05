# Context Definition and Agent Hydration Pipeline

**Status:** Proposed | **Date:** 2026-05-28

## Problem

The current code is in a transitional state:

- `Agent` still carries fields like `SystemPrompt` that are really part of context definition
- `ContextDefinition` is currently runtime-shaped, not definition-shaped
- `ContextMap` partially duplicates data that conceptually belongs to the definition layer
- validation currently happens too low in the stack (for example in `ContextMap.BuildMessages`)

We need a simple model that separates:

1. authored configuration,
2. normalized definition data,
3. hydrated runtime objects.

## Goal

Adopt a simple staged pipeline:

```txt
YAML -> ContextDefinition -> ContextMap
```

Where:

- `YAML` is the authored source format
- `ContextDefinition` is the normalized, source-of-truth definition object
- `ContextMap` is the hydrated runtime representation used to build prompt context for inference

This should be designed to support future hydration steps such as:

- environment variable injection
- template evaluation
- derived/default chunk expansion
- registry-based lookups

without making the runtime execution path more complex than necessary.

For ownership of model-vs-agent YAML concerns, see also:

- `docs/planning/model-and-agent-yaml.md`

## Non-goals

- adding an inbound “context layer” for provider responses
- implementing full templating or env-var expansion now
- finalizing every YAML field before the ingestion pipeline exists
- redesigning the CLI entrypoint

## Design principles

### 1. Keep the pipeline shallow

We want two internal layers after YAML, not many:

```txt
YAML -> ContextDefinition -> ContextMap
```

Avoid introducing extra intermediate objects unless they are clearly necessary.

### 2. Definition objects should be declarative

Definition types should describe what the author wants, not hold runtime interfaces.

That means definition-layer types should avoid fields like:

- `ContextChunk`
- `TruncationPolicy`

Those belong to hydration/runtime.

### 3. Runtime objects should be executable

`ContextMap` should hold the actual hydrated chunk and policy implementations needed at inference time.

### 4. Validation belongs in ingestion/hydration

Structural validation should happen before runtime message building.

`ContextMap.BuildMessages` should focus on:

- building chunk output
- token counting
- truncation
- rebalance

and not carry general config validation long-term.

### 5. System prompt is context

`Agent.SystemPrompt` should be folded into context definition. It is not a sibling concept.

## Proposed object roles

## `Agent`

The `Agent` definition should hold top-level agent concerns:

- identity / metadata
- model selection by logical model name
- context definition

It should not separately own raw context content such as a system prompt once context definitions are properly modeled.

### Direction

- move `SystemPrompt` into the context definition shape
- make context configuration flow through one authoritative field

Possible future shape:

```go
type Agent struct {
    ID          string
    Name        string `yaml:"name"`
    Description string `yaml:"description,omitempty"`
    Version     string
    Model       string            `yaml:"model"`
    Context     ContextDefinition `yaml:"context"`
}
```

Model defaults and hard limits should come from model definitions resolved through a model registry, then be applied during hydration.

## `ContextDefinition`

`ContextDefinition` should be the normalized source-of-truth definition object.

This is the thing we conceptually get after YAML parsing and normalization, but before runtime hydration.

It should contain declarative values such as:

- agent-owned usable input budget
- chunk declarations
- budget percentages
- policy names/identifiers
- prompts or templates
- chunk ordering/priority
- tool exposure policy, if that becomes part of context configuration

### Important property

`ContextDefinition` should be definition-shaped, not runtime-shaped.

That means avoiding interface-typed fields like:

- `Chunk ContextChunk`
- `Policy TruncationPolicy`

at the definition layer.

### Possible direction

```go
type ContextDefinition struct {
    InputBudget int               `yaml:"inputBudget"`
    Chunks       []ChunkDefinition `yaml:"chunks"`
}

type ChunkDefinition struct {
    Type      string  `yaml:"type"`
    Prompt    string  `yaml:"prompt,omitempty"`
    BudgetPct float64 `yaml:"budgetPct,omitempty"`
    Priority  int     `yaml:"priority,omitempty"`
    Flexible  bool    `yaml:"flexible,omitempty"`
    Policy    string  `yaml:"policy,omitempty"`
}
```

This exact shape is only illustrative. The key point is that it stays declarative.

## `ContextMap`

`ContextMap` should be the hydrated runtime representation used during inference.

Hydration turns declarative definition data into executable runtime objects:

- `ChunkDefinition` -> `ContextChunk`
- policy identifier -> `TruncationPolicy`
- prompt/template/env references -> resolved content
- agent-selected model name -> hydrated model defaults and limits

### Runtime responsibilities

- build chunk output from a session
- apply token counting
- apply truncation policies
- rebalance to fit the context limit
- return prompt-ready `[]ContextItem`

### Direction

`ContextMap` should become a runtime object, not a second definition store.

For example, something roughly like:

```go
type ContextMap struct {
    Definition ContextDefinition
    Chunks     []HydratedChunkSpec
    Tokenizer  Tokenizer
}

type HydratedChunkSpec struct {
    BudgetPct float64
    Priority  int
    Flexible  bool
    Chunk     ContextChunk
    Policy    TruncationPolicy
}
```

Whether `Definition` remains embedded in the runtime object is optional; the key is to avoid duplicated, competing sources of truth.

## Hydration boundary

We should make hydration explicit.

Conceptually:

```go
func FromDefinition(def ContextDefinition, deps HydrationDeps) (ContextMap, error)
```

Where `deps` may eventually include:

- env lookup
- template functions / rendering context
- tool registry
- validation settings
- model defaults

This can start small and grow only as needed.

## Validation responsibilities

Validation should move upward.

### Definition / ingestion validation

Examples:

- `inputBudget > 0`
- chunk types are known
- policies are known
- required fields are present for a given chunk type
- budget percentages are sane enough
- illegal combinations are rejected
- template references are resolvable
- referenced model exists
- `inputBudget` does not exceed model hard context window once model hydration is applied

### Runtime execution validation

Examples:

- a specific chunk fails to build from current session state
- hydrated prompt rendering fails unexpectedly
- final context still cannot fit after policy-respecting rebalance

## System prompt migration

`Agent.SystemPrompt` should be treated as transitional.

Target direction:

- represent system prompts as a context chunk definition
- hydrate them into `SystemChunk`
- remove separate `SystemPrompt` ownership from `Agent`

Illustrative YAML direction:

```yaml
context:
  contextLimit: 32768
  chunks:
    - type: system
      prompt: |
        You are a cheerful, accurate weather assistant.
      budgetPct: 0.05
      policy: fail
      priority: 10

    - type: messages
      flexible: true
      policy: hard
      priority: 5
```

## Builder vs hydrator

There are two naming options for the construction API:

### Builder style

```go
agent := NewAgentBuilder().
    WithModel(modelDef).
    WithContextDefinition(ctxDef).
    Build()
```

### Hydrator / compiler style

```go
runtimeAgent, err := HydrateAgent(agentDef, deps)
```

Either can work. Given the need for validation, interpolation, defaults, and late binding, “hydration” or “compile” may describe the operation more accurately than “builder”.

## Recommended incremental plan

1. Move `SystemPrompt` conceptually into context definition planning and stop expanding it as a separate long-term field.
2. Introduce a declarative `ChunkDefinition` shape.
3. Change `ContextDefinition` to use declarative chunk definitions rather than runtime interfaces.
4. Introduce `ContextMap.FromDefinition(...)` hydration logic.
5. Move structural validation into the definition ingestion / hydration path.
6. Simplify `ContextMap` so it only owns runtime-execution data.
7. Revisit `Agent` construction around a builder/hydrator once the boundary is clearer.

## Risks

| Risk | Mitigation |
|---|---|
| Over-designing the YAML too early | Keep the definition schema minimal at first |
| Confusing normalized definition vs hydrated runtime state | Make hydration boundary explicit in naming and code |
| Moving validation too late | Push structural checks into ingestion/hydration early |
| Breaking current agent wiring too aggressively | Migrate `SystemPrompt` and chunk declarations incrementally |

## Open questions

1. Should tools be part of context definition, or remain provider/request configuration derived from registries?
2. Should `ContextDefinition` retain model-linked fields like `ContextLimit`, or should those be injected from model hydration?
3. How much templating power do we actually want in authored definitions?
4. Do we want named reusable chunk presets, or only inline chunk declarations?
5. Does hydration need access to runtime environment only, or also to per-agent metadata/state?

## Current recommendation

Do not immediately collapse today’s `ContextMap` and `ContextDefinition` as they currently exist.

Instead:

- make `ContextDefinition` more definition-shaped,
- make `ContextMap` more runtime-shaped,
- and let `FromDefinition(...)` become the explicit boundary between them.

That preserves the simple two-stage internal model while giving us room for future authored-definition features.
