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
	absPath, ok := pathWithinRoot(t.RootDir, relPath)
	if !ok {
		return ExecutionResult{ToolName: "replace_text", DisplayContent: fmt.Sprintf("replace error: path %q escapes the tool root", relPath), IsError: true}, nil
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

// pathWithinRoot resolves a tool path beneath root. Absolute paths and
// relative traversal outside root are rejected: write tools must never be
// able to escape a disposable sandbox (or their configured live root).
func pathWithinRoot(root, requested string) (string, bool) {
	if filepath.IsAbs(requested) {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", false
	}
	absPath, err := filepath.Abs(filepath.Join(absRoot, requested))
	if err != nil {
		return "", false
	}
	if absPath != absRoot && !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Lstat(absPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", false
	}
	if resolvedPath != resolvedRoot && !strings.HasPrefix(resolvedPath, resolvedRoot+string(filepath.Separator)) {
		return "", false
	}
	return resolvedPath, true
}
