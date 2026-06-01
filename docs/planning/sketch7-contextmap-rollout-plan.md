# Sketch7 Context Map Definition and Persistence Rollout Plan

**Status:** Proposed | **Date:** 2026-06-01

## Why this document exists

Sketch7 now has a real `context/` package and a minimally explicit inference-shaping layer.

That is already paying off:

- repo-awareness context improved first-step model orientation,
- provider replay is more stable because payload shape is more explicit,
- and prompt-visible transcript handling is now separated more clearly from the broader session log.

However, two follow-on needs are now visible:

1. the authored agent context definition will eventually need a slightly richer policy surface,
2. and arbitrary context sections/chunks need a durable history model so that hidden steering does not disappear into implementation detail.

This document captures the intended next-step shape for both.

---

## Design goals

We want to:

- keep the authored context-map definition minimal,
- avoid recreating a large chunk-policy DSL,
- preserve the distinction between conversation history and context history,
- make context injections durable and inspectable,
- and make it possible to answer: **what did the model actually see?**

We do **not** want to:

- turn YAML into an executable prompt-building language,
- collapse context chunks into ordinary chat history,
- or reintroduce sketch6-style abstraction sprawl.

---

## Part 1: minimal authored context-map policy fields

### Current shape

Today `defs.ProjectionDefinition` is effectively:

```go
type ProjectionDefinition struct {
	Type   string
	Prompt string
	Name   string
	Chart  string
}
```

This is enough for section ordering and simple inclusion, but not for refresh/select behavior of generated context sections.

### Proposed next-step shape

```go
type ProjectionDefinition struct {
	Type               string
	Prompt             string
	Name               string
	Chart              string
	RefreshEveryNTurns *int
	RetentionMode      string
}
```

### Why only these two fields

The intended additional policy surface is deliberately narrow:

- `refreshEveryNTurns`
- `retentionMode`

If either field is not present, it is nil/empty and ignored.

This keeps the authored context definition focused on:

- section order,
- refresh cadence for generated sections,
- and retention/selection behavior for generated-section history or transcript replay.

We explicitly do **not** want a larger policy object yet.

### Why we are not adding `sticky`

We currently do not think `sticky` is necessary.

Reasoning:

- section order in the authored context definition should determine payload order,
- transcript retention is already a separate concern,
- and when/if we add section-level pressure later, we should first see whether ordering plus `retentionMode` is enough.

Adding a separate `sticky` field now would likely introduce more policy surface than we need.

---

## Intended semantics of the two fields

### `refreshEveryNTurns`

Used for generated/dynamic sections.

Examples:

- `repo_context`
- later `task_summary`
- later `changed_files_summary`

Meaning:

- the runtime may regenerate or refresh the section only every N prompt-visible turns,
- intermediate payloads should use the latest effective snapshot.

If absent:

- the runtime uses section-type defaults,
- or recomputes every turn if the section is cheap/simple.

### `retentionMode`

Used for high-level selection/trimming policy.

Examples:

- for generated context sections: `latest_effective`
- for transcript/messages: `coherent_tail`

If absent:

- the runtime uses section-type defaults.

### Important note

`retentionMode` should be validated by section type, not as a single global enum for all projections.

That allows one shared field name without turning it into a junk drawer.

---

## Validation rules

The first implementation should use narrow validation rules.

### `system`

Allowed:

- `prompt`

Ignored / rejected for now:

- `refreshEveryNTurns`
- `retentionMode`

Reason:

- system sections are authored directly and do not currently need refresh or history-selection policy.

### `repo_context`

Allowed:

- `refreshEveryNTurns`
- `retentionMode: latest_effective`

Rejected:

- unknown retention modes

### `interaction`

Allowed for now:

- no extra fields

Possible future extension:

- `retentionMode` if interaction snapshots become durable/versioned separately.

### `cognitive_state`

Allowed for now:

- no extra fields

Possible future extension:

- `retentionMode` if cognitive sections are versioned separately from state reduction.

### `workflow_state`

Allowed for now:

- no extra fields

### `binding`

Allowed for now:

- no extra fields

### `messages`

Allowed:

- `retentionMode: coherent_tail`

Rejected:

- `refreshEveryNTurns`

Reason:

- messages are derived each turn from session history rather than refreshed on a cadence.

### General rule

Unknown `Type` values should still fail validation.
Unknown or inapplicable `retentionMode` values should fail clearly.
Inapplicable `refreshEveryNTurns` values should also fail clearly rather than silently doing nothing forever.

---

## Example authored YAML

### Near-term coding agent shape

```yaml
context:
	inputBudget: 24000
	projections:
	  - type: system
	    name: system
	    prompt: >
	      You are maelstrom-code, a precise coding agent.
	      Prefer high-signal discovery before editing.
	      Use the narrowest tool that can answer the question.
	      Validate changes after editing.

	  - type: repo_context
	    refreshEveryNTurns: 12
	    retentionMode: latest_effective

	  - type: interaction

	  - type: cognitive_state

	  - type: messages
	    retentionMode: coherent_tail
```

### Plausible later extension

```yaml
context:
	inputBudget: 28000
	projections:
	  - type: system
	    name: system
	    prompt: You are a precise coding agent.

	  - type: repo_context
	    refreshEveryNTurns: 12
	    retentionMode: latest_effective

	  - type: task_summary
	    refreshEveryNTurns: 4
	    retentionMode: latest_effective

	  - type: workflow_state

	  - type: messages
	    retentionMode: coherent_tail
```

