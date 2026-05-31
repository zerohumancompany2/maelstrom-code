package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ReplaceTextTool struct {
	RootDir string
}

func (ReplaceTextTool) Definition() Definition {
	return Definition{
		Name:        "replace_text",
		Description: "Replace exact text in a file when it matches exactly once",
		Parameters: map[string]string{
			"path":     "string",
			"old_text": "string",
			"new_text": "string",
		},
	}
}

func (t ReplaceTextTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	relPath := request.Call.Call.Arguments["path"]
	oldText := request.Call.Call.Arguments["old_text"]
	newText := request.Call.Call.Arguments["new_text"]
	if strings.TrimSpace(relPath) == "" {
		return ExecutionResult{ToolName: "replace_text", DisplayContent: "replace error: path is required", IsError: true}, nil
	}
	if oldText == "" {
		return ExecutionResult{ToolName: "replace_text", DisplayContent: "replace error: old_text is required", IsError: true}, nil
	}
	absPath := relPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(t.RootDir, relPath)
	}
	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		return ExecutionResult{ToolName: "replace_text", DisplayContent: fmt.Sprintf("replace error: %v", err), IsError: true}, nil
	}
	content := string(contentBytes)
	occurrences := strings.Count(content, oldText)
	if occurrences == 0 {
		return ExecutionResult{ToolName: "replace_text", DisplayContent: "replace error: old_text not found", IsError: true}, nil
	}
	if occurrences > 1 {
		return ExecutionResult{ToolName: "replace_text", DisplayContent: fmt.Sprintf("replace error: old_text matched %d times; expected exactly once", occurrences), IsError: true}, nil
	}
	updated := strings.Replace(content, oldText, newText, 1)
	if err := os.WriteFile(absPath, []byte(updated), 0o644); err != nil {
		return ExecutionResult{ToolName: "replace_text", DisplayContent: fmt.Sprintf("replace error: %v", err), IsError: true}, nil
	}
	return ExecutionResult{
		ToolName:       "replace_text",
		DisplayContent: fmt.Sprintf("replaced text in %s", relPath),
	}, nil
}
