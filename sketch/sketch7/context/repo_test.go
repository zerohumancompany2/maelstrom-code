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

func TestBuildSectionsWorkflowStateIncludesProgressReadiness(t *testing.T) {
	agentDef := defs.AgentDefinition{Context: defs.ContextDefinition{Projections: []defs.ProjectionDefinition{{Type: "workflow_state"}}}}
	history := logs.NewSessionHistory("session-workflow-progress-001")
	history.Append(logs.WorkflowTransitionRefRecord{SessionBaseRecord: history.NextRecord("workflow_transition_ref"), WorkflowID: "wf-001", FromState: "intake", ToState: "repo_orientation", Trigger: "begin_repo_orientation"})
	session := runtime.SessionView{
		Workflow: &runtime.WorkflowView{
			WorkflowID:   "wf-001",
			CurrentState: "repo_orientation",
			Description:  "Conversation to execution",
			Context:      "Use this workflow for coding tasks.",
		},
	}
	sections := BuildSections(agentDef, session, history, RepoContextOptions{})
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	content := sections[0].Content
	if !strings.Contains(content, "Completed checkpoints: repo_orientation") {
		t.Fatalf("content = %q, want completed checkpoint info", content)
	}
	if !strings.Contains(content, "Remaining before planning: clarification") {
		t.Fatalf("content = %q, want remaining clarification checkpoint", content)
	}
	if !strings.Contains(content, "Planning readiness: not ready") {
		t.Fatalf("content = %q, want readiness status", content)
	}
}
