# Workflow Stress Test Findings

Date: 2026-07-10

## Scope

This report closes the workflow stress test plan
(`docs/done/maelstrom-code-workflow-stress-test-plan.md`): Tier 1
(single-session workflow finalization under tight bounds) and Tier 2 (a
four-stage agent relay against one persisted workflow instance). It answers:

1. Does workflow finalization hold up live, at rates comparable to cognitive
   finalization?
2. Does a workflow instance carry enough substance for a multi-agent relay
   where later agents see only the workflow artifacts?
3. What core bugs did live data surface, and what verb-surface gaps should
   Phase 5 address?
4. Does §12 of `maelstrom-code-future-considerations.md` (cross-session
   workflow budgets) get promoted from deferred to needed?

Everything below ran against three OpenRouter models: qwen3-coder-30b,
minimax-m2.5, glm-4.7-flash. Batch files live in `.maelstrom/evals/`
(gitignored): `workflow-triage.jsonl`, `workflow-triage-r2.jsonl`,
`workflow-combined.jsonl`, `workflow-team-planning.jsonl`.

## Tier 1: workflow and combined finalization (issue-triage)

Two rounds of `evals/decks/workflow-triage.yaml` (4 cases x 2 repeats x 3
models) plus `evals/decks/workflow-combined.yaml` (engineered bound collision,
1 case x 2 repeats x 3 models).

| batch | runs | pass rate | notes |
|---|---|---|---|
| workflow-triage (r1) | 24 | 0.71 | before mid-session attribution fix |
| workflow-triage-r2 | 24 | 0.62 | after fix; `workflow_max_inference_turns: 18` now visible |
| workflow-combined | 6 | 0.83 | 3 true combined finalization events, all valid |

Headline results:

- Workflow finalization works live. Across rounds, roughly 44 workflow-bucket
  outputs were produced with only 2 invalid; every workflow-bound run reached
  final state `done`. Workflow-bucket validity was consistently *better* than
  cognitive-bucket validity for the same models — the artifact contract in the
  task frame is an effective forcing function.
- Combined finalization (cognitive and workflow bounds colliding on the same
  turn) validated live: events with reasons like
  `cognitive_max_inference_turns+workflow_max_inference_turns`, all producing
  valid dual-bucket outputs.
- Failures are attributable from summaries alone (acceptance met): they were
  cognitive-bucket noise (glm-4.7-flash producing invalid/missing cognitive
  observations) or transient provider errors, never workflow-machinery
  failures.
- Model spread: minimax-m2.5 strongest (8/8, 7/8), qwen3-coder-30b middle
  (5/8, 5/8), glm-4.7-flash weakest (4/8, 3/8) — driven almost entirely by
  cognitive output discipline, not workflow behavior.

## Tier 2: the four-stage relay (change-planning)

`evals/decks/workflow-team-planning.yaml` runs ONE staged case: four
sequential `workflow-reader` sessions bind to the same persisted
change-planning workflow instance (intake → inspecting → planning →
reviewing → done). Only the intake prompt contains the change request; every
later stage's prompt says to recover context from the workflow artifacts.
Sequencing lives entirely in `evals/` glue (stage run IDs, bind/unbind on
both sides, skip-after-failure records); the core runner stayed
orchestration-free.

Result (1 repeat x 3 models = 12 stage sessions):

