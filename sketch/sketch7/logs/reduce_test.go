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
	history.Append(StateExitRecord{SessionBaseRecord: history.NextRecord("state_exit"), Chart: "cognitive", StateName: "observe", Reason: "max_tool_calls"})
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
	if stats.StateExitReasons["max_tool_calls"] != 1 {
		t.Fatalf("state exit reasons = %+v, want max_tool_calls=1", stats.StateExitReasons)
	}
	if stats.ByState["observe"].MissingRequired != 1 {
		t.Fatalf("per-state stats = %+v", stats.ByState)
	}
	if stats.ByTool["replace_text"].InvalidProposals != 1 {
		t.Fatalf("per-tool stats = %+v", stats.ByTool)
	}
}

func TestReduceSessionStatsAttributesOutputToolRetryAndProvider(t *testing.T) {
	history := NewSessionHistory("session-102")
	history.AgentID = "agent-attribution"
	history.Append(InferenceEnvelopeRecord{SessionBaseRecord: history.NextRecord("inference_envelope"), ModelRef: "model-a", ProviderRef: "provider-a"})
	history.Append(CognitiveTransitionRecord{SessionBaseRecord: history.NextRecord("cognitive_transition"), FromState: "observe", ToState: "act", Trigger: "begin"})
	history.Append(WorkflowTransitionRefRecord{SessionBaseRecord: history.NextRecord("workflow_transition_ref"), FromState: "planning", ToState: "implementing", Trigger: "start"})
	output := OutputContractEvaluationRecord{
		SessionBaseRecord: history.NextRecord("output_contract_evaluation"),
		StateName:         "act",
		SchemaName:        "coding_step_v1",
		ParseStatus:       "valid_json",
		ValidationStatus:  "missing_required_fields",
		MissingFields:     []string{"summary"},
		ActionType:        "tool",
		ToolName:          "replace_text",
		CompletionSignal:  false,
	}
	history.Append(output)
	validation := ToolValidationRecord{
		SessionBaseRecord: history.NextRecord("tool_validation"),
		CallID:            "call-3",
		ToolName:          "replace_text",
		Valid:             false,
		Reason:            "missing_required_arguments",
		MissingFields:     []string{"old_text"},
	}
	history.Append(validation)
	history.Append(RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "missing_required_arguments", Attempt: 1, Recovered: false, DerivedFrom: []string{validation.RecordID()}})
	history.Append(CompletionRecord{SessionBaseRecord: history.NextRecord("completion"), Completed: false, StopReason: "loop_guard", Iteration: 8})

	stats := ReduceSessionStats(history)
	if stats.AgentID != "agent-attribution" {
		t.Fatalf("agent id = %q", stats.AgentID)
	}
	if stats.ModelRefs["model-a"] != 1 || stats.ProviderRefs["provider-a"] != 1 {
		t.Fatalf("model/provider attribution missing: models=%+v providers=%+v", stats.ModelRefs, stats.ProviderRefs)
	}
	if stats.Output.BySchema["coding_step_v1"] != 1 || stats.Output.ByActionType["tool"] != 1 || stats.Output.ByTool["replace_text"] != 1 {
		t.Fatalf("output attribution missing: %+v", stats.Output)
	}
	if stats.Output.MissingFieldCounts["summary"] != 1 || stats.ByState["act"].MissingFieldCounts["summary"] != 1 {
		t.Fatalf("missing field attribution missing: output=%+v state=%+v", stats.Output.MissingFieldCounts, stats.ByState["act"].MissingFieldCounts)
	}
	if stats.ByTool["replace_text"].InvalidReasons["missing_required_arguments"] != 1 || stats.ByTool["replace_text"].MissingFieldCounts["old_text"] != 1 {
		t.Fatalf("tool attribution missing: %+v", stats.ByTool["replace_text"])
	}
	if stats.Retry.Attribution.ByCauseKind["tool_validation"] != 1 || stats.Retry.Attribution.ByTool["replace_text"] != 1 {
		t.Fatalf("retry attribution missing: %+v", stats.Retry.Attribution)
	}
	if stats.Completion.LatestCognitiveState != "act" || stats.Completion.LatestWorkflowState != "implementing" || stats.Completion.LatestIteration != 8 {
		t.Fatalf("completion attribution missing: %+v", stats.Completion)
	}
}

func TestReduceSessionStatsAttributesRetryFromOutputRecord(t *testing.T) {
	history := NewSessionHistory("session-103")
	output := OutputContractEvaluationRecord{SessionBaseRecord: history.NextRecord("output_contract_evaluation"), StateName: "observe", ToolName: "read_file", ValidationStatus: "missing_schema_output"}
	history.Append(output)
	history.Append(RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "missing_schema_output", Attempt: 1, Recovered: true, DerivedFrom: []string{output.RecordID()}})

	stats := ReduceSessionStats(history)
	if stats.Retry.Attribution.ByCauseKind["output_contract_evaluation"] != 1 {
		t.Fatalf("retry cause attribution = %+v", stats.Retry.Attribution.ByCauseKind)
	}
	if stats.Retry.Attribution.ByState["observe"] != 1 || stats.Retry.Attribution.ByTool["read_file"] != 1 {
		t.Fatalf("retry state/tool attribution = %+v", stats.Retry.Attribution)
	}
}
