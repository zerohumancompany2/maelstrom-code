package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/maelstrom/internal/logs"
	"github.com/comalice/maelstrom/internal/provider"
)

func TestReadSymbolToolReadsMethodDefinition(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sample.go")
	content := `package sample

type Worker struct{}

func (w *Worker) Execute() string {
	return "done"
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := ReadSymbolTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-symbol-001")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-symbol-001", ToolName: "read_symbol", Arguments: map[string]string{"path": "sample.go", "symbol": "Execute"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if !strings.Contains(result.DisplayContent, "sample.go::*Worker.Execute") && !strings.Contains(result.DisplayContent, "sample.go::Worker.Execute") {
		t.Fatalf("DisplayContent = %q, want method header", result.DisplayContent)
	}
	if !strings.Contains(result.DisplayContent, `return "done"`) {
		t.Fatalf("DisplayContent = %q, want method body", result.DisplayContent)
	}
}

func TestReadSymbolToolReadsTypeDefinition(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sample.go")
	content := `package sample

type Worker struct {
	Name string
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := ReadSymbolTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-symbol-002")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-symbol-002", ToolName: "read_symbol", Arguments: map[string]string{"path": "sample.go", "symbol": "Worker"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if !strings.Contains(result.DisplayContent, "sample.go::Worker") {
		t.Fatalf("DisplayContent = %q, want type header", result.DisplayContent)
	}
}

func TestReadSymbolToolErrorsOnMissingSymbol(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sample.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := ReadSymbolTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-symbol-003")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-symbol-003", ToolName: "read_symbol", Arguments: map[string]string{"path": "sample.go", "symbol": "Missing"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestReadSymbolToolRejectsUnsupportedFileType(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := ReadSymbolTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-symbol-004")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-symbol-004", ToolName: "read_symbol", Arguments: map[string]string{"path": "sample.txt", "symbol": "hello"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if !strings.Contains(result.DisplayContent, "unsupported file type") {
		t.Fatalf("DisplayContent = %q, want unsupported type error", result.DisplayContent)
	}
}
