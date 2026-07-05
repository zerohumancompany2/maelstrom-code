# Sketch7 Cognitive-State Stress Test Plan

**Status:** Preliminary  
**Date:** 2026-06-24  
**Scope:** Stress test only the cognitive-state portion of sketch7 using a local OpenAI-compatible model endpoint configured by `.env.test`.

## Short answer

We are ready for controlled, read-only cognitive-state stress tests.

We are not ready to let agents freely mutate this repo. The runtime is close, but write-enabled autonomous tests should wait for provider tool narrowing, empty-policy tool rejection, `maxToolCalls` enforcement, and a disposable worktree harness.

## What we are testing

The stress test should answer these questions:

1. Does hidden cognitive state framing improve useful task behavior compared to a flat agent prompt?
2. Do bounded states produce valid finalization JSON under pressure?
3. Do different cognitive patterns produce different failure modes?
4. Does context filling stay compact enough to leave room for repo evidence?
5. Does the local model respect tool gates and no-tools finalization?
6. Can we measure state lifecycle, retries, and output validity from durable records without manual inspection?

## Assumptions

- `.env.test` exists and defines an OpenAI-compatible base URL such as `OPENAI_API_BASE`.
- The local model can support chat completions and, eventually, tool-call syntax compatible with the sketch7 provider boundary.
- Initial stress tests should be run against read-only tools or fake tools.
- Any write-capable test must run in a throwaway copy or worktree.

Do not print secrets from `.env.test` in test output.

## Research notes: reasoning prompts and efficient context filling

The useful lesson is not "make the model reveal a huge chain of thought." The useful lesson is "give the model a compact cognitive scaffold, make it externalize only decision-relevant artifacts, and validate the artifact."

Relevant findings:

- **Chain-of-thought prompting** can improve complex reasoning in large models by prompting intermediate reasoning steps, but it costs tokens and can become verbose. Source: Wei et al., "Chain-of-Thought Prompting Elicits Reasoning in Large Language Models" (2022), https://arxiv.org/abs/2201.11903.
- **Self-consistency** improves chain-of-thought by sampling multiple reasoning paths and selecting the consistent answer, but it multiplies inference cost. Use sparingly for hard decisions or evaluator runs, not every state turn. Source: Wang et al., "Self-Consistency Improves Chain of Thought Reasoning in Language Models" (2022), https://arxiv.org/abs/2203.11171.
- **Least-to-most prompting** decomposes complex tasks into smaller subproblems and solves them in sequence. This is useful research background for workflow design and task decomposition, but it should not be copied directly into cognitive states. Source: Zhou et al., "Least-to-Most Prompting Enables Complex Reasoning in Large Language Models" (2022), https://arxiv.org/abs/2205.10625.
- **ReAct** interleaves reasoning and external actions. This is useful background for tool-use policy inside an **Act** or **Seek evidence** mode, but a ReAct loop is closer to workflow/control structure than a pure thought-mode vocabulary. Source: Yao et al., "ReAct: Synergizing Reasoning and Acting in Language Models" (2022), https://arxiv.org/abs/2210.03629.
- **Tree of Thoughts** shows value in exploring multiple candidate paths and self-evaluating them for planning/search tasks. For us, this should inform a bounded deliberation mode, not become the default statechart. Source: Yao et al., "Tree of Thoughts" (2023), https://arxiv.org/abs/2305.10601.
- **Reflexion** stores compact verbal feedback in an episodic memory buffer to improve later trials. This maps to durable session records and short reflection artifacts, not full hidden reasoning dumps. Source: Shinn et al., "Reflexion" (2023), https://arxiv.org/abs/2303.11366.
- **Self-Refine** uses iterative feedback and refinement at test time. This supports critique/calibration modes, but we should cap iterations and validate outputs to avoid infinite polishing. Source: Madaan et al., "Self-Refine" (2023), https://arxiv.org/abs/2303.17651.
- **Lost in the Middle** shows models often use information at the beginning/end of long context better than information in the middle. This argues for compact state-task chunks, recency-weighted summaries, and putting the exact output contract near the final response request. Source: Liu et al., "Lost in the Middle" (2023), https://arxiv.org/abs/2307.03172.
- **Evaluator alignment is iterative.** Evaluation criteria can drift as people inspect outputs. Start with code-checkable metrics, then add human grading only after output examples exist. Source: Shankar et al., "Who Validates the Validators?" (2024), https://arxiv.org/abs/2404.12272.

