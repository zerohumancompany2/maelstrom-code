# Autoresearch Tier 1 Review Report

Date: 2026-06-04

## Scope

This report reviews the current findings in `docs/experiments/autoresearch/` after the first live-loop autoresearch runs against the Tier 1 benchmark suite.

The goal of this review is to answer four questions:

1. What is the current autoresearch framework actually optimizing?
2. What do the recorded Tier 1 results suggest improved over time?
3. What failure modes and methodological weaknesses are most important right now?
4. What should the next experiment round prioritize?

## Artifacts reviewed

Primary artifacts reviewed for this report:

- framework and benchmark design:
  - `docs/experiments/autoresearch/README.md`
  - `docs/experiments/autoresearch/variants.yaml`
  - `docs/experiments/autoresearch/score_benchmarks.py`
  - `docs/experiments/autoresearch/autoresearch_loop.py`
- operational state and ledger:
  - `docs/experiments/autoresearch/autoresearch_results.tsv`
  - `docs/experiments/autoresearch/manager_state.json`
- representative result bundles and logs under:
  - `docs/experiments/autoresearch/results/`
  - `docs/experiments/autoresearch/live_loop_logs/`
  - `docs/experiments/autoresearch/manager_logs/`

## What the framework is trying to optimize

The current autoresearch setup is a prompt/control-surface optimization loop for bounded-task repo analysis.

It is not training model weights and it is not mutating arbitrary implementation code. Instead, it is trying to improve controller behavior by changing a narrow set of inputs to the bounded-task runner, especially:

- `system_prompt_append`
- `coverage_repair_message_template`

The target task is fixed:

> Review our implementation of context ofr inferrence in sketch/sketch7 and report how it functions. List any issues you see. Make recommendations on architectural improvements.

The Tier 1 benchmark suite then scores the resulting run against a small set of operational capabilities, including:

- required-file discovery
- test-file discovery
- adjacent integration discovery
- JSON/schema validity
- forced-report obedience
- repair-turn obedience

In other words, the current loop is best understood as a supervised search over prompt and runtime guidance that attempts to improve bounded analysis behavior on a fixed inspection-and-reporting task.

## Strengths of the current setup

### 1. The benchmark design philosophy is strong

The `README.md` in this directory is thoughtful and disciplined. It explicitly pushes the work away from vague judgments like "better behavior" and toward testable capability slices with measurable observables.

Important strengths include:

- preference for binary and count-based metrics
- clear distinction between end-to-end and isolated micro-benchmarks
- explicit capability taxonomy
- emphasis on evidence-grounded evaluation rather than purely qualitative review

This is a strong foundation and worth preserving.

### 2. The mutation surface is intentionally narrow

The current loop restricts automatic experimentation to prompt/control overrides rather than broad code mutation. That is a good choice for this stage because it:

- keeps the search space interpretable
- reduces accidental architectural churn
- makes keep/discard decisions easier to audit

### 3. The experiment record is inspectable

The append-only ledger in `autoresearch_results.tsv` and the saved result bundles provide a useful paper trail. Even with noisy results, the current setup makes it possible to reason about trajectory, failures, and runner behavior after the fact.

## Tier 1 results observed so far

From the current ledger, there are 11 scored rows and 4 failed rows.

Scored overall pass rates are:

- `0.50`
- `0.50`
- `0.67`
- `0.33`
- `0.50`
- `0.67`
- `0.50`
- `0.33`
- `0.50`
- `0.83`
- `0.67`

### High-level interpretation

The trend is noisy but mildly positive.

- early runs cluster mostly around `0.33` to `0.50`
- later runs reach `0.67` repeatedly
- one later run reaches `0.83`

This is enough to say that the setup has produced some real movement, but not enough to say that the loop is converged or that improvements are robust.

### What appears to have improved

#### Required-file discovery improved

Early runs often missed some required files. Later runs more often achieved full required-file recall, which suggests the prompt/control variants are having some effect on evidence gathering.

#### Schema validity improved

Earlier runs included schema-validity misses. Later runs more often produced the required JSON-only output cleanly and on the first try.

#### Adjacent-file discovery improved somewhat, but remains unstable

Some later runs expanded beyond the narrow `context/` package into adjacent integration evidence such as `main.go`, which is a positive change. However, this behavior is still inconsistent and appears to regress across runs.

#### Repair compliance appears relatively strong

When the runtime pushes the model into a repair turn, the model often complies successfully. This suggests the repair path may already be stronger than the initial forced-report boundary.

## Major failure modes

### 1. Forced-report boundary obedience remains the clearest persistent weakness

The strongest later runs still show a recurring pattern:

1. the runtime imposes a reporting boundary
2. the model makes one more tool call anyway
3. the model then repairs and reports correctly

This means the system is better at recovering from a boundary violation than obeying the boundary immediately.

That is a useful result. It identifies a concrete bottleneck for the next round: first-pass post-boundary obedience.

### 2. Adjacent evidence gathering is still not reliable

The core `context/` package is usually found and inspected, but adjacent integration evidence remains inconsistent. The model sometimes broadens correctly and sometimes stays too local.

Because the task asks for functional explanation, issues, and architectural recommendations, weak adjacent discovery directly limits answer quality even when control behavior looks superficially successful.

### 3. Operational runner failures are still part of the story

Four ledger rows are outright failures. These are not low scores; they are aborted or failed runs.

That matters because the current results are mixing together:

