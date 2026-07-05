package evals

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func writeModelFixture(t *testing.T, path, name string) {
	t.Helper()
	raw := "apiVersion: maelstrom/v1\nkind: Model\nname: " + name + "\nproviders:\n  - name: fake\n    modelRef: " + name + "-ref\nlimits:\n  contextWindow: 4096\n  maxOutputTokens: 512\ndefaults:\n  temperature: 0.0\n  topP: 1.0\ncapabilities:\n  tools: true\n  reasoning: false\n  multimodal: false\n  streaming: false\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write model: %v", err)
	}
}

func writeAgentFixture(t *testing.T, path, modelName string) {
	t.Helper()
	raw := "apiVersion: maelstrom/v1\nkind: Agent\nname: test-agent\nmodel: " + modelName + "\ntools: []\ncontext:\n  inputBudget: 2000\n  projections:\n    - type: state_task\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      prompt: Return a summary.\n      outputs:\n        schema: cognitive_step_v1\n        requiredFields: [summary]\n  transitions: []\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}
}

const smokeDeckEvalBlock = "    eval:\n      name: case-1\n      require_completed: false\n      max_invalid_outputs: 1\n      max_invalid_tool_proposals: 0\n      max_tool_execution_failures: 0\n      max_unrecovered_retries: 0\n      max_missing_required: 1\n      max_wrong_state: 0\n"

func smokeProvider() *provider.FakeProvider {
	return &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{provider.AssistantOutput{Content: `{"summary":"done"}`}}}}
}

// writeSmokeDeckFixture writes a minimal model/agent/deck fixture and returns
// a RunnerConfig wired to a fake provider.
func writeSmokeDeckFixture(t *testing.T) RunnerConfig {
	t.Helper()
	tempDir := t.TempDir()
	deckPath := filepath.Join(tempDir, "deck.yaml")
	modelPath := filepath.Join(tempDir, "model.yaml")
	agentPath := filepath.Join(tempDir, "agent.yaml")
	outputPath := filepath.Join(tempDir, "out.jsonl")
	writeModelFixture(t, modelPath, "test-model")
	writeAgentFixture(t, agentPath, "test-model")
	deckRaw := "name: smoke\ncases:\n  - id: case-1\n    model: " + modelPath + "\n    agent: " + agentPath + "\n    prompt: Summarize.\n" + smokeDeckEvalBlock
	if err := os.WriteFile(deckPath, []byte(deckRaw), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}
	return RunnerConfig{DeckPath: deckPath, OutputPath: outputPath, Provider: smokeProvider(), RootDir: tempDir}
}

// writeMatrixDeckFixture writes a deck with two deck-level models and one
// case without a case-level model override.
func writeMatrixDeckFixture(t *testing.T) RunnerConfig {
	t.Helper()
	tempDir := t.TempDir()
	deckPath := filepath.Join(tempDir, "deck.yaml")
	modelAPath := filepath.Join(tempDir, "model-a.yaml")
	modelBPath := filepath.Join(tempDir, "model-b.yaml")
	agentPath := filepath.Join(tempDir, "agent.yaml")
	outputPath := filepath.Join(tempDir, "out.jsonl")
	writeModelFixture(t, modelAPath, "model-a")
	writeModelFixture(t, modelBPath, "model-b")
	writeAgentFixture(t, agentPath, "model-a")
	deckRaw := "name: matrix\nmodels:\n  - " + modelAPath + "\n  - " + modelBPath + "\ncases:\n  - id: case-1\n    agent: " + agentPath + "\n    prompt: Summarize.\n" + smokeDeckEvalBlock
	if err := os.WriteFile(deckPath, []byte(deckRaw), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}
	return RunnerConfig{DeckPath: deckPath, OutputPath: outputPath, Provider: smokeProvider(), RootDir: tempDir}
}

