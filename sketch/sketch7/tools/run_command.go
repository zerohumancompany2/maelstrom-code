package tools

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RunCommandTool struct {
	RootDir        string
	DefaultTimeout time.Duration
}

func (RunCommandTool) Definition() Definition {
	return Definition{
		Name:        "run_command",
		Description: "Run a shell command for validation or inspection",
		Parameters: map[string]string{
			"command":         "string",
			"workdir":         "string",
			"timeout_seconds": "integer",
		},
	}
}

func (t RunCommandTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	command := strings.TrimSpace(request.Call.Call.Arguments["command"])
	if command == "" {
		return ExecutionResult{ToolName: "run_command", DisplayContent: "command error: command is required", IsError: true}, nil
	}
	workdir := strings.TrimSpace(request.Call.Call.Arguments["workdir"])
	if workdir == "" {
		workdir = t.RootDir
	} else if !filepath.IsAbs(workdir) {
		workdir = filepath.Join(t.RootDir, workdir)
	}
	timeout := t.DefaultTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if raw := strings.TrimSpace(request.Call.Call.Arguments["timeout_seconds"]); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			return ExecutionResult{ToolName: "run_command", DisplayContent: fmt.Sprintf("command error: invalid timeout_seconds %q", raw), IsError: true}, nil
		}
		timeout = time.Duration(seconds) * time.Second
	}

	result := runSubprocess(workdir, timeout, "bash", "-lc", command)
	output := formatCommandOutput(command, workdir, result.Stdout, result.Stderr, result.ExitCode, result.TimedOut)
	if result.TimedOut {
		return ExecutionResult{ToolName: "run_command", DisplayContent: output, IsError: true}, nil
	}
	if result.ExitCode != 0 {
		return ExecutionResult{ToolName: "run_command", DisplayContent: output, IsError: true}, nil
	}
	return ExecutionResult{ToolName: "run_command", DisplayContent: output}, nil
}

func formatCommandOutput(command, workdir, stdout, stderr string, code int, timedOut bool) string {
	parts := []string{
		fmt.Sprintf("Command: %s", command),
		fmt.Sprintf("Workdir: %s", workdir),
	}
	if timedOut {
		parts = append(parts, "Timed out: true")
	}
	parts = append(parts, fmt.Sprintf("Exit code: %d", code))
	if strings.TrimSpace(stdout) != "" {
		parts = append(parts, "Stdout:", strings.TrimRight(stdout, "\n"))
	}
	if strings.TrimSpace(stderr) != "" {
		parts = append(parts, "Stderr:", strings.TrimRight(stderr, "\n"))
	}
	return strings.Join(parts, "\n")
}
