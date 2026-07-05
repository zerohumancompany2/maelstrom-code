package evals

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
)

func TestLoadTaskDeckParsesMinimalDeck(t *testing.T) {
	deck, err := LoadTaskDeck(filepath.Join("decks", "tiny-readonly.yaml"))
	if err != nil {
		t.Fatalf("LoadTaskDeck: %v", err)
	}
	if deck.Name != "tiny-readonly" {
		t.Fatalf("deck name = %q", deck.Name)
	}
	if len(deck.Cases) != 1 || deck.Cases[0].ID != "stateless-summary" {
		t.Fatalf("deck cases = %+v", deck.Cases)
	}
}

func TestRunDeckWritesJSONLRecord(t *testing.T) {
	tempDir := t.TempDir()
	deckPath := filepath.Join(tempDir, "deck.yaml")
	modelPath := filepath.Join(tempDir, "model.yaml")
	agentPath := filepath.Join(tempDir, "agent.yaml")
	outputPath := filepath.Join(tempDir, "out.jsonl")
	if err := os.WriteFile(modelPath, []byte("apiVersion: maelstrom/v1\nkind: Model\nname: test-model\nproviders:\n  - name: fake\n    modelRef: fake-model\nlimits:\n  contextWindow: 4096\n  maxOutputTokens: 512\ndefaults:\n  temperature: 0.0\n  topP: 1.0\ncapabilities:\n  tools: true\n  reasoning: false\n  multimodal: false\n  streaming: false\n"), 0o644); err != nil {
		t.Fatalf("write model: %v", err)
	}
	if err := os.WriteFile(agentPath, []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: test-agent\nmodel: test-model\ntools: []\ncontext:\n  inputBudget: 2000\n  projections:\n    - type: state_task\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      prompt: Return a summary.\n      outputs:\n        schema: cognitive_step_v1\n        requiredFields: [summary]\n  transitions: []\n"), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}
	deckRaw := "name: smoke\ncases:\n  - id: case-1\n    model: " + modelPath + "\n    agent: " + agentPath + "\n    prompt: Summarize.\n    eval:\n      name: case-1\n      require_completed: false\n      max_invalid_outputs: 1\n      max_invalid_tool_proposals: 0\n      max_tool_execution_failures: 0\n      max_unrecovered_retries: 0\n      max_missing_required: 1\n      max_wrong_state: 0\n"
	if err := os.WriteFile(deckPath, []byte(deckRaw), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}
	fakeProvider := &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{provider.AssistantOutput{Content: `{"summary":"done"}`}}}}
	if err := RunDeck(RunnerConfig{DeckPath: deckPath, OutputPath: outputPath, Provider: fakeProvider, RootDir: tempDir}); err != nil {
		t.Fatalf("RunDeck: %v", err)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	var record RunRecord
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	if record.CaseID != "case-1" || record.AgentID != "test-agent" {
		t.Fatalf("record = %+v", record)
	}
	if record.Error != "" {
		t.Fatalf("record error = %q", record.Error)
	}
	if record.Eval.Name != "case-1" {
		t.Fatalf("eval result = %+v", record.Eval)
	}
}
