# Developing Maelstrom

## Required checks

```bash
gofmt -w cmd internal
go vet ./...
go test ./...
go build ./cmd/maelstrom
```

YAML definitions under `agents/` and `workflows/` are loaded by repository
tests, so schema or tool-surface drift should fail the suite.

## Architectural boundary

The core exposes lifecycle verbs and derives views from durable records. It
does not decide which agent or session should run next. Sequencing belongs in
drivers such as the eval harness; runtime, logs, and reducers remain free of
orchestration policy.

## Tool safety

- The production CLI registry is read-only.
- Eval write tools require `sandbox: true`.
- Eval `run_command` is absent unless the case supplies a non-empty exact
  allowlist; it runs without a shell.
- Paths for edits, command workdirs, and post-run file assertions are clamped
  lexically and after symlink resolution.
- Never add a live-root write override as a shortcut.

## Error surfacing

Failures the model can correct should be represented as tool results or
durable runtime records. Reserve returned Go errors for failures that prevent
the runtime from continuing safely.

## Historical code

Do not restore old sketch implementations into the active tree. The final
pre-promotion state is available at Git tag `sketch-era-final`.
