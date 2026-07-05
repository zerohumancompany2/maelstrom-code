package evals

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
)

func writeRunRecordsJSONL(t *testing.T, records []RunRecord) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create batch file: %v", err)
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatalf("encode record: %v", err)
		}
	}
	return path
}

func TestLoadRunRecordsKeepsLatestPerRunID(t *testing.T) {
	path := writeRunRecordsJSONL(t, []RunRecord{
		{RunID: "deck:case-1:1", CaseID: "case-1", Passed: false},
		{RunID: "deck:case-2:1", CaseID: "case-2", Passed: true},
		{RunID: "deck:case-1:1", CaseID: "case-1", Passed: true},
	})
	records, err := LoadRunRecords(path)
	if err != nil {
		t.Fatalf("LoadRunRecords: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].RunID != "deck:case-1:1" || !records[0].Passed {
		t.Fatalf("expected latest record for case-1, got %+v", records[0])
	}
	if records[1].RunID != "deck:case-2:1" {
		t.Fatalf("unexpected second record %+v", records[1])
	}
}

func TestLoadRunRecordsMissingFile(t *testing.T) {
	_, err := LoadRunRecords(filepath.Join(t.TempDir(), "missing.jsonl"))
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestSummarizeRunsAggregates(t *testing.T) {
	records := []RunRecord{
		{
			RunID: "deck:case-1:1", CaseID: "case-1", AgentID: "agent-a", ModelID: "model-a", Passed: true,
			Stats: logs.SessionStats{StopReasons: map[string]int{"tool_calls": 2, "end_turn": 1}},
		},
		{
			RunID: "deck:case-1:2", CaseID: "case-1", AgentID: "agent-a", ModelID: "model-b", Passed: false,
			Eval:  EvalResult{Checks: []EvalCheck{{Name: "max_invalid_outputs", Passed: false}}},
			Stats: logs.SessionStats{StopReasons: map[string]int{"end_turn": 1}, Output: logs.OutputStats{Invalid: 3}},
		},
		{
			RunID: "deck:case-2:1", CaseID: "case-2", AgentID: "agent-b", ModelLabel: "model-b", Passed: false, Error: "boom",
		},
	}
	summary := SummarizeRuns(records)
	if summary.Total != 3 || summary.Passed != 1 || summary.Failed != 2 || summary.Errored != 1 {
		t.Fatalf("summary counts = %+v", summary)
	}
	if summary.PassRate < 0.33 || summary.PassRate > 0.34 {
		t.Fatalf("pass rate = %f", summary.PassRate)
	}
	if summary.InvalidOutputs != 3 {
		t.Fatalf("invalid outputs = %d", summary.InvalidOutputs)
	}
	if summary.StopReasons["end_turn"] != 2 || summary.StopReasons["tool_calls"] != 2 {
		t.Fatalf("stop reasons = %+v", summary.StopReasons)
	}
	if summary.FailedChecks["max_invalid_outputs"] != 1 {
		t.Fatalf("failed checks = %+v", summary.FailedChecks)
	}
	caseOne := summary.ByCase["case-1"]
	if caseOne.Total != 2 || caseOne.Passed != 1 || caseOne.PassRate != 0.5 {
		t.Fatalf("case-1 summary = %+v", caseOne)
	}
	agentB := summary.ByAgent["agent-b"]
	if agentB.Total != 1 || agentB.Errored != 1 || agentB.PassRate != 0 {
		t.Fatalf("agent-b summary = %+v", agentB)
	}
	modelA := summary.ByModel["model-a"]
	if modelA.Total != 1 || modelA.Passed != 1 || modelA.PassRate != 1 {
		t.Fatalf("model-a summary = %+v", modelA)
	}
	// model-b groups by ModelID when present and falls back to ModelLabel.
	modelB := summary.ByModel["model-b"]
	if modelB.Total != 2 || modelB.Passed != 0 || modelB.Errored != 1 {
		t.Fatalf("model-b summary = %+v", modelB)
	}
}

func TestSummarizeBatchFileRoundTrip(t *testing.T) {
	path := writeRunRecordsJSONL(t, []RunRecord{
		{RunID: "deck:case-1:1", CaseID: "case-1", Passed: false},
		{RunID: "deck:case-1:1", CaseID: "case-1", Passed: true},
	})
	summary, err := SummarizeBatchFile(path)
	if err != nil {
		t.Fatalf("SummarizeBatchFile: %v", err)
	}
	if summary.Total != 1 || summary.Passed != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}
