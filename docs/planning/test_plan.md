# Test Plan

## Foundation Changes (prerequisite)

1. **Provider interface** — Extract `Provider` and `ProviderResponse` interfaces so `OpenRouterAPI`/`OpenRouterResponse` implement them. Enables fake providers in tests without HTTP or the openrouter library.
2. **Remove `ToSessionItem()`** — Dead code; singular version always returns empty `UserMessage`.
3. **Wire reasoning extraction** — `ToSessionItems()` doesn't extract reasoning from API responses yet; `AssistantMessage.Reasoning` stays empty on the way back.

## Seam 1: `internal/types` — SessionItem contract

| # | Test | What it verifies |
|---|---|---|
| 1.1 | Each message type implements `SessionItem` | Compile-time interface satisfaction |
| 1.2 | `ToolCallRequestMessage.ToString()` includes ID, name, args | Structured output is deterministic |
| 1.3 | `AssistantMessage` preserves reasoning alongside content | Both fields survive round-trip |

## Seam 2: `session` — Append and accumulation

| # | Test | What it verifies |
|---|---|---|
| 2.1 | `New()` produces empty session | Zero-value correctness |
| 2.2 | `Append()` preserves insertion order | Items come out in the same order they went in |
| 2.3 | `AppendMany()` ≡ N `Append()` calls | Bulk append is equivalent |
| 2.4 | Mixed item types coexist | User + Assistant + ToolCall + ToolResult + System in one session |
| 2.5 | `NewUserMessage()` wraps correctly | Factory function fidelity |

## Seam 3: `context` — BuildInferenceBundle

| # | Test | What it verifies |
|---|---|---|
| 3.1 | Empty session → empty message bundle | No phantom messages |
| 3.2 | All session items appear in bundle | 1:1 copy fidelity |
| 3.3 | Bundle.Model and Bundle.Tools propagate | Config passes through |
| 3.4 | `NewFromDefinition` copies, doesn't alias | Mutating original definition doesn't affect ContextMap |

## Seam 4: `providers` — Send conversion (with fake)

| # | Test | What it verifies |
|---|---|---|
| 4.1 | `Send` converts UserMessage → API message | Content passes through |
| 4.2 | `Send` converts AssistantMessage with reasoning | Reasoning field lands on the API type |
| 4.3 | `Send` converts ToolCallRequestMessage → assistant with tool calls | ID, name, args map correctly |
| 4.4 | `Send` converts ToolCallResultMessage → tool message | ID and content map correctly |
| 4.5 | `Send` converts ToolDefinitions → API tools | Name, desc, params, strict propagate |
| 4.6 | Arguments serialization handles string, []byte, map, unknown | All cases produce valid JSON |
| 4.7 | Model name propagates to request | Bundle.Model → request.Model |

## Seam 5: `providers` — Response to SessionItems

| # | Test | What it verifies |
|---|---|---|
| 5.1 | Nil response → nil items | Graceful degradation |
| 5.2 | Empty choices → nil items | No panic |
| 5.3 | Plain text → single AssistantMessage | Content extracted |
| 5.4 | Text + reasoning → AssistantMessage with both fields | Reasoning extracted from response |
| 5.5 | Tool calls → ToolCallRequestMessages | Count and order preserved |
| 5.6 | Tool calls only, no text → no empty AssistantMessage | No phantom items |
| 5.7 | Multiple tool calls → correct count | All preserved |

## Seam 6: `tools` — Execution

| # | Test | What it verifies |
|---|---|---|
| 6.1 | Known tool returns non-empty result | "weather" → content |
| 6.2 | Unknown tool returns empty string | Graceful degradation |
| 6.3 | ToolCallID preserved in result | Passthrough fidelity |

## Principles

- One test, one logical item. No compound assertions.
- Tests live in `_test.go` files adjacent to the code they test.
- Provider tests use a fake, never touch the network or the openrouter library.
- No tests for debug/pretty-print methods.
- Error propagation is deferred — current code panics on errors; we'll address that in a separate pass.