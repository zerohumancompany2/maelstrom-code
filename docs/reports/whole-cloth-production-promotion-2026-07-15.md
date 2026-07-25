# Whole-Cloth Production Promotion

Date: 2026-07-15

## Decision

The repository did not merge the sketch7 runtime into the legacy product
line. It replaced the legacy line whole cloth.

The final pre-promotion state is preserved at annotated Git tag
`sketch-era-final`. Obsolete code was deleted rather than copied into an
in-tree archive, keeping search, Go tooling, and agent context focused on one
runtime.

## Result

```text
cmd/maelstrom/   production CLI
internal/        sole runtime implementation
agents/          agent YAML definitions
workflows/       workflow YAML definitions
models/          model YAML definitions
evals/decks/     resumable eval batteries
evals/testdata/  sandbox write fixtures
docs/done/       completed implementation plans
```

The Go module is now `github.com/comalice/maelstrom`.

Removed:

- legacy `cmd/is` and `cmd/sketch` through `cmd/sketch5`;
- the superseded legacy `internal/` implementation;
- sketch6;
- root scratch notes and the generated `dump.txt` workflow.

Promoted:

- sketch7 packages into `internal/`;
- the CLI into `cmd/maelstrom`;
- agents, workflows, models, eval decks, and write fixtures into stable
  top-level locations;
- completed sketch-era plans into `docs/done/`.

`go mod tidy` removed the old OpenRouter wrapper, UUID, and stateless-machine
dependencies. The production runtime uses its own OpenAI-compatible provider
and statechart implementation.

## Verification

- `go test ./...` passes.
- `go vet ./...` passes.
- `go build ./cmd/maelstrom` passes.
- A dead-endpoint smoke run against `evals/decks/tiny-readonly.yaml` loads the
  promoted deck and emits one attributable provider-error record.
- Repository agent/workflow catalog tests load definitions from their new
  top-level paths.
- No Go or YAML source references the old module or `sketch/sketch7` paths.

## Migration commits

- `978f9ae` deletes superseded implementations after creating the archive tag.
- `ebe93b7` promotes the runtime and renames the module.

The active implementation now satisfies Phase 5's acceptance condition: one
clear runtime line, no implied live sketch branches, and all new code landing
in production directories.
