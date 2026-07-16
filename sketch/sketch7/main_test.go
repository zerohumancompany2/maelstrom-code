package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
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

func TestParseArgsAcceptsEvalDeckWithoutPrompt(t *testing.T) {
	args, err := parseArgs([]string{"--eval-deck", "sketch/sketch7/evals/decks/tiny-readonly.yaml", "--eval-out", "out.jsonl"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if args.evalDeckPath == "" || args.evalOutputPath != "out.jsonl" {
		t.Fatalf("args = %+v", args)
	}
}

func TestParseArgsAcceptsEvalSummaryWithoutPrompt(t *testing.T) {
	args, err := parseArgs([]string{"--eval-summary", "out.jsonl", "--format", "json"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if args.evalSummaryPath != "out.jsonl" || args.statsFormat != "json" {
		t.Fatalf("args = %+v", args)
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
		SessionID:        "session-report",
		Output:           logs.OutputStats{Invalid: 2, MissingRequired: 1, ByValidationStatus: map[string]int{"missing_workflow_bucket": 1}, ByParseStatus: map[string]int{"valid_json_wrapped": 1}},
		Tools:            logs.ToolStats{InvalidProposals: 1, ExecutionFailures: 1},
		Retry:            logs.RetryStats{Unrecovered: 1},
		Completion:       logs.CompletionStats{LatestCompleted: false, LatestStopReason: "loop_guard"},
		StopReasons:      map[string]int{"max_tool_calls": 1},
		StateExitReasons: map[string]int{"max_tool_calls": 1},
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
	if !strings.Contains(output, `"bucket_validation_statuses"`) || !strings.Contains(output, `"bound_stop_reasons"`) {
		t.Fatalf("expected bucket/bound attribution in output, got %s", output)
	}
}

func TestReportAttributionIncludesBucketAndBoundGroups(t *testing.T) {
	stats := logs.SessionStats{
		Output:           logs.OutputStats{ByValidationStatus: map[string]int{"missing_workflow_bucket": 2, "valid": 1}, ByParseStatus: map[string]int{"valid_json_wrapped": 2, "valid_json": 1}},
		StopReasons:      map[string]int{"max_tool_calls": 3, "assistant_only": 1},
		StateExitReasons: map[string]int{"max_tool_calls": 3},
	}
	attr := reportAttribution(stats)
	bucketStatuses, ok := attr["bucket_validation_statuses"].([]countEntry)
	if !ok || len(bucketStatuses) != 1 || bucketStatuses[0].Key != "missing_workflow_bucket" {
		t.Fatalf("bucket statuses = %#v", attr["bucket_validation_statuses"])
	}
	boundStops, ok := attr["bound_stop_reasons"].([]countEntry)
	if !ok || len(boundStops) != 1 || boundStops[0].Key != "max_tool_calls" {
		t.Fatalf("bound stops = %#v", attr["bound_stop_reasons"])
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

func TestDefaultAgentDefinitionIsSingleStateBaseline(t *testing.T) {
	def := defaultAgentDefinition("test-model")
	if def.Cognitive.InitialState != "observe" {
		t.Fatalf("initialState = %q, want observe", def.Cognitive.InitialState)
	}
	if len(def.Cognitive.States) != 1 {
		t.Fatalf("got %d states, want 1", len(def.Cognitive.States))
	}
	if len(def.Cognitive.Transitions) != 0 {
		t.Fatalf("got %d transitions, want 0", len(def.Cognitive.Transitions))
	}
	state := def.Cognitive.States[0]
	if state.Name != "observe" {
		t.Fatalf("state name = %q, want observe", state.Name)
	}
	if !containsTool(state.EnabledTools, "read_file") || !containsTool(state.EnabledTools, "replace_text") {
		t.Fatalf("enabled tools = %+v, want baseline read/edit coverage", state.EnabledTools)
	}
	if !hasProjection(def.Context.Projections, "state_task") {
		t.Fatalf("projections = %+v, want state_task projection", def.Context.Projections)
	}
}

func TestBuildToolRegistryIncludesTransitionState(t *testing.T) {
	registry := buildToolRegistry(defs.AgentDefinition{
		Cognitive: defs.StatechartDefinition{
			InitialState: "observe",
			States:       []defs.StateDefinition{{Name: "observe"}, {Name: "act"}},
			Transitions:  []defs.TransitionDefinition{{Trigger: "go", From: "observe", To: "act"}},
		},
	}, nil)
	if err := registry.MustHave("transition_state"); err != nil {
		t.Fatalf("expected transition_state in registry: %v", err)
	}
}

func TestBuildToolRegistryGatesWriteTools(t *testing.T) {
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe"}}}}
	readOnly := buildToolRegistry(agentDef, nil)
	for _, name := range []string{"replace_text", "run_command"} {
		if err := readOnly.MustHave(name); err == nil {
			t.Fatalf("read-only baseline registry must not contain %s", name)
		}
	}
	if err := readOnly.MustHave("read_file"); err != nil {
		t.Fatalf("read tools missing from baseline: %v", err)
	}
}

func hasProjection(itemsDef []defs.ProjectionDefinition, want string) bool {
	for _, item := range itemsDef {
		if item.Type == want {
			return true
		}
	}
	return false
}

func containsTool(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
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
