# OpenCode Runner-Manager Design

This note defines how the autoresearch system should be supervised when OpenCode is the harness. The key assumption is that manager execution will be triggered externally on a schedule (for example by cron), while OpenCode session continuity provides conversational memory between manager wake-ups.

---

## 1. Key OpenCode CLI capabilities that matter here

From the OpenCode CLI docs, the relevant primitives are:

### 1.1 Non-interactive execution

OpenCode supports headless prompt execution with:

```bash
opencode run [message..]
```

This is the correct primitive for cron-driven automation.

### 1.2 Session continuation

OpenCode supports continuing the most recent or a specific prior session:

```bash
opencode run --continue "..."
opencode run --session <session-id> "..."
```

This means the runner-manager should not rely only on a markdown log or JSON state file for continuity. Session continuity can carry forward:

- recent reasoning context,
- prior managerial decisions,
- known open issues,
- and ongoing experiment direction.

### 1.3 Session forking

OpenCode also supports forking a prior session when continuing:

```bash
opencode run --continue --fork "..."
opencode run --session <session-id> --fork "..."
```

This is useful when the manager wants to preserve a successful supervisory trajectory while branching a one-off investigation or alternate strategy.

### 1.4 Machine-readable output

`opencode run` supports:

```bash
--format json
```

This is useful if the cron wrapper ever needs to parse structured session events or detect failures programmatically.

### 1.5 Headless server attachment

OpenCode supports a long-lived backend via:

```bash
opencode serve
```

and then later runs can attach using:

```bash
opencode run --attach http://localhost:4096 "..."
```

This is attractive if cold-start overhead becomes significant or if MCP/tool startup costs are large.

### 1.6 Config/runtime injection

OpenCode supports config layering via:

- project `opencode.json`
- global config
- `OPENCODE_CONFIG`
- `OPENCODE_CONFIG_DIR`
- `OPENCODE_CONFIG_CONTENT`

This means the manager can use:

- project-local agent definitions,
- project-local commands,
- constrained permissions,
- and runtime inline config overrides for automation contexts.

---

## 2. Recommended control model

The correct control model is:

### 2.1 Cron triggers manager wake-ups

Every 15 minutes, cron invokes a manager prompt via `opencode run`.

### 2.2 The manager resumes a persistent session

The cron invocation should generally continue a single long-lived manager session, either:

- with `--continue` if only one manager session is active,
- or preferably with `--session <manager-session-id>` for explicit targeting.

### 2.3 Durable state still exists, but is secondary

Durable state remains useful for:

- explicit machine-readable checkpoints,
- last review timestamps,
- current active benchmark suite,
- current best variant id,
- worker status,
- and crash recovery.

But it should **not** be the only memory channel. The OpenCode session itself should carry managerial continuity.

### 2.4 The manager should treat cron as a heartbeat, not as a mandatory retarget interval

A 15-minute wake-up is a health-check cadence. It should not force the manager to intervene every time.

On wake, the manager can decide:

- continue current line unchanged,
- wait longer for more data,
- retarget the benchmark family,
- add/remove variants,
- patch harness/scorer,
- or pause due to infra issues.

---

## 3. Recommended session strategy

### 3.1 Use one named long-lived manager session

Recommendation:
- establish one dedicated OpenCode session for the runner-manager for this repo.
- record its session id in durable state, for example in `manager_state.json`.

This is more robust than relying on `--continue`, because `--continue` is ambiguous if a human uses OpenCode in the same repo for a separate task.

### 3.2 Use `--session <id>` instead of bare `--continue` once the session id is known

Recommended cron-style pattern:

```bash
opencode run --session <manager-session-id> --dir /path/to/repo "Manager tick prompt..."
```

This avoids accidentally resuming the wrong session.

### 3.3 Use session forking only for side investigations

Examples:
- investigating a suspected scorer bug,
- exploring a new benchmark family without disrupting the main manager trajectory,
- or trying a risky retargeting plan.

The main supervision loop should stay on one primary session.

---

## 4. Durable state: what it should contain

Even with session resumption, durable state is still important.

Recommended file:

- `docs/experiments/autoresearch/manager_state.json`

Suggested contents:

