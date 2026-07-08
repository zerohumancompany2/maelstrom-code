# Maelstrom Code Workflow Stress Test Plan

**Status:** Draft
**Date:** 2026-07-07
**Scope:** Exercise workflow states bottom-up, the same way cognitive states, the core inference loop, and tool use were exercised. Read-only first; write-enabled workflow tests wait for Phase 4 gating.

## Short answer

Phase 3 made workflow states real: bound-driven workflow finalization, per-bucket
output validation, workflow lifecycle mirroring, and bucket-level reporting all
exist and are tested at the unit level. None of it has been exercised against
live models. This plan does for workflow states what the cognitive stress tests
and the model battery did for cognitive states.

The dev-team framing is the north star, but it enters as a **workflow shape**,
not an orchestration subsystem. Everything in Tiers 1–2 is deck/harness glue
over existing core mechanics.

## Boundary constraints (read first)

This plan is subordinate to future-considerations §13 ("Orchestration stays
above the core"):

- The core is multi-agent in definition, multi-session in execution,
  orchestration-free in decision. It supplies lifecycle verbs without volition.
- **No new core abstractions may be introduced by Tiers 1–2.** The only core
  changes these tiers may produce are bug fixes within existing
  responsibilities (finalization, bounds, tool policy, records, reducers).
- The eval harness is the primitive orchestrator: it decides which agent binds
  next and sequences sessions. That logic stays in `evals/` (glue), never in
  `runner/`, `runtime/`, or `logs/`.
- Handoff is not a core concept. Agent B binding to a workflow whose history
  contains agent A's finalized outputs *is* the handoff.
- Tier 2 doubles as a **verb-surface audit**: friction encountered while the
  harness drives start → bind → finalize → unbind → next-agent tells us which
  lifecycle verbs are missing from the core's programmatic surface or tangled
  into `main.go`. Record that friction; it feeds the Phase 5 API shape.

## What we are testing

1. Do workflow output contracts produce valid, useful process artifacts under
   pressure (bound-driven workflow finalization with real models)?
2. Do combined cognitive + workflow finalization turns work live, not just in
   unit tests (wrapper schema, per-bucket validation, partial-pass retries)?
3. Does a workflow instance survive across sessions — can a second agent bind
   to a persisted workflow and act correctly on the first agent's finalized
   outputs?
4. Are workflow transitions valid and durable (statechart-arbitrated, mirrored
   to workflow history, reconstructable without the session log)?
5. Do workflow tool policies actually narrow agent behavior per workflow state?
6. Can we measure all of the above from durable records and eval summaries
   without manual inspection?
7. Where do small models fail first: cognitive discipline, workflow discipline,
   or the seam between the two?

## What exists today

- Deck cases already accept a per-case `workflow:` path; the runner hydrates
  it, creates a fresh workflow instance per run, and appends bind records on
  both sides.
- Workflow-state bounds force finalization when outputs are declared
  (`WorkflowBoundHitReason`), with validated transitions via the workflow
  statechart and mirrored `workflow_state_exit` / `workflow_transition`
  records.
- Per-chart output stats (`Output.ByChart`) and finalization stats
  (`Finalization.ByStopReason` / `ByBoundReason`) flow through session stats
  into batch summaries.
- Save/load round-trips workflow histories (`logs.SaveState`/`LoadState`),
  including the new record kinds.

## What is missing (all harness/deck glue)

1. **No workflow YAMLs exist.** `sketch/sketch7/workflows/` needs its first
   definitions.
2. **Sequenced sessions against one workflow instance.** The runner creates a
   fresh instance per run. Tier 2 needs deck support for a case that runs an
   ordered list of (agent, prompt) stages against one persisted workflow
   history, with unbind/bind records between stages.
3. **Workflow-aware eval checks.** `SessionEvalCase` has no workflow
   assertions. Needed: max invalid workflow outputs (from `ByChart`),
   required workflow stop reason / finalization reason, required final
   workflow state, required workflow artifact fields present.
4. **Cross-stage assertions.** Tier 2 needs checks that a later stage's
   session actually consumed the earlier stage's artifact (cheap version:
   `final_output_contains` referencing artifact content; better version:
   context-snapshot inspection).

## Non-goals

- No open-ended writes (Phase 4 prerequisite).
- No scheduler, queues, triggers, or agent messaging.
- No autonomous agent daemons or concurrent multi-agent runs. Tier 2 is
  strictly sequential.
