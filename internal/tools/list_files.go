package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ListFilesTool struct {
	RootDir string
}

func (ListFilesTool) Definition() Definition {
	return Definition{
		Name:        "list_files",
		Description: "List files and directories under a path for safe discovery",
		Parameters: map[string]string{
			"path":         "string",
			"recursive":    "boolean",
			"include_glob": "string",
			"max_results":  "integer",
		},
		Required: []string{},
	}
}

func (t ListFilesTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	relPath := strings.TrimSpace(request.Call.Call.Arguments["path"])
	if relPath == "" {
		relPath = "."
	}
	maxResults := 50
	if raw := strings.TrimSpace(request.Call.Call.Arguments["max_results"]); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return ExecutionResult{ToolName: "list_files", DisplayContent: fmt.Sprintf("list error: invalid max_results %q", raw), IsError: true}, nil
		}
		maxResults = parsed
	}
	recursive := parseBoolArg(request.Call.Call.Arguments["recursive"])
	includeGlob := strings.TrimSpace(request.Call.Call.Arguments["include_glob"])

	absPath := relPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(t.RootDir, relPath)
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return ExecutionResult{ToolName: "list_files", DisplayContent: fmt.Sprintf("list error: %v", err), IsError: true}, nil
	}

	results := make([]string, 0, len(entries))
	if recursive {
		results, err = t.walkEntries(absPath, relPath, includeGlob, maxResults)
		if err != nil {
			return ExecutionResult{ToolName: "list_files", DisplayContent: fmt.Sprintf("list error: %v", err), IsError: true}, nil
		}
	} else {
		for _, entry := range entries {
			name := entry.Name()
			if isHiddenJunk(name) {
				continue
			}
			relEntry := normalizeListedPath(relPath, name, entry.IsDir())
			if includeGlob != "" && !matchesListGlob(includeGlob, relEntry, name) {
				continue
			}
			results = append(results, relEntry)
			if len(results) >= maxResults {
				break
			}
		}
		sort.Strings(results)
	}

	return ExecutionResult{ToolName: "list_files", DisplayContent: formatListEntries(relPath, results, maxResults)}, nil
}

func (t ListFilesTool) walkEntries(absPath, relPath, includeGlob string, maxResults int) ([]string, error) {
	results := []string{}
	err := filepath.WalkDir(absPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == absPath {
			return nil
		}
		name := d.Name()
		if isHiddenJunk(name) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(t.RootDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			rel += "/"
		}
		if includeGlob != "" && !matchesListGlob(includeGlob, rel, name) {
			return nil
		}
		results = append(results, rel)
		if len(results) >= maxResults {
			return errSearchLimitReached
		}
		return nil
	})
	if err != nil && err != errSearchLimitReached {
		return nil, err
	}
	sort.Strings(results)
	return results, nil
}

func parseBoolArg(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeListedPath(base, name string, isDir bool) string {
	joined := name
	if base != "" && base != "." {
		joined = filepath.ToSlash(filepath.Join(base, name))
	}
	if isDir {
		joined += "/"
	}
	return filepath.ToSlash(joined)
}

func matchesListGlob(includeGlob, relPath, base string) bool {
	matched, err := filepath.Match(includeGlob, filepath.Base(strings.TrimSuffix(relPath, "/")))
	if err == nil && matched {
		return true
	}
	matched, err = filepath.Match(includeGlob, relPath)
	return err == nil && matched
}

func isHiddenJunk(name string) bool {
	return name == ".git" || name == "node_modules" || name == ".DS_Store"
}

func formatListEntries(path string, entries []string, maxResults int) string {
	if path == "" {
		path = "."
	}
	if len(entries) == 0 {
		return fmt.Sprintf("Path: %s\nNo entries.", path)
	}
	lines := []string{fmt.Sprintf("Path: %s", path)}
	if len(entries) >= maxResults {
		lines = append(lines, fmt.Sprintf("Showing first %d entries. Narrow with path or include_glob for more specific results.", len(entries)))
	}
	lines = append(lines, entries...)
	return strings.Join(lines, "\n")
}
