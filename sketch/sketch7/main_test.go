package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
)

func TestParseArgsAcceptsStatsFormatAndSelectors(t *testing.T) {
	args, err := parseArgs([]string{"--session-id", "session-123", "--stats", "--section", "tools", "--tool", "read_file", "--state-filter", "act", "--format", "json"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !args.showStats {
		t.Fatal("expected showStats to be true")
	}
	if args.statsSection != "tools" {
		t.Fatalf("statsSection = %q, want tools", args.statsSection)
	}
	if args.statsTool != "read_file" {
		t.Fatalf("statsTool = %q, want read_file", args.statsTool)
	}
	if args.statsState != "act" {
		t.Fatalf("statsState = %q, want act", args.statsState)
	}
	if args.statsFormat != "json" {
		t.Fatalf("statsFormat = %q, want json", args.statsFormat)
	}
}

func TestParseArgsAcceptsReportFlag(t *testing.T) {
	args, err := parseArgs([]string{"--session-id", "session-123", "--report", "--format", "json"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !args.showReport {
		t.Fatal("expected showReport to be true")
	}
	if args.statsFormat != "json" {
		t.Fatalf("statsFormat = %q, want json", args.statsFormat)
	}
}

func TestParseArgsAcceptsAggregateByAgentWithoutPrompt(t *testing.T) {
	args, err := parseArgs([]string{"--aggregate-by-agent", "--format", "json"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !args.aggregateByAgent {
		t.Fatal("expected aggregateByAgent to be true")
	}
}

func TestPrintSessionStatsJSONOutputsStructuredPayload(t *testing.T) {
	stats := logs.SessionStats{
		SessionID:    "session-200",
		RecordCounts: logs.RecordCounts{Total: 3},
		Output:       logs.OutputStats{Total: 1, Valid: 1, ByValidationStatus: map[string]int{"valid": 1}, ByParseStatus: map[string]int{"valid_json": 1}},
		Tools:        logs.ToolStats{ByReason: map[string]int{}},
		Retry:        logs.RetryStats{ByReason: map[string]int{}},
		ByTool:       map[string]logs.PerToolStats{"read_file": {Proposed: 1, Executed: 1, ExecutionSuccess: 1}},
		ByState:      map[string]logs.PerStateStats{"act": {OutputEvaluations: 1, Valid: 1}},
		StopReasons:  map[string]int{"assistant_only": 1},
	}
	args := cliArgs{showStats: true, statsFormat: "json"}
	output := captureStdout(t, func() { printSessionStats(stats, args) })
	if !strings.Contains(output, `"session_id": "session-200"`) {
		t.Fatalf("expected session id in output, got %s", output)
	}
	if !strings.Contains(output, `"record_counts"`) {
		t.Fatalf("expected record_counts in output, got %s", output)
	}
}

func TestPrintSessionStatsJSONToolFilterOutputsToolPayload(t *testing.T) {
	stats := logs.SessionStats{
		ByTool: map[string]logs.PerToolStats{"read_file": {Proposed: 2, Executed: 1}},
	}
	args := cliArgs{showStats: true, statsFormat: "json", statsTool: "read_file"}
	output := captureStdout(t, func() { printSessionStats(stats, args) })
	if !strings.Contains(output, `"tool": "read_file"`) {
		t.Fatalf("expected tool name in output, got %s", output)
	}
	if !strings.Contains(output, `"proposed": 2`) {
		t.Fatalf("expected tool stats in output, got %s", output)
	}
}

func TestRunWithStatsReadsSavedSessionAndPrintsStats(t *testing.T) {
	tempDir := t.TempDir()
	oldArgs := os.Args
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
		os.Args = oldArgs
	}()

	sessionDir := filepath.Join(tempDir, ".maelstrom", "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	history := logs.NewSessionHistory("session-stats")
	history.Append(logs.UserMessageRecord{SessionBaseRecord: history.NextRecord("user"), Content: "hello"})
	history.Append(logs.CompletionRecord{SessionBaseRecord: history.NextRecord("completion"), Completed: true, StopReason: "assistant_only", Iteration: 1})
	statePath := filepath.Join(sessionDir, "session-stats.json")
	if err := logs.SaveState(statePath, history, nil); err != nil {
		t.Fatalf("save state: %v", err)
	}

	os.Args = []string{"sketch7", "--session-id", "session-stats", "--stats", "--format", "json"}
	output := captureStdout(t, func() {
		if err := run(); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})
	if !strings.Contains(output, `"session_id": "session-stats"`) {
		t.Fatalf("expected session stats json, got %s", output)
	}
	if !strings.Contains(output, `"assistant_only"`) {
		t.Fatalf("expected stop reason in output, got %s", output)
	}
}

func TestPrintSessionReportJSONOutputsRecommendations(t *testing.T) {
	stats := logs.SessionStats{
		SessionID:  "session-report",
		Output:     logs.OutputStats{Invalid: 2, MissingRequired: 1},
		Tools:      logs.ToolStats{InvalidProposals: 1, ExecutionFailures: 1},
		Retry:      logs.RetryStats{Unrecovered: 1},
		Completion: logs.CompletionStats{LatestCompleted: false, LatestStopReason: "loop_guard"},
	}
	args := cliArgs{showReport: true, statsFormat: "json"}
	output := captureStdout(t, func() { printSessionReport(stats, args) })
	if !strings.Contains(output, `"recommended_changes"`) {
		t.Fatalf("expected recommended changes in output, got %s", output)
	}
	if !strings.Contains(output, `"attribution"`) {
		t.Fatalf("expected attribution in output, got %s", output)
	}
	if !strings.Contains(output, `"files"`) || !strings.Contains(output, `"layer"`) {
		t.Fatalf("expected structured recommendation fields in output, got %s", output)
	}
	if !strings.Contains(output, `"dominant_failure_modes"`) {
		t.Fatalf("expected dominant failure modes in output, got %s", output)
	}
	if !strings.Contains(output, `"loop_guard"`) {
		t.Fatalf("expected completion information in output, got %s", output)
	}
}

func TestRunWithReportReadsSavedSessionAndPrintsReport(t *testing.T) {
	tempDir := t.TempDir()
	oldArgs := os.Args
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
		os.Args = oldArgs
	}()

	sessionDir := filepath.Join(tempDir, ".maelstrom", "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	history := logs.NewSessionHistory("session-report")
	history.Append(logs.OutputContractEvaluationRecord{SessionBaseRecord: history.NextRecord("output_contract_evaluation"), StateName: "act", SchemaName: "coding_step_v1", ParseStatus: "plain_text", ValidationStatus: "missing_required_fields", MissingFields: []string{"summary"}})
	history.Append(logs.CompletionRecord{SessionBaseRecord: history.NextRecord("completion"), Completed: false, StopReason: "loop_guard", Iteration: 8})
	statePath := filepath.Join(sessionDir, "session-report.json")
	if err := logs.SaveState(statePath, history, nil); err != nil {
		t.Fatalf("save state: %v", err)
	}

	os.Args = []string{"sketch7", "--session-id", "session-report", "--report"}
	output := captureStdout(t, func() {
		if err := run(); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})
	if !strings.Contains(output, "dominant failure modes:") {
		t.Fatalf("expected report output, got %s", output)
	}
	if !strings.Contains(output, "recommended changes:") {
		t.Fatalf("expected recommendations in output, got %s", output)
	}
	if !strings.Contains(output, "attribution:") {
		t.Fatalf("expected attribution in output, got %s", output)
	}
	if !strings.Contains(output, "sketch/sketch7/prompt/projection.go") {
		t.Fatalf("expected likely file attribution in output, got %s", output)
	}
	if !strings.Contains(output, "loop_guard") {
		t.Fatalf("expected stop reason in output, got %s", output)
	}
}

func TestRunWithAggregateByAgentPrintsAggregateJSON(t *testing.T) {
	tempDir := t.TempDir()
	oldArgs := os.Args
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
		os.Args = oldArgs
	}()

	sessionDir := filepath.Join(tempDir, ".maelstrom", "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	history := logs.NewSessionHistory("session-agg")
	history.AgentID = "maelstrom-code"
	history.Append(logs.CompletionRecord{SessionBaseRecord: history.NextRecord("completion"), Completed: true, StopReason: "assistant_only", Iteration: 1})
	if err := logs.SaveState(filepath.Join(sessionDir, "session-agg.json"), history, nil); err != nil {
		t.Fatalf("save state: %v", err)
	}

	os.Args = []string{"sketch7", "--aggregate-by-agent", "--format", "json"}
	output := captureStdout(t, func() {
		if err := run(); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})
	if !strings.Contains(output, `"agent_id": "maelstrom-code"`) {
		t.Fatalf("expected aggregated agent output, got %s", output)
	}
	if !strings.Contains(output, `"session_count": 1`) {
		t.Fatalf("expected session count in output, got %s", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	fn()
	_ = w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	_ = r.Close()
	return buf.String()
}