- No `nextAgent:`-style routing fields that the runtime acts on. The deck's
  stage list *is* the orchestration decision, made by us, in glue.
- No new workflow record kinds unless a tier exposes a reconstruction gap.

## Tiers

### Tier 1 — read-only, single-agent bound workflow

One agent, one session, bound at start to a workflow whose current state
declares outputs and bounds. The workflow budget forces workflow (or combined)
finalization mid-task.

Candidate workflow: `issue-triage.yaml`

```text
triaging (outputs: triage_v1 {decision, evidence, suspect_files}; bounds)
  -> finish -> done
```

Cases (against this repo, mirroring the autoresearch deck's task style):

- triage a described bug ("finalization retries overcount") → decision +
  suspect files,
- triage a feature request ("add per-model provider routing hints") → scope
  assessment + touchpoints,
- combined-finalization case: agent uses the ooda-reader cognitive chart with
  its own output contract, so bound exhaustion demands both buckets at once,
- control: same prompts, no workflow bound (isolates workflow-scaffold cost).

Answers questions 1, 2, 4, 5.

### Tier 2 — read-only, multi-role sequenced handoff

The harness runs N stages sequentially against one workflow instance.
Workflow: `change-planning.yaml`

```text
intake        (outputs: intake_v1 {request_summary, unknowns})
  -> triaged        -> inspecting
inspecting    (outputs: inspection_v1 {relevant_files, findings})
  -> inspected      -> planning
planning      (outputs: plan_v1 {steps, risks, validation})
  -> planned        -> reviewing
reviewing     (outputs: review_v1 {verdict, objections})
  -> approved       -> done
```

Stages bind different agents (triager → repo-reader → planner → reviewer);
initially they can share one model, then vary models per stage to find the
weakest role. Each stage's workflow state has tight bounds so every stage ends
in workflow finalization — the artifact is the handoff.

Assertions per stage: valid workflow bucket, correct transition, next stage's
output references prior artifacts (files named at inspecting appear in the
plan, plan steps appear in the review).

Answers questions 3, 6, 7, and audits the verb surface.

### Tier 3 — sandboxed write workflow (blocked on Phase 4)

Same shape as Tier 2 with an `implementing` stage doing real edits in a
disposable worktree, plus a `validating` stage running gated commands. Not
designed further here; it inherits whatever Tiers 1–2 fix.

## Metrics

Primary (all derivable today or after the eval-check glue):

- workflow bucket validity rate (`Output.ByChart["workflow"]`),
- finalization stop reasons and bound reasons (`Finalization.*`),
- workflow transition validity (invalid_transition_signal counts by chart),
- per-stage pass rate and artifact completeness (missing-field counts by
  chart),
- retry recovery on finalization turns,
- workflow-vs-control deltas on identical prompts (Tier 1 control cases),
- per-model, per-role breakdowns (existing summary grouping).

Secondary (manual audit on failures): whether the model's workflow artifact is
*useful*, not just schema-valid — same rubric spirit as the autoresearch deck.

## Execution shape

- Models: start with the three proven from autoresearch (qwen3-coder-30b,
  minimax-m2.5, glm-4.7-flash); same OpenRouter env conventions.
- Decks: `evals/decks/workflow-triage.yaml` (Tier 1),
  `evals/decks/workflow-team-planning.yaml` (Tier 2).
- Same resumable JSONL batch flow; expect Tier 2 records to carry a stage
  index in the run ID.

## Work items (ordered)

1. First workflow YAMLs + catalog/hydration coverage (they exercise loader
   paths no test touches yet).
2. Workflow-aware eval checks on `SessionEvalCase` (+ reducer-backed).
3. Tier 1 deck; run against the three-model set; audit; fix core bugs found.
4. Deck/runner support for sequenced stages against one persisted workflow
   instance (glue only; unbind/bind records between stages).
5. Tier 2 deck; run; audit; record verb-surface friction for Phase 5.
6. Write findings report (`docs/reports/`), including whether §12
   (cross-session workflow budgets) got promoted from deferred to needed.

## Acceptance

- Tier 1: live workflow and combined finalization succeed at rates comparable
  to cognitive finalization for the same models; failures are attributable
  from summaries alone (no session-log spelunking).
- Tier 2: a workflow instance built by four sequential agent bindings is fully
  reconstructable from workflow history alone; at least one model completes
  the full pipeline with all artifacts schema-valid; cross-stage artifact
  references are verifiable by eval checks.
- Zero new core abstractions; a written list of verb-surface gaps for Phase 5.
