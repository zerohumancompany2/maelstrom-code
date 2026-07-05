# Context Budget & Truncation

**Status:** In Progress | **Date:** 2025-05-20

## Problem

`ContextChunk.Build(s)` returns items with no budget awareness. Chunks can't decide whether to return full history or a truncated version. The three-layer context system (Definition → Chunk → Map) was designed for context budget management, but the seam doesn't support it yet.

## Goal

`ContextMap` owns the total token budget. Chunks build freely. The map measures, then applies per-chunk truncation based on policy and priority, then handles global rebalance if needed.

## Architecture

### New types

#### `ContextItem`

Implemented as a closed interface with concrete context message variants:

```go
type ContextItem interface {
    isContextItem()
}
```

- preserves the provider-shaping layer separate from `SessionItem`
- carries enough data to map into provider wire formats
- currently uses concrete message structs rather than a provenance wrapper

#### `Tokenizer`

```go
type Tokenizer interface {
    CountTokens(items ...ContextItem) int
}

type HeuristicTokenizer struct{} // ~4 chars = 1 token
```

Starts as a heuristic. Interface allows swap to tiktoken later.

#### `TruncationPolicy`

```go
type TruncationPolicy interface {
    Trim(items []ContextItem, budget int, tokenizer Tokenizer) ([]ContextItem, error)
}
```

Implementations:

- `HardTruncate` — drops items from the front (oldest first) until under budget
- `FailTruncate` — returns error if over budget (for system prompts, cognitive state — things that shouldn't be split)

#### `ChunkSpec`

```go
type ChunkSpec struct {
    Chunk      ContextChunk
    BudgetPct  float64          // fraction of total budget (0.0–1.0)
    Policy     TruncationPolicy // how to trim if over budget
    Priority   int              // higher = more important, trimmed last in global rebalance
    Flexible   bool             // if true, expands to fill remaining budget after fixed chunks
}
```

Example allocation for 32k window:

| Chunk | BudgetPct | Policy | Priority | Flexible |
|---|---|---|---|---|
| System prompt | 0.05 | FailTruncate | 10 | false |
| Cognitive state | 0.02 | HardTruncate | 9 | false |
| Workflow state | 0.03 | HardTruncate | 8 | false |
| Available tools | 0.05 | HardTruncate | 7 | false |
| Chat history | 0.00 | HardTruncate | 5 | **true** |
| Nudge | 0.02 | HardTruncate | 1 | false |

Chat history takes whatever's left. Nudge is first to go during rebalance.

### Build flow in `ContextMap.BuildMessages`

```txt
1. Validate `ContextLimit`
   → error if `ContextLimit <= 0`

2. Calculate per-chunk budgets: BudgetPct × ContextLimit
   → Flexible chunks use the remaining budget after fixed-budget chunks are accounted for

3. Build each chunk: chunk.Build(s)
   → Returns []ContextItem, chunks build freely, no budget awareness needed

4. Truncate each chunk to its budget: policy.Trim(items, budget, tokenizer)
   → HardTruncate drops oldest items first
   → FailTruncate returns error if over budget

5. Global rebalance if total > ContextLimit:
   → Sort chunks by Priority ascending
   → Reuse each chunk's own policy with a tighter budget target
   → Skip chunks whose policy refuses additional trimming

6. Flatten chunk outputs in original chunk order
   → []ContextItem returned to caller
```

### Data flow (updated)

```txt
chunk.Build(s) → []ContextItem{
    ContextSystemMessage{Content: "..."},
    ContextUserMessage{Content: "..."},
    ...
}
    │
    ▼
truncation.Trim([]ContextItem, budget, tokenizer)
    │
    ▼
[]ContextItem → provider.Send()
```

## Design decisions

**Truncation at the map level, not inside chunks.** Chunks build freely and return full output. The map measures, then applies the chunk's policy. Keeps chunks simple — they don't need to know about tokens, budgets, or tokenizers.

**BudgetPct + Flexible, not weight.** Percentage is explicit and auditable. Flexible is a binary flag on the chunk that should absorb remainder.

**Priority is for global rebalance only.** Per-chunk truncation happens regardless of priority. Priority only matters when total still exceeds `ContextLimit` after per-chunk truncation.

**FailTruncate for non-splitable content.** System prompts, cognitive state — things that should fail loudly rather than be chopped mid-sentence. FailTruncate is the safety net. We validate what we can (literal text or hydrated go templates) during YAML ingestion, and issue warnings and/or errors when certain thresholds are surpassed.

## Changes

### New files

| File | Contents |
|---|---|
| `internal/context/contextitem.go` | `ContextItem` interface + concrete context message types |
| `internal/context/tokenizer.go` | `Tokenizer` interface + `HeuristicTokenizer` |
| `internal/context/truncation.go` | `TruncationPolicy` interface + `HardTruncate`, `FailTruncate` |
| `internal/context/chunkspec.go` | `ChunkSpec` struct |

### Changed files

| File | Change |
|---|---|
| `context/chunk.go` | `Build` returns `[]ContextItem` instead of `[]SessionItem` |
| `context/definition.go` | `Chunks []ChunkSpec` instead of `[]ContextChunk` |
| `context/map.go` | Context limit sourced from agent model config; `Tokenizer`; rewrite `BuildMessages` with budget/truncation/rebalance |
| `agent/agent.go` | `BuildContextDefinition` populates `ChunkSpec` with budget/policy/priority |
| `cmd/is/main.go` | Wire tokenizer into context map |
| `context/map_test.go` | New tests for budget, truncation, rebalance, flexible fill |

### Stays the same

| Seam | Reason |
|---|---|
| `Provider.Send(messages, opts)` | Content/config split is correct |
| `Session` | Store interface is separate work |
| `ContextChunk` interface (concept) | Still the right abstraction for "how to project session into prompt" |

## Execution order

1. `contextitem.go` — foundation, no dependencies
2. `tokenizer.go` — depends on contextitem
3. `truncation.go` — depends on tokenizer
4. `chunkspec.go` — depends on truncation
5. `chunk.go` — adapt `Build` signature
6. `definition.go` — change `Chunks` type
7. `map.go` — rewrite `BuildMessages`
8. `agent.go` — populate `ChunkSpec`
9. `main.go` — wire tokenizer
10. `map_test.go` — new tests + adapt existing
11. `go test ./...`

## Risks

| Risk | Mitigation |
|---|---|
| `ContextDefinition.Chunks` type change | Internal-only, no external import |
| Invalid context limit config | `BuildMessages` now errors clearly when `ContextLimit <= 0` |
| Heuristic tokenizer inaccurate | It's a bound, not a target — better to under-fill than overflow |
| Global rebalance O(n²) worst case | Chunk count is small (< 10), not a concern |
| `HardTruncate` on atomic content | `FailTruncate` available for chunks that can't be split |

## Out of scope

- `Summarize` truncation policy (needs LLM call)
- Streaming provider support
- Cognitive state / workflow / nudge chunks (new capability, separate work)
- Session store interface
- `ProviderOptions.Temperature` wiring (separate completion)
