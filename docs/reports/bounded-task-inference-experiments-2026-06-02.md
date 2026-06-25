# Bounded Task Inference Experiments Report

Date: 2026-06-02

## Scope

This report summarizes an initial experiment series using a deliberately naive Python harness to test a different control strategy from sketch7's explicit workflow/cognitive-state rendering.

The question was not whether to design a new architecture, but whether a much simpler runtime could validate the following hypothesis:

> model behavior improves when orchestration state is kept internal and the model only sees the current bounded subtask, a limited tool surface, and a concrete completion contract.

## Experiment setup

A small script was added at:

- `docs/experiments/bounded_task_runner.py`

The script was intentionally simple:

- one file,
- linear control flow,
- OpenAI-compatible chat completion calls,
- minimal tool support,
- no reusable architecture,
- no visible workflow/cognitive state,
- and no persistence.

### Internal bounded tasks

The runtime used a fixed sequence of hidden tasks:

1. `understand_request`
2. `repo_inspection`
3. `readiness_check`

The model was not told about workflow machinery or state transitions. It only saw:

- the current task objective,
- prior completed task artifacts,
- a small allowed tool set for the current task,
- and a required JSON output shape for completion.

### Tool surface

The script exposed only a tiny read-only tool set:

- `list_files`
- `search_files`
- `read_file`

### Initial runtime policy

The first version used:

- a maximum of 6 turns per task,
- free tool use within those turns,
- and hard failure if the model had not emitted valid completion JSON by the time the turn budget ended.

### Revised runtime policy

After the first run, the script was revised to add:

- a visible turn marker ("you have N turns before you must report"),
- a forced final reporting prompt after the tool budget is exhausted,
- stronger instructions that tool use is over,
- and one repair turn if the model fails to return valid JSON.

## Prompt used for the main run

The evaluation prompt was:

> Review our implementation of context ofr inferrence in sketch/sketch7 and report how it functions. List any issues you see. Make recommendations on architectural improvements.

## Observed behavior

## A. Initial run before forced reporting improvements

### What worked

The model handled the first bounded task cleanly:

- `understand_request` completed in one turn,
- it correctly determined that repository inspection was needed,
- and it produced valid JSON immediately.

During `repo_inspection`, the model behaved in a focused and plausible way:

- it navigated to `sketch/sketch7`,
- then `sketch/sketch7/context`,
- then read relevant source files such as `context.go` and `sections.go`.

This was a positive sign because the model:

- did not drift into control-plane narration,
- did not become meta about hidden workflow machinery,
- and did not thrash through obviously irrelevant parts of the repository.

### What failed

The run failed because the model used all 6 turns for exploration and never emitted completion JSON before the task budget ended.

This indicated that:

- bounded subtasks alone are not sufficient,
- the runtime also needs an explicit synthesis boundary,
- and a plain hard cutoff produces failure even when the inspection behavior is otherwise reasonable.

## B. Run with turn markers and forced reporting

The revised harness performed materially better.

### Behavior during repo inspection

The model again chose a focused inspection path:

- `sketch/sketch7/context/context.go`
- `sketch/sketch7/context/sections.go`
- `sketch/sketch7/context/repo.go`
- `sketch/sketch7/context/context_test.go`

This remained coherent and task-relevant.

### First forced reporting attempt

After the tool budget ended, the runtime instructed the model that:

- tool use was over,
- no more repository inspection was possible,
- it must now report from gathered evidence only,
- and the response had to be JSON matching the required schema.

With this stronger boundary, the model produced a substantive report-like JSON structure summarizing:

- how context payload assembly works,
- how sections are derived,
- how repo context is generated,
- and several candidate issues and recommendations.

However, the first forced-report response still required a repair turn before it satisfied the script's simple schema validation.

### Repair turn

A single repair instruction was enough.

On the repair turn, the model returned valid JSON and the task completed successfully.

The final `readiness_check` task then completed immediately and correctly judged that enough information had been gathered to proceed.

## Main findings

## 1. Hidden orchestration plus bounded subtasks appears promising

The strongest positive signal from these runs is that the model behaved reasonably well without being shown workflow/cognitive-state machinery.

Instead, it was sufficient to provide:

- the current bounded task,
- a narrow tool surface,
- a visible turn budget,
- and an output contract.

This is important because it suggests that the model does not need to be a strong explicit state-machine participant in order to behave coherently.

## 2. The model still needs a forced synthesis boundary

Even with bounded subtasks, the model tends toward one-more-read inertia.

Without a forced reporting phase, it will often keep inspecting until budget exhaustion. The naive hard-fail behavior from the first version demonstrated this clearly.

The revised run suggests that a good bounded-task runtime likely needs:

- a discovery budget,
- a forced report phase,
- and at least one repair opportunity.

## 3. Completion repair is cheap and effective

