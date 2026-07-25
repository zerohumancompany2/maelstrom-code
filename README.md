# Maelstrom

Maelstrom is a local-model-first Go agent runtime for bounded, recoverable
repository work. Agents and workflows are declared in YAML; the runtime owns
tool gating, state transitions, output validation, durable session history,
and bounded finalization.

This repository has one production runtime. The earlier experimental code is
preserved in Git history at tag `sketch-era-final`, not in the active tree.

## Layout

```text
cmd/maelstrom/   CLI entrypoint
internal/        runtime, reducers, providers, tools, and eval harness
agents/          example agent definitions
workflows/       workflow definitions
models/          model definitions
evals/decks/     resumable read-only and sandboxed-write eval batteries
evals/testdata/  write-tier fixtures copied into disposable sandboxes
docs/            plans, reports, and experiment history
```

Module: `github.com/comalice/maelstrom`

## Build and test

```bash
go build ./cmd/maelstrom
go test ./...
go vet ./...
```

## Run an agent

Set an OpenAI-compatible endpoint:

```bash
export OPENAI_API_BASE=https://openrouter.ai/api/v1
export OPENAI_API_KEY=...
```

Then run a declared agent, model, and optional workflow:

```bash
go run ./cmd/maelstrom \
  --model models/openrouter-minimax-m2.5.yaml \
  --agent agents/workflow-reader.yaml \
  --workflow workflows/issue-triage.yaml \
  --session-id example \
  --prompt "Triage this repository issue."
```

Sessions persist under `.maelstrom/sessions/` when `--session-id` is used.
The normal CLI tool registry is intentionally read-only.

## Eval decks

Run a resumable deck:

```bash
go run ./cmd/maelstrom \
  --eval-deck evals/decks/workflow-triage.yaml \
  --eval-out .maelstrom/evals/workflow-triage.jsonl
```

Summarize it:

```bash
go run ./cmd/maelstrom \
  --eval-summary .maelstrom/evals/workflow-triage.jsonl

go run ./cmd/maelstrom \
  --eval-summary .maelstrom/evals/workflow-triage.jsonl \
  --format json
```

`evals/decks/sandbox-write-microtasks.yaml` is the separate write-enabled
tier. Write tools are exposed only inside disposable repository copies, and
`run_command` requires an exact no-shell allowlist. The source repository is
never the write root for an autonomous eval run.

## Runtime principles

- multi-agent in definition, multi-session in execution;
- orchestration-free core: lifecycle verbs and durable records, no hidden
  planner deciding which agent runs next;
- effective tool policy is an intersection, and an empty intersection means
  no tools;
- state bounds force validated finalization rather than accepting prose;
- workflow artifacts are durable handoffs between separately bound sessions;
- write autonomy is gated by disposable execution roots and observable disk
  assertions.

See `docs/planning/maelstrom-code-completion-plan.md` for the active product
path and `docs/reports/` for live evaluation findings.
