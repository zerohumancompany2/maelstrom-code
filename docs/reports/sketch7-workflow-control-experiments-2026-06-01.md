# Sketch7 Workflow / Control Experiments Report

Date: 2026-06-01

## Scope

This report summarizes recent sketch7 experiments around:

- explicit inference context shaping,
- workflow activation and workflow-state visibility,
- durable CLI turn-by-turn testing,
- repo-aware guidance,
- and model control via workflow/cognitive state projections.

The goal was not to declare a final architecture, but to identify which directions appear promising under real local-model pressure.

## Environment

- sketch root: `sketch/sketch7`
- local model: `zh-qwen36-27b-thinking`
- provider path: OpenAI-compatible local endpoint
- common run pattern:

```bash
source .env.test
go run ./sketch/sketch7 \
  --workflow <workflow.yaml> \
  --session-id <id> \
  --stop-token STOP \
  --prompt "..."
```

## Major implementation changes exercised during these experiments

### 1. Explicit inference context returned

Sketch7 reintroduced a lightweight `context/` package with:

- typed sections/messages/payloads,
- derived transcript shaping,
- repo context,
- workflow-state context,
- and lightweight context snapshot persistence.

This corrected an earlier over-simplification where inference payload shape had become too implicit.

### 2. Provider replay correctness improved

Provider replay was fixed so assistant tool calls and tool results preserve real tool names and `tool_call_id`s.
This materially improved tool-using local-model behavior.

### 3. Discovery quality improved

The following changes materially improved repo discovery:

- `search_files` ranking/shaping improvements
- introduction of `list_files`
- `repo_context` section with filetype and git summary

These changes significantly reduced wrong-language (`*.py`) startup behavior.

### 4. Durable CLI turn harness added

Sketch7 now supports:

- persisted session/workflow state,
- `--session-id` / `--state`,
- `--stop-token`,
- and per-turn CLI workflow experiments.

This allowed repeated cross-process workflow testing.

### 5. Persistence-on-error fixed

State is now saved even when the loop exits with an error (especially `loop guard tripped`).
This was essential for inspecting failed workflow runs.

## Key experiment threads and findings

---

## A. Baseline workflow activation experiments

### Initial finding

Passing `--workflow ...` into the CLI originally did **not** actually activate workflow behavior.

Root cause:

- workflow YAML was loaded,
- but not retrieved/bound/passed into the loop,
- so the runtime did not present workflow state to the model.

### Fix

The CLI was updated to:

- retrieve the workflow definition,
- create workflow history for fresh sessions,
- append initial binding records,
- and pass workflow state into the loop.

### Result

Workflow activation became real:

- binding records appeared,
- workflow context could be derived,
- and workflow-state projection became possible.

---

## B. Workflow ordering experiments

### Initial design

The initial workflow order assumed:

- requirements collection first,
- then repo orientation,
- then planning.

### Observation

This fit poorly for bugfix-style tasks.
In practice, repo-first exploration before clarification often made more sense.

### Revision

The workflow was revised to a flexible front half:

- `intake`
- `repo_orientation`
- `clarification`

with both orderings allowed before planning.

### Result

This was a good change.
Bugfix-style behavior became easier to interpret as valid workflow behavior rather than noncompliance.

---

## C. Workflow-state visibility experiments

### Intermediate problem

Even after workflow activation, the model still did not appear to react to workflow state.

Root cause:

- the default agent definition in `main.go` was missing `workflow_state` (and effectively `binding`) from its context projections.

### Fix

Added projections for:

- `binding`
- `workflow_state`

to the default agent definition.

### Result

Persisted session histories then showed real workflow-state context snapshots in the inference payload.
The model began referencing workflow/binding concepts more explicitly.

This was a necessary fix.

---

## D. Workflow progress projection experiments

### Change

The workflow-state section was extended to include:

- completed checkpoints,
- remaining checkpoints before planning,
- planning readiness,
- and suggested next transitions.

### Result

This made the workflow progress model visible in the payload.
However, progress projection alone did **not** solve non-convergence.

The model could see the guidance, but still often kept exploring instead of:

- summarizing,
- asking clarifying questions,
- or transitioning.

---

## E. Stronger workflow wording experiments

### Change

Workflow-state rendering was made more directive and action-oriented:

- explicit objective,
- explicit prohibition on premature planning,
- explicit remaining prerequisites,
- suggested next transitions.

### Result

This improved the **framing** of the model's first assistant turn.
The model became more workflow-aware in its narration.

But it still:

- did not converge reliably,
- and still looped in orientation.

Conclusion:

- stronger workflow wording helps,
- but wording alone is insufficient.

---

## F. Explicit-state vs guidance-first workflow variants

Two workflow variants were created:

1. `conversation_to_execution_explicit.yaml`
   - encourages direct state-machine reasoning
2. `conversation_to_execution_guidance.yaml`
   - keeps the machine more hidden and uses natural operational guidance

### Observed behavior

#### Explicit-state variant

The model became more meta:

- it explored workflow YAML itself,
- paid attention to orchestration artifacts,
- and drifted toward workflow/control-plane introspection.

This was judged a negative signal.

#### Guidance-first variant

The model still explored a lot, but stayed more task-focused:

- context sections,
- sketch7 structure,
- codebase shape,

rather than strongly drifting into workflow machinery.

### Conclusion

The guidance-first direction appears more promising than the explicit-state direction.

---

## G. Cognitive-state experiments

### Question

Is the model actually using the cognitive state machine?

### Finding

No evidence of that was found in real runs.

Persisted session histories showed:

- no `cognitive_transition` records
- no `transition_state` tool calls for cognitive state

So cognitive states currently exist in architecture and tests, but are not active in live model behavior.

### Change attempted

Generic workflow/cognitive-state operating guidance was added to the default system prompt.

### Result

This may have improved first-turn framing slightly, but it did **not** cause the model to begin using `transition_state` or shifting cognitive states.

Conclusion:

- cognitive states are still latent infrastructure,
- not active control behavior.

---

## H. Tool-surface and workflow control mismatch

One consistent finding across runs:

- workflow state may say only a narrow set of tools are enabled,
- but the model still has access to the broader cognitive/agent tool surface,
- and continues using those tools.

This implies that workflow-state tool policy is currently more advisory than authoritative.

That is likely one of the major remaining control-plane gaps.

## Summary of what worked

The following changes clearly improved sketch7 behavior:

- reintroducing explicit inference context,
- fixing provider replay,
- improving search/discovery tools,
- adding repo context,
- activating workflow binding in the CLI,
- projecting workflow state and progress,
- and adding persistence for failed runs.

These changes were all worthwhile.

## Summary of what did not work well enough

The following did **not** produce the hoped-for control effect on their own:

- raw explicit workflow-state semantics,
- stronger workflow wording by itself,
- generic cognitive/workflow boilerplate by itself,
- expectation that the model would naturally start using `transition_state`.

## Current best interpretation

The experiments suggest:

1. **Guidance-first workflow rendering is more promising than explicit machine-state rendering.**
2. **The model does not naturally become a good state-machine participant just because state is described.**
3. **Cognitive states are not yet real in practice.**
4. **Workflow/tool control likely needs either stronger runtime enforcement or a different shaping strategy.**
5. **Keeping raw workflow/cognitive machinery partially hidden and translating it into task-facing guidance is likely the better path.**

## Likely next experiment families

The next wave of experiments should probably compare a few control strategies rather than continue tweaking one renderer.

Promising directions:

### 1. Guidance-channel experiments

- synthetic user guidance message
- translated task-facing guidance section
- stronger phase-completion prompts

### 2. Readiness-check / checkpoint experiments

- ask the model whether enough information has been gathered
- ask what information is still missing
- require a short structured readiness answer before another long exploratory turn

### 3. Stronger runtime control experiments

- narrower effective tool gating by workflow/cognitive phase
- bounded orientation heuristics
- convergence detector / anti-inertia heuristics

### 4. Cognitive-state strategy decision

Decide whether cognitive states should be:

- model-driven via stronger protocol,
- runtime-driven,
- or mostly internal and hidden from the model.

## Recommended current direction

Based on the experiments so far, the most promising near-term direction is:

> keep workflows and cognitive states real internally, but render mostly task-facing operational guidance to the model rather than raw machine semantics.

At the same time, start experimenting with stronger convergence mechanisms rather than assuming prompt wording alone will create disciplined state-machine behavior.
