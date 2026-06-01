package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
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
