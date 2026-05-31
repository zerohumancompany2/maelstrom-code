package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ReadFileTool struct {
	RootDir string
}

func (ReadFileTool) Definition() Definition {
	return Definition{
		Name:        "read_file",
		Description: "Read a file or a line range from a file",
		Parameters: map[string]string{
			"path":       "string",
			"start_line": "integer",
			"end_line":   "integer",
		},
	}
}

func (t ReadFileTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	relPath := request.Call.Call.Arguments["path"]
	if strings.TrimSpace(relPath) == "" {
		return ExecutionResult{ToolName: "read_file", DisplayContent: "read error: path is required", IsError: true}, nil
	}
	absPath := relPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(t.RootDir, relPath)
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return ExecutionResult{ToolName: "read_file", DisplayContent: fmt.Sprintf("read error: %v", err), IsError: true}, nil
	}
	lines := splitLines(string(content))
	start, end, err := parseLineRange(request.Call.Call.Arguments, len(lines))
	if err != nil {
		return ExecutionResult{ToolName: "read_file", DisplayContent: fmt.Sprintf("read error: %v", err), IsError: true}, nil
	}
	slice := lines[start-1 : end]
	formatted := formatLinesWithNumbers(slice, start)
	return ExecutionResult{
		ToolName:       "read_file",
		DisplayContent: formatted,
	}, nil
}

func parseLineRange(args map[string]string, lineCount int) (int, int, error) {
	start := 1
	end := lineCount
	if raw := strings.TrimSpace(args["start_line"]); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start_line %q", raw)
		}
		start = parsed
	}
	if raw := strings.TrimSpace(args["end_line"]); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid end_line %q", raw)
		}
		end = parsed
	}
	if lineCount == 0 {
		return 1, 0, nil
	}
	if start < 1 || end < 1 {
		return 0, 0, fmt.Errorf("line range must be positive")
	}
	if start > end {
		return 0, 0, fmt.Errorf("start_line cannot be greater than end_line")
	}
	if start > lineCount || end > lineCount {
		return 0, 0, fmt.Errorf("line range %d-%d exceeds file length %d", start, end, lineCount)
	}
	return start, end, nil
}

func formatLinesWithNumbers(lines []string, startLine int) string {
	if len(lines) == 0 {
		return ""
	}
	formatted := make([]string, 0, len(lines))
	for i, line := range lines {
		formatted = append(formatted, fmt.Sprintf("%d: %s", startLine+i, line))
	}
	return strings.Join(formatted, "\n")
}

func splitLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return []string{}
	}
	return strings.Split(content, "\n")
}