- actual model/control quality
- model variance
- runner/harness instability

Until those are tracked separately, some of the perceived benchmark noise will remain hard to interpret.

## Methodological weaknesses in the current loop

### 1. The loop currently compares variants using one run at a time

`autoresearch_loop.py` currently keeps or discards based on a single scored run. Given the observed spread from `0.33` to `0.83`, this is too noisy to support strong selection decisions.

At current variance levels, N=1 comparisons are not a trustworthy optimization signal.

### 2. The setup is effectively optimizing one task instance

The fixed prompt in `autoresearch_loop.py` targets one specific review task in `sketch/sketch7/context/`.

This becomes more concerning when combined with the current variants, several of which explicitly mention benchmark-specific adjacent files such as:

- `sketch/sketch7/main.go`
- `sketch/sketch7/runtime/view.go`
- `sketch/sketch7/logs/session.go`

Those instructions may improve the benchmark, but they also create a serious overfitting risk. The loop may be learning this exact task rather than improving general bounded repo-inspection behavior.

### 3. The implemented scorer is narrower than the full benchmark vision

The benchmark design document lays out a broad taxonomy, including synthesis, workflow, clarification, and context/history behavior. The implemented scorer currently supports a narrower operational slice.

That is not necessarily bad, but it means the current findings should be described honestly as Tier 1 control/discovery/format findings rather than broad proof of overall system improvement.

### 4. The manager appears to be operationally alive but strategically quiet

`manager_state.json` shows the live loop is still running and manager ticks are completing, but the reviewed state does not show meaningful strategic intervention yet. In particular, `best_variant_id` remains `baseline`.

That suggests the manager layer is not yet adding much experimental intelligence beyond orchestration and monitoring.

## Main interpretation

The current autoresearch findings are encouraging, but not yet decisive.

The most defensible conclusions right now are:

1. the benchmark framework is conceptually strong
2. the loop has likely produced some real gains in required evidence gathering and structured output compliance
3. the largest persistent behavioral weakness is first-pass forced-report obedience
4. the largest methodological weakness is noisy, single-run, single-task comparison

Put differently: the experiment direction looks good, but the evidence standard for claiming durable capability improvement has not yet been met.

## Recommended next steps

### Metric focus decision

As a follow-up to this review, the intended direction for the next round is to stop optimizing against a broad composite score and instead focus on a single primary metric.

The reason is strategic, not merely methodological. Maelstrom's larger vision is to become a durable runtime for reliable, long-running, context-aware autonomous systems. In that framing, the most important near-term question is not whether the model can produce valid JSON or satisfy a miscellaneous benchmark bundle. The deeper question is whether the runtime can reliably cause the agent to gather sufficient evidence before it acts or reports.

For the current experiment family, the proposed north-star metric is:

**Evidence Sufficiency Success Rate**

At a high level, a run should count as successful only if it gathers the minimum evidence required for the task before final reporting. For the current bounded review task, that means prioritizing:

- required core-file coverage
- relevant test-file coverage when tests exist
- at least one adjacent integration file when architectural explanation or recommendations are requested
- successful final reporting from gathered evidence

Under this approach, other metrics remain important, but they become guardrails rather than the main optimization target. In particular:

- post-boundary tool-use obedience
- schema / JSON validity
- repair success
- runner completion rate

should still be tracked so that the loop does not improve evidence gathering by regressing on control or reliability.

This single-metric focus better matches the product vision in `docs/vision.md`, especially the emphasis on:

- context-aware intelligence
- reliable long-running behavior
- trustworthy operation
- and deliberate information assembly at inference time

The intended next phase of harness work should therefore treat evidence sufficiency as the north-star research target and relegate the broader aggregate pass rate to a secondary dashboard metric.

### 1. Move from single-run evaluation to multi-run evaluation

This should be the top priority.

Each candidate variant should be evaluated across multiple trials, with selection based on an aggregate such as mean or median pass rate plus completion rate. Without this, the loop is too exposed to ordinary model variance.

### 2. Add at least one more benchmark task

A second task in a different region of `sketch/sketch7` would immediately improve confidence that gains are general rather than benchmark-specific.

This is the cleanest defense against prompt-level overfitting.

### 3. Create a dedicated micro-benchmark for post-boundary obedience

Since first-pass boundary compliance is the clearest persistent failure, it deserves a focused benchmark and dedicated variant work.

Useful measurements would include:

- any tool call after the boundary
- time-to-final-report after boundary
- whether the first post-boundary response is valid JSON only

### 4. Track operational reliability separately from quality

Runner completion rate, failure phase, and likely failure cause should be elevated to first-class metrics. This will help distinguish harness fragility from behavior regressions.

### 5. Reduce task-specific hardcoding in variants

The current variants are understandable as short-term probes, but future retained variants should avoid leaning too heavily on benchmark-specific filenames. Otherwise, the framework risks evolving benchmark-specific recipes rather than generally better controller behavior.

## Bottom line

This first autoresearch round produced a useful result:

- the framework is good enough to expose meaningful behavioral differences
- some Tier 1 improvements are visible
- the primary control bottleneck is now clearer

However, the present evidence is still too noisy and too task-specific to support strong claims of robust optimization.

The next round should focus less on adding more prompt variants and more on strengthening the experiment method itself:

- multi-trial comparison
- multi-task evaluation
- a dedicated obedience benchmark
- and explicit separation of runner reliability from model quality
