package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/maelstrom/internal/logs"
	"github.com/comalice/maelstrom/internal/provider"
)

func TestFindReferencesToolUsesInternalFallback(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "one.go"), []byte("package sample\nfunc Run() {\n\tExecute()\n}\n"), 0o644); err != nil {
		t.Fatalf("write one.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "two.go"), []byte("package sample\nfunc Execute() {}\n"), 0o644); err != nil {
		t.Fatalf("write two.go: %v", err)
	}
	tool := FindReferencesTool{
		RootDir: tempDir,
		LookupPath: func(name string) (string, error) {
			return "", os.ErrNotExist
		},
		MaxResults: 10,
	}
	history := logs.NewSessionHistory("session-refs-001")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-refs-001", ToolName: "find_references", Arguments: map[string]string{"symbol": "Execute", "include_glob": "*.go"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if !strings.Contains(result.DisplayContent, "Symbol: Execute") {
		t.Fatalf("DisplayContent = %q, want symbol header", result.DisplayContent)
	}
	if !strings.Contains(result.DisplayContent, "one.go:3:") {
		t.Fatalf("DisplayContent = %q, want reference match", result.DisplayContent)
	}
}

func TestFindReferencesToolErrorsOnMissingSymbol(t *testing.T) {
	tool := FindReferencesTool{RootDir: t.TempDir()}
	history := logs.NewSessionHistory("session-refs-002")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-refs-002", ToolName: "find_references", Arguments: map[string]string{}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestFindReferencesToolReportsNoMatches(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "one.go"), []byte("package sample\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatalf("write one.go: %v", err)
	}
	tool := FindReferencesTool{
		RootDir: tempDir,
		LookupPath: func(name string) (string, error) {
			return "", os.ErrNotExist
		},
	}
	history := logs.NewSessionHistory("session-refs-003")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-refs-003", ToolName: "find_references", Arguments: map[string]string{"symbol": "Execute"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if !strings.Contains(result.DisplayContent, "No references found") {
		t.Fatalf("DisplayContent = %q, want no-matches message", result.DisplayContent)
	}
}