```json
{
  "manager_session_id": "...",
  "current_objective": "Improve tier1 discovery/control pass rate for bounded_v3 variants",
  "active_suite": "tier1",
  "active_variants_file": "docs/experiments/autoresearch/variants.yaml",
  "best_variant_id": "baseline",
  "last_reviewed_results_path": "docs/experiments/autoresearch/results/...json",
  "last_reviewed_at": "2026-06-04T03:00:00Z",
  "next_meaningful_review_after": "2026-06-04T03:30:00Z",
  "status": "healthy-progressing",
  "notes": [
    "bounded_v3 still failing adjacent integration coverage",
    "coverage message variants need stronger adjacent targeting"
  ]
}
```

This file is not the primary memory, but it is the primary **machine-readable control surface**.

---

## 5. Recommended cron wrapper behavior

### 5.1 Cron should invoke a stable wrapper, not raw prompts inline

Instead of embedding long prompts directly in crontab, use a wrapper script that:

1. reads `manager_state.json`
2. determines manager session id
3. composes the current tick prompt
4. calls `opencode run --session ...`
5. records stdout/stderr and exit status

### 5.2 Prefer explicit session targeting

Recommended command shape:

```bash
opencode run \
  --session <manager-session-id> \
  --dir /home/albert/git/maelstrom-v7 \
  "Manager tick: review autoresearch progress, decide whether to continue, retarget, patch, or pause. Update durable state before exiting."
```

### 5.3 Consider `--format json` if the wrapper will inspect outputs programmatically

Example:

```bash
opencode run \
  --session <manager-session-id> \
  --dir /home/albert/git/maelstrom-v7 \
  --format json \
  "Manager tick ..."
```

This is useful if the wrapper wants to detect:

- session failures,
- explicit manager decisions,
- or emitted checkpoint content.

---

## 6. Manager decision policy per wake-up

On each scheduled wake-up, the manager should do five things.

### 6.1 Observe

Inspect:

- `autoresearch_results.tsv`
- newest files in `docs/experiments/autoresearch/results/`
- newest raw run artifacts
- current variants file
- any harness/scorer failures

### 6.2 Classify the current state

Suggested state classes:

- `healthy-progressing`
- `healthy-but-flat`
- `infra-broken`
- `benchmark-broken`
- `search-space-exhausted`
- `ready-to-retarget`

### 6.3 Decide whether enough new data exists

The manager should check a durable timestamp such as:

- `next_meaningful_review_after`

If awakened before that time and there is no obvious failure condition, it should do only a lightweight health check and exit.

This is how we reconcile:

- frequent cron wakeups,
- with slower experiment feedback loops.

### 6.4 Act

Examples:

- continue current variant search unchanged
- add stronger variants
- patch benchmark scoring
- retarget from discovery to formatting/control
- pause due to credential/infrastructure issues

### 6.5 Persist

Before exit, update:

- `manager_state.json`
- optional manager log
- any variant/spec files that changed

---

## 7. Recommended timing model

### 7.1 Heartbeat interval

Default:
- 15 minutes

This is the cron wake-up frequency.

### 7.2 Minimum meaningful review interval

Default:
- 30 to 60 minutes

This is stored in durable state. It prevents the manager from overreacting to tiny sample sizes.

### 7.3 Strategic review interval

Default:
- 2 to 4 hours

Even if progress looks healthy, the manager should periodically reassess:

- whether the benchmark family is still the right one,
- whether the mutation surface is exhausted,
- whether we are overfitting,
- and whether thresholds/metrics need revision.

---

## 8. Why durable state is still necessary even with session resume

Session resume is powerful, but not sufficient by itself because:

1. sessions can grow large or become noisy,
2. external wrappers need machine-readable control state,
3. crash recovery should not depend only on conversational transcript,
4. and a future automation layer may need to inspect status without parsing natural language.

So the best model is:

- **session memory for reasoning continuity**
- **durable state for machine control**

Not one or the other.

---

## 9. Recommended next implementation pieces

To operationalize this under OpenCode, the next pieces should be:

1. `manager_state.json` schema and initialization
2. `manager_tick.md` or prompt template for each wake-up
3. a cron-safe wrapper script that invokes `opencode run --session <id>`
4. optional lock file to avoid overlapping manager invocations
5. optional use of `opencode serve` if startup overhead becomes annoying

---

## 10. Bottom line

The best OpenCode-native design is:

- cron wakes the manager every 15 minutes,
- the manager resumes a dedicated OpenCode session using `--session <id>`,
- durable state captures machine-readable control information,
- the session provides supervisory memory and strategic continuity,
- and the manager is free to defer intervention until enough new evidence exists.

This is better than using only a markdown state file, and better than relying only on session continuation.
