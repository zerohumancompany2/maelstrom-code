package context

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/comalice/maelstrom/internal/logs"
)

type RepoContextOptions struct {
	RootDir             string
	RefreshEveryTurns   int
	MaxFilesToInspect   int
	MaxTopLevelEntries  int
	MaxExtensionsToShow int
}

type RepoContextCache struct {
	mu       sync.Mutex
	lastTurn int
	lastText string
}

func (c *RepoContextCache) Summary(opts RepoContextOptions, history *logs.SessionHistory) string {
	if latest := latestContextSnapshot(history, "repo_context"); latest != nil {
		refreshEvery := opts.RefreshEveryTurns
		if refreshEvery <= 0 {
			refreshEvery = 12
		}
		turn := InteractionTurnCount(history)
		if turn-latest.GeneratedAtTurn < refreshEvery {
			return latest.Content
		}
	}
	turn := InteractionTurnCount(history)
	refreshEvery := opts.RefreshEveryTurns
	if refreshEvery <= 0 {
		refreshEvery = 12
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastText != "" && turn-c.lastTurn < refreshEvery {
		return c.lastText
	}
	summary := buildRepoSummary(opts)
	c.lastTurn = turn
	c.lastText = summary
	return summary
}

func latestContextSnapshot(history *logs.SessionHistory, logicalKey string) *logs.ContextSnapshotRecord {
	if history == nil {
		return nil
	}
	for i := len(history.Records) - 1; i >= 0; i-- {
		rec, ok := history.Records[i].(logs.ContextSnapshotRecord)
		if ok && rec.LogicalKey == logicalKey {
			copy := rec
			return &copy
		}
	}
	return nil
}

func buildRepoSummary(opts RepoContextOptions) string {
	root := opts.RootDir
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	maxFiles := opts.MaxFilesToInspect
	if maxFiles <= 0 {
		maxFiles = 2000
	}
	maxTop := opts.MaxTopLevelEntries
	if maxTop <= 0 {
		maxTop = 8
	}
	maxExt := opts.MaxExtensionsToShow
	if maxExt <= 0 {
		maxExt = 5
	}
	fileCounts, markerFiles, _ := scanRepo(root, maxFiles)
	topDirs, _ := topLevelEntries(root, maxTop)
	branch, dirty, staged := gitSummary(root)
	extSummary := summarizeExtensions(fileCounts, maxExt)
	parts := []string{"Repository context:"}
	if branch != "" {
		parts = append(parts, fmt.Sprintf("- git: branch=%s, dirty=%t, staged=%t", branch, dirty, staged))
	}
	if extSummary != "" {
		parts = append(parts, fmt.Sprintf("- common file types: %s", extSummary))
	}
	if len(markerFiles) > 0 {
		parts = append(parts, fmt.Sprintf("- key files: %s", strings.Join(markerFiles, ", ")))
	}
	if len(topDirs) > 0 {
		parts = append(parts, fmt.Sprintf("- top-level entries: %s", strings.Join(topDirs, ", ")))
	}
	return strings.Join(parts, "\n")
}

func InteractionTurnCount(history *logs.SessionHistory) int {
	if history == nil {
		return 0
	}
	turns := 0
	for _, record := range history.Records {
		switch record.(type) {
		case logs.UserMessageRecord, logs.AssistantMessageRecord, logs.ToolCallRequestRecord, logs.ToolCallResultRecord:
			turns++
		}
	}
	return turns
}

func scanRepo(root string, maxFiles int) (map[string]int, []string, error) {
	counts := map[string]int{}
	markerSet := map[string]struct{}{}
	markers := []string{"go.mod", "package.json", "pyproject.toml", "Cargo.toml", "Makefile"}
	inspected := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if isIgnoredRepoDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		inspected++
		if inspected > maxFiles {
			return filepath.SkipDir
		}
		base := filepath.Base(path)
		for _, marker := range markers {
			if base == marker {
				markerSet[marker] = struct{}{}
			}
		}
		ext := strings.ToLower(filepath.Ext(base))
		if ext == "" {
			ext = "[no extension]"
		}
		counts[ext]++
		return nil
	})
	markerFiles := make([]string, 0, len(markerSet))
	for marker := range markerSet {
		markerFiles = append(markerFiles, marker)
	}
	sort.Strings(markerFiles)
	return counts, markerFiles, err
}

func topLevelEntries(root string, maxEntries int) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	results := make([]string, 0, len(entries))
	for _, entry := range entries {
		if isIgnoredRepoDir(entry.Name()) {
			continue
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		results = append(results, name)
	}
	sort.Strings(results)
	if len(results) > maxEntries {
		results = results[:maxEntries]
	}
	return results, nil
}

func summarizeExtensions(counts map[string]int, maxEntries int) string {
	type pair struct {
		Ext   string
		Count int
	}
	total := 0
	pairs := make([]pair, 0, len(counts))
	for ext, count := range counts {
		total += count
		pairs = append(pairs, pair{Ext: ext, Count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Count == pairs[j].Count {
			return pairs[i].Ext < pairs[j].Ext
		}
		return pairs[i].Count > pairs[j].Count
	})
	if len(pairs) > maxEntries {
		pairs = pairs[:maxEntries]
	}
	parts := make([]string, 0, len(pairs))
	for _, item := range pairs {
		pct := 0
		if total > 0 {
			pct = int(float64(item.Count) / float64(total) * 100)
		}
		parts = append(parts, fmt.Sprintf("%s %d%%", item.Ext, pct))
	}
	return strings.Join(parts, ", ")
}

func gitSummary(root string) (branch string, dirty bool, staged bool) {
	branch = strings.TrimSpace(runGit(root, "rev-parse", "--abbrev-ref", "HEAD"))
	status := runGit(root, "status", "--porcelain")
	if strings.TrimSpace(status) == "" {
		return branch, false, false
	}
	for _, line := range strings.Split(strings.TrimRight(status, "\n"), "\n") {
		if len(line) < 2 {
			continue
		}
		if line[0] != ' ' && line[0] != '?' {
			staged = true
		}
		if line[1] != ' ' || strings.HasPrefix(line, "??") {
			dirty = true
		}
	}
	return branch, dirty, staged
}

func runGit(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func isIgnoredRepoDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".cache":
		return true
	default:
		return false
	}
}
