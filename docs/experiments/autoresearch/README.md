# Autoresearch Adaptation for Maelstrom Experiments

This directory defines how to adapt an `autoresearch`-style autonomous experimentation loop to the work currently happening in `docs/experiments` and `docs/reports`.

The central idea is to stop treating "model behavior" as a single broad quality and instead break it into narrowly scoped, operationally testable capabilities. Each capability should be benchmarked with:

- a fixed task setup,
- a deliberately constrained objective,
- explicit expected observables,
- and quantitative or binary scoring where possible.

The purpose is to create a benchmark suite that an autonomous experiment loop can actually optimize against.

---

## 1. Why this exists

Karpathy's `autoresearch` works because it has a very clean loop:

- one mutable experiment target,
- one stable evaluation harness,
- one primary metric,
- short bounded experiments,
- and keep/discard decisions.

Our current experimentation in `maelstrom-v7` is more complicated:

- we are evaluating controller/runtime behavior rather than model-training code,
- our outputs include transcripts, structured reports, and raw logs,
- our current judgments mix quantitative metrics with qualitative interpretation,
- and the thing being optimized may be prompt wording, hidden task decomposition, tool shaping, repair prompts, or control logic.

To make autonomous experimentation viable, we need sharper evaluation surfaces. That means converting current efforts into a taxonomy of testable capabilities and then defining benchmark tasks and scoring schemas for each capability family.

---

## 2. Benchmark design principles

Each benchmark should answer all of the following explicitly:

1. **What capability is being tested?**
2. **What inputs are fixed?**
3. **What outputs or actions are expected?**
4. **How is the result scored?**
5. **What is intentionally *not* being tested?**

This prevents vague evaluations such as "better behavior" and pushes us toward measurable outcomes like:

- did the model read required files A/B/C?
- did it produce schema-valid JSON on the first try?
- did it stop using tools when forced into report mode?
- did it ask clarification questions instead of fabricating?

### 2.1 Preferred metric types

In order of preference:

1. **Binary pass/fail**
2. **Count-based quantitative metrics**
3. **Recall / precision style metrics**
4. **Weighted aggregate scores**
5. **Human qualitative review** only as a secondary or tie-breaker metric

### 2.2 Two benchmark classes

We should build two complementary classes of benchmarks.

#### A. End-to-end benchmarks

These test a full behavior chain, such as:

- understand request,
- discover relevant code,
- gather evidence,
- synthesize a report,
- format the output correctly.

These are useful for overall system comparisons, but are not ideal for autonomous optimization because multiple capabilities are confounded.

#### B. Isolated micro-benchmarks

These isolate one capability at a time.

Examples:

- **discovery-only**: score only which files were found/read
- **format-only**: preload evidence/history and score only output schema compliance
- **synthesis-only**: preload evidence bundle and score factual completeness / hallucination rate
- **clarification-only**: preload insufficient evidence and score whether the right questions are asked
- **control-only**: enforce a no-more-tools boundary and score obedience

These isolated benchmarks should be the primary optimization targets for autoresearch-style iteration.

---

## 3. Taxonomy of capability categories

The current work should be split into the following top-level categories.

1. **Request interpretation**
2. **Initial repo orientation**
3. **Discovery coverage**
4. **Evidence sufficiency judgment**
5. **Tool-use policy**
6. **Evidence-grounded synthesis**
7. **Output construction / formatting**
8. **Clarification behavior**
9. **Workflow / transition control**
10. **Context / history usage**
11. **Efficiency / cost**

Each category is defined below with granular sub-capabilities and recommended metrics.

---

## 4. Detailed taxonomy

### 4.1 Request interpretation

This category measures what the model understands before substantial repo interaction begins.

#### 4.1.1 Task-type classification

Can the model correctly classify whether the request is:

- review,
- explanation,
- bug investigation,
- implementation,
- planning,
- clarification,
- or mixed?

**Metrics**
- exact label match (binary)
- multi-label precision/recall where applicable

#### 4.1.2 Need-for-inspection judgment

Can the model correctly determine whether repository inspection is needed?

**Metrics**
- binary correctness

#### 4.1.3 Scope extraction

Can the model identify the subsystem, package, or workflow region named by the prompt?

Example:
- prompt mentions `context inference in sketch/sketch7`
- expected scope includes `sketch/sketch7/context/` and possibly adjacent integration files

