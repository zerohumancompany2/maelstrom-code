package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type FindReferencesTool struct {
	RootDir      string
	LookupPath   func(name string) (string, error)
	MaxResults   int
	UseWordBound bool
}

func (FindReferencesTool) Definition() Definition {
	return Definition{
		Name:        "find_references",
		Description: "Find likely references to a symbol in the repository",
		Parameters: map[string]string{
			"symbol":       "string",
			"include_glob": "string",
			"max_results":  "integer",
		},
	}
}

func (t FindReferencesTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	symbol := strings.TrimSpace(request.Call.Call.Arguments["symbol"])
	if symbol == "" {
		return ExecutionResult{ToolName: "find_references", DisplayContent: "references error: symbol is required", IsError: true}, nil
	}
	includeGlob := strings.TrimSpace(request.Call.Call.Arguments["include_glob"])
	maxResults := t.MaxResults
	if maxResults <= 0 {
		maxResults = 20
	}
	if raw := strings.TrimSpace(request.Call.Call.Arguments["max_results"]); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return ExecutionResult{ToolName: "find_references", DisplayContent: fmt.Sprintf("references error: invalid max_results %q", raw), IsError: true}, nil
		}
		maxResults = parsed
	}
	pattern := regexp.QuoteMeta(symbol)
	if t.UseWordBound || t.UseWordBound == false {
		pattern = `\b` + pattern + `\b`
	}
	searchTool := SearchFilesTool{RootDir: t.RootDir, LookupPath: t.LookupPath}
	matches, backend, err := searchTool.search(pattern, includeGlob, maxResults, 10_000_000_000)
	if err != nil {
		return ExecutionResult{ToolName: "find_references", DisplayContent: fmt.Sprintf("references error: %v", err), IsError: true}, nil
	}
	return ExecutionResult{ToolName: "find_references", DisplayContent: formatReferenceMatches(symbol, backend, matches)}, nil
}

func formatReferenceMatches(symbol, backend string, matches []SearchMatch) string {
	if len(matches) == 0 {
		return fmt.Sprintf("Symbol: %s\nBackend: %s\nNo references found.", symbol, backend)
	}
	lines := []string{fmt.Sprintf("Symbol: %s", symbol), fmt.Sprintf("Backend: %s", backend)}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Path == matches[j].Path {
			return matches[i].Line < matches[j].Line
		}
		return matches[i].Path < matches[j].Path
	})
	for _, match := range matches {
		lines = append(lines, fmt.Sprintf("%s:%d: %s", match.Path, match.Line, match.Snippet))
	}
	return strings.Join(lines, "\n")
}

// keep imports used for future file/path constrained evolution
var _ = os.ErrNotExist
var _ = filepath.Separator
