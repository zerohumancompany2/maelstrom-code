package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
)

func TestReplaceTextToolReplacesExactSingleMatch(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "example.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := ReplaceTextTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-replace-001")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-replace-001", ToolName: "replace_text", Arguments: map[string]string{"path": "example.txt", "old_text": "beta", "new_text": "BETA"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	if string(updated) != "alpha\nBETA\ngamma\n" {
		t.Fatalf("updated file = %q, want replaced content", string(updated))
	}
}

func TestReplaceTextToolErrorsWhenTextMissing(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "example.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := ReplaceTextTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-replace-002")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-replace-002", ToolName: "replace_text", Arguments: map[string]string{"path": "example.txt", "old_text": "missing", "new_text": "new"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if !strings.Contains(result.DisplayContent, "old_text not found") {
		t.Fatalf("DisplayContent = %q, want missing-text error", result.DisplayContent)
	}
}

func TestReplaceTextToolErrorsWhenTextMatchesMoreThanOnce(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "example.txt")
	if err := os.WriteFile(path, []byte("beta\nalpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := ReplaceTextTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-replace-003")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-replace-003", ToolName: "replace_text", Arguments: map[string]string{"path": "example.txt", "old_text": "beta", "new_text": "BETA"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if !strings.Contains(result.DisplayContent, "matched 2 times") {
		t.Fatalf("DisplayContent = %q, want exact-once error", result.DisplayContent)
	}
}

func TestReplaceTextToolErrorsWhenFileMissing(t *testing.T) {
	tool := ReplaceTextTool{RootDir: t.TempDir()}
	history := logs.NewSessionHistory("session-replace-004")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-replace-004", ToolName: "replace_text", Arguments: map[string]string{"path": "missing.txt", "old_text": "x", "new_text": "y"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}
