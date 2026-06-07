# Maelstrom Evals Strategy

This document reframes evaluation for Maelstrom around a simple idea:

**before we optimize broad agent behavior, we need hard numbers on runtime reliability.**

The recent experiments were useful for narrowing direction, but they mixed together prompt shape, workflow design, task choice, and runtime behavior. The next phase should make the evaluation surface much more explicit.

In practice, this means we should evaluate Maelstrom in layers:

1. **runtime protocol reliability first**
2. **tool-loop reliability second**
3. **minimal coding-agent task execution third**
4. **external benchmark adapters after the internal substrate is stable**

This keeps the eval effort aligned with `docs/vision.md`: Maelstrom is supposed to become a reliable, durable runtime for long-running agents and workflows, not just a collection of prompts that sometimes look good.

## Why this framing changed

The experimentation phase suggested several things clearly:

- optimizing prompts without isolating runtime failure modes produces noisy conclusions
- many failures are really **protocol and control failures**, not capability failures
- long-horizon success depends on improving the reliability of small steps
- malformed structure and tool misuse should be treated as first-class signals

This lines up with both:

- **Forge** style guardrails for reliable tool-calling
- **Dirac** style emphasis on precise tool design and context curation
- and the broader lesson from decomposition-heavy work such as the MAKER paper: reliability improves when tasks are broken into smaller, more tightly specified units

## Core recommendation

For the next phase, Maelstrom should be evaluated starting from a **minimal coding agent**:

- a **single cognitive mode**
- a tight loop on itself
- no workflow orchestration required for the first eval layer
- a small, auditable tool surface
- explicit required outputs that can be validated mechanically

This is not a retreat from statecharts. It is a way to use statecharts more effectively later.

The direction is:

- statecharts should increasingly define **tiny state-scoped tasks**
- each state should have a clear **required input contract** and **required output contract**
- ingestion and hydration should wire those contracts into runtime
- evals should tell us whether the contracts are being satisfied reliably

In other words, we should stop expecting the agent to reason *about* the state machine itself and instead use the state machine to constrain the local task it is solving.

## Evaluation layers

## Layer 1: Runtime protocol evals

This layer measures whether Maelstrom can produce and validate machine-usable outputs.

### Examples

- valid cognitive/control JSON
- required fields present
- enum values valid
- no missing tool argument payloads
- no illegal assistant/tool output shapes
- no schema drift

### Why it matters

If this layer is weak, every higher-level benchmark will be noisy and hard to interpret.

## Layer 2: Tool-loop reliability evals

This layer measures whether the runtime can survive tool-calling in a disciplined, recoverable way.

### Examples

- failed tool calls / total tool calls
- malformed tool call rate
- rescued malformed tool call rate
- unrecoverable malformed tool call rate
- retry success rate
- loop completion rate
- max-step exhaustion rate
- wrong-tool selection rate

### Why it matters

This is the closest current analog to Forge's contribution: add a reliability layer around the loop before worrying about richer orchestration.

## Layer 3: Minimal coding-task evals

This layer measures whether a constrained Maelstrom agent can carry out small real coding tasks.

### Examples

- inspect a file and report a symbol summary
- make a single exact replacement
- run a validation command and report outcome
- complete a read -> edit -> validate loop

### Why it matters

This is where we begin validating whether the runtime and tools are actually useful for coding, not just structurally valid.

## Layer 4: External benchmark adapters

Only after the lower layers are stable should we integrate broader external benchmarks.

### Priority order

1. **Forge scenarios** for tool-calling reliability
2. **Terminal-Bench 2.x** subsets for terminal and coding task realism
3. **SWE-Bench Lite / Verified** for patch-generation and issue resolution
4. **Dirac-inspired custom refactor tasks** for precision editing and context efficiency

## Immediate runtime metrics

The first eval phase should focus on granular, mechanically checkable metrics.

Each metric below is paired with:

- what the metric means
- what changes it should drive
- and which files are likely to be involved

## 1. Cognitive output schema success rate

### Definition

The fraction of model turns that produce a valid, required control/cognitive output shape.

For the first pass, this should be a strict schema with required fields.

### What to measure

- valid JSON rate
- required field presence rate
- valid enum/value rate
- schema-conformant rate
- rescued schema-valid-after-repair rate
- unrecoverable invalid rate

### Recommended changes if weak

- add stricter output schema validation in the loop
- add rescue parsing for near-miss JSON
- add retry nudges that explicitly restate the required output shape
- move state/task requirements into hydration so each state has an explicit IO contract

### Likely files

- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/prompt/projection.go`
- `sketch/sketch7/compile/hydrate.go`
- `sketch/sketch7/defs/agent.go`
- `sketch/sketch7/defs/workflow.go`

## 2. Tool call validity rate

### Definition

The fraction of proposed tool calls that reference a known tool and satisfy the expected argument shape.

### What to measure

- valid tool name rate
- valid argument-shape rate
- required-argument presence rate
- tool call parse rescue rate
- unknown tool request rate

### Recommended changes if weak

- validate every tool call before execution
- add schema-aware tool argument validators
- add canonical tool error feedback and retry behavior
- simplify or tighten tool interfaces where ambiguity is causing failures

### Likely files

- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/tools/*.go`
- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/compile/hydrate.go`

## 3. Tool execution success rate

### Definition

The fraction of executed tool calls that succeed without returning a runtime/tool error.

### What to measure

- successful tool executions / total tool executions
- failed tool executions / total tool executions
- per-tool failure rate
- repeat-failure rate after retry

### Recommended changes if weak

- improve exact-once edit semantics
- improve file-path and argument validation
- improve timeout/error surfacing in command tools
- reduce overly broad or ambiguous tool behavior

### Likely files

- `sketch/sketch7/tools/read_file.go`
- `sketch/sketch7/tools/replace_text.go`
- `sketch/sketch7/tools/run_command.go`
- `sketch/sketch7/tools/*.go`
- related tests under `sketch/sketch7/tools/*_test.go`

## 4. Retry recovery rate

### Definition

The fraction of malformed or invalid turns that become valid after a repair/retry cycle.

### What to measure

- retries attempted
- retries successful
- retries exhausted
- repeated identical error rate

### Recommended changes if weak

- improve retry prompts/nudges
- preserve explicit tool error messages in canonical channels
- distinguish between rescuable parse failures and genuine reasoning failures
- track red-flag output categories more explicitly

### Likely files

- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/provider/openai_compatible.go`
- future eval harness code under `docs/evals/` or `maelstrom-evals/`

## 5. Loop completion rate

### Definition

The fraction of runs that terminate correctly rather than drifting, exhausting steps, or stalling.

### What to measure

- successful completion rate
- max-step exhaustion rate
- stalled loop rate
- terminal-response correctness rate

### Recommended changes if weak

- tighten terminal output contract
- separate tool-intent from final-response intent more explicitly
- reduce unnecessary tool surface in the minimal agent
- add explicit completion signaling in the required output contract

### Likely files

- `sketch/sketch7/runner/loop.go`
- `sketch/sketch7/provider/provider.go`
- `sketch/sketch7/main.go`
- `sketch/sketch7/runner/loop_test.go`

## 6. Minimal coding task success rate

### Definition

The fraction of constrained coding tasks completed correctly by the minimal agent.

### What to measure

- task success rate
- exact edit correctness
- validation command success
- tool calls per successful task
- time-to-first-correct-result

### Recommended changes if weak

- improve tool ergonomics inspired by Dirac
- reduce context bloat
- improve edit precision and read bandwidth
- tighten task setup and acceptance criteria in eval harnesses

### Likely files

- `sketch/sketch7/main.go`
- `sketch/sketch7/tools/*.go`
- `sketch/sketch7/context/*`
- future adapters and eval harness code

## Statecharts as task decomposition, not self-awareness

An important design direction for later work is emerging clearly:

- agents should not need to reason explicitly about the underlying machine model
- instead, authored statecharts should define **tiny local tasks**
- each state should specify:
  - what inputs are available
  - what outputs are required
  - what tools are visible/enabled
  - what completion condition allows transition

This means future eval work should pressure the system toward:

- **state-local IO contracts**
- hydration that turns authored contracts into runtime validators/projections
- runtime that scores success based on contract satisfaction

That design work belongs in planning and implementation docs, but the eval layer should be written now so it can drive those changes rather than follow them.

## Proposed repo structure

There are two reasonable ways to organize this.

## Option A: Start in-repo, split later

Useful while runtime and eval substrate are changing together.

```txt
docs/
  evals/
    runtime/
    coding/
    external/
  planning/
  reports/
```

With supporting code either under:

```txt
sketch/sketch7/
internal/
scripts/
```

## Option B: Separate `maelstrom-evals` repo later

Once the interfaces stabilize, use a clean external repo.

```txt
maelstrom-evals/
├── README.md
├── pyproject.toml / requirements.txt
├── .gitignore
├── scripts/
│   ├── run_runtime_evals.py
│   ├── run_forge_eval.py
│   ├── run_terminal_bench.py
│   ├── run_swe_bench.py
│   └── common/
├── configs/
│   ├── models.yaml
│   ├── agent_profiles/
│   ├── workflows/
│   └── ablations/
├── adapters/
├── evals/
│   ├── runtime/
│   ├── coding/
│   └── external/
├── results/
│   ├── raw/
│   └── reports/
├── docker/
└── tests/
```

## External benchmark notes

## 1. Forge scenarios

Start here once the minimal runtime reliability metrics exist.

- use Forge as a tool-calling reality check
- compare bare Maelstrom loop vs guarded Maelstrom loop
- track whether Maelstrom-native guardrails close the gap before proxying through Forge

## 2. Terminal-Bench 2.x

Use after the minimal coding agent can reliably complete internal microtasks.

- start with subsets
- prefer coding/debug/setup tasks that stress read/edit/validate loops
- track success and efficiency, but only after lower-level runtime metrics are stable

## 3. SWE-Bench

Treat as a later-stage coding-agent benchmark.

- start with Lite/Verified
- keep strong trace logging so failures can still be explained by lower-level metrics

## 4. Dirac-inspired tasks

Dirac is valuable both as an inspiration for tool design and as a source of custom coding tasks.

Borrow especially from:

- precise edit targeting
- context curation
- high-signal multi-file operations

## Rollout sequence

1. define the minimal eval agent profile
2. add strict control/cognitive output contracts
3. add validation, rescue parsing, and retry metrics
4. build internal runtime eval cases
5. build minimal coding microtask evals
6. integrate Forge scenarios
7. integrate Terminal-Bench subsets
8. add SWE-Bench and Dirac-inspired custom refactor tasks

## Immediate next steps

1. create a planning doc under `docs/planning/` for state-scoped IO contracts and hydration changes
2. define the first runtime eval result schema
3. implement the minimal single-state coding agent profile
4. add metrics capture for:
   - cognitive schema validity
   - tool validity
   - tool execution success
   - retry recovery
   - loop completion

## Operating rule

Every eval metric should do useful engineering work.

That means:

- every metric must map to one or more recommended runtime/tool changes
- every recommended change should point at concrete files
- reports should summarize not just scores, but which fixes the scores are asking for

If a metric cannot drive a concrete change, it is probably too vague.