func TestRunDeckWritesJSONLRecord(t *testing.T) {
	config := writeSmokeDeckFixture(t)
	outputPath := config.OutputPath
	if err := RunDeck(config); err != nil {
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

func TestRunDeckResumeSkipsCompletedRuns(t *testing.T) {
	config := writeSmokeDeckFixture(t)
	if err := RunDeck(config); err != nil {
		t.Fatalf("first RunDeck: %v", err)
	}
	if err := RunDeck(config); err != nil {
		t.Fatalf("second RunDeck: %v", err)
	}
	raw, err := os.ReadFile(config.OutputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines after resume, want 1 (no duplicates)", len(lines))
	}
}

func TestLoadTaskDeckAppliesTimeoutInheritance(t *testing.T) {
	tempDir := t.TempDir()
	deckPath := filepath.Join(tempDir, "deck.yaml")
	deckRaw := "name: timeouts\ntimeoutSeconds: 120\ncases:\n  - id: inherits\n    agent: agent.yaml\n    prompt: p\n  - id: overrides\n    agent: agent.yaml\n    prompt: p\n    timeoutSeconds: 30\n"
	if err := os.WriteFile(deckPath, []byte(deckRaw), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}
	deck, err := LoadTaskDeck(deckPath)
	if err != nil {
		t.Fatalf("LoadTaskDeck: %v", err)
	}
	if deck.Cases[0].TimeoutSeconds != 120 {
		t.Fatalf("inherited timeout = %d, want 120", deck.Cases[0].TimeoutSeconds)
	}
	if deck.Cases[1].TimeoutSeconds != 30 {
		t.Fatalf("override timeout = %d, want 30", deck.Cases[1].TimeoutSeconds)
	}
}

func TestCaseTimeoutDefaults(t *testing.T) {
	if got := caseTimeout(TaskCase{}); got != defaultCaseTimeout {
		t.Fatalf("default timeout = %v, want %v", got, defaultCaseTimeout)
	}
	if got := caseTimeout(TaskCase{TimeoutSeconds: 30}); got != 30*time.Second {
		t.Fatalf("explicit timeout = %v, want 30s", got)
	}
}

func TestRunDeckRunsModelMatrix(t *testing.T) {
	config := writeMatrixDeckFixture(t)
	if err := RunDeck(config); err != nil {
		t.Fatalf("RunDeck: %v", err)
	}
	raw, err := os.ReadFile(config.OutputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (one per model)", len(lines))
	}
	seenModels := map[string]bool{}
	seenRunIDs := map[string]bool{}
	for _, line := range lines {
		var record RunRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("unmarshal record: %v", err)
		}
		if record.Error != "" {
			t.Fatalf("record error = %q", record.Error)
		}
		if record.ModelID == "" || record.ModelRef == "" || record.ModelLabel == "" {
			t.Fatalf("missing model refs in record %+v", record)
		}
		seenModels[record.ModelID] = true
		seenRunIDs[record.RunID] = true
	}
	if !seenModels["model-a"] || !seenModels["model-b"] {
		t.Fatalf("models seen = %v", seenModels)
	}
	if len(seenRunIDs) != 2 {
		t.Fatalf("run IDs not distinct: %v", seenRunIDs)
	}
	// Resume should add nothing.
	if err := RunDeck(config); err != nil {
		t.Fatalf("resume RunDeck: %v", err)
	}
	raw, err = os.ReadFile(config.OutputPath)
	if err != nil {
		t.Fatalf("re-read output: %v", err)
	}
	if got := len(strings.Split(strings.TrimSpace(string(raw)), "\n")); got != 2 {
		t.Fatalf("got %d lines after resume, want 2", got)
	}
}

func TestLoadTaskDeckUnmarshalsEvalFields(t *testing.T) {
	config := writeSmokeDeckFixture(t)
	deck, err := LoadTaskDeck(config.DeckPath)
	if err != nil {
		t.Fatalf("LoadTaskDeck: %v", err)
	}
	// Regression: SessionEvalCase previously lacked yaml tags, so eval
	// thresholds in deck files were silently ignored.
	if deck.Cases[0].Eval.MaxInvalidOutputs != 1 || deck.Cases[0].Eval.MaxMissingRequired != 1 {
		t.Fatalf("eval thresholds not unmarshaled: %+v", deck.Cases[0].Eval)
	}
	if deck.Cases[0].Eval.Name != "case-1" {
		t.Fatalf("eval name = %q", deck.Cases[0].Eval.Name)
	}
}

func TestLoadAutoresearchDeck(t *testing.T) {
	deck, err := LoadTaskDeck(filepath.Join("decks", "autoresearch-repo-inspection.yaml"))
	if err != nil {
		t.Fatalf("LoadTaskDeck: %v", err)
	}
	if len(deck.Cases) != 12 {
		t.Fatalf("got %d cases, want 12", len(deck.Cases))
	}
	byID := map[string]TaskCase{}
	for _, tc := range deck.Cases {
		byID[tc.ID] = tc
		if tc.TimeoutSeconds != 240 {
			t.Fatalf("case %q timeout = %d, want inherited 240", tc.ID, tc.TimeoutSeconds)
		}
		if _, err := os.Stat(filepath.Join("..", "..", "..", tc.AgentPath)); err != nil {
			t.Fatalf("case %q agent path: %v", tc.ID, err)
		}
		for _, required := range tc.Eval.RequiredFilesRead {
			if _, err := os.Stat(filepath.Join("..", "..", "..", required)); err != nil {
				t.Fatalf("case %q required file: %v", tc.ID, err)
			}
		}
	}
	discovery := byID["discovery-required-files"]
	if len(discovery.Eval.RequiredFilesRead) != 3 {
		t.Fatalf("discovery required files = %+v", discovery.Eval.RequiredFilesRead)
	}
	synthesis := byID["synthesis-mechanism-summary"]
	if len(synthesis.Eval.FinalOutputContains) != 3 {
		t.Fatalf("synthesis terms = %+v", synthesis.Eval.FinalOutputContains)
	}
	focus := byID["workflow-task-focus"]
	if focus.Eval.MaxFilesRead != 6 {
		t.Fatalf("task focus budget = %d, want 6", focus.Eval.MaxFilesRead)
	}
}
