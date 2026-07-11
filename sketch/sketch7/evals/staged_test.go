package evals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

type stagedScriptedProvider struct {
	responses []provider.Response
	index     int
}

func (s *stagedScriptedProvider) BuildRequest(agent runtime.Agent, payload prompt.Payload, tools []provider.ToolDefinition) (provider.Request, error) {
	return provider.BuildRequest(agent, payload, tools)
}

func (s *stagedScriptedProvider) Send(request provider.Request) (provider.Response, error) {
	if s.index >= len(s.responses) {
		return provider.Response{}, fmt.Errorf("no scripted response for request %d", s.index)
	}
	response := s.responses[s.index]
	s.index++
	return response, nil
}

// stagedWorkflowProvider is behavior-driven rather than count-driven: it
// inspects each request to decide the response, so it does not depend on the
// exact number of turns the loop takes per stage.
//   - Workflow finalization requests (bucketed response format naming a
//     workflow bucket) consume the next scripted transition trigger.
//   - The first normal turn after a finalization answers with a plain valid
//     cognitive output so the session ends (assistant_only).
//   - Any other normal turn proposes a read_file tool call, keeping the
//     session alive until the workflow bound (maxInferenceTurns: 1) forces
//     finalization on the following turn.
type stagedWorkflowProvider struct {
	transitions       []string
	transitionIndex   int
	callCounter       int
	afterFinalization bool
}

func (s *stagedWorkflowProvider) BuildRequest(agent runtime.Agent, payload prompt.Payload, tools []provider.ToolDefinition) (provider.Request, error) {
	return provider.BuildRequest(agent, payload, tools)
}

func (s *stagedWorkflowProvider) Send(request provider.Request) (provider.Response, error) {
	if wantsWorkflowBucket(request) {
		if s.transitionIndex >= len(s.transitions) {
			return provider.Response{}, fmt.Errorf("no scripted transition for finalization request %d", s.transitionIndex)
		}
		trigger := s.transitions[s.transitionIndex]
		s.transitionIndex++
		s.afterFinalization = true
		content := fmt.Sprintf(`{"workflow":{"summary":"stage artifact","transition":%q}}`, trigger)
		return provider.Response{Outputs: []provider.Output{provider.AssistantOutput{Content: content}}}, nil
	}
	if s.afterFinalization {
		s.afterFinalization = false
		return provider.Response{Outputs: []provider.Output{provider.AssistantOutput{Content: `{"summary":"wrapping up"}`}}}, nil
	}
	s.callCounter++
	raw, _ := json.Marshal(map[string]string{"path": "notes.txt"})
	call := provider.ToolCall{CallID: fmt.Sprintf("call-%d", s.callCounter), ToolName: "read_file", Arguments: map[string]string{"path": "notes.txt"}, RawArgs: raw}
	return provider.Response{Outputs: []provider.Output{provider.ToolRequestOutput{Call: call}}}, nil
}

func wantsWorkflowBucket(request provider.Request) bool {
	if request.ResponseFormat == nil {
		return false
	}
	for _, bucket := range request.ResponseFormat.Buckets {
		if bucket.Name == "workflow" {
			return true
		}
	}
	return false
}

const stagedWorkflowYAML = `apiVersion: maelstrom/v1
kind: Workflow
name: staged-test-flow
description: Two-hop staged test workflow.
context: Test workflow context.
statechart:
  initialState: intake
  states:
    - name: intake
      description: Intake stage.
      allowedTriggers: [triaged]
      outputs:
        schema: intake_v1
        requiredFields: [summary, transition]
      bounds:
        maxInferenceTurns: 1
        maxFinalizationRetries: 1
    - name: review
      description: Review stage.
      allowedTriggers: [approved]
      outputs:
        schema: review_v1
        requiredFields: [summary, transition]
      bounds:
        maxInferenceTurns: 1
        maxFinalizationRetries: 1
    - name: done
      description: Complete.
  transitions:
    - trigger: triaged
      from: intake
      to: review
    - trigger: approved
      from: review
      to: done
`

func writeStagedWorkflowFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(stagedWorkflowYAML), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}

