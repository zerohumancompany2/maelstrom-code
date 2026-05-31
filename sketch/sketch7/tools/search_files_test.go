package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
)

func TestSearchFilesToolUsesInternalFallbackWhenExternalToolsUnavailable(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "one.go"), []byte("package sample\nfunc ReduceBindingState() {}\n"), 0o644); err != nil {
		t.Fatalf("write file one: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "two.txt"), []byte("nothing here\n"), 0o644); err != nil {
		t.Fatalf("write file two: %v", err)
	}
	tool := SearchFilesTool{
		RootDir:        tempDir,
		DefaultTimeout: 5 * time.Second,
		LookupPath: func(name string) (string, error) {
			return "", os.ErrNotExist
		},
	}
	history := logs.NewSessionHistory("session-search-001")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-search-001", ToolName: "search_files", Arguments: map[string]string{"pattern": "ReduceBindingState"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if !strings.Contains(result.DisplayContent, "Backend: internal") {
		t.Fatalf("DisplayContent = %q, want internal backend marker", result.DisplayContent)
	}
	if !strings.Contains(result.DisplayContent, "one.go:2: func ReduceBindingState() {}") {
		t.Fatalf("DisplayContent = %q, want matching file result", result.DisplayContent)
	}
}

func TestSearchFilesToolHonorsIncludeGlob(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "one.go"), []byte("package sample\nfunc ReduceBindingState() {}\n"), 0o644); err != nil {
		t.Fatalf("write one.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "two.txt"), []byte("ReduceBindingState\n"), 0o644); err != nil {
		t.Fatalf("write two.txt: %v", err)
	}
	tool := SearchFilesTool{
		RootDir:        tempDir,
		DefaultTimeout: 5 * time.Second,
		LookupPath: func(name string) (string, error) {
			return "", os.ErrNotExist
		},
	}
	history := logs.NewSessionHistory("session-search-002")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-search-002", ToolName: "search_files", Arguments: map[string]string{"pattern": "ReduceBindingState", "include_glob": "*.go"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.DisplayContent, "two.txt") {
		t.Fatalf("DisplayContent = %q, did not expect txt file in filtered results", result.DisplayContent)
	}
}

func TestSearchFilesToolErrorsOnMissingPattern(t *testing.T) {
	tool := SearchFilesTool{RootDir: t.TempDir(), DefaultTimeout: 5 * time.Second}
	history := logs.NewSessionHistory("session-search-003")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-search-003", ToolName: "search_files", Arguments: map[string]string{}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestSearchFilesToolParsesSearchOutput(t *testing.T) {
	output := "./a.go:12: func A() {}\n./b.go:3: func B() {}\n"
	matches := parseSearchOutput(output)
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(matches))
	}
	if matches[0].Path != "a.go" || matches[0].Line != 12 {
		t.Fatalf("first match = %+v, want a.go:12", matches[0])
	}
}
