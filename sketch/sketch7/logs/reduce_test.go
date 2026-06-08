package logs

import "testing"

func TestReduceSessionStatsAggregatesRecords(t *testing.T) {
	history := NewSessionHistory("session-100")
	history.Append(UserMessageRecord{SessionBaseRecord: history.NextRecord("user"), Content: "do the thing"})
	history.Append(OutputContractEvaluationRecord{
		SessionBaseRecord: history.NextRecord("output_contract_evaluation"),
		StateName:         "act",
		SchemaName:        "coding_step_v1",
		ParseStatus:       "valid_json",
		ValidationStatus:  "valid",
	})
	history.Append(ToolValidationRecord{
		SessionBaseRecord: history.NextRecord("tool_validation"),
		CallID:            "call-1",
		ToolName:          "read_file",
		Valid:             true,
	})
	history.Append(ToolCallRequestRecord{SessionBaseRecord: history.NextRecord("tool_call_request"), CallID: "call-1", ToolName: "read_file", Arguments: `{"path":"foo.go"}`})
	history.Append(ToolCallResultRecord{SessionBaseRecord: history.NextRecord("tool_call_result"), CallID: "call-1", ToolName: "read_file", Content: "file contents", IsError: false})
	history.Append(RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "missing_required_arguments", Attempt: 1, Recovered: false})
	history.Append(CompletionRecord{SessionBaseRecord: history.NextRecord("completion"), Completed: true, StopReason: "assistant_only", Iteration: 1})

	stats := ReduceSessionStats(history)
	if stats.SessionID != "session-100" {
		t.Fatalf("session id = %q, want session-100", stats.SessionID)
	}
	if stats.RecordCounts.Total != 7 {
		t.Fatalf("total records = %d, want 7", stats.RecordCounts.Total)
	}
	if stats.Output.Total != 1 || stats.Output.Valid != 1 {
		t.Fatalf("output stats = %+v, want total=1 valid=1", stats.Output)
	}
	if stats.Tools.Proposed != 1 || stats.Tools.ValidProposals != 1 {
		t.Fatalf("tool proposal stats = %+v, want proposed=1 valid=1", stats.Tools)
	}
	if stats.Tools.Executed != 1 || stats.Tools.ExecutionSuccess != 1 {
		t.Fatalf("tool execution stats = %+v, want executed=1 success=1", stats.Tools)
	}
	if stats.Retry.Total != 1 || stats.Retry.Unrecovered != 1 {
		t.Fatalf("retry stats = %+v, want total=1 unrecovered=1", stats.Retry)
	}
	if !stats.Completion.LatestCompleted || stats.Completion.LatestStopReason != "assistant_only" {
		t.Fatalf("completion stats = %+v", stats.Completion)
	}
	if stats.ByTool["read_file"].Executed != 1 {
		t.Fatalf("per-tool stats = %+v, want read_file executed=1", stats.ByTool)
	}
	if stats.ByState["act"].Valid != 1 {
		t.Fatalf("per-state stats = %+v, want act valid=1", stats.ByState)
	}
}

func TestReduceSessionStatsCountsFailures(t *testing.T) {
	history := NewSessionHistory("session-101")
	history.Append(OutputContractEvaluationRecord{
		SessionBaseRecord: history.NextRecord("output_contract_evaluation"),
		StateName:         "observe",
		SchemaName:        "coding_step_v1",
		ParseStatus:       "plain_text",
		ValidationStatus:  "missing_required_fields",
		MissingFields:     []string{"state", "summary"},
	})
	history.Append(ToolValidationRecord{
		SessionBaseRecord: history.NextRecord("tool_validation"),
		CallID:            "call-2",
		ToolName:          "replace_text",
		Valid:             false,
		Reason:            "missing_required_arguments",
		MissingFields:     []string{"new_text"},
	})
	history.Append(ToolCallResultRecord{SessionBaseRecord: history.NextRecord("tool_call_result"), CallID: "call-2", ToolName: "replace_text", Content: "failed", IsError: true})
	history.Append(CompletionRecord{SessionBaseRecord: history.NextRecord("completion"), Completed: false, StopReason: "loop_guard", Iteration: 8})

	stats := ReduceSessionStats(history)
	if stats.Output.Invalid != 1 || stats.Output.MissingRequired != 1 {
		t.Fatalf("output stats = %+v, want invalid=1 missing_required=1", stats.Output)
	}
	if stats.Tools.InvalidProposals != 1 {
		t.Fatalf("tool stats = %+v, want invalid proposals=1", stats.Tools)
	}
	if stats.Tools.ExecutionFailures != 1 {
		t.Fatalf("tool execution stats = %+v, want execution failures=1", stats.Tools)
	}
	if stats.Completion.Incomplete != 1 || stats.StopReasons["loop_guard"] != 1 {
		t.Fatalf("completion stats = %+v stop reasons = %+v", stats.Completion, stats.StopReasons)
	}
	if stats.ByState["observe"].MissingRequired != 1 {
		t.Fatalf("per-state stats = %+v", stats.ByState)
	}
	if stats.ByTool["replace_text"].InvalidProposals != 1 {
		t.Fatalf("per-tool stats = %+v", stats.ByTool)
	}
}