This is about as far as we should go before stronger evidence says we need more.

---

## Part 2: durable context-map persistence

### Why persistence is needed

Arbitrary context sections are powerful.
They can improve behavior substantially, but they can also create hidden behavior steering.

If a generated chunk influences inference but is not durably recorded, we lose:

- inspectability,
- replayability,
- debuggability,
- and trustworthy reconstruction of what the model actually saw.

That is not acceptable long-term.

### We need three distinct histories/concepts

1. **conversation history**
   - user messages
   - assistant messages
   - assistant tool calls
   - tool results

2. **context history**
   - generated context sections/chunks and their refreshes

3. **inference history**
   - exact envelopes sent to the model

These are related, but they are not the same thing.

### Important rule

Context chunks should not simply be inserted into ordinary transcript replay as fake system messages.

Instead:

- keep them as first-class non-chat records in session history,
- derive transcript replay separately,
- and derive context selection separately.

---

## Proposed durable records

### `ContextSnapshotRecord`

Represents a generated context section/chunk.

Likely fields:

- `RecordID`
- `LogicalKey` (example: `repo_context`)
- `SectionName`
- `SectionType`
- `SourceKind` (static, derived_repo, derived_workflow, derived_memory, etc.)
- `Content`
- `ContentHash`
- `GeneratedAtTurn`
- `RefreshEveryNTurns` (optional snapshot metadata)
- `RetentionMode` (optional snapshot metadata)
- `SupersedesRecordID` (optional)

### `InferenceEnvelopeRecord`

Represents the exact bundle sent to inference.

Likely fields:

- `RecordID`
- `PayloadID`
- `ModelRef`
- `ProviderRef`
- `IncludedContextRecordIDs`
- `IncludedTranscriptRecordIDs`
- `ToolSchemaHash` or `IncludedToolNames`
- `PayloadHash`
- maybe a normalized payload snapshot or payload ref

These records do not need to be prompt-visible themselves.
They need to be durable and inspectable.

---

## Refreshed chunk model: latest-effective semantics

This is the main lifecycle question.

Example:

- repo context generated at turn 12,
- refreshed at turn 24,
- payload built at turn 25,
- only the turn-24 repo snapshot should normally be used,
- but both turn-12 and turn-24 versions should remain in durable history.

### Recommended model

- keep append-only `ContextSnapshotRecord`s,
- assign each snapshot a `LogicalKey`,
- allow multiple snapshots over time for the same key,
- and at payload build time choose the **latest effective snapshot** for that key unless the projection’s `retentionMode` says otherwise.

This is the clean compromise between:

- durable history,
- inspectability,
- and clean payload construction.

### Why this works

It lets us answer both:

- **what context existed over time?**
- **what context was actually active in this payload?**

without flooding the live payload with stale historical versions.

---

## How ordering should work

When building the inference payload:

1. walk the authored context projection list in order,
2. for each projection:
   - static projections build directly,
   - generated projections select latest-effective snapshot or refresh if needed,
   - transcript/messages derive from conversation history with transcript retention mode,
3. serialize the resulting ordered sections and transcript.

The authored projection order should define payload order.
We should not add an additional ordering/priority field unless experience proves we need one.

---

## Transcript derivation remains separate

Transcript derivation means:

- reading the full session history,
- selecting only prompt-visible conversation/tool records,
- grouping or trimming them according to transcript policy,
- and excluding non-chat records such as context snapshots and inference envelopes.

This separation is important.

If we blur transcript replay and context history together, we will create confusing payloads and brittle trimming behavior.

---

## Staged rollout plan

### Stage 1: authored policy fields

Implement:

- `RefreshEveryNTurns *int`
- `RetentionMode string`

in `defs.ProjectionDefinition`, YAML loading, and validation.

Use them first for:

- `repo_context`
- `messages`

### Stage 2: read policy from current runtime

Update `context/` runtime logic to read:

- `repo_context.refreshEveryNTurns`
- `repo_context.retentionMode`
- `messages.retentionMode`

Defaults remain in code when values are absent.

### Stage 3: add durable context snapshot records

Add `ContextSnapshotRecord` to `logs/session.go` and record generated sections as first-class non-chat history records.

Initial use:

- `repo_context`

### Stage 4: add durable inference envelope records

Add `InferenceEnvelopeRecord` and persist:

- payload ID,
- selected context snapshot refs,
- selected transcript refs,
- provider/model refs,
- and enough normalized shape to audit what went out.

### Stage 5: latest-effective selection from history

Move generated context selection from purely in-memory ephemeral caching to:

- history-aware snapshot lookup,
- refresh-if-needed behavior,
- latest-effective selection by logical key.

### Stage 6: extend to more generated chunks

Only after the above is stable, consider extending the same model to:

- task summaries,
- changed-files summaries,
- policy nudges,
- or other generated context sections.

---

## Things we should resist

We should still avoid:

- large generic chunk DSLs,
- many nested policy objects,
- letting every section invent custom semantics in YAML,
- and treating session history as if it were identical to prompt transcript.

The point is to make context maps more durable and explicit, not to rebuild a speculative prompt programming language.

---

## Short conclusion

The intended next step is:

- keep authored context-map policy minimal,
- add only `refreshEveryNTurns` and `retentionMode`,
- persist generated context sections durably as their own history records,
- persist final inference envelopes durably,
- and select latest-effective generated snapshots by logical section key during payload build.

That gives us a stronger, more inspectable context-map architecture without losing the sketch7 simplification wins.
