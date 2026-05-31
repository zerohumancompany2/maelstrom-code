package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
)

func TestReadFileToolReadsEntireFile(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "example.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := ReadFileTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-read-001")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-read-001", ToolName: "read_file", Arguments: map[string]string{"path": "example.txt"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if !strings.Contains(result.DisplayContent, "1: alpha") || !strings.Contains(result.DisplayContent, "3: gamma") {
		t.Fatalf("DisplayContent = %q, want numbered full file", result.DisplayContent)
	}
}

func TestReadFileToolReadsLineRange(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "example.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\ndelta\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := ReadFileTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-read-002")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-read-002", ToolName: "read_file", Arguments: map[string]string{"path": "example.txt", "start_line": "2", "end_line": "3"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DisplayContent != "2: beta\n3: gamma" {
		t.Fatalf("DisplayContent = %q, want 2: beta\\n3: gamma", result.DisplayContent)
	}
}

func TestReadFileToolErrorsOnMissingFile(t *testing.T) {
	tool := ReadFileTool{RootDir: t.TempDir()}
	history := logs.NewSessionHistory("session-read-003")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-read-003", ToolName: "read_file", Arguments: map[string]string{"path": "missing.txt"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestReadFileToolErrorsOnInvalidRange(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "example.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := ReadFileTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-read-004")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-read-004", ToolName: "read_file", Arguments: map[string]string{"path": "example.txt", "start_line": "3", "end_line": "2"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if !strings.Contains(result.DisplayContent, "start_line cannot be greater than end_line") {
		t.Fatalf("DisplayContent = %q, want range error", result.DisplayContent)
	}
}