**Metrics**
- scope-target recall
- false-positive scope count

#### 4.1.4 Deliverable recognition

Can the model identify the expected output type?

Examples:
- report,
- JSON completion object,
- recommendations,
- clarification questions,
- patch,
- plan.

**Metrics**
- binary correctness
- structured field match

---

### 4.2 Initial repo orientation

This category measures whether the model starts in the right place.

#### 4.2.1 Entry-point discovery

Does the model find the likely starting directory or file for the named subsystem?

**Metrics**
- whether first `N` file operations include expected entry point(s)
- first-relevant-action turn index

#### 4.2.2 Language / ecosystem targeting

Does the model orient into the correct language or runtime region?

Example:
- Go subsystem prompt should not start by reading unrelated Python files unless the task explicitly targets the Python harness

**Metrics**
- correct-language fraction in first `N` file reads
- wrong-language startup penalty

#### 4.2.3 Search query quality

Are early list/search actions likely to reveal the right evidence?

**Metrics**
- expected query pattern coverage
- number of low-yield search actions
- whether the search surface exposed the required files

---

### 4.3 Discovery coverage

This category measures what the model actually finds and reads.

#### 4.3.1 Required-file discovery

Given a task, did the model read all required files?

**Metrics**
- required recall = `required_files_read / total_required_files`

#### 4.3.2 Useful-file discovery

Did it also read optional but valuable supporting files?

**Metrics**
- useful recall = `useful_files_read / total_useful_files`

#### 4.3.3 Adjacent-file coverage

Did it move beyond the narrow target directory when the task required integration evidence?

**Metrics**
- adjacent coverage pass/fail
- adjacent recall across predefined adjacent files

#### 4.3.4 Test-file coverage

Did it read relevant tests when they exist?

**Metrics**
- test coverage pass/fail
- test recall

#### 4.3.5 Discovery precision

How much irrelevant reading occurred?

**Metrics**
- discovery precision = `relevant_reads / total_reads`
- irrelevant read count

#### 4.3.6 Discovery cost

How expensive was the discovery phase?

**Metrics**
- tool calls until minimum coverage achieved
- total file reads
- total list/search calls

---

### 4.4 Evidence sufficiency judgment

This category measures whether the model knows when enough evidence has been gathered.

#### 4.4.1 Premature synthesis

Did the model try to report before the minimum evidence checklist was satisfied?

**Metrics**
- binary
- premature report attempt count

#### 4.4.2 Over-exploration

Did the model continue reading after the minimum sufficient evidence threshold had been met?

**Metrics**
- extra tool calls beyond threshold
- excess-read count

#### 4.4.3 Unknowns calibration

Did the model correctly represent what remains unknown?

**Metrics**
- unknown precision
- unknown recall
- unsupported unknown assertions count

---

### 4.5 Tool-use policy

This category isolates action selection quality.

#### 4.5.1 Tool necessity

Did the model use tools only when needed?

**Metrics**
- unnecessary tool call count

#### 4.5.2 Tool sequencing

Did it use tools in a sensible order?

Examples:
- list directory before reading guessed paths
- inspect tests before making strong quality claims

**Metrics**
- order compliance score against expected partial order constraints

#### 4.5.3 Tool selection quality

Did it choose the right tool type for the job?

**Metrics**
- expected-tool match at key decision points
- misuse count (e.g. repeated reads when list/search would have been better)

#### 4.5.4 Tool budget discipline

Did it stay within the intended action budget?

**Metrics**
- total tool call count
- over-budget rate

---

### 4.6 Evidence-grounded synthesis

This category should usually be tested with a fixed evidence bundle to isolate synthesis from discovery.

#### 4.6.1 Mechanism explanation completeness

Given evidence bundle `E`, did the output explain all required mechanism facts?

**Metrics**
- fact recall

#### 4.6.2 Issue identification completeness

Did it identify expected issues from the supplied evidence?

**Metrics**
- issue recall
- issue precision

#### 4.6.3 Recommendation relevance

Did the recommendations correspond to the observed issues?

**Metrics**
- issue-to-recommendation alignment score
- unsupported recommendation count

#### 4.6.4 Hallucination rate

Did it claim unsupported facts?

**Metrics**
- unsupported claim count
- hallucination pass/fail when zero-tolerance is required

#### 4.6.5 Uncertainty calibration

