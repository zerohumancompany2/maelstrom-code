package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/maelstrom/internal/logs"
	"github.com/comalice/maelstrom/internal/provider"
)

func TestGetFileSkeletonToolBuildsGoOutline(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sample.go")
	content := `package sample

type Worker struct{}

var version = "v1"

func Run() {}

func (w *Worker) Execute() {}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := GetFileSkeletonTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-skel-001")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-skel-001", ToolName: "get_file_skeleton", Arguments: map[string]string{"path": "sample.go"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if !strings.Contains(result.DisplayContent, "package sample") {
		t.Fatalf("DisplayContent = %q, want package line", result.DisplayContent)
	}
	if !strings.Contains(result.DisplayContent, "type Worker struct") {
		t.Fatalf("DisplayContent = %q, want type summary", result.DisplayContent)
	}
	if !strings.Contains(result.DisplayContent, "func Run(...)") {
		t.Fatalf("DisplayContent = %q, want function summary", result.DisplayContent)
	}
	if !strings.Contains(result.DisplayContent, "func (*Worker) Execute(...)") {
		t.Fatalf("DisplayContent = %q, want method summary", result.DisplayContent)
	}
}

func TestGetFileSkeletonToolRejectsUnsupportedFileType(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tool := GetFileSkeletonTool{RootDir: tempDir}
	history := logs.NewSessionHistory("session-skel-002")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-skel-002", ToolName: "get_file_skeleton", Arguments: map[string]string{"path": "sample.txt"}}},
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
