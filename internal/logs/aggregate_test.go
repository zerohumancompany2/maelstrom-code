package logs

import (
	"path/filepath"
	"testing"
)

func TestAggregateSessionStatsByAgent(t *testing.T) {
	tempDir := t.TempDir()

	s1 := NewSessionHistory("session-1")
	s1.AgentID = "agent-a"
	s1.Append(OutputContractEvaluationRecord{SessionBaseRecord: s1.NextRecord("output_contract_evaluation"), StateName: "act", ValidationStatus: "valid", ParseStatus: "valid_json"})
	s1.Append(CompletionRecord{SessionBaseRecord: s1.NextRecord("completion"), Completed: true, StopReason: "assistant_only", Iteration: 1})
	if err := SaveState(filepath.Join(tempDir, "session-1.json"), s1, nil); err != nil {
		t.Fatalf("save session 1: %v", err)
	}

	s2 := NewSessionHistory("session-2")
	s2.AgentID = "agent-a"
	s2.Append(OutputContractEvaluationRecord{SessionBaseRecord: s2.NextRecord("output_contract_evaluation"), StateName: "act", ValidationStatus: "missing_required_fields", ParseStatus: "plain_text", MissingFields: []string{"summary"}})
	s2.Append(CompletionRecord{SessionBaseRecord: s2.NextRecord("completion"), Completed: false, StopReason: "loop_guard", Iteration: 8})
	if err := SaveState(filepath.Join(tempDir, "session-2.json"), s2, nil); err != nil {
		t.Fatalf("save session 2: %v", err)
	}

	s3 := NewSessionHistory("session-3")
	s3.AgentID = "agent-b"
	s3.Append(ToolValidationRecord{SessionBaseRecord: s3.NextRecord("tool_validation"), ToolName: "read_file", Valid: false, Reason: "missing_required_arguments"})
	s3.Append(RetryRecord{SessionBaseRecord: s3.NextRecord("retry"), Reason: "missing_required_arguments", Recovered: false})
	if err := SaveState(filepath.Join(tempDir, "session-3.json"), s3, nil); err != nil {
		t.Fatalf("save session 3: %v", err)
	}

	report, err := AggregateSessionStatsByAgent(tempDir)
	if err != nil {
		t.Fatalf("aggregate by agent: %v", err)
	}
	if len(report.Agents) != 2 {
		t.Fatalf("agent count = %d, want 2", len(report.Agents))
	}
	aggA := report.Agents["agent-a"]
	if aggA.SessionCount != 2 {
		t.Fatalf("agent-a session count = %d, want 2", aggA.SessionCount)
	}
	if aggA.Combined.Output.Total != 2 {
		t.Fatalf("agent-a output total = %d, want 2", aggA.Combined.Output.Total)
	}
	if aggA.Combined.Completion.Completed != 1 || aggA.Combined.Completion.Incomplete != 1 {
		t.Fatalf("agent-a completion stats = %+v", aggA.Combined.Completion)
	}
	if aggA.Combined.Output.MissingFieldCounts["summary"] != 1 {
		t.Fatalf("agent-a missing field counts = %+v", aggA.Combined.Output.MissingFieldCounts)
	}
	if len(aggA.TopInvalidSessions) != 1 || aggA.TopInvalidSessions[0].SessionID != "session-2" {
		t.Fatalf("agent-a top invalid sessions = %+v", aggA.TopInvalidSessions)
	}
	if len(aggA.IncompleteSessions) != 1 || aggA.IncompleteSessions[0].SessionID != "session-2" {
		t.Fatalf("agent-a incomplete sessions = %+v", aggA.IncompleteSessions)
	}
	aggB := report.Agents["agent-b"]
	if aggB.SessionCount != 1 {
		t.Fatalf("agent-b session count = %d, want 1", aggB.SessionCount)
	}
	if aggB.Combined.Tools.InvalidProposals != 1 {
		t.Fatalf("agent-b invalid proposals = %d, want 1", aggB.Combined.Tools.InvalidProposals)
	}
	if aggB.RetryFailureRate != 1 {
		t.Fatalf("agent-b retry failure rate = %f, want 1", aggB.RetryFailureRate)
	}
	if len(aggB.TopRetryFailureSessions) != 1 || aggB.TopRetryFailureSessions[0].SessionID != "session-3" {
		t.Fatalf("agent-b retry contributors = %+v", aggB.TopRetryFailureSessions)
	}
}