- **minimax-m2.5 completed the full pipeline** (acceptance met): each stage
  advanced exactly one state, all workflow artifacts schema-valid, verdict
  `approved` delivered from `reviewing`. Cross-stage handoff is provable from
  content: the inspecting session enumerated and resolved intake's specific
  `unknowns` (e.g. "does `--format json` already work with
  `--eval-summary`", "is a dedicated `--eval-format` flag wanted") even
  though its prompt contained none of them; planning cited inspection's
  file:line findings; reviewing checked the plan's steps against the
  repository and approved. The relay even correctly carried the discovery
  that the requested feature *already exists* (`--eval-summary` +
  `--format json`) coherently through all four stages.
- **glm-4.7-flash held the relay for two stages, then overran**: its planning
  session finalized planning → reviewing and then *kept going*, finalizing
  reviewing → done inside the same session (combined finalization). The
  stage-4 agent then bound to an already-complete workflow and could produce
  no workflow output (`valid_workflow_outputs: 0`). The workflow history
  stayed consistent throughout; what broke was the glue's assumption that one
  session consumes exactly one state.
- **qwen3-coder-30b showed durable-progress-vs-stage-failure friction**: its
  intake session finalized intake → inspecting (artifact recorded), then hit
  the 360s stage timeout while continuing to work. The stage recorded an
  error, so the remaining stages were skipped — even though the handoff
  artifact for stage 2 existed and was valid.
- The new `by_stage` summary grouping attributed all of the above without
  session-log spelunking.

Reconstructability caveat: the acceptance line asks that the workflow
instance be "fully reconstructable from workflow history alone". The staged
harness keeps `WorkflowHistory` in memory per case; reconstruction is
demonstrated functionally (later stages render earlier stages' artifacts from
the reduced history, pinned by `TestRunDeckStagedCaseSharesWorkflow`), not by
inspecting a persisted history file. Persisting workflow histories from eval
runs is a small follow-up if we want forensic inspection.

## Core bugs found and fixed via live data

1. **Mid-session finalization attribution** (fixed, `4b606c4`): finalization
   events that continue the session (workflow transition → continue) produced
   no CompletionRecord and were invisible in `Finalization` stats. r1 showed
   only `cognitive_max_inference_turns`; r2 (after fix) surfaced
   `workflow_max_inference_turns: 18`. Success events are now counted from
   `StateExitRecord`/`WorkflowStateExitRecord.BoundReason` with a dedupe rule
   for combined events.
2. **Workflow artifacts had no payload** (fixed, `778344f`): workflow history
   recorded transitions but not the validated artifact content, so a relay
   had nothing to hand off. `WorkflowArtifactRecord` now persists the
   validated workflow-bucket JSON; the runtime reduces latest-per-state
   artifacts and the context builder renders them (budgeted: 1500 runes per
   artifact, 6000 total) into later bound sessions. Tier 2 would have been
   vacuous without this.
3. Accepted, no action: artifact content is model-produced JSON replayed into
   later prompts — a prompt-injection surface inherent to any artifact
   handoff. Noted for whenever untrusted inputs enter workflows.

## Verb-surface gaps for Phase 5 (acceptance: written list)

Observed friction, in priority order. All are surface (verb/view) gaps; none
require new core abstractions:

1. **Binding scope**: there is no way to bind a session to a workflow *state*
   rather than the workflow. A session that finalizes its state keeps running
   with a fresh budget in the next state (glm overrun). Phase 5 should offer
   a bind variant (or bind option) meaning "this session's work ends when the
   current workflow state exits" — the runtime already has the exit event; it
   just needs to be usable as a session stop condition. Prompt-level "then
   end your work" steering is the only lever today and it is unreliable.
2. **Durable-progress query**: glue (and any future driver) needs a cheap
   verb/view answering "did state X finalize, and where is its artifact?" so
   a died-after-handoff session (qwen timeout) doesn't abort a relay whose
   next input already exists. The reducer has the data
   (`WorkflowView.Artifacts`, state exits); it needs a queryable surface at
   the harness level.
3. **Pre-bind state inspection**: binding to an already-complete workflow
   produces sessions that cannot satisfy workflow-output expectations (glm
   stage 4). The glue should be able to ask "current state / is final?"
   before binding. (Available via `ReduceWorkflowState` internally; not
   exposed as a deliberate verb.)
4. **Relay resume semantics**: skip-after-failure records intentionally mark
   a staged case complete on resume, so one transient provider error
   permanently fails that (model, repeat) relay in that batch file. Fine for
   evals; a real driver will want "retry failed stage against the surviving
   workflow instance", which is exactly gap 2 plus a re-run verb.

## §12 assessment: cross-session workflow budgets

**Stays deferred.** The live failure mode was not budget exhaustion across
sessions — no relay came close to cumulative bound pressure. The observed
problem is the inverse: a single session consuming *multiple* workflow states
(gap 1 above), which is a binding-scope question, not a cross-session budget
question. Nothing in the Tier 2 data needs global workflow budgets; adopting
them now would add accounting without addressing the friction we actually
saw. Revisit if relays grow long enough that per-state bounds stop protecting
wall-clock/cost in aggregate.

## Acceptance scorecard

- Tier 1 live finalization comparable to cognitive, failures attributable
  from summaries: **met**.
- Tier 2 full pipeline by at least one model, artifacts schema-valid: **met**
  (minimax-m2.5; n=1 repeat per model — rates here are indicative, not
  statistical).
- Cross-stage artifact references verifiable by eval checks: **partially
  met** — the state-chain checks (`required_final_workflow_state` per stage)
  are eval-enforced; content-level cross-references were verified by
  inspection of `final_assistant`, not by an automated check. A term-overlap
  check between consecutive artifacts is a candidate follow-up.
- Zero new core abstractions: **met** (staged sequencing is evals/ glue;
  core gained only records/reducers/rendering for artifacts, which are data,
  not orchestration).
- Verb-surface gap list for Phase 5: **this report, section above**.
