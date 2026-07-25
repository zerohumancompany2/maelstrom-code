# sketch7

`sketch7` is the current YAML-configurable agent-runtime sketch for experimenting with bounded
agent cognition, task-local context assembly, tool gating, and durable session records.

## Included example agents

- `agents/stateless-reader.yaml`: an essentially stateless, read-only baseline agent.
- `agents/ooda-reader.yaml`: a read-only OODA agent with explicit Observe, Orient, Decide, and Act
  cognitive modes. The model emits state-local JSON signals; the runtime owns transitions.

These are useful as paired comparison agents for the stress-test plan in
`docs/planning/sketch7-cognitive-state-stress-test-plan.md`.

## Purpose of agent cognitive states

In sketch7, a cognitive state is not meant to encode a business workflow like “implementation” or
“testing.” Instead, it defines **what kind of thinking should govern the next bounded inference
window**.

That matters for a few reasons:

- **Prompt discipline:** each state carries a short local task prompt instead of one giant agent
  instruction block.
- **Tool discipline:** a state can narrow which tools are visible or enabled.
- **Bounded behavior:** a state can cap inference turns, tool calls, and finalization retries.
- **Inspectable outputs:** a state can declare an output schema and required fields so the runtime
  can validate what the model produced.
- **Runtime-owned transitions:** a model can emit a validated state-local signal like
  `"transition":"observed"`; the runtime checks that signal against the current state's allowed
  triggers and appends lifecycle records.
- **Durable lifecycle records:** entering, exiting, retrying, and finalizing a state becomes
  measurable in session logs.

For example, an OODA agent can:

- **Observe** to formalize the request and gather facts,
- **Orient** to interpret facts against constraints and risk,
- **Decide** to select the smallest sufficient next move, then
- **Act** to execute or finalize without reopening broad deliberation.

By contrast, a “stateless” baseline can be represented as a single cognitive state. That still fits
the runtime, while giving us a comparison point against multi-state cognition.

## Concise basics for implementing an agent in YAML

An agent definition is a YAML document with:

- top-level identity (`apiVersion`, `kind`, `name`, `description`)
- a referenced logical model (`model`)
- an allowlist of tools (`tools`)
- context projections (`context.projections`)
- a cognitive statechart (`cognitive`)

### Minimal shape

```yaml
apiVersion: maelstrom/v1
kind: Agent
name: example-agent
description: Minimal example
model: zh-qwen36-27b-thinking
tools:
  - read_file
context:
  inputBudget: 24000
  projections:
    - type: system
      name: system
      prompt: You are a helpful repository agent.
    - type: state_task
    - type: messages
cognitive:
  initialState: respond
  states:
    - name: respond
      prompt: Answer the request directly.
  transitions: []
```

### Common projection types

- `system`: static instruction text
- `repo_context`: derived repo summary
- `state_task`: derived task/state frame with tools, bounds, inputs, outputs
- `binding`: workflow-binding information when present
- `interaction`: session interaction mode
- `messages`: coherent recent conversation tail

### Common state fields

- `name`: state identifier
- `description`: optional human-readable summary
- `prompt`: task-local instruction for that thought mode
- `visibleTools`: tools the model should see in the prompt
- `enabledTools`: tools the runtime will actually permit
- `allowedTriggers`: transition triggers expected from this state
- `inputs.required` / `inputs.optional`: expected upstream information
- `outputs.schema`: logical output contract name
- `outputs.requiredFields`: required JSON fields for contract validation
- `outputs.strict`: whether extra fields should be rejected
- `completion.successWhen`: compact success criteria
- `bounds.*`: caps such as `maxInferenceTurns`, `maxToolCalls`, and
  `maxFinalizationRetries`

### Stateless vs OODA patterns

- **Stateless baseline:** use one state, no transitions, and typically a small read-only tool set.
- **OODA agent:** define four states (`observe`, `orient`, `decide`, `act`) and transitions between
  them in YAML. The model does not call a transition tool; it emits a validated transition signal in
  its state-local JSON output.

### Running with a YAML agent

```bash
go run ./sketch/sketch7 \
  --agent sketch/sketch7/agents/ooda-reader.yaml \
  --prompt "Inspect sketch7 and summarize its cognitive-state support"
```

If no model YAML is supplied, `sketch7` falls back to its built-in default model definition. The
agent YAML’s `model` value should match that logical model name unless you also provide a custom
model definition.