// writeToolAgentFixture writes an agent that can call read_file, so a scripted
// tool-call turn keeps the session alive until workflow bounds force
// finalization.
func writeToolAgentFixture(t *testing.T, path, modelName string) {
	t.Helper()
	raw := "apiVersion: maelstrom/v1\nkind: Agent\nname: staged-agent\nmodel: " + modelName + "\ntools:\n  - read_file\ncontext:\n  inputBudget: 2000\n  projections:\n    - type: state_task\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      prompt: Return a summary.\n      outputs:\n        schema: cognitive_step_v1\n        requiredFields: [summary]\n  transitions: []\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}
}

func TestLoadTaskDeckValidatesStagedCases(t *testing.T) {
	tempDir := t.TempDir()
	agentPath := filepath.Join(tempDir, "agent.yaml")
	writeAgentFixture(t, agentPath, "test-model")
	modelPath := filepath.Join(tempDir, "model.yaml")
	writeModelFixture(t, modelPath, "test-model")
	workflowPath := filepath.Join(tempDir, "flow.yaml")
	writeStagedWorkflowFixture(t, workflowPath)

	tests := []struct {
		name      string
		yaml      string
		wantErr   string
		checkDeck func(t *testing.T, deck TaskDeck)
	}{
		{
			name: "valid staged case with default stage id",
			yaml: "name: valid\ncases:\n  - id: two-stage\n    workflow: " + workflowPath + "\n    model: " + modelPath + "\n    stages:\n      - id: first\n        agent: " + agentPath + "\n        prompt: Do stage one.\n      - agent: " + agentPath + "\n        prompt: Do stage two.\n",
			checkDeck: func(t *testing.T, deck TaskDeck) {
				if len(deck.Cases[0].Stages) != 2 {
					t.Fatalf("stages = %d", len(deck.Cases[0].Stages))
				}
				if deck.Cases[0].Stages[0].ID != "first" {
					t.Fatalf("stage 0 id = %q", deck.Cases[0].Stages[0].ID)
				}
				if deck.Cases[0].Stages[1].ID != "stage-2" {
					t.Fatalf("stage 1 id = %q, want stage-2", deck.Cases[0].Stages[1].ID)
				}
				if deck.Cases[0].Stages[0].Eval.Name != "two-stage:first" {
					t.Fatalf("stage 0 eval name = %q", deck.Cases[0].Stages[0].Eval.Name)
				}
				if deck.Cases[0].Stages[1].Eval.Name != "two-stage:stage-2" {
					t.Fatalf("stage 1 eval name = %q, want two-stage:stage-2", deck.Cases[0].Stages[1].Eval.Name)
				}
			},
		},
		{
			name:    "stages without workflow",
			yaml:    "name: no-wf\ncases:\n  - id: c1\n    model: " + modelPath + "\n    stages:\n      - agent: " + agentPath + "\n        prompt: p\n",
			wantErr: "must set a workflow path",
		},
		{
			name:    "stages with case-level prompt",
			yaml:    "name: has-prompt\ncases:\n  - id: c1\n    workflow: " + workflowPath + "\n    model: " + modelPath + "\n    prompt: should not be here\n    stages:\n      - agent: " + agentPath + "\n        prompt: p\n",
			wantErr: "must not set a case-level prompt",
		},
		{
			name:    "stages with case-level agent",
			yaml:    "name: has-agent\ncases:\n  - id: c1\n    workflow: " + workflowPath + "\n    model: " + modelPath + "\n    agent: " + agentPath + "\n    stages:\n      - agent: " + agentPath + "\n        prompt: p\n",
			wantErr: "must not set a case-level agent",
		},
		{
			name:    "stage missing agent",
			yaml:    "name: no-agent\ncases:\n  - id: c1\n    workflow: " + workflowPath + "\n    model: " + modelPath + "\n    stages:\n      - prompt: p\n",
			wantErr: "missing agent",
		},
		{
			name:    "stage missing prompt",
			yaml:    "name: no-prompt\ncases:\n  - id: c1\n    workflow: " + workflowPath + "\n    model: " + modelPath + "\n    stages:\n      - agent: " + agentPath + "\n",
			wantErr: "missing prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deckPath := filepath.Join(tempDir, tt.name+".yaml")
			if err := os.WriteFile(deckPath, []byte(tt.yaml), 0o644); err != nil {
				t.Fatalf("write deck: %v", err)
			}
			deck, err := LoadTaskDeck(deckPath)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkDeck != nil {
				tt.checkDeck(t, deck)
			}
		})
	}
}