Did it appropriately flag uncertainty where evidence was incomplete?

**Metrics**
- binary or checklist-based score

---

### 4.7 Output construction / formatting

This category isolates output-shaping ability from repo reasoning.

#### 4.7.1 Schema compliance

Did the model produce output matching the required schema?

**Metrics**
- valid-on-first-try pass/fail

#### 4.7.2 Field completeness

Were all required fields present and populated appropriately?

**Metrics**
- pass/fail
- missing-field count

#### 4.7.3 Type correctness

Did all fields use the correct types?

**Metrics**
- pass/fail
- wrong-type count

#### 4.7.4 No extra chatter

Did the model avoid extra prose outside the required structure?

**Metrics**
- pass/fail

#### 4.7.5 Repairability

If the first output was invalid, did a single repair prompt fix it?

**Metrics**
- repaired-in-one-turn pass/fail
- repair turns required

---

### 4.8 Clarification behavior

This category should be tested when evidence is intentionally insufficient.

#### 4.8.1 Clarification necessity detection

Did the model ask questions instead of fabricating?

**Metrics**
- binary

#### 4.8.2 Question quality

Were the questions the right ones?

**Metrics**
- expected-question recall
- irrelevant-question count

#### 4.8.3 Avoidance of unnecessary clarification

When enough information is available, did it avoid needless questions?

**Metrics**
- unnecessary clarification count

---

### 4.9 Workflow / transition control

This category measures compliance with runtime boundaries and task transitions.

#### 4.9.1 Transition readiness recognition

Did the model switch from exploration to reporting at the right time?

**Metrics**
- turn index of transition relative to sufficiency threshold
- early/late transition penalty

#### 4.9.2 Forced-report obedience

When told tool use is over, did it stop using tools and report?

**Metrics**
- pass/fail
- post-boundary tool call count

#### 4.9.3 Repair-turn obedience

When given a repair instruction, did it repair instead of resuming exploration?

**Metrics**
- pass/fail
- off-protocol action count after repair prompt

#### 4.9.4 Tool-policy compliance

When the runtime says only a narrow tool subset is allowed, does the model comply?

**Metrics**
- forbidden-tool attempt count

---

### 4.10 Context / history usage

This category measures whether the model uses the context it is explicitly given.

#### 4.10.1 Supplied-history fact usage

With a prebuilt history, did the model extract and use the right facts?

**Metrics**
- required-fact recall from supplied history

#### 4.10.2 Context-section utilization

Did it make use of the relevant context sections provided by the runtime?

**Metrics**
- section-dependent fact coverage

#### 4.10.3 Conflict handling

When context contains tension or unresolved uncertainty, did it represent that correctly?

**Metrics**
- pass/fail or checklist score

---

### 4.11 Efficiency / cost

This category is not primary, but is important once minimum correctness thresholds are met.

#### 4.11.1 Tool-call count

**Metrics**
- integer count

#### 4.11.2 Model turns

**Metrics**
- integer count

#### 4.11.3 Token usage

**Metrics**
- provider token counts

#### 4.11.4 Time to completion

**Metrics**
- wall-clock duration

#### 4.11.5 Cost at fixed quality

Derived metric:
- lowest cost among variants that satisfy quality thresholds

---

## 5. Candidate benchmark inventory derived from current work

This section translates current experiments and reports into benchmark candidates.

### 5.1 Existing source material

The benchmark inventory below is derived primarily from:

- `docs/experiments/bounded_task_runner.py`
- `docs/reports/bounded-task-inference-experiments-2026-06-02.md`
- `docs/reports/bounded-task-comparison-and-coverage-gating-2026-06-03.md`
- `docs/reports/sketch7-workflow-control-experiments-2026-06-01.md`

### 5.2 Benchmark family: request interpretation

#### Benchmark: `interpretation.repo_inspection_needed.context_inference`

**Purpose**
- Determine whether the model correctly judges that repo inspection is necessary for the context-inference review prompt.

**Prompt**
- `Review our implementation of context ofr inferrence in sketch/sketch7 and report how it functions. List any issues you see. Make recommendations on architectural improvements.`

**Expected outcome**
- `repo_inspection_needed = true`

**Metrics**
- binary correctness

**Source basis**
- `bounded_task_runner.py` task `understand_request`
- bounded-task experiment report

---

### 5.3 Benchmark family: discovery coverage

