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
	// Allowlist, when non-empty, switches the tool into its safe-command
	// tier: the command is split into fields and executed directly (no
	// shell, so metacharacters are inert literal arguments), the normalized
	// command must match an allowlist entry exactly or extend one with
	// further arguments, and workdir may not escape RootDir.
	Allowlist []string
}

func (t RunCommandTool) Definition() Definition {
	description := "Run a shell command for validation or inspection"
	if len(t.Allowlist) > 0 {
		description = fmt.Sprintf(
			"Run a validation command (no shell; the command is executed directly). Only these command prefixes are allowed: %s",
			strings.Join(t.Allowlist, ", "))
	}
	return Definition{
		Name:        "run_command",
		Description: description,
		Parameters: map[string]string{
			"command":         "string",
			"workdir":         "string",
			"timeout_seconds": "integer",
		},
		Required: []string{"command"},
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

	var result SubprocessResult
	if len(t.Allowlist) > 0 {
		fields := strings.Fields(command)
		normalized := strings.Join(fields, " ")
		if !commandAllowed(normalized, t.Allowlist) {
			return ExecutionResult{ToolName: "run_command", DisplayContent: fmt.Sprintf("command error: %q is not in the safe-command allowlist (allowed prefixes: %s)", command, strings.Join(t.Allowlist, ", ")), IsError: true}, nil
		}
		if !workdirWithinRoot(workdir, t.RootDir) {
			return ExecutionResult{ToolName: "run_command", DisplayContent: fmt.Sprintf("command error: workdir %q escapes the tool root", workdir), IsError: true}, nil
		}
		result = runSubprocess(workdir, timeout, fields[0], fields[1:]...)
	} else {
		result = runSubprocess(workdir, timeout, "bash", "-lc", command)
	}
	output := formatCommandOutput(command, workdir, result.Stdout, result.Stderr, result.ExitCode, result.TimedOut)
	if result.TimedOut {
		return ExecutionResult{ToolName: "run_command", DisplayContent: output, IsError: true}, nil
	}
	if result.ExitCode != 0 {
		return ExecutionResult{ToolName: "run_command", DisplayContent: output, IsError: true}, nil
	}
	return ExecutionResult{ToolName: "run_command", DisplayContent: output}, nil
}

// commandAllowed reports whether a field-normalized command matches an
// allowlist entry exactly or extends one with further arguments. Prefix
// matching is on whole tokens: "go test" allows "go test ./..." but not
// "go testfoo". Because allowlisted commands run without a shell, extra
// arguments cannot smuggle in command chaining.
func commandAllowed(normalized string, allowlist []string) bool {
	for _, entry := range allowlist {
		entry = strings.Join(strings.Fields(entry), " ")
		if entry == "" {
			continue
		}
		if normalized == entry || strings.HasPrefix(normalized, entry+" ") {
			return true
		}
	}
	return false
}

// workdirWithinRoot reports whether workdir resolves inside root. Used only
// in allowlist mode so a safe-tier command cannot run outside the tool root.
func workdirWithinRoot(workdir, root string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absWorkdir, err := filepath.Abs(workdir)
	if err != nil {
		return false
	}
	return absWorkdir == absRoot || strings.HasPrefix(absWorkdir, absRoot+string(filepath.Separator))
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
