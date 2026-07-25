.PHONY: build test vet check smoke

build:
	go build ./cmd/maelstrom

test:
	go test ./...

vet:
	go vet ./...

check: vet test build

smoke: build
	OPENAI_API_BASE=http://127.0.0.1:9 OPENAI_API_KEY=dead \
		./maelstrom --eval-deck evals/decks/tiny-readonly.yaml \
		--eval-out /tmp/maelstrom-smoke.jsonl || true