#### Benchmark: `discovery.context_inference.required_files.v1`

**Purpose**
- Measure whether the model reads the core `context/` implementation files for the context-inference review task.

**Required files**
- `sketch/sketch7/context/context.go`
- `sketch/sketch7/context/sections.go`
- `sketch/sketch7/context/repo.go`

**Metrics**
- required recall
- discovery cost

**Source basis**
- bounded-task runs consistently inspected these files

#### Benchmark: `discovery.context_inference.test_coverage.v1`

**Purpose**
- Measure whether the model reads relevant tests.

**Required files**
- `sketch/sketch7/context/context_test.go`

**Metrics**
- binary pass/fail
- test recall

**Source basis**
- reports explicitly track `read_test_files`

#### Benchmark: `discovery.context_inference.adjacent_integration.v1`

**Purpose**
- Measure whether the model moves beyond `context/` into adjacent integration evidence.

**Candidate adjacent files**
- `sketch/sketch7/main.go`
- `sketch/sketch7/logs/session.go`
- `sketch/sketch7/runner/loop.go`

**Metrics**
- adjacent coverage pass/fail
- adjacent recall

**Source basis**
- bounded-task runner hint logic names these files explicitly
- comparison report treats adjacent non-`context/` Go coverage as a key objective

#### Benchmark: `discovery.context_inference.precision.v1`

**Purpose**
- Measure how much irrelevant wandering happens during discovery.

**Metrics**
- relevant-read count
- irrelevant-read count
- discovery precision

**Source basis**
- workflow-control and bounded-task reports repeatedly discuss drift vs focused discovery

---

### 5.4 Benchmark family: evidence sufficiency and stopping

#### Benchmark: `control.repo_inspection.no_premature_report.v1`

**Purpose**
- Determine whether the model refrains from reporting before required evidence thresholds are met.

**Minimum evidence checklist**
- at least one implementation file in target area
- at least one relevant test file
- at least one adjacent non-target integration file

**Metrics**
- premature report pass/fail
- premature report attempt count

**Source basis**
- bounded v1/v2/v3 failure analysis
- comparison report recommendation for explicit checklist enforcement

#### Benchmark: `control.repo_inspection.forced_report_obedience.v1`

**Purpose**
- Test whether the model obeys a no-more-tools reporting boundary.

**Metrics**
- post-boundary tool call count
- valid report produced pass/fail

**Source basis**
- bounded-task runner forced reporting and repair flow

#### Benchmark: `control.repo_inspection.repair_obedience.v1`

**Purpose**
- Test whether a repair prompt yields corrected JSON without resumed exploration.

**Metrics**
- repaired in one turn pass/fail
- off-protocol tool call count after repair instruction

**Source basis**
- bounded-task runner
- bounded-task inference experiment report

---

### 5.5 Benchmark family: output construction / formatting

#### Benchmark: `format.repo_inspection.schema_validity.v1`

**Purpose**
- With either fixed evidence or bounded task artifacts, test whether the model emits JSON matching the `repo_inspection` schema.

**Schema**
- `relevant_files: list`
- `findings: list`
- `unknowns: list`
- `enough_context: bool`

**Metrics**
- valid first try pass/fail
- missing-field count
- wrong-type count
- extra-text pass/fail

**Source basis**
- `bounded_task_runner.py` schema validation

#### Benchmark: `format.readiness_check.schema_validity.v1`

**Purpose**
- Test whether the model can construct a valid `readiness_check` output given fixed upstream artifacts.

**Schema**
- `needs_clarification: bool`
- `questions: list`
- `ready_for_planning: bool`
- `reason: str`

**Metrics**
- valid first try pass/fail
- repaired in one turn pass/fail

**Source basis**
- bounded-task runner `readiness_check`

---

### 5.6 Benchmark family: synthesis quality

#### Benchmark: `synthesis.context_inference.mechanism_summary.v1`

**Purpose**
- With a fixed evidence bundle, test whether the model correctly summarizes how context inference functions.

**Expected fact groups**
- how context payload assembly works
- how sections are derived
- how repo context is produced
- where evidence is incomplete without broader call-chain reads

**Metrics**
- fact recall
- unsupported-claim count

**Source basis**
- bounded-task inference report summary of model outputs

#### Benchmark: `synthesis.context_inference.issue_finding.v1`

