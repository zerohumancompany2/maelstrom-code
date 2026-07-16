package tools

import (
	"os"
	"path/filepath"
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

func runCommandRequest(callID, command string, extra map[string]string) ExecutionRequest {
	args := map[string]string{"command": command}
	for key, value := range extra {
		args[key] = value
	}
	return ExecutionRequest{
		History: logs.NewSessionHistory("session-" + callID),
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: callID, ToolName: "run_command", Arguments: args}},
	}
}

func TestRunCommandToolAllowlistPermitsExactCommand(t *testing.T) {
	tool := RunCommandTool{RootDir: t.TempDir(), DefaultTimeout: 5 * time.Second, Allowlist: []string{"printf hello"}}
	result, err := tool.Execute(runCommandRequest("call-allow-001", "printf hello", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}
	if !strings.Contains(result.DisplayContent, "hello") {
		t.Fatalf("DisplayContent = %q, want command output", result.DisplayContent)
	}
}

func TestRunCommandToolAllowlistRejectsUnlistedCommand(t *testing.T) {
	tool := RunCommandTool{RootDir: t.TempDir(), DefaultTimeout: 5 * time.Second, Allowlist: []string{"go test", "go build"}}
	for _, command := range []string{
		"rm -rf /",
		"go testfoo", // token-boundary: prefix must match whole tokens
		"gofmt -l .", // "go" prefix does not cover gofmt
		"bash -c 'go test'",
		"go test ./...", // arguments may not extend an allowlisted command
	} {
		result, err := tool.Execute(runCommandRequest("call-allow-002", command, nil))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", command, err)
		}
		if !result.IsError || !strings.Contains(result.DisplayContent, "allowlist") {
			t.Fatalf("command %q: expected allowlist rejection, got %+v", command, result)
		}
	}
}

func TestRunCommandToolAllowlistExecutesWithoutShell(t *testing.T) {
	tempDir := t.TempDir()
	command := "printf hi; touch " + tempDir + "/pwned"
	tool := RunCommandTool{RootDir: tempDir, DefaultTimeout: 5 * time.Second, Allowlist: []string{command}}
	// Under bash -lc this would chain into touch; without a shell the
	// metacharacters are inert literal arguments to printf.
	result, err := tool.Execute(runCommandRequest("call-allow-003", command, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}
	if _, statErr := os.Stat(tempDir + "/pwned"); statErr == nil {
		t.Fatal("shell chaining executed: pwned file exists")
	}
}

func TestRunCommandToolAllowlistClampsWorkdir(t *testing.T) {
	tool := RunCommandTool{RootDir: t.TempDir(), DefaultTimeout: 5 * time.Second, Allowlist: []string{"printf hi"}}
	for _, workdir := range []string{"/", "../..", "/tmp"} {
		result, err := tool.Execute(runCommandRequest("call-allow-004", "printf hi", map[string]string{"workdir": workdir}))
		if err != nil {
			t.Fatalf("unexpected error for workdir %q: %v", workdir, err)
		}
		if !result.IsError || !strings.Contains(result.DisplayContent, "escapes the tool root") {
			t.Fatalf("workdir %q: expected escape rejection, got %+v", workdir, result)
		}
	}
}

func TestRunCommandToolAllowlistRejectsSymlinkedWorkdir(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	tool := RunCommandTool{RootDir: root, DefaultTimeout: 5 * time.Second, Allowlist: []string{"printf hi"}}
	result, err := tool.Execute(runCommandRequest("call-allow-symlink", "printf hi", map[string]string{"workdir": "linked"}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.DisplayContent, "escapes the tool root") {
		t.Fatalf("result = %+v, want symlinked workdir rejection", result)
	}
}

func TestRunCommandToolAllowlistDefinitionNamesPrefixes(t *testing.T) {
	tool := RunCommandTool{RootDir: t.TempDir(), Allowlist: []string{"go test", "go build"}}
	def := tool.Definition()
	if !strings.Contains(def.Description, "go test") || !strings.Contains(def.Description, "go build") {
		t.Fatalf("Description = %q, want allowlist prefixes surfaced", def.Description)
	}
	plain := RunCommandTool{RootDir: t.TempDir()}
	if strings.Contains(plain.Definition().Description, "allowed") {
		t.Fatalf("plain description should not mention allowlist: %q", plain.Definition().Description)
	}
}
