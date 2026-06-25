# Bounded Task Comparison and Coverage Gating Report

Date: 2026-06-03

## Scope

This report extends the earlier bounded-task experiment report with a direct comparison between:

- a plain tool-using loop,
- the original bounded hidden-task controller (`bounded`),
- and a coverage-gated bounded variant (`bounded_v2`).
- and an action-keyed hinting variant (`bounded_v2a`).
- and a hybrid hinting-plus-enforcement variant (`bounded_v3`).

The goal of this round was to answer two questions:

1. Does hidden bounded-task control improve behavior relative to a plain loop?
2. If bounded control closes too early, can simple evidence-coverage gating improve it?

## Experiment artifacts

### Saved raw data

- Bounded v1 raw JSON:
  - `docs/experiments/raw_data/2026-06-03T04-29-37Z-bounded-review-our-implementation-of-context-ofr-inferrence-in-sketc.json`
- Plain raw JSON:
  - `docs/experiments/raw_data/2026-06-03T04-37-37Z-plain-review-our-implementation-of-context-ofr-inferrence-in-sketc.json`
- Plain text log:
  - `docs/experiments/raw_data/2026-06-03T04-37-37Z-plain-review-our-implementation-of-context-ofr-inferrence-in-sketc.log`
- Bounded v2 raw JSON:
  - `docs/experiments/raw_data/2026-06-03T05-25-54Z-bounded_v2-review-our-implementation-of-context-ofr-inferrence-in-sketc.json`
- Bounded v2 text log:
  - `docs/experiments/raw_data/2026-06-03T05-25-54Z-bounded_v2-review-our-implementation-of-context-ofr-inferrence-in-sketc.log`
- Bounded v2a raw JSON:
  - `docs/experiments/raw_data/2026-06-03T05-38-06Z-bounded_v2a-review-our-implementation-of-context-ofr-inferrence-in-sketc.json`
- Bounded v2a text log:
  - `docs/experiments/raw_data/2026-06-03T05-38-06Z-bounded_v2a-review-our-implementation-of-context-ofr-inferrence-in-sketc.log`
- Bounded v3 raw JSON:
  - `docs/experiments/raw_data/2026-06-04T02-14-59Z-bounded_v3-review-our-implementation-of-context-ofr-inferrence-in-sketc.json`
- Bounded v3 text log:
  - `docs/experiments/raw_data/2026-06-04T02-14-59Z-bounded_v3-review-our-implementation-of-context-ofr-inferrence-in-sketc.log`

### Prompt used

All comparison runs used the same prompt:

> Review our implementation of context ofr inferrence in sketch/sketch7 and report how it functions. List any issues you see. Make recommendations on architectural improvements.

## Controller variants

## Plain

The plain mode is a generic tool loop:

- same tool surface,
- no hidden task decomposition,
- no structured completion artifacts,
- and only a forced final answer if the run does not stop on its own.

## Bounded (v1)

The original bounded mode hides orchestration state and uses fixed hidden tasks:

1. `understand_request`
2. `repo_inspection`
3. `readiness_check`

Progress is gated by structured JSON output for each task.

## Bounded v2

`bounded_v2` keeps the same hidden task structure but adds a simple coverage gate during `repo_inspection`.

If reporting begins without reading:

- at least one `_test.go` file, and
- at least one adjacent non-`context/` Go file,

the runtime injects a repair-like message instructing the model to gather broader evidence before reporting.

## Bounded v2a

`bounded_v2a` explores a different idea.

Instead of asking the model to proactively broaden its evidence base, the runtime augments certain tool returns with a small "possibly relevant to your current task" block.

These hints are intentionally conservative:

- local to the current action,
- limited to at most three paths,
- and only include short path-level hints such as nearby tests, same-directory implementation files, or obvious adjacent integration files.

The purpose was to test whether the model could be gently pulled toward broader evidence without explicit coverage-gating pressure.

## Bounded v3

`bounded_v3` combines the two previous ideas:

- local, conservative tool-return hints from `bounded_v2a`, and
- stronger completion pressure from `bounded_v2`.

The intent was to test whether:

