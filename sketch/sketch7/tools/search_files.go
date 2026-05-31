package tools

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type SearchFilesTool struct {
	RootDir        string
	DefaultTimeout time.Duration
	LookupPath     func(name string) (string, error)
}

type SearchMatch struct {
	Path    string
	Line    int
	Snippet string
}

type rankedSearchMatch struct {
	Match SearchMatch
	Score int
}

func (SearchFilesTool) Definition() Definition {
	return Definition{
		Name:        "search_files",
		Description: "Search file contents with rg, grep, or an internal fallback",
		Parameters: map[string]string{
			"pattern":      "string",
			"include_glob": "string",
			"max_results":  "integer",
		},
	}
}

func (t SearchFilesTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	pattern := request.Call.Call.Arguments["pattern"]
	if strings.TrimSpace(pattern) == "" {
		return ExecutionResult{ToolName: "search_files", DisplayContent: "search error: pattern is required", IsError: true}, nil
	}
	includeGlob := strings.TrimSpace(request.Call.Call.Arguments["include_glob"])
	maxResults := 20
	if raw := strings.TrimSpace(request.Call.Call.Arguments["max_results"]); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return ExecutionResult{ToolName: "search_files", DisplayContent: fmt.Sprintf("search error: invalid max_results %q", raw), IsError: true}, nil
		}
		maxResults = parsed
	}
	timeout := t.DefaultTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	matches, backend, err := t.search(pattern, includeGlob, maxResults, timeout)
	if err != nil {
		return ExecutionResult{ToolName: "search_files", DisplayContent: fmt.Sprintf("search error: %v", err), IsError: true}, nil
	}
	return ExecutionResult{ToolName: "search_files", DisplayContent: formatSearchMatches(pattern, backend, matches, maxResults)}, nil
}

func (t SearchFilesTool) search(pattern, includeGlob string, maxResults int, timeout time.Duration) ([]SearchMatch, string, error) {
	lookup := t.LookupPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	if _, err := lookup("rg"); err == nil {
		matches, err := t.searchWithRg(pattern, includeGlob, maxResults, timeout)
		return matches, "rg", err
	}
	if _, err := lookup("grep"); err == nil {
		matches, err := t.searchWithGrep(pattern, includeGlob, maxResults, timeout)
		return matches, "grep", err
	}
	matches, err := t.searchInternal(pattern, includeGlob, maxResults)
	return matches, "internal", err
}

func (t SearchFilesTool) searchWithRg(pattern, includeGlob string, maxResults int, timeout time.Duration) ([]SearchMatch, error) {
	args := []string{"-n", "--no-heading", "-m", strconv.Itoa(maxResults), pattern, "."}
	if includeGlob != "" {
		args = append([]string{"-g", includeGlob}, args...)
	}
	result := runSubprocess(t.RootDir, timeout, "rg", args...)
	if result.TimedOut {
		return nil, fmt.Errorf("rg timed out")
	}
	if result.ExitCode != 0 && strings.TrimSpace(result.Stdout) == "" {
		return []SearchMatch{}, nil
	}
	return rankSearchMatches(pattern, parseSearchOutput(result.Stdout), maxResults), nil
}

func (t SearchFilesTool) searchWithGrep(pattern, includeGlob string, maxResults int, timeout time.Duration) ([]SearchMatch, error) {
	args := []string{"-R", "-n", "-m", strconv.Itoa(maxResults), pattern, "."}
	if includeGlob != "" {
		args = append([]string{"--include", includeGlob}, args...)
	}
	result := runSubprocess(t.RootDir, timeout, "grep", args...)
	if result.TimedOut {
		return nil, fmt.Errorf("grep timed out")
	}
	if result.ExitCode != 0 && strings.TrimSpace(result.Stdout) == "" {
		return []SearchMatch{}, nil
	}
	return rankSearchMatches(pattern, parseSearchOutput(result.Stdout), maxResults), nil
}

func (t SearchFilesTool) searchInternal(pattern, includeGlob string, maxResults int) ([]SearchMatch, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	var matches []SearchMatch
	err = filepath.Walk(t.RootDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(t.RootDir, path)
		if err != nil {
			return err
		}
		if includeGlob != "" {
			matched, err := filepath.Match(includeGlob, filepath.Base(rel))
			if err != nil {
				return err
			}
			if !matched {
				return nil
			}
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if re.MatchString(text) {
				matches = append(matches, SearchMatch{Path: rel, Line: line, Snippet: text})
				if len(matches) >= maxResults {
					return errSearchLimitReached
				}
			}
		}
		return scanner.Err()
	})
	if err != nil && err != errSearchLimitReached {
		return nil, err
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Path == matches[j].Path {
			return matches[i].Line < matches[j].Line
		}
		return matches[i].Path < matches[j].Path
	})
	return rankSearchMatches(pattern, matches, maxResults), nil
}

