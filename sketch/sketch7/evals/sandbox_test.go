package evals

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

func TestCreateSandboxCopiesAndExcludes(t *testing.T) {
	source := t.TempDir()

	if err := os.WriteFile(filepath.Join(source, "keep.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write keep.txt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(source, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "pkg", "code.go"), []byte("package pkg"), 0o644); err != nil {
		t.Fatalf("write code.go: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(source, ".git", "config"), 0o755); err != nil {
		t.Fatalf("mkdir .git/config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(source, ".lumora", "index"), 0o755); err != nil {
		t.Fatalf("mkdir .lumora/index: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(source, "vendor"), 0o755); err != nil {
		t.Fatalf("mkdir vendor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "vendor", "dep.txt"), []byte("dep"), 0o644); err != nil {
		t.Fatalf("write dep.txt: %v", err)
	}

	if err := os.Symlink("keep.txt", filepath.Join(source, "link.txt")); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	dir, cleanup, err := createSandbox(source, []string{"vendor"})
	if err != nil {
		t.Fatalf("createSandbox: %v", err)
	}
	defer cleanup()

	if got, err := os.ReadFile(filepath.Join(dir, "keep.txt")); err != nil || string(got) != "hello" {
		t.Fatalf("keep.txt = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "pkg", "code.go")); err != nil || string(got) != "package pkg" {
		t.Fatalf("pkg/code.go = %q, err=%v", got, err)
	}

	for _, absent := range []string{".git", ".lumora", "vendor", "link.txt"} {
		if _, err := os.Stat(filepath.Join(dir, absent)); !os.IsNotExist(err) {
			t.Fatalf("%q should not exist in sandbox, stat err=%v", absent, err)
		}
	}

	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("sandbox dir should be gone after cleanup, stat err=%v", err)
	}
}

func TestLoadTaskDeckRejectsAllowlistWithoutSandbox(t *testing.T) {
	tempDir := t.TempDir()
	deckPath := filepath.Join(tempDir, "deck.yaml")

	negative := "name: sandbox-deck\ncases:\n  - id: c1\n    agent: /tmp/whatever-agent.yaml\n    model: /tmp/whatever-model.yaml\n    prompt: p\n    commandAllowlist: [\"go test\"]\n"
	if err := os.WriteFile(deckPath, []byte(negative), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}
	if _, err := LoadTaskDeck(deckPath); err == nil || !strings.Contains(err.Error(), "without sandbox") {
		t.Fatalf("LoadTaskDeck error = %v, want substring %q", err, "without sandbox")
	}

	positive := "name: sandbox-deck\ncases:\n  - id: c1\n    agent: /tmp/whatever-agent.yaml\n    model: /tmp/whatever-model.yaml\n    prompt: p\n    sandbox: true\n    sandboxExcludes: [\"docs\"]\n    commandAllowlist: [\"go test\"]\n"
	positivePath := filepath.Join(tempDir, "deck-ok.yaml")
	if err := os.WriteFile(positivePath, []byte(positive), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}
	deck, err := LoadTaskDeck(positivePath)
	if err != nil {
		t.Fatalf("LoadTaskDeck positive: %v", err)
	}
	tc := deck.Cases[0]
	if !tc.Sandbox {
		t.Fatalf("Sandbox = false, want true")
	}
	if !strings.EqualFold(strings.Join(tc.SandboxExcludes, ","), "docs") || len(tc.SandboxExcludes) != 1 || tc.SandboxExcludes[0] != "docs" {
		t.Fatalf("SandboxExcludes = %q, want [docs]", tc.SandboxExcludes)
	}
	if len(tc.CommandAllowlist) != 1 || tc.CommandAllowlist[0] != "go test" {
		t.Fatalf("CommandAllowlist = %q, want [go test]", tc.CommandAllowlist)
	}
}

func TestBuildEvalToolRegistryGatesWriteTools(t *testing.T) {
	agentDef := defs.AgentDefinition{
		Cognitive: defs.StatechartDefinition{
			InitialState: "observe",
			States:       []defs.StateDefinition{{Name: "observe"}},
		},
	}

	readonly := buildEvalToolRegistry(t.TempDir(), TaskCase{}, agentDef, nil)
	if err := readonly.MustHave("replace_text"); err == nil {
		t.Fatalf("replace_text should be absent for non-sandbox case")
	}
	if err := readonly.MustHave("run_command"); err == nil {
		t.Fatalf("run_command should be absent for non-sandbox case")
	}
	if err := readonly.MustHave("read_file"); err != nil {
		t.Fatalf("read_file should be present: %v", err)
	}

	sandboxed := buildEvalToolRegistry(t.TempDir(), TaskCase{Sandbox: true}, agentDef, nil)
	if err := sandboxed.MustHave("replace_text"); err != nil {
		t.Fatalf("replace_text should be present for sandbox case: %v", err)
	}
	if err := sandboxed.MustHave("run_command"); err != nil {
		t.Fatalf("run_command should be present for sandbox case: %v", err)
	}
	if err := sandboxed.MustHave("read_file"); err != nil {
		t.Fatalf("read_file should be present: %v", err)
	}
}

func TestEvaluateFileArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("Alpha Beta"), 0o644); err != nil {
		t.Fatalf("write notes.md: %v", err)
	}

	// Passing case.
	passing := SessionEvalCase{RequiredFileContains: []FileContainsCheck{{Path: "notes.md", Terms: []string{"alpha", "beta"}}}}
	passingResult := EvalResult{Passed: true}
	EvaluateFileArtifacts(root, passing, &passingResult)
	if !passingResult.Passed {
		t.Fatalf("Passed = false, want true; checks = %+v", passingResult.Checks)
	}
	if len(passingResult.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(passingResult.Checks))
	}
	if passingResult.Checks[0].Name != "file_contains:notes.md" {
		t.Fatalf("check name = %q, want file_contains:notes.md", passingResult.Checks[0].Name)
	}
	if !passingResult.Checks[0].Passed {
		t.Fatalf("check should pass: %+v", passingResult.Checks[0])
	}

	// Failing terms.
	failingTerms := SessionEvalCase{RequiredFileContains: []FileContainsCheck{{Path: "notes.md", Terms: []string{"gamma"}}}}
	failingTermsResult := EvalResult{Passed: true}
	EvaluateFileArtifacts(root, failingTerms, &failingTermsResult)
	if failingTermsResult.Passed {
		t.Fatalf("Passed = true, want false (gamma missing)")
	}
	if !strings.Contains(failingTermsResult.Checks[0].Actual, "missing: gamma") {
		t.Fatalf("actual = %q, want substring %q", failingTermsResult.Checks[0].Actual, "missing: gamma")
	}

	// Missing file.
	missingFile := SessionEvalCase{RequiredFileContains: []FileContainsCheck{{Path: "absent.md", Terms: []string{"anything"}}}}
	missingResult := EvalResult{Passed: true}
	EvaluateFileArtifacts(root, missingFile, &missingResult)
	if missingResult.Passed {
		t.Fatalf("Passed = true, want false (file absent)")
	}
	if !strings.Contains(missingResult.Checks[0].Actual, "file not readable") {
		t.Fatalf("actual = %q, want substring %q", missingResult.Checks[0].Actual, "file not readable")
	}
}

func writeReplaceTextAgentFixture(t *testing.T, path, modelName string) {
	t.Helper()
	raw := "apiVersion: maelstrom/v1\nkind: Agent\nname: sandbox-agent\nmodel: " + modelName + "\ntools:\n  - replace_text\ncontext:\n  inputBudget: 2000\n  projections:\n    - type: state_task\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      prompt: Return a summary.\n      outputs:\n        schema: cognitive_step_v1\n        requiredFields: [summary]\n  transitions: []\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}
}

func TestRunDeckSandboxedCaseEditsCopyNotSource(t *testing.T) {
	tempDir := t.TempDir()
	modelPath := filepath.Join(tempDir, "model.yaml")
	agentPath := filepath.Join(tempDir, "agent.yaml")
	deckPath := filepath.Join(tempDir, "deck.yaml")
	outputPath := filepath.Join(tempDir, "out.jsonl")

	writeModelFixture(t, modelPath, "test-model")
	writeReplaceTextAgentFixture(t, agentPath, "test-model")
	if err := os.WriteFile(filepath.Join(tempDir, "target.txt"), []byte("BEFORE marker"), 0o644); err != nil {
		t.Fatalf("write target.txt: %v", err)
	}

	deckRaw := "name: sandbox-edit\ncases:\n  - id: edit-case\n    agent: " + agentPath + "\n    model: " + modelPath + "\n    prompt: Edit the file.\n    sandbox: true\n    eval:\n      name: edit-case\n      required_file_contains:\n        - path: target.txt\n          terms: [\"AFTER\"]\n"
	if err := os.WriteFile(deckPath, []byte(deckRaw), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}

	prov := &stagedScriptedProvider{responses: []provider.Response{
		{Outputs: []provider.Output{provider.ToolRequestOutput{Call: provider.ToolCall{
			CallID:    "call-1",
			ToolName:  "replace_text",
			Arguments: map[string]string{"path": "target.txt", "old_text": "BEFORE", "new_text": "AFTER"},
		}}}},
		{Outputs: []provider.Output{provider.AssistantOutput{Content: `{"summary":"done"}`}}},
	}}

	config := RunnerConfig{DeckPath: deckPath, OutputPath: outputPath, Provider: prov, RootDir: tempDir}
	if err := RunDeck(config); err != nil {
		t.Fatalf("RunDeck: %v", err)
	}

	records, err := LoadRunRecords(outputPath)
	if err != nil {
		t.Fatalf("LoadRunRecords: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Error != "" {
		t.Fatalf("record error = %q", record.Error)
	}
	if !record.Passed {
		t.Fatalf("record Passed = false, want true; eval = %+v", record.Eval)
	}

	// The file_contains check must have passed.
	foundCheck := false
	for _, check := range record.Eval.Checks {
		if check.Name == "file_contains:target.txt" {
			if !check.Passed {
				t.Fatalf("file_contains check failed: %+v", check)
			}
			foundCheck = true
			break
		}
	}
	if !foundCheck {
		t.Fatalf("no file_contains:target.txt check in eval: %+v", record.Eval.Checks)
	}

	// Core safety assertion: the original source file is untouched.
	original, err := os.ReadFile(filepath.Join(tempDir, "target.txt"))
	if err != nil {
		t.Fatalf("read original target.txt: %v", err)
	}
	if !strings.Contains(string(original), "BEFORE") {
		t.Fatalf("original target.txt = %q, want to still contain %q", original, "BEFORE")
	}
	if strings.Contains(string(original), "AFTER") {
		t.Fatalf("original target.txt = %q, must NOT contain %q (edit should land only in sandbox)", original, "AFTER")
	}
}

// sandboxStagedProvider is a purpose-built provider for the staged-sandbox
// test. It inspects each request to decide the response:
//   - Workflow finalization requests (bucketed response format naming a
//     workflow bucket) consume the next scripted transition trigger.
//   - The first normal turn after a finalization answers with a plain valid
//     cognitive output so the session ends (assistant_only).
//   - Any other normal turn proposes a replace_text tool call (editing
//     target.txt BEFORE->AFTER), keeping the session alive until the workflow
//     bound (maxInferenceTurns: 1) forces finalization on the following turn.
type sandboxStagedProvider struct {
	transitions       []string
	transitionIndex   int
	afterFinalization bool
	editDone          bool
	callCounter       int
}

func (s *sandboxStagedProvider) BuildRequest(agent runtime.Agent, payload prompt.Payload, tools []provider.ToolDefinition) (provider.Request, error) {
	return provider.BuildRequest(agent, payload, tools)
}

func (s *sandboxStagedProvider) Send(request provider.Request) (provider.Response, error) {
	if wantsWorkflowBucket(request) {
		if s.transitionIndex >= len(s.transitions) {
			return provider.Response{}, nil
		}
		trigger := s.transitions[s.transitionIndex]
		s.transitionIndex++
		s.afterFinalization = true
		content := `{"workflow":{"summary":"artifact","transition":` + quoteJSON(trigger) + `}}`
		return provider.Response{Outputs: []provider.Output{provider.AssistantOutput{Content: content}}}, nil
	}
	if s.afterFinalization {
		s.afterFinalization = false
		return provider.Response{Outputs: []provider.Output{provider.AssistantOutput{Content: `{"summary":"ok"}`}}}, nil
	}
	if !s.editDone {
		s.editDone = true
		s.callCounter++
		args := map[string]string{"path": "target.txt", "old_text": "BEFORE", "new_text": "AFTER"}
		raw, _ := json.Marshal(args)
		call := provider.ToolCall{CallID: "call-1", ToolName: "replace_text", Arguments: args, RawArgs: raw}
		return provider.Response{Outputs: []provider.Output{provider.ToolRequestOutput{Call: call}}}, nil
	}
	return provider.Response{Outputs: []provider.Output{provider.AssistantOutput{Content: `{"summary":"ok"}`}}}, nil
}

// quoteJSON returns a JSON-encoded string literal for the given value.
func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestRunDeckStagedSandboxSpansStages(t *testing.T) {
	tempDir := t.TempDir()
	modelPath := filepath.Join(tempDir, "model.yaml")
	agentPath := filepath.Join(tempDir, "agent.yaml")
	workflowPath := filepath.Join(tempDir, "flow.yaml")
	deckPath := filepath.Join(tempDir, "deck.yaml")
	outputPath := filepath.Join(tempDir, "out.jsonl")

	writeModelFixture(t, modelPath, "test-model")
	writeReplaceTextAgentFixture(t, agentPath, "test-model")
	writeStagedWorkflowFixture(t, workflowPath)
	if err := os.WriteFile(filepath.Join(tempDir, "target.txt"), []byte("BEFORE marker"), 0o644); err != nil {
		t.Fatalf("write target.txt: %v", err)
	}

	deckRaw := "name: staged-sandbox\ncases:\n  - id: two-stage\n    workflow: " + workflowPath + "\n    model: " + modelPath + "\n    sandbox: true\n    stages:\n      - id: first\n        agent: " + agentPath + "\n        prompt: Edit the file in stage one.\n        eval:\n          name: first\n      - id: second\n        agent: " + agentPath + "\n        prompt: Check the file in stage two.\n        eval:\n          name: second\n          required_file_contains:\n            - path: target.txt\n              terms: [\"AFTER\"]\n"
	if err := os.WriteFile(deckPath, []byte(deckRaw), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}

	prov := &sandboxStagedProvider{transitions: []string{"triaged"}}
	config := RunnerConfig{DeckPath: deckPath, OutputPath: outputPath, Provider: prov, RootDir: tempDir}
	if err := RunDeck(config); err != nil {
		t.Fatalf("RunDeck: %v", err)
	}

	records, err := LoadRunRecords(outputPath)
	if err != nil {
		t.Fatalf("LoadRunRecords: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}

	for i, r := range records {
		t.Logf("record %d: %+v", i, r)
		if r.Error != "" {
			t.Fatalf("record %d error = %q", i, r.Error)
		}
	}

	// Stage 2's eval must see the AFTER edit in the shared sandbox.
	r2 := records[1]
	if r2.StageID != "second" {
		t.Fatalf("record 2 StageID = %q, want second", r2.StageID)
	}
	foundCheck := false
	for _, check := range r2.Eval.Checks {
		if check.Name == "file_contains:target.txt" {
			if !check.Passed {
				t.Fatalf("stage 2 file_contains check failed: %+v", check)
			}
			foundCheck = true
			break
		}
	}
	if !foundCheck {
		t.Fatalf("stage 2 missing file_contains:target.txt check: %+v", r2.Eval.Checks)
	}

	// Core safety assertion: the original source file is untouched.
	original, err := os.ReadFile(filepath.Join(tempDir, "target.txt"))
	if err != nil {
		t.Fatalf("read original target.txt: %v", err)
	}
	if !strings.Contains(string(original), "BEFORE") {
		t.Fatalf("original target.txt = %q, want to still contain %q", original, "BEFORE")
	}
	if strings.Contains(string(original), "AFTER") {
		t.Fatalf("original target.txt = %q, must NOT contain %q (edit should land only in sandbox)", original, "AFTER")
	}
}
