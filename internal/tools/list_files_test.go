package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/maelstrom/internal/logs"
	"github.com/comalice/maelstrom/internal/provider"
)

func TestListFilesToolListsTopLevelEntries(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "one.go"), []byte("package sample\n"), 0o644); err != nil {
		t.Fatalf("write one.go: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tempDir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	tool := ListFilesTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-list-001")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-list-001", ToolName: "list_files", Arguments: map[string]string{}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if !strings.Contains(result.DisplayContent, "one.go") {
		t.Fatalf("DisplayContent = %q, want one.go", result.DisplayContent)
	}
	if !strings.Contains(result.DisplayContent, "nested/") {
		t.Fatalf("DisplayContent = %q, want nested/", result.DisplayContent)
	}
}

func TestListFilesToolRecursiveAndGlob(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "pkg", "inner"), 0o755); err != nil {
		t.Fatalf("mkdir tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "pkg", "inner", "a.go"), []byte("package inner\n"), 0o644); err != nil {
		t.Fatalf("write a.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "pkg", "inner", "b.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}
	tool := ListFilesTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-list-002")
	request := ExecutionRequest{
		History: history,
		Call: provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-list-002", ToolName: "list_files", Arguments: map[string]string{
			"path":         "pkg",
			"recursive":    "true",
			"include_glob": "*.go",
		}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.DisplayContent, "b.txt") {
		t.Fatalf("DisplayContent = %q, did not expect b.txt", result.DisplayContent)
	}
	if !strings.Contains(result.DisplayContent, "pkg/inner/a.go") {
		t.Fatalf("DisplayContent = %q, want recursive go file", result.DisplayContent)
	}
}

func TestListFilesToolHonorsLimitMessage(t *testing.T) {
	tempDir := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if err := os.WriteFile(filepath.Join(tempDir, name), []byte("package sample\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	tool := ListFilesTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-list-003")
	request := ExecutionRequest{
		History: history,
		Call: provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-list-003", ToolName: "list_files", Arguments: map[string]string{
			"max_results": "2",
		}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.DisplayContent, "Showing first 2 entries") {
		t.Fatalf("DisplayContent = %q, want limit message", result.DisplayContent)
	}
	if strings.Count(result.DisplayContent, ".go") != 2 {
		t.Fatalf("DisplayContent = %q, want exactly 2 listed files", result.DisplayContent)
	}
}