**Purpose**
- With a fixed evidence bundle, test whether the model identifies expected issues and limits claims appropriately.

**Metrics**
- issue recall
- issue precision
- overclaim count

**Source basis**
- comparison report discussion of grounded vs partially grounded claims

#### Benchmark: `synthesis.context_inference.recommendation_alignment.v1`

**Purpose**
- Test whether recommendations follow from observed issues and evidence gaps.

**Metrics**
- recommendation alignment score
- unsupported recommendation count

**Source basis**
- repeated report requirement to make architectural recommendations

---

### 5.7 Benchmark family: workflow and control compliance

#### Benchmark: `workflow.guidance_vs_explicit_state.task_focus.v1`

**Purpose**
- Compare whether guidance-first rendering keeps the model more task-focused than explicit workflow-state rendering.

**Observables**
- reads relevant code vs workflow YAML
- proportion of workflow/control-plane introspection
- completion of task-relevant actions

**Metrics**
- task-focus score
- workflow-introspection penalty

**Source basis**
- workflow-control experiment report

#### Benchmark: `workflow.tool_policy_compliance.v1`

**Purpose**
- Determine whether the model obeys workflow- or task-imposed tool constraints.

**Metrics**
- forbidden-tool attempts
- policy compliance rate

**Source basis**
- workflow-control report notes tool policy is currently advisory, not authoritative

#### Benchmark: `workflow.cognitive_state_usage.v1`

**Purpose**
- Detect whether the model actually uses cognitive-state machinery when present.

**Metrics**
- `transition_state` usage count
- cognitive transition record count
- compliance pass/fail if such usage is explicitly requested

**Source basis**
- workflow-control report section on latent cognitive state infrastructure

---

### 5.8 Benchmark family: prompt / hint shaping

#### Benchmark: `discovery.hints.local_relevance_uplift.v1`

**Purpose**
- Determine whether action-keyed hints increase discovery coverage.

**Metrics**
- delta in test-file coverage
- delta in adjacent-file coverage
- delta in tool-call cost

**Source basis**
- bounded v2a and v3 experiments

#### Benchmark: `discovery.explicit_adjacent_targeting.v1`

**Purpose**
- Test whether explicit targeted next-file suggestions improve adjacent integration coverage relative to passive hints.

**Metrics**
- adjacent-file recall
- turns to adjacent-file read
- extra-cost delta

**Source basis**
- comparison report recommended next experiment direction

---

## 6. Scoring schema proposals

The categories below should have stable, explicit scoring rules so autonomous experimentation can compare variants automatically.

### 6.1 Discovery scoring schema

Discovery should be scored primarily on **coverage** and **precision**, with cost used as a secondary ranking signal.

#### 6.1.1 Inputs

Each benchmark should define:

- `required_files`
- `useful_files`
- `irrelevant_patterns` or manually labeled irrelevant reads
- optional `adjacent_files`
- optional `required_tests`

#### 6.1.2 Core metrics

- `required_recall = required_files_read / required_files_total`
- `useful_recall = useful_files_read / useful_files_total`
- `adjacent_recall = adjacent_files_read / adjacent_files_total`
- `test_recall = required_tests_read / required_tests_total`
- `discovery_precision = relevant_reads / total_reads`
- `discovery_cost = weighted_tool_cost`

Where:
- `relevant_reads = required + useful + adjacent + required_tests`
- `weighted_tool_cost` can start simple as total tool calls, or later weight `read_file` more heavily than `list_files`

#### 6.1.3 Gate-first evaluation

For most discovery benchmarks, use hard gates before any aggregate score:

1. required recall must equal `1.0` for pass, or exceed a configured threshold
2. test recall must equal `1.0` when tests are part of the benchmark goal
3. adjacent recall must equal `1.0` or exceed threshold when cross-boundary evidence is the target

Only compare cost or precision after those gates are satisfied.

#### 6.1.4 Composite score (optional)

If a scalar score is needed:

```text
DiscoveryScore =
  0.50 * required_recall +
  0.20 * test_recall +
  0.20 * adjacent_recall +
  0.10 * discovery_precision
  - cost_penalty
```

Where `cost_penalty` is small and only matters among otherwise successful variants.

**Recommendation**
- Prefer gate-first comparison over a single scalar whenever possible.

---

### 6.2 Formatting scoring schema

