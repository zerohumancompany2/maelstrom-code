package tools

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

type SubprocessResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

func runSubprocess(workdir string, timeout time.Duration, name string, args ...string) SubprocessResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workdir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return SubprocessResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: subprocessExitCode(err),
		TimedOut: ctx.Err() == context.DeadlineExceeded,
	}
}

func subprocessExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}
