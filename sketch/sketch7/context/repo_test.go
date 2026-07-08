package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

func TestBuildRepoSummaryIncludesFileTypesAndTopLevelEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "cmd"), 0o755); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# example\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	summary := buildRepoSummary(RepoContextOptions{RootDir: root, MaxFilesToInspect: 50, MaxTopLevelEntries: 5, MaxExtensionsToShow: 3})
	if !strings.Contains(summary, "Repository context:") {
		t.Fatalf("summary = %q, want header", summary)
	}
	if !strings.Contains(summary, ".go") {
		t.Fatalf("summary = %q, want .go extension summary", summary)
	}
	if !strings.Contains(summary, "go.mod") {
		t.Fatalf("summary = %q, want marker file", summary)
	}
	if !strings.Contains(summary, "cmd/") {
		t.Fatalf("summary = %q, want top-level directory", summary)
	}
}

func TestRepoContextCacheRefreshesByTurnCount(t *testing.T) {
	cache := RepoContextCache{}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	history := logs.NewSessionHistory("session-repo-001")
	first := cache.Summary(RepoContextOptions{RootDir: root, RefreshEveryTurns: 10}, history)
	if first == "" {
		t.Fatal("expected non-empty summary")
	}
	history.Append(logs.UserMessageRecord{SessionBaseRecord: history.NextRecord("user"), Content: "hello"})
	second := cache.Summary(RepoContextOptions{RootDir: root, RefreshEveryTurns: 10}, history)
	if second != first {
		t.Fatalf("expected cached summary before refresh window; first=%q second=%q", first, second)
	}
}

func TestBuildSectionsStateTaskRendersTaskFacingGuidance(t *testing.T) {
	agentDef := defs.AgentDefinition{Context: defs.ContextDefinition{Projections: []defs.ProjectionDefinition{{Type: "state_task"}}}}
	history := logs.NewSessionHistory("session-workflow-progress-001")
	history.Append(logs.WorkflowTransitionRefRecord{SessionBaseRecord: history.NextRecord("workflow_transition_ref"), WorkflowID: "wf-001", FromState: "intake", ToState: "repo_orientation", Trigger: "begin_repo_orientation"})
	session := runtime.SessionView{
		Agent: runtime.Agent{ToolNames: []string{"read_file", "run_command", "replace_text"}},
		Cognitive: runtime.CognitiveView{
			Prompt:       "Gather evidence before editing.",
			EnabledTools: []string{"read_file", "run_command"},
			Inputs:       defs.StateInputContract{Required: []string{"task_statement"}, Optional: []string{"repo_context"}},
			Outputs:      defs.StateOutputContract{SchemaName: "cognitive_step_v1", RequiredFields: []string{"summary", "completion_signal"}},
			Bounds:       defs.StateBoundsContract{MaxInferenceTurns: 3, MaxToolCalls: 6, MaxFinalizationRetries: 1},
		},
		Workflow: &runtime.WorkflowView{
			WorkflowID:      "wf-001",
			CurrentState:    "repo_orientation",
			Description:     "Conversation to execution",
			Context:         "Use this workflow for coding tasks.",
			EnabledTools:    []string{"read_file"},
			Inputs:          defs.StateInputContract{Required: []string{"acceptance_criteria"}},
			Outputs:         defs.StateOutputContract{SchemaName: "workflow_step_v1", RequiredFields: []string{"artifact_status"}},
			Completion:      defs.StateCompletionContract{SuccessWhen: []string{"artifact_status == ready"}},
			AllowedTriggers: []string{"orientation_complete"},
		},
	}
	sections := BuildSections(agentDef, session, history, RepoContextOptions{})
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	content := sections[0].Content
	if sections[0].LogicalKey != "state_task" || sections[0].SourceKind != "derived_state_task" {
		t.Fatalf("section = %+v, want state_task derived section", sections[0])
	}
	if !strings.Contains(content, "Current task: Gather evidence before editing.") {
		t.Fatalf("content = %q, want current task prompt", content)
	}
	if !strings.Contains(content, "Available tools: read_file") {
		t.Fatalf("content = %q, want intersected available tools", content)
	}
	if !strings.Contains(content, "Required output schema: cognitive_step_v1") {
		t.Fatalf("content = %q, want cognitive output schema", content)
	}
	if !strings.Contains(content, "Workflow output schema: workflow_step_v1") {
		t.Fatalf("content = %q, want workflow output schema", content)
	}
	if !strings.Contains(content, "Workflow next-step signals: orientation_complete") {
		t.Fatalf("content = %q, want workflow next-step signals", content)
	}
	if !strings.Contains(content, "Required inputs: task_statement") {
		t.Fatalf("content = %q, want cognitive input expectations", content)
	}
	if !strings.Contains(content, "Task bounds: max inference turns=3; max tool calls=6; max finalization retries=1") {
		t.Fatalf("content = %q, want cognitive bounds", content)
	}
	for _, forbidden := range []string{"Cognitive mode", "Workflow directive", "Suggested next transitions"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("content = %q, must not contain old state-machine phrase %q", content, forbidden)
		}
	}
}