func TestRunDeckStagedCaseSharesWorkflow(t *testing.T) {
	tempDir := t.TempDir()
	modelPath := filepath.Join(tempDir, "model.yaml")
	agentPath := filepath.Join(tempDir, "agent.yaml")
	workflowPath := filepath.Join(tempDir, "flow.yaml")
	deckPath := filepath.Join(tempDir, "deck.yaml")
	outputPath := filepath.Join(tempDir, "out.jsonl")

	writeModelFixture(t, modelPath, "test-model")
	writeToolAgentFixture(t, agentPath, "test-model")
	writeStagedWorkflowFixture(t, workflowPath)
	if err := os.WriteFile(filepath.Join(tempDir, "notes.txt"), []byte("stage notes\n"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	deckRaw := "name: staged-smoke\ncases:\n  - id: two-stage\n    workflow: " + workflowPath + "\n    model: " + modelPath + "\n    stages:\n      - id: first\n        agent: " + agentPath + "\n        prompt: Do stage one.\n      - id: second\n        agent: " + agentPath + "\n        prompt: Do stage two.\n"
	if err := os.WriteFile(deckPath, []byte(deckRaw), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}

	// Stage 1 finalizes intake with "triaged" (intake -> review); stage 2
	// finalizes review with "approved" (review -> done). Stage 2 can only
	// legally fire "approved" if it observes the SHARED workflow instance
	// that stage 1 left in review.
	prov := &stagedWorkflowProvider{transitions: []string{"triaged", "approved"}}

	config := RunnerConfig{DeckPath: deckPath, OutputPath: outputPath, Provider: prov, RootDir: tempDir}
	if err := RunDeck(config); err != nil {
		t.Fatalf("RunDeck: %v", err)
	}

	records, err := LoadRunRecords(outputPath)
	if err != nil {
		t.Fatalf("LoadRunRecords: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}

	for i, r := range records {
		t.Logf("record %d: %+v", i, r)
		t.Logf("record %d stats: %+v", i, r.Stats)
	}

	r1 := records[0]
	r2 := records[1]

	if r1.StageID != "first" {
		t.Fatalf("record 1 StageID = %q, want first", r1.StageID)
	}
	if r2.StageID != "second" {
		t.Fatalf("record 2 StageID = %q, want second", r2.StageID)
	}
	if r1.StageIndex != 1 {
		t.Fatalf("record 1 StageIndex = %d, want 1", r1.StageIndex)
	}
	if r2.StageIndex != 2 {
		t.Fatalf("record 2 StageIndex = %d, want 2", r2.StageIndex)
	}
	if r1.RunID == r2.RunID {
		t.Fatalf("run IDs not distinct: %s", r1.RunID)
	}
	if !strings.Contains(r1.RunID, "stage1-first") {
		t.Fatalf("record 1 RunID = %q, want stage1-first", r1.RunID)
	}
	if !strings.Contains(r2.RunID, "stage2-second") {
		t.Fatalf("record 2 RunID = %q, want stage2-second", r2.RunID)
	}
	if r1.Error != "" {
		t.Fatalf("record 1 Error = %q", r1.Error)
	}
	if r2.Error != "" {
		t.Fatalf("record 2 Error = %q", r2.Error)
	}
	if r1.Stats.FinalWorkflowState != "review" {
		t.Fatalf("record 1 FinalWorkflowState = %q, want review", r1.Stats.FinalWorkflowState)
	}
	if r2.Stats.FinalWorkflowState != "done" {
		t.Fatalf("record 2 FinalWorkflowState = %q, want done", r2.Stats.FinalWorkflowState)
	}
	// Both stages must have advanced through REAL workflow finalization (bound
	// hit -> finalization turn -> transition), not some incidental path.
	if r1.Stats.Finalization.ByBoundReason["workflow_max_inference_turns"] != 1 {
		t.Fatalf("record 1 finalization reasons = %+v, want workflow_max_inference_turns: 1", r1.Stats.Finalization.ByBoundReason)
	}
	if r2.Stats.Finalization.ByBoundReason["workflow_max_inference_turns"] != 1 {
		t.Fatalf("record 2 finalization reasons = %+v, want workflow_max_inference_turns: 1", r2.Stats.Finalization.ByBoundReason)
	}

	// Resume: should add no new lines.
	config.Provider = &stagedWorkflowProvider{transitions: []string{"triaged", "approved"}}
	if err := RunDeck(config); err != nil {
		t.Fatalf("resume RunDeck: %v", err)
	}
	records2, err := LoadRunRecords(outputPath)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if len(records2) != 2 {
		t.Fatalf("got %d records after resume, want 2", len(records2))
	}
}

func TestRunDeckStagedCaseSkipsAfterFailure(t *testing.T) {
	tempDir := t.TempDir()
	modelPath := filepath.Join(tempDir, "model.yaml")
	agentPath := filepath.Join(tempDir, "agent.yaml")
	workflowPath := filepath.Join(tempDir, "flow.yaml")
	deckPath := filepath.Join(tempDir, "deck.yaml")
	outputPath := filepath.Join(tempDir, "out.jsonl")

	writeModelFixture(t, modelPath, "test-model")
	writeAgentFixture(t, agentPath, "test-model")
	writeStagedWorkflowFixture(t, workflowPath)

	deckRaw := "name: staged-fail\ncases:\n  - id: two-stage\n    workflow: " + workflowPath + "\n    model: " + modelPath + "\n    stages:\n      - id: first\n        agent: " + agentPath + "\n        prompt: Do stage one.\n      - id: second\n        agent: " + agentPath + "\n        prompt: Do stage two.\n"
	if err := os.WriteFile(deckPath, []byte(deckRaw), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}

	// Empty responses: first Send errors.
	prov := &stagedScriptedProvider{responses: nil}
	config := RunnerConfig{DeckPath: deckPath, OutputPath: outputPath, Provider: prov, RootDir: tempDir}
	if err := RunDeck(config); err != nil {
		t.Fatalf("RunDeck: %v", err)
	}

	records, err := LoadRunRecords(outputPath)
	if err != nil {
		t.Fatalf("LoadRunRecords: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}

	for i, r := range records {
		t.Logf("record %d: %+v", i, r)
	}

	r1 := records[0]
	r2 := records[1]

	if r1.Error == "" {
		t.Fatalf("record 1 should have error, got empty")
	}
	if r2.Error != "skipped: prior stage failed" {
		t.Fatalf("record 2 Error = %q, want 'skipped: prior stage failed'", r2.Error)
	}
	if r2.Passed {
		t.Fatalf("record 2 Passed = true, want false")
	}
	if r2.StageID != "second" {
		t.Fatalf("record 2 StageID = %q, want second", r2.StageID)
	}
	// The skipped record must land under the exact stage run ID so resume
	// treats the whole case as complete.
	if !strings.Contains(r2.RunID, "stage2-second") {
		t.Fatalf("record 2 RunID = %q, want stage2-second component", r2.RunID)
	}

	// Resume: skipped records count as completed, no new lines.
	prov2 := &stagedScriptedProvider{responses: nil}
	config.Provider = prov2
	if err := RunDeck(config); err != nil {
		t.Fatalf("resume RunDeck: %v", err)
	}
	records2, err := LoadRunRecords(outputPath)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if len(records2) != 2 {
		t.Fatalf("got %d records after resume, want 2", len(records2))
	}
}

func TestLoadWorkflowTeamPlanningDeck(t *testing.T) {
	deck, err := LoadTaskDeck(filepath.Join("decks", "workflow-team-planning.yaml"))
	if err != nil {
		t.Fatalf("LoadTaskDeck: %v", err)
	}
	if len(deck.Models) != 3 {
		t.Fatalf("models = %d, want 3", len(deck.Models))
	}
	if len(deck.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(deck.Cases))
	}
	tc := deck.Cases[0]
	if tc.WorkflowPath != "sketch/sketch7/workflows/change-planning.yaml" {
		t.Fatalf("workflow = %q", tc.WorkflowPath)
	}
	if tc.TimeoutSeconds != 360 {
		t.Fatalf("timeout = %d, want inherited 360", tc.TimeoutSeconds)
	}
	// The relay contract: stage IDs mirror the workflow states and each
	// stage's eval pins the state the NEXT stage should bind into.
	wantStages := []struct{ id, nextState string }{
		{"intake", "inspecting"},
		{"inspecting", "planning"},
		{"planning", "reviewing"},
		{"reviewing", "done"},
	}
	if len(tc.Stages) != len(wantStages) {
		t.Fatalf("stages = %d, want %d", len(tc.Stages), len(wantStages))
	}
	for i, want := range wantStages {
		stage := tc.Stages[i]
		if stage.ID != want.id {
			t.Fatalf("stage %d id = %q, want %q", i, stage.ID, want.id)
		}
		if stage.Eval.RequiredFinalWorkflowState != want.nextState {
			t.Fatalf("stage %q required final state = %q, want %q", stage.ID, stage.Eval.RequiredFinalWorkflowState, want.nextState)
		}
		if stage.Eval.MinValidWorkflowOutputs != 1 {
			t.Fatalf("stage %q min valid workflow outputs = %d, want 1", stage.ID, stage.Eval.MinValidWorkflowOutputs)
		}
		if !stage.Eval.RequireCompleted {
			t.Fatalf("stage %q must require completion", stage.ID)
		}
		if strings.TrimSpace(stage.Prompt) == "" {
			t.Fatalf("stage %q missing prompt", stage.ID)
		}
		if _, err := os.Stat(filepath.Join("..", "..", "..", stage.Agent)); err != nil {
			t.Fatalf("stage %q agent path: %v", stage.ID, err)
		}
	}
	// Only the intake prompt carries the change request; later stages must
	// recover it from workflow artifacts (the handoff under test).
	if !strings.Contains(tc.Stages[0].Prompt, "Change request:") {
		t.Fatalf("intake prompt missing change request")
	}
	for _, stage := range tc.Stages[1:] {
		if strings.Contains(stage.Prompt, "Change request:") {
			t.Fatalf("stage %q prompt restates the change request; it must rely on workflow artifacts", stage.ID)
		}
		if !strings.Contains(stage.Prompt, "workflow artifacts") {
			t.Fatalf("stage %q prompt should direct the agent to the workflow artifacts", stage.ID)
		}
	}
	for _, pathValue := range append([]string{tc.WorkflowPath}, deck.Models...) {
		if _, err := os.Stat(filepath.Join("..", "..", "..", pathValue)); err != nil {
			t.Fatalf("deck path: %v", err)
		}
	}
}

func TestSummarizeRunsGroupsByStage(t *testing.T) {
	records := []RunRecord{
		{CaseID: "c1", StageID: "s1", Passed: true, RunID: "r1"},
		{CaseID: "c1", StageID: "s2", Passed: false, RunID: "r2"},
		{CaseID: "c1", StageID: "", Passed: true, RunID: "r3"},
	}

	summary := SummarizeRuns(records)

	if len(summary.ByStage) != 2 {
		t.Fatalf("ByStage keys = %d, want 2: %+v", len(summary.ByStage), summary.ByStage)
	}
	s1 := summary.ByStage["c1:s1"]
	if s1.Total != 1 || s1.Passed != 1 || s1.PassRate != 1.0 {
		t.Fatalf("c1:s1 = %+v, want total=1 passed=1 passRate=1.0", s1)
	}
	s2 := summary.ByStage["c1:s2"]
	if s2.Total != 1 || s2.Passed != 0 || s2.PassRate != 0.0 {
		t.Fatalf("c1:s2 = %+v, want total=1 passed=0 passRate=0.0", s2)
	}

	// ByCase should aggregate all three.
	c1 := summary.ByCase["c1"]
	if c1.Total != 3 || c1.Passed != 2 {
		t.Fatalf("ByCase c1 = %+v, want total=3 passed=2", c1)
	}
	if c1.PassRate != 2.0/3.0 {
		t.Fatalf("ByCase c1 PassRate = %v, want %v", c1.PassRate, 2.0/3.0)
	}

	// Verify JSON serialization includes by_stage.
	raw, _ := json.Marshal(summary)
	if !strings.Contains(string(raw), "by_stage") {
		t.Fatalf("JSON output missing by_stage field")
	}
}