Formatting should be treated mostly as pass/fail, with repairability as a secondary metric.

#### 6.2.1 Core metrics

- `valid_first_try` (0/1)
- `valid_after_one_repair` (0/1)
- `missing_fields_count`
- `wrong_type_count`
- `extra_text_present` (0/1)

#### 6.2.2 Recommended decision rule

- **Pass** if valid on first try and no extra text
- **Soft pass** if invalid initially but valid after one repair
- **Fail** otherwise

#### 6.2.3 Scalar score (optional)

```text
FormattingScore =
  1.00 if valid_first_try
  0.70 if valid_after_one_repair
  0.00 otherwise
  - 0.10 if extra_text_present
```

Clamp to `[0, 1]`.

---

### 6.3 Synthesis scoring schema

Synthesis should focus on completeness, grounding, and overclaim avoidance.

#### 6.3.1 Inputs

Each synthesis benchmark should define:

- `required_facts`
- `expected_issues`
- `expected_recommendations` or recommendation classes
- `forbidden_claims` or unsupported-claim detection heuristics

#### 6.3.2 Core metrics

- `fact_recall = facts_covered / required_facts_total`
- `issue_recall = issues_covered / expected_issues_total`
- `issue_precision = valid_issues / all_issues_asserted`
- `recommendation_alignment = aligned_recommendations / total_recommendations`
- `unsupported_claim_count`

#### 6.3.3 Gate-first decision rule

A synthesis output should fail if:

- unsupported claim count exceeds threshold,
- or fact recall falls below minimum threshold.

Then rank passing outputs by issue recall and recommendation alignment.

#### 6.3.4 Scalar score (optional)

```text
SynthesisScore =
  0.40 * fact_recall +
  0.25 * issue_recall +
  0.20 * issue_precision +
  0.15 * recommendation_alignment
  - hallucination_penalty
```

Where:
- `hallucination_penalty = min(0.50, 0.10 * unsupported_claim_count)`

---

### 6.4 Control / transition scoring schema

Control should be scored mainly as protocol obedience.

#### 6.4.1 Core metrics

- `premature_report_attempts`
- `post_boundary_tool_calls`
- `repair_protocol_violations`
- `forbidden_tool_attempts`
- `transition_turn_delta` relative to sufficiency threshold

#### 6.4.2 Recommended decision rule

- **Fail** if any forbidden tool attempts occur in a benchmark that explicitly disallows them
- **Fail** if post-boundary tool calls occur after a hard forced-report instruction
- **Fail** if repair prompt causes renewed exploration instead of repair
- Otherwise rank by transition timing and cost

#### 6.4.3 Scalar score (optional)

```text
ControlScore = 1.0
  - 0.30 * min(1, premature_report_attempts)
  - 0.40 * min(1, post_boundary_tool_calls)
  - 0.40 * min(1, forbidden_tool_attempts)
  - 0.20 * min(1, repair_protocol_violations)
  - timing_penalty
```

Where `timing_penalty` is a small bounded penalty for early or late transition relative to the benchmark's sufficiency threshold.

---

## 7. How these categories map to current experiments

This section reframes the existing reports in benchmark language.

### 7.1 `bounded-task-inference-experiments-2026-06-02`

Main capability slices already present:

- request interpretation
- bounded discovery inside `repo_inspection`
- forced-report obedience
- repair-turn compliance
- basic formatting validity

Main lesson:
- hidden bounded tasks plus forced synthesis is promising,
- but discovery and synthesis quality remain separable concerns.

### 7.2 `bounded-task-comparison-and-coverage-gating-2026-06-03`

Main capability slices already present:

- discovery coverage
- test-file coverage
- adjacent integration coverage
- premature closure resistance
- hint effectiveness
- cost comparison

Main lesson:
- coverage gating changes behavior,
- but current enforcement is too soft,
- and passive hints are too weak to reliably produce adjacent-file discovery.

### 7.3 `sketch7-workflow-control-experiments-2026-06-01`

Main capability slices already present:

- task focus vs control-plane introspection
- workflow rendering style effectiveness
- workflow/tool policy compliance
- cognitive-state usage in real runs
- convergence pressure vs prompt wording

Main lesson:
- guidance-first rendering is better than explicit state-machine framing,
- and stronger runtime control is probably needed where tool or transition obedience matters.

---

## 8. Recommended initial benchmark suite