- hints make nearby high-value files more visible,
- while stronger acceptance criteria prevent premature closure.

In particular, `repo_inspection` was meant to encourage both:

- at least one relevant test file, and
- at least one adjacent non-`context/` Go file.

## Metrics now captured

The experiment harness now records session-level metrics for new runs:

- model turns
- assistant messages
- tool calls
- tool calls by name
- forced reports
- repair turns
- completion status
- unique files read
- directories listed
- search patterns
- whether tests were read
- whether adjacent non-`context/` Go files were read
- completed tasks
- provider token usage (if reported by the provider)

Not all earlier runs have complete metrics, but the saved transcripts still support qualitative comparison.

## Findings

## 1. Plain vs bounded (v1)

### Bounded v1 was more disciplined

The original bounded controller consistently kept the model inside a cleaner process:

- request understanding happened explicitly,
- repo inspection stayed focused,
- and the runtime forced synthesis into a structured artifact.

This reduced drift and made the run easier to reason about.

### Plain was broader and less controlled

The plain run explored more organically. In the successful preserved run, this broader exploration actually helped in some ways:

- it read tests,
- it looked at adjacent log interfaces,
- and it produced a more narrative final answer.

However, this came at the cost of weaker control. In earlier plain behavior, the loop was also more prone to continuing exploration rather than converging.

### Main interpretation

The comparison suggests:

- bounded control helps process discipline,
- but bounded v1 may close too early,
- while plain mode sometimes benefits from following evidence farther outward.

So bounded v1 improved control, but did not clearly dominate plain mode on review quality for this task.

## 2. Bounded v2 improved one aspect of evidence gathering, but not enough

The purpose of `bounded_v2` was to test whether bounded control could be improved by requiring broader evidence before accepting a report.

### What improved

The coverage gate changed behavior.

Specifically:

- the model initially tried to report,
- the runtime rejected that as under-covered,
- and the model then read `sketch/sketch7/context/context_test.go`.

This is meaningful. It shows that action after a report attempt can be redirected by a simple runtime requirement.

### What did not improve enough

The run still failed to satisfy the full intended coverage goal.

The final metrics show:

- `read_test_files = true`
- `read_non_context_go_files = false`

So the model responded to the gate partially, but still did not inspect an adjacent non-`context/` Go file before final completion.

In other words:

- v2 produced broader evidence than v1,
- but the current coverage gate is still too soft to guarantee the desired exploration pattern.

## 3. Metrics from bounded v2

From the preserved `bounded_v2` run:

- `model_turns`: 12
- `assistant_messages`: 5
- `tool_calls`: 7
- `tool_calls_by_name`:
  - `list_files`: 3
  - `read_file`: 4
- `forced_reports`: 1
- `repair_turns`: 2
- `completed`: true
- `read_test_files`: true
- `read_non_context_go_files`: false
- `provider_usage.total_tokens`: 62596

These metrics are useful because they show the cost of the current strategy and make future comparisons more concrete.

## 5. Bounded v2a was cheaper and smoother, but did not improve evidence coverage

The `bounded_v2a` run completed successfully and cleanly.

### What improved

Compared with `bounded_v2`, the run was materially cheaper:

- `model_turns`: 9
- `tool_calls`: 6
- `repair_turns`: 0
- `provider_usage.total_tokens`: 30668

This is substantially lower than the `bounded_v2` run, which used 62596 total tokens and required repair turns.

The hinting also did not destabilize the run. The model tolerated the extra information in tool returns without obvious confusion.

### What did not improve

Despite receiving hints that surfaced:

- `context_test.go`,
- `repo_test.go`,
- and `main.go`,

the model still only read:

- `context.go`
- `sections.go`
- `repo.go`

The final metrics show:

- `read_test_files = false`
- `read_non_context_go_files = false`

So in this sample, passive tool-return hints did not materially change the exploration pattern.

### Interpretation of v2a

This suggests that action-keyed hinting is:

- cheap,
- safe,
- and non-disruptive,

but also too weak, on its own, to cause broader evidence gathering in the way needed for this review task.

That does not make the idea uninteresting. It simply means that hinting alone is not an adequate substitute for stronger exploration control when evidence coverage matters.

