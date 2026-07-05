# Conversion Boundary Errors

**Status:** In Progress | **Date:** 2026-05-22

## Problem

We had two important conversion seams that used to `panic` when they encountered an unsupported type:

- `context.SessionItemToContextItem`
- `providers.ConvertItem`

That was acceptable as a short-term assertion while the type set was small and closed, but it made the runtime brittle when new message variants were introduced and one layer was not updated in lockstep.

## Goal

Move boundary conversions from panic-driven failure to explicit error returns where practical, while preserving strong assumptions about the closed internal type set.

## Constraints

- Runtime definitions are treated as immutable; source files are edited and the runtime is rebuilt/reloaded rather than mutated in place.
- We do not want a broad cross-package refactor in the middle of the current context-budgeting work.

## Follow-up plan

1. Audit conversion seams and classify them as either:
   - internal sealed assertions, or
   - package/API boundaries that should return errors.
2. ✅ Convert provider-facing conversion helpers to return `(T, error)`.
3. ✅ Convert `context.SessionItemToContextItem` to return `(ContextItem, error)` and propagate that through `MessagesChunk.Build`.
4. ✅ Add regression tests for unsupported-type handling.
5. Remaining: decide whether any other internal conversion seams should stay assertion-based, and if so, document them explicitly.

## Notes

- Provider request building now fails explicitly when unsupported `ContextItem` values are encountered.
- Context-session conversion now fails explicitly when unsupported `SessionItem` values are encountered.
- If we keep any remaining panic-based seams, they should be deliberate sealed-type assertions with explicit documentation.