For the first autoresearch-style loop, keep the suite small and focused.

### Tier 1: highest-value initial benchmarks

1. `discovery.context_inference.required_files.v1`
2. `discovery.context_inference.test_coverage.v1`
3. `discovery.context_inference.adjacent_integration.v1`
4. `format.repo_inspection.schema_validity.v1`
5. `control.repo_inspection.forced_report_obedience.v1`
6. `control.repo_inspection.repair_obedience.v1`

These are the most automation-friendly and align with current pain points.

### Tier 2: next benchmarks

7. `synthesis.context_inference.mechanism_summary.v1`
8. `synthesis.context_inference.issue_finding.v1`
9. `workflow.guidance_vs_explicit_state.task_focus.v1`
10. `workflow.tool_policy_compliance.v1`

### Tier 3: later expansions

11. clarification-only benchmarks
12. context/history-only benchmarks
13. multi-prompt cross-subsystem discovery benchmarks
14. benchmark suites for implementation/planning tasks, not only review tasks

---

## 9. Suggested benchmark spec format

A machine-readable benchmark format should be introduced later. For now, this is the recommended shape.

```yaml
id: discovery.context_inference.adjacent_integration.v1
category: discovery
sub_category: adjacent_coverage
prompt: >
  Review our implementation of context ofr inferrence in sketch/sketch7 and
  report how it functions. List any issues you see. Make recommendations on
  architectural improvements.
setup:
  mode: bounded_v3
  allowed_tools:
    - list_files
    - search_files
    - read_file
required_files:
  - sketch/sketch7/context/context.go
  - sketch/sketch7/context/sections.go
required_tests:
  - sketch/sketch7/context/context_test.go
adjacent_files:
  - sketch/sketch7/main.go
  - sketch/sketch7/logs/session.go
  - sketch/sketch7/runner/loop.go
metrics:
  - required_recall
  - test_recall
  - adjacent_recall
  - discovery_precision
  - tool_calls
thresholds:
  required_recall: 1.0
  test_recall: 1.0
  adjacent_recall: 1.0
ranking:
  primary:
    - adjacent_recall
    - discovery_precision
  secondary:
    - tool_calls
```

---

## 10. How this should shape the autoresearch adaptation

An autoresearch-style loop for this repo should optimize benchmarked capabilities, not vague notions of "better model behavior".

That loop should eventually:

1. select a benchmark family,
2. mutate a narrow experiment surface,
3. run the benchmark,
4. compute metrics,
5. keep/discard the change,
6. log results in a stable machine-readable format.

### 10.1 Recommended mutable surfaces for early experiments

Initially limit autonomous changes to a small set such as:

- prompt wording for bounded subtasks
- forced-report prompt wording
- repair prompt wording
- coverage-gate wording
- hint-generation wording or selection policy
- benchmark configuration files

Avoid unrestricted autonomous changes across the whole repo until benchmark quality is trusted.

### 10.2 Recommended early non-goals

Do **not** initially try to optimize all of these at once:

- discovery quality,
- synthesis quality,
- workflow control,
- clarification behavior,
- and efficiency.

Instead, run separate optimization tracks.

Examples:

- **Track A:** optimize discovery coverage on context-inference benchmarks
- **Track B:** optimize output formatting reliability on fixed-history formatting benchmarks
- **Track C:** optimize forced-report and repair obedience on control benchmarks

---

## 11. Near-term implementation steps

1. Create a benchmark-spec directory in this folder.
2. Convert the context-inference discovery task into explicit benchmark YAML/JSON.
3. Add a scorer that reads raw experiment artifacts and computes benchmark metrics.
4. Add isolated formatting benchmarks with prebuilt histories.
5. Add isolated control benchmarks for forced-report and repair obedience.
6. Only after those are stable, build the autonomous keep/discard loop.

---

## 12. Running the benchmark system

This folder now includes a dedicated runner and scorer for benchmark evaluation.

### Files