## Practical context-filling rules

For cognitive-state agents, prefer:

1. **Short state prompt.** One local task, one decision policy, one output contract.
2. **Compact evidence ledger.** Store file paths, symbols, commands, and conclusions; avoid pasted bulk unless necessary.
3. **Bounded scratch output.** Ask for `rationale_summary`, `evidence`, `decision`, and `risks`, not free-form hidden reasoning.
4. **Freshness over volume.** Keep current task, latest evidence, and output schema near the end of the assembled context.
5. **State-local summaries.** On state exit, persist a compact summary artifact that the next state can consume.
6. **No raw statechart semantics.** The model sees the task frame, not transition graph mechanics.
7. **Escalate deliberation only when useful.** Use ToT/self-consistency for ambiguous design choices, not simple edits or lookups.

## Wikipedia review: OODA and adjacent loops

The OODA loop is the closest fit among the reviewed Wikipedia links because its useful core is a compact cycle of cognitive posture: observe the situation, orient against context and constraints, decide the next commitment, then act and feed the result back into observation. For agent design, this should not mean encoding a domain workflow or a list of precise action types. It should mean selecting a bounded **mode of thought** for the next inference/tool window.

Useful takeaways from the OODA page:

- **Continuous feedback:** action results become later observations; state-exit artifacts should be compact enough to feed the next state.
- **Late commitment:** the agent should avoid committing before it has enough workspace/user/tool context, especially in ambiguous repo tasks.
- **Agility over ritual:** OODA is useful only if the modes change behavior; otherwise it becomes a vague productivity label.
- **Vagueness risk:** because OODA can be stretched to mean almost anything, each mode needs a local output contract and stress signal.

The See also links mostly reinforce what not to encode as core cognitive states:

- **Cybernetics / feedback loop:** useful background for recording feedback, state lifecycle, and outcome-to-next-state summaries. These are runtime/evaluation concerns more than state names.
- **Decision cycle / learning cycle:** broad framing for repeated decisions and learning from outcomes, but too generic to be a state chart by itself.
- **Double-loop learning:** useful for reflection runs where the agent revises assumptions, goals, or decision rules after failures; not a default per-turn loop.
- **Situation awareness:** a strong alternate cognitive chart because it separates perception, comprehension, and projection before response.
- **Mental model:** useful inside Orient/Comprehend modes: the agent should maintain a provisional model of the repo/task/tool environment and update it from evidence.
- **Problem solving:** names the general task class, but is too broad to define cognitive states without becoming a workflow.
- **PDCA, DMAIC, SWOT, nursing process, SIPDE/driver education:** mostly domain/process frameworks. They may inspire domain-specific modes or negative controls, but they should not become the baseline cognitive vocabulary.
- **Maneuver warfare:** contributes the idea of agility and disrupting stale assumptions, but adversarial framing should stay out of ordinary repo-assistant cognition unless the task is explicitly adversarial.
- **United States Army Strategist:** role/training context, not a useful cognitive-state model.

Design rule after this review: a cognitive state should answer, "what kind of thinking should govern this bounded window?" not "where are we in a domain procedure?" Domain-specific modes are allowed only when they change the agent's reasoning discipline and can still run against the same task deck as the general modes.

## Proposed cognitive mode patterns to test

Correction: the stress test should not encode task workflows as cognitive states. A workflow says "where are we in the job?" A cognitive state says "what mode of thought should govern the next bounded inference/tool window?" In particular, **Act** should not mean the system has encoded a taxonomy of user-visible actions. It means the agent is in an execution/finalization posture after observation, orientation, and decision have selected the smallest sufficient move.

The same workflow task should be runnable under different cognitive charts. Example: "inspect the repo and propose wrapper JSON validation tests" is the workflow/task. OODA is one possible cognitive chart for thinking through that task.

The OODA page is useful mostly as a reminder that the loop is about continuous feedback, late commitment, and agility, not a rigid task procedure. It is also vague enough to become anything if we are sloppy. So the test should treat OODA-like charts as **thought-mode contracts**, not named productivity rituals or domain-specific workflows.

Each pattern below should become one or more test agents with hidden statechart mechanics and task-facing prompts. The model should not see state names as control-plane labels.

### 1. OODA as thought-mode baseline

Cognitive modes:

- **Observe:** reason about what came before, the current workspace/repo state, visible evidence, user request, and missing facts. For user requests, formalize the request and distinguish explicit asks from inferred asks before solving.
- **Orient:** interpret observations against constraints, available tools/tool gates, repo conventions, risk, uncertainty, and what additional information is needed. This is where the agent forms situational awareness and a provisional mental model of the task environment.
- **Decide:** choose a concrete next plan or final answer strategy. Commit to the smallest sufficient next move, with criteria for success/failure and a stop condition.
- **Act:** execute the chosen move if tools are allowed, or produce the bounded final artifact if finalizing. This mode should follow the decision rather than reopen broad deliberation.

Best for:

- general repo tasks
- ambiguous user requests
- task framing before implementation
- read-only investigation before write access

Stress signals:

- Does Observe formalize the user request instead of jumping to solution?
- Does Orient identify available tools and missing information without over-planning?
- Does Decide produce a small, executable plan with a stop condition?
- Does Act follow the selected plan rather than inventing a new one or encoding unrelated action categories?

### 2. Situation-awareness modes

Inspired by situation awareness rather than a task workflow.

Cognitive modes:

- **Perceive:** identify relevant facts in the environment: files, current transcript, constraints, visible failures, tool affordances.
- **Comprehend:** explain what those facts mean for the user's request and current repo state.
- **Project:** anticipate likely consequences, next evidence needs, risk of action, and what could go wrong if the agent proceeds.
- **Respond:** take the minimal response/action consistent with the projection.

Best for:

- complex repo orientation
- unfamiliar code surfaces
- deciding whether enough context exists to proceed

Stress signals:

- Does the model separate observed fact from interpretation?
- Does it forecast risks before acting?
- Does it avoid filling context with irrelevant file dumps?

### 3. Hypothesis-calibration modes

This is not a debugging workflow. It is a thinking discipline for uncertainty.

Cognitive modes:

- **Frame hypothesis:** state the current best explanation/approach and why it might be wrong.
- **Seek evidence:** identify the most informative evidence that would confirm or disconfirm it.
- **Update belief:** revise confidence based on evidence; preserve alternatives if evidence is weak.
- **Commit or continue:** decide whether to act, gather more evidence, or finalize with uncertainty.

Best for:

- bugs and flaky behavior
- architecture questions with competing explanations
- local-model hallucination control

Stress signals:

- Does it seek disconfirming evidence?
- Does confidence change when evidence changes?
- Does it avoid endless uncertainty loops?

### 4. Information-foraging modes

Useful for codebase navigation and context efficiency.

Cognitive modes:

- **Set information need:** define the exact unknown that blocks progress.
- **Follow scent:** choose the most promising file/symbol/search path.
- **Harvest evidence:** extract only the relevant fact, path, symbol, or snippet.
- **Compress context:** summarize evidence into a small ledger and drop irrelevant detail.

Best for:

- repo exploration
- symbol tracing
- reducing context bloat

Stress signals:

- Does each tool call have a narrow information need?
- Does evidence get compressed instead of pasted wholesale?
- Does it stop foraging when enough evidence exists?

### 5. Context-quality modes

This chart is specific to LLM agents. Its purpose is to improve inference by managing information quality: separating knowns from assumptions, finding high-value evidence, rejecting false positives, compressing useful context, and deciding when enough evidence exists to act.

Cognitive modes:

- **Inventory:** separate known facts, assumptions, unknowns, stale context, and blocking unknowns.
- **Target:** define the narrow information need, the decision it supports, expected evidence shape, and likely false positives.
- **Triage:** classify retrieved evidence by decision value: direct, corroborating, contextual, duplicate, stale, or distractor.
- **Compress:** retain only high-protein facts with traceable file/symbol/tool references; drop irrelevant detail.
- **Assess readiness:** decide whether to act, gather more evidence, finalize with uncertainty, or stop as blocked.

Best for:

- ambiguous repo tasks
- long-context sessions
- local-model hallucination control
- reducing noisy retrieval
- deciding when enough evidence exists to proceed

Stress signals:

- Does the agent name the decision that evidence is supposed to support?
- Does it distinguish facts, assumptions, unknowns, and stale context?
- Does it predict and reject false-positive evidence?
- Does it classify evidence by decision value instead of keyword similarity?
- Does it compress evidence into a compact, traceable ledger?
- Does it stop gathering context once uncertainty is acceptable?

### 6. Deliberation modes

Use when a task requires choice among alternatives, not just evidence gathering.

Cognitive modes:

- **Generate options:** produce a small set of plausible approaches.
- **Apply criteria:** evaluate options against user constraints, repo constraints, risk, and cost.
- **Select:** choose one option and explain the tradeoff compactly.
- **Prepare action:** translate the selected option into the next minimal step or final answer.

Best for:

- API shape decisions
- implementation order
- architecture tradeoffs

Stress signals:

- Does it cap options instead of brainstorming forever?
- Are criteria stable across the evaluation?
- Does it make an actual choice?

### 7. Critique/calibration modes

This is a self-checking mode chart, not a document-production workflow.

Cognitive modes:

- **State answer:** produce the current answer/plan/artifact candidate.
- **Check contract:** compare it against the user request, state output schema, available evidence, and constraints.
- **Find defects:** identify missing evidence, invalid assumptions, ambiguity, or overreach.
- **Repair or finalize:** make the smallest correction or finalize if the contract is satisfied.

Best for:

- final answer quality
- planning docs
- schema proposals
- reducing confident nonsense from small/local models

Stress signals:

- Does critique find concrete defects?
- Does repair address only those defects?
- Does it avoid performative self-praise?

### 8. Reflection/learning modes

Use across trials, not necessarily inside every task.

Cognitive modes:

- **Recall prior lesson:** retrieve compact lessons from earlier failures/successes.
- **Apply lesson:** adapt current behavior based on the relevant lesson.
- **Observe outcome:** compare expected vs actual result.
- **Store lesson:** write a short future-useful lesson, not a full transcript.

Best for:

- repeated local-model runs
- comparing cognitive charts
- reducing recurring failure patterns

Stress signals:

- Are lessons short and reusable?
- Does the model apply lessons instead of merely repeating them?
- Does the lesson buffer stay small?

## Initial test-agent matrix

Start with OODA and a few mode-chart variants, not many workflow-shaped agents:

| Agent | Cognitive mode chart | Tools | Primary metric |
|---|---|---|---|
| `ooda_core_reader` | OODA baseline | read/search/list only | request formalization + evidence-grounded finalization |
| `situation_awareness_reader` | Perceive/comprehend/project/respond | read/search/list only | fact/interpretation separation |
| `hypothesis_calibrator` | Hypothesis/evidence/update/commit | read/search/list only | belief updates from evidence |
| `foraging_reader` | Need/scent/harvest/compress | read/search/list only | context efficiency |
| `context_quality_reader` | Inventory/target/triage/compress/readiness | read/search/list only | high-protein evidence retrieval + false-positive rejection |
| `critique_calibrator` | Answer/check/defect/repair | no tools or read-only | contract compliance and defect repair |

