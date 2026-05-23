# ContextItem Interface Ergonomics

**Status:** In Progress | **Date:** 2026-05-22

## Problem

`ContextItem` currently relies on marker methods and external type switches for shared behavior such as token counting content extraction.

Today this is workable, but it creates friction in a few places:

- `contextItemContent(...)` centralizes knowledge of every concrete `ContextItem`
- adding new item variants requires updating unrelated helper functions
- the marker signature `isContextItem() bool` is more awkward than idiomatic Go marker methods

## Goal

Make `ContextItem` easier to extend and more idiomatic without over-designing the abstraction.

## Follow-up plan

1. ✅ Change marker methods from `isContextItem() bool` to `isContextItem()`.
2. Evaluate whether shared behavior should move onto the interface, for example a token/text projection method.
3. Remove or shrink central type-switch helpers where possible.
4. ✅ Update tests that exercise the revised interface usage indirectly through context/provider conversion flows.

## Constraints

- We are intentionally keeping enough information in the internal representation to support multiple provider wire formats.
- Token counting fidelity improvements are not part of this document unless they become necessary for the ergonomics work.

## Notes

- The marker-method cleanup is complete.
- The main remaining ergonomics question is whether helpers like `contextItemContent(...)` should remain centralized or move toward behavior on the interface.