- `benchmarks/` — machine-readable benchmark specs
- `score_benchmarks.py` — low-level scorer that evaluates one or more specs against raw run artifacts
- `run_benchmarks.py` — dedicated runner for benchmark suites and saved result bundles
- `autoresearch_loop.py` — variant-aware benchmark loop with keep/discard decisions, variant specs, and time/iteration bounds
- `variants.yaml` — prompt/control variant definitions for bounded-task experiments
- `manager_state.json` — machine-readable runner-manager checkpoint state
- `manager_tick_prompt.txt` — base supervisory prompt used for cron-triggered manager wake-ups
- `manager_tick.py` — cron-safe OpenCode wrapper that resumes the dedicated manager session
- `manager_tick.sh` — simple shell wrapper suitable for cron invocation
- `live_loop.py` — repo-local scheduler/orchestrator that runs experiment batches and manager ticks in a repeat loop
- `live_loop.sh` — simple shell wrapper for running the live loop directly
- `results/` — optional output location for saved benchmark run JSON bundles
- `autoresearch_results.tsv` — append-only ledger of loop executions

### Common commands

List benchmark ids:

```bash
python docs/experiments/autoresearch/run_benchmarks.py --list-benchmarks
```

List available raw runs:

```bash
python docs/experiments/autoresearch/run_benchmarks.py --list-runs
```

Run the Tier 1 suite across all saved raw runs:

```bash
python docs/experiments/autoresearch/run_benchmarks.py --suite tier1
```

Run the Tier 1 suite only on `bounded_v3` runs:

```bash
python docs/experiments/autoresearch/run_benchmarks.py --suite tier1 --mode bounded_v3
```

Run a single benchmark and save the result bundle:

```bash
python docs/experiments/autoresearch/run_benchmarks.py \
  --benchmark discovery.context_inference.adjacent_integration.v1 \
  --mode bounded_v3 \
  --save
```

Run the variant-aware autoresearch loop with an iteration bound:

```bash
python docs/experiments/autoresearch/autoresearch_loop.py \
  --suite tier1 \
  --max-iterations 3 \
  --notes "variant search over control prompts"
```

Run the variant-aware autoresearch loop with a wall-clock time bound:

```bash
python docs/experiments/autoresearch/autoresearch_loop.py \
  --suite tier1 \
  --hours 2 \
  --notes "two-hour bounded search"
```

You can also combine both bounds; the loop stops when either limit is reached.

Initialize the dedicated OpenCode manager session id:

```bash
python docs/experiments/autoresearch/manager_tick.py --init-session-id <session-id>
```

Run one cron-style manager tick manually:

```bash
python docs/experiments/autoresearch/manager_tick.py
```

Or via the shell wrapper:

```bash
docs/experiments/autoresearch/manager_tick.sh
```

Run the repo-local live loop once for a single batch + manager cycle:

```bash
OPENAI_API_BASE=localhost:8931/ \
  docs/experiments/autoresearch/live_loop.sh --run-once
```

Run the live loop continuously with a 15-minute interval between cycles:

```bash
OPENAI_API_BASE=localhost:8931/ \
  docs/experiments/autoresearch/live_loop.sh --interval-seconds 900
```

This loop currently evaluates prompt/control variants defined in `variants.yaml`, generates fresh raw runs with `bounded_task_runner.py`, scores them against the selected benchmark suite, and records keep/discard decisions in `autoresearch_results.tsv`.

### Current status

The current scorer is strongest for benchmarks backed directly by the existing `bounded_task_runner.py` artifacts, especially:

- discovery benchmarks,
- formatting benchmarks,
- and control / protocol-obedience benchmarks.

The current autoresearch loop is now variant-aware, but its mutation surface is intentionally narrow:

- bounded-task system prompt append text
- coverage-repair message text

The current runner-manager layer is OpenCode-native:

- cron or another scheduler wakes `manager_tick.py`
- the wrapper resumes a dedicated OpenCode session using `--session <id>`
- `manager_state.json` stores machine-readable checkpoint state
- the resumed session carries supervisory memory between wake-ups

This keeps early autonomous search focused on prompt/control shaping rather than broad code mutation.

Tier 2 synthesis and workflow benchmarks are currently more spec-first than fully automated. They are included now so the benchmark vocabulary and inventory stay stable while scorer support catches up.

---

## 13. Summary

The key shift is:

> stop optimizing broad "behavior" and start optimizing narrowly defined measurable capabilities.

The most immediately useful benchmark families are:

- **discovery coverage**,
- **formatting validity**,
- **forced-report obedience**,
- **repair-turn obedience**,
- and later **evidence-grounded synthesis**.

Those are concrete enough to support a real autoresearch-style loop, while still being directly grounded in the problems surfaced by the current experiments.