## 6. Bounded v3 improved pressure, but still failed to cross the key boundary

The `bounded_v3` hybrid run completed successfully and did change behavior somewhat.

### What improved

Compared with `bounded_v2a`, the hybrid did succeed in reading a relevant test file:

- `sketch/sketch7/context/context_test.go`

This indicates that stronger completion pressure still has some value when paired with hints.

### What still failed

The run still did **not** read an adjacent non-`context/` Go file.

Final metrics show:

- `read_test_files = true`
- `read_non_context_go_files = false`

So even the combined strategy did not cause the model to move outward into broader integration/interface evidence.

### Behavioral significance

This result is important because it suggests the remaining issue is not merely lack of pressure.

Even with:

- stronger completion gating,
- and local hints showing plausible next files,

the model still preferred the most local continuation path.

In practice, it returned to nearby `context/` evidence rather than stepping outward to adjacent files like `main.go`, `logs/session.go`, or `runner/loop.go`.

### Metrics from bounded v3

From the preserved `bounded_v3` run:

- `model_turns`: 12
- `assistant_messages`: 5
- `tool_calls`: 7
- `tool_calls_by_name`:
  - `list_files`: 4
  - `read_file`: 3
- `forced_reports`: 1
- `repair_turns`: 2
- `completed`: true
- `read_test_files`: true
- `read_non_context_go_files`: false
- `provider_usage.total_tokens`: 47711

This makes `bounded_v3` costlier than `bounded_v2a`, cheaper than `bounded_v2`, and still incomplete with respect to the evidence-coverage goal.

## 4. The main failure mode is becoming clearer

At this point, the most likely failure mode of bounded control is not that hidden-task orchestration is a bad idea.

The more likely issue is:

> the runtime is still asking the model to decide what additional evidence is needed, rather than making the evidence requirements structurally unavoidable.

The `bounded_v2` result supports that reading:

- the model complied partially,
- but still attempted to finish before all requested coverage was satisfied.

That means the control surface improved, but the enforcement semantics are still weak.

`bounded_v2a` sharpens this interpretation further:

- showing nearby useful files is not enough,
- the model often still prefers to synthesize from what it already has,
- and if broader coverage is important, the runtime likely needs to do more than merely surface possibilities.

`bounded_v3` narrows the conclusion even more:

- hints plus pressure can produce partial compliance,
- but the model still does not reliably infer the *specific cross-boundary file* that would satisfy broader architectural evidence needs.

That suggests the next control improvement should likely be more explicit about *which* supporting files are likely relevant, rather than merely telling the model to broaden evidence in the abstract.

## Qualitative summary by variant

## Plain

### Strengths

- more organic exploration
- more likely to wander into tests and adjacent interfaces
- often richer narrative final answer

### Weaknesses

- weaker convergence discipline
- more prone to indefinite exploration
- less structured artifact production

## Bounded v1

### Strengths

- good process regularity
- clear decomposition
- clean structured outputs
- reduced drift

### Weaknesses

- closes too early
- evidence base can be too narrow
- review quality can suffer from premature synthesis

## Bounded v2

### Strengths

- retains bounded control benefits
- demonstrates that runtime coverage nudges can influence exploration
- improves over v1 by pushing the model into at least some broader evidence gathering

### Weaknesses

- only partial compliance with broader evidence requirements
- still falls back into completion before all desired evidence is gathered
- current coverage repair is too advisory

## Bounded v2a

### Strengths

- lowest-cost bounded variant so far
- no repair-turn churn in the sampled run
- hinting is simple, interpretable, and non-disruptive

### Weaknesses

- nearby-file hints were largely ignored
- did not increase evidence breadth in the sampled run
- too passive to drive broader inspection behavior by itself

## Bounded v3

### Strengths

- combines hinting with stronger completion pressure
- does improve evidence gathering relative to v2a by at least pulling in a relevant test file
- remains operationally stable

### Weaknesses

- still fails the adjacent non-`context/` Go coverage goal
- still relies on the model to infer *which* broader file matters most
- incurs additional cost without fully solving the cross-boundary evidence problem

## Current conclusions

At this stage, the most defensible conclusions are:

1. **Hidden bounded-task control remains promising as a control strategy.**
2. **Plain looping may still outperform bounded-v1 on some review-style tasks when broader evidence gathering matters.**
3. **Coverage gating improves bounded behavior, but the current v2 implementation is too soft to guarantee sufficient exploration.**
4. **Action-keyed hinting is cheap and safe, but not strong enough by itself to improve evidence coverage.**
5. **Hinting plus stronger pressure improves behavior somewhat, but still does not reliably make the model cross into adjacent integration evidence.**
6. **The core open problem is now narrower: how explicit should the runtime be about the next supporting files to inspect?**

## Plausible next experiment directions

### 1. Bounded v3: explicit checklist enforcement

Make repo-inspection completion impossible until a simple checklist is satisfied, for example:

- read at least one implementation file in the target area,
- read at least one relevant test file,
- read at least one adjacent non-target interface/implementation file.

If the checklist is incomplete:

- do not accept final report,
- do not immediately convert to JSON repair,
- continue the inspection task until either the checklist is satisfied or a hard cap is reached.

This would be the direct continuation of the bounded-control line.

### 2. Action-keyed relevance augmentation (proposed v2a)

A different direction is to stop asking the model to proactively discover what else it should inspect and instead provide additional context in response to its actions.

For example:

- after a `read_file` or `list_files` call,
- the runtime could attach a short "possibly relevant to your current task" block,
- pointing to nearby tests, adjacent interfaces, or files related to recent search terms or prompt concepts.

This would move some of the discovery burden from the model into the runtime and may be especially effective for local models.

This idea is attractive because it still keeps orchestration hidden while making the tool surface more supportive.

That experiment has now been run once in a conservative form. The initial result suggests that pure hinting is probably too passive to be the main mechanism for improving evidence coverage.

So future action-keyed augmentation should likely be treated as a supporting mechanism, not the whole solution.

### 3. Hybrid next step: hinting plus stronger acceptance criteria

The most plausible follow-on is a hybrid:

- keep v2a-style compact local hints,
- but pair them with stronger completion acceptance criteria.

This would test whether hints help once the runtime also refuses premature closure.

That hybrid has now been tested once in `bounded_v3`. The initial result suggests that even this is not sufficient when the model must still infer the specific adjacent file to inspect.

### 4. Explicit targeted next-file suggestions

The most plausible next experiment is now a more explicit variant of action-keyed assistance.

Instead of saying only:

- broaden evidence,
- or possibly relevant nearby files,

the runtime could say something like:

- "To strengthen evidence beyond `context/`, inspect one of: `sketch/sketch7/main.go`, `sketch/sketch7/logs/session.go`, `sketch/sketch7/runner/loop.go`."

This would test whether the actual missing ingredient is not pressure or hinting in general, but explicit cross-boundary targeting.

### 5. Stronger evaluation rubric

Future rounds should add a manual scoring rubric for:

- task adherence
- relevance of exploration
- evidence grounding
- completeness
- convergence discipline
- overclaim rate

The current metrics improve observability, but they do not yet fully capture answer quality.

## Recommended near-term direction

The next most interesting test is likely not additional pure hinting, because `bounded_v2a` appears too passive on its own, and `bounded_v3` still leaves the model to infer too much.

Instead, the next likely-best direction is a **hybrid**:

> pair compact action-keyed hints with stronger acceptance criteria so the model both sees likely next evidence and cannot prematurely report without satisfying broader coverage requirements.

That statement was directionally right, but the first hybrid suggests it still needs one more refinement: the runtime may need to identify the likely next supporting files more concretely.

Why this is compelling now:

- bounded v2 showed that the model only partially complies with advisory evidence requirements,
- bounded v2a showed that passive hints alone are not enough,
- bounded v3 showed that hints plus stronger pressure still do not reliably cross the adjacent-file boundary,
- and the plain run suggests that broader evidence often matters.

So the next experiment should likely test whether **explicit targeted adjacent-file suggestions** work better than advisory coverage-gating, passive hinting, or hinting-plus-pressure alone.