Run the same task deck through each cognitive chart. Do not give each chart a different workflow; that would confound the experiment.

Add deliberation and reflection charts after the harness can compare runs cleanly.

## Task deck

Use real outstanding repo tasks, but make the first deck read-only or planning-only:

1. Inspect `sketch7` and explain the remaining workflow-finalization work.
2. Propose tests for wrapper JSON bucket validation.
3. Find where tool policy is enforced and identify edge cases.
4. Design a small schema for state-local output optional fields.
5. Inspect current session reducers and propose lifecycle metrics.
6. Given a failing synthetic transcript, explain which state bound should fire.
7. Compare two implementation orders for core completion plan workstreams.

Later write-enabled deck, only in disposable worktree:

1. Fix empty tool-policy intersection.
2. Extract output parser from `runner/loop.go`.
3. Add optional output fields to contracts.
4. Add one lifecycle reducer metric.
5. Add provider tool narrowing tests.

## Harness shape

### Phase 0: dry provider / fake model

- Feed deterministic fake model responses through each cognitive chart.
- Assert lifecycle records, finalization records, retries, and tool validation.
- No local model needed.

### Phase 1: local model, no tools

- Use `.env.test` for OpenAI-compatible endpoint.
- Ask each agent to solve short planning tasks without tools.
- Measure JSON validity and context length.

### Phase 2: local model, read-only tools

- Enable file listing, file reading, and content search only.
- Run the read-only task deck.
- Forbid write tools and command execution.

### Phase 3: local model, safe command tools

- Allow only safe test/status commands from a fixed allowlist.
- Record command proposals separately from command execution.
- Keep repo mutation disabled.

### Phase 4: disposable write-enabled worktree

- Create a temporary copy or git worktree.
- Enable patch/write tools only inside the disposable root.
- Require tests after mutation.
- Destroy or archive worktree after run.

## Metrics

Minimum metrics per run:

- completion status and stop reason
- output parse status
- output validation status
- missing fields
- finalization retry count
- state enter/exit pairing
- inference turns per state
- tool calls proposed/executed/rejected
- `tool_not_enabled` count
- context snapshot byte size by section
- final answer usefulness grade

Suggested derived scores:

- **Contract compliance:** valid JSON finalization / total finalizations
- **Tool discipline:** valid tool calls / proposed tool calls
- **Evidence efficiency:** cited useful evidence / tool calls
- **Bound discipline:** states exiting by expected reason / bound-hit states
- **Context efficiency:** useful evidence tokens / total context tokens, approximated by byte counts until tokenization exists

## Evaluation stance

Do not trust one local-model run. Use repeated seeds if supported or repeated runs if not.

A useful first benchmark is not "did the agent solve everything?" It is:

- Did it stay inside the state-local contract?
- Did it provide inspectable evidence?
- Did it stop when bounded?
- Did it fail in ways the runtime can classify?

## Safety and sanity checks

Before any repo-mutating test:

- run in disposable worktree
- verify git status before and after
- tool policy edge case fixed
- provider tool definitions narrowed
- command tool allowlist enforced
- max tool calls enforced
- test timeout enforced outside the model loop
- no secrets printed from `.env.test`

## Recommendation

Start with Phase 0 and Phase 1 immediately. They exercise the cognitive-state runtime without risking repo damage.

Then do Phase 2 read-only with three agents and five tasks. Inspect the logs manually once, then add reducers for whatever you had to inspect by hand.

Do not run Phase 4 until the core completion plan's first two workstreams are done. The current runtime is good enough for cognitive-state research; it is not yet hardened enough for unsupervised write access.