The first forced report was already substantively useful, but not valid enough for the harness. One repair turn was enough to obtain a usable artifact.

That suggests a practical pattern:

- let the model explore,
- force synthesis,
- validate,
- then repair once if necessary.

This appears more robust than expecting a perfect first completion.

## 4. Control success and content completeness are separate questions

Although the bounded-task harness worked reasonably well as a control mechanism, the actual analytical content from the model was only partially complete.

For example:

- it focused only on `context/` files,
- it inferred some unknowns that are actually answerable elsewhere in sketch7,
- and some claims were only partially grounded because it had not inspected the broader call chain.

This does not undermine the control result. It simply means that:

- the runtime shape may be working,
- while task decomposition and evidence-gathering policy still need refinement.

## 5. The script exposed inertia in a cleaner way than sketch7 did

One of the valuable side effects of the naive harness is that it made model inertia easier to interpret.

In sketch7, non-convergence can be confounded by:

- explicit workflow wording,
- visible cognitive state,
- broader tool surfaces,
- and richer runtime machinery.

In the naive harness, the same underlying tendency showed up much more plainly:

- the model wanted one more read,
- and needed a stronger boundary to switch from exploration to reporting.

That is a useful simplification.

## Current interpretation

The current best interpretation is:

1. **Keeping orchestration internal and exposing only the current bounded task is a promising control strategy.**
2. **A visible budget and explicit final reporting phase are important.**
3. **A single repair turn is likely worth keeping as part of the normal completion protocol.**
4. **This strategy appears more promising than expecting the model to naturally manage explicit workflow/cognitive state.**
5. **The remaining problem is less about visible state and more about exploration-to-synthesis handoff.**

## Limitations of this experiment

This was only an early probe and has important limits:

- only one main prompt was evaluated,
- the tool surface was tiny and read-only,
- the task sequence was fixed and hand-written,
- there was no comparison baseline script in the same style,
- and the quality judgment was qualitative rather than benchmarked.

So the result should be treated as directional, not conclusive.

## Plausible next experiments

The next experiments should continue to preserve the script's simplicity while testing a few sharper questions.

### 1. Baseline comparison run

Create a sibling script or mode with the same tools and same model but **without bounded internal tasks**.

Compare:

- total tool calls,
- relevance of files inspected,
- whether the model stops on its own,
- quality of final report,
- and amount of meta drift.

This would test whether bounded-task framing is actually buying something relative to a plain tool loop.

### 2. Separate tool budget from report budget

Instead of a single 6-turn cap, try:

- up to N tool turns,
- then 1 forced report turn,
- then 1 repair turn if needed.

This would make the protocol cleaner and probably closer to the real intended design.

### 3. Stronger citation requirements

Require every finding to cite evidence in `file:lineno` form.

Then check whether that:

- improves factual grounding,
- reduces unsupported architectural claims,
- or causes the model to use turns more carefully.

### 4. Vary task granularity

Compare a few decompositions:

- `understand_request -> repo_inspection -> readiness_check`
- `understand_request -> locate_relevant_files -> inspect_context_implementation -> readiness_check`
- `understand_request -> inspect_payload_assembly -> inspect_repo_context -> readiness_check`

This would help identify whether the current task units are too coarse or about right.

### 5. Add a lightweight evidence register

Without changing the script much, require the model during the forced report phase to produce:

- findings,
- unknowns,
- and explicit evidence references.

This may help separate:

- things the model actually observed,
- from things it inferred loosely.

### 6. Test with implementation-oriented prompts

So far the experiment focused on review/orientation behavior. A next step would be to test:

- bugfix-style prompts,
- implementation planning prompts,
- and validation prompts.

This would show whether bounded hidden-state control also helps beyond analysis/review.

### 7. Try a no-turn-marker variant

The turn marker appears helpful, but it should be tested directly.

Compare:

- hidden bounded tasks with turn markers,
- hidden bounded tasks without turn markers,
- and hidden bounded tasks with only a final forced report.

This would isolate how much value the visible countdown itself adds.

### 8. Add a minimal write/validate phase later

Only after read-only experiments are better understood, introduce a tiny write-capable phase with:

- one edit tool,
- one validation tool,
- and the same completion-gated protocol.

That would test whether the bounded-task pattern also works for implementation, not just repo understanding.

## Recommended near-term direction

The most useful immediate next step is probably:

> keep iterating in the naive Python harness long enough to compare bounded-task control against a plain loop baseline and to tune the exploration-to-report boundary.

It still seems premature to churn sketch7 architecture further before that comparison is clearer.

At this point the naive harness is doing what it should do:

- it isolates the control idea,
- it exposes failure modes cleanly,
- and it gives a simpler environment for deciding whether hidden-state bounded-task orchestration is worth carrying back into the larger system.
