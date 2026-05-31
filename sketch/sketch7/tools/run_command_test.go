package tools

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
)

func TestRunCommandToolSuccess(t *testing.T) {
	tool := RunCommandTool{RootDir: t.TempDir(), DefaultTimeout: 5 * time.Second}
	history := logs.NewSessionHistory("session-cmd-001")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-cmd-001", ToolName: "run_command", Arguments: map[string]string{"command": "printf 'hello'"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if !strings.Contains(result.DisplayContent, "Exit code: 0") || !strings.Contains(result.DisplayContent, "hello") {
		t.Fatalf("DisplayContent = %q, want success output", result.DisplayContent)
	}
}

func TestRunCommandToolNonZeroExitIsError(t *testing.T) {
	tool := RunCommandTool{RootDir: t.TempDir(), DefaultTimeout: 5 * time.Second}
	history := logs.NewSessionHistory("session-cmd-002")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-cmd-002", ToolName: "run_command", Arguments: map[string]string{"command": "echo fail >&2; exit 7"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if !strings.Contains(result.DisplayContent, "Exit code: 7") {
		t.Fatalf("DisplayContent = %q, want exit code 7", result.DisplayContent)
	}
	if !strings.Contains(result.DisplayContent, "fail") {
		t.Fatalf("DisplayContent = %q, want stderr output", result.DisplayContent)
	}
}

func TestRunCommandToolTimeoutIsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash sleep timing not portable to windows")
	}
	tool := RunCommandTool{RootDir: t.TempDir(), DefaultTimeout: 5 * time.Second}
	history := logs.NewSessionHistory("session-cmd-003")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-cmd-003", ToolName: "run_command", Arguments: map[string]string{"command": "sleep 2", "timeout_seconds": "1"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected timeout error result")
	}
	if !strings.Contains(result.DisplayContent, "Timed out: true") {
		t.Fatalf("DisplayContent = %q, want timeout marker", result.DisplayContent)
	}
}