var errSearchLimitReached = fmt.Errorf("search limit reached")

func parseSearchOutput(output string) []SearchMatch {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	matches := make([]SearchMatch, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		lineNum, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		matches = append(matches, SearchMatch{Path: strings.TrimPrefix(parts[0], "./"), Line: lineNum, Snippet: parts[2]})
	}
	return matches
}

func rankSearchMatches(pattern string, matches []SearchMatch, maxResults int) []SearchMatch {
	if len(matches) == 0 {
		return nil
	}
	tokens := queryTokens(pattern)
	ranked := make([]rankedSearchMatch, 0, len(matches))
	for _, match := range matches {
		ranked = append(ranked, rankedSearchMatch{Match: match, Score: scoreSearchMatch(tokens, match)})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		if ranked[i].Match.Path != ranked[j].Match.Path {
			return ranked[i].Match.Path < ranked[j].Match.Path
		}
		return ranked[i].Match.Line < ranked[j].Match.Line
	})
	limit := len(ranked)
	if maxResults > 0 && limit > maxResults {
		limit = maxResults
	}
	ordered := make([]SearchMatch, 0, limit)
	for _, item := range ranked[:limit] {
		ordered = append(ordered, item.Match)
	}
	return ordered
}

func scoreSearchMatch(tokens []string, match SearchMatch) int {
	score := 0
	pathLower := strings.ToLower(match.Path)
	baseLower := strings.ToLower(filepath.Base(match.Path))
	snippetLower := strings.ToLower(strings.TrimSpace(match.Snippet))

	for _, token := range tokens {
		if token == "" {
			continue
		}
		if strings.Contains(baseLower, token) {
			score += 40
		}
		if strings.Contains(pathLower, token) {
			score += 15
		}
		if strings.Contains(snippetLower, token) {
			score += 10
		}
	}

	if strings.HasSuffix(pathLower, ".go") {
		score += 15
	}
	if strings.HasSuffix(pathLower, "_test.go") {
		score -= 20
	}
	if isLikelySourcePath(pathLower) {
		score += 12
	}
	if isLikelyLowValuePath(pathLower) {
		score -= 35
	}
	if looksLikeDefinitionLine(snippetLower) {
		score += 25
	}
	if looksLikeCommentLine(snippetLower) {
		score -= 8
	}

	depth := strings.Count(filepath.Clean(match.Path), string(filepath.Separator))
	score -= int(math.Min(float64(depth*2), 12))

	return score
}

func isLikelySourcePath(path string) bool {
	for _, part := range []string{"/cmd/", "/internal/", "/pkg/", "/src/", "/tools/", "/services/", "/handlers/"} {
		if strings.Contains(path, part) {
			return true
		}
	}
	return false
}

func isLikelyLowValuePath(path string) bool {
	for _, part := range []string{"/docs/", "/examples/", "/vendor/", "/node_modules/", "/dist/", "/build/", "/.git/"} {
		if strings.Contains(path, part) {
			return true
		}
	}
	base := filepath.Base(path)
	if strings.HasPrefix(base, "readme") || strings.HasPrefix(base, "license") {
		return true
	}
	return false
}

func looksLikeDefinitionLine(snippet string) bool {
	for _, prefix := range []string{"func ", "type ", "var ", "const ", "func(", "type ", "name:"} {
		if strings.Contains(snippet, prefix) {
			return true
		}
	}
	return false
}

func looksLikeCommentLine(snippet string) bool {
	trimmed := strings.TrimSpace(snippet)
	return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "*")
}

func queryTokens(pattern string) []string {
	lower := strings.ToLower(pattern)
	splitter := func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	}
	parts := strings.FieldsFunc(lower, splitter)
	seen := map[string]struct{}{}
	tokens := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		if len(part) < 2 {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		tokens = append(tokens, part)
	}
	compact := strings.Trim(lower, "^$ ")
	if compact != "" && !strings.ContainsAny(compact, `.*+?()[]{}|\\`) {
		if _, ok := seen[compact]; !ok {
			tokens = append(tokens, compact)
		}
	}
	return tokens
}

func formatSearchMatches(pattern, backend string, matches []SearchMatch, maxResults int) string {
	if len(matches) == 0 {
		return fmt.Sprintf("Backend: %s\nNo matches.", backend)
	}
	lines := []string{fmt.Sprintf("Backend: %s", backend)}
	if maxResults > 0 && len(matches) >= maxResults {
		lines = append(lines, fmt.Sprintf("Showing top %d ranked matches for %q. Refine pattern or include_glob for narrower results.", len(matches), pattern))
	}
	for _, match := range matches {
		lines = append(lines, fmt.Sprintf("%s:%d: %s", match.Path, match.Line, match.Snippet))
	}
	return strings.Join(lines, "\n")
}
