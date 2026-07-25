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

func TestReduceSessionStatsTracksFilesReadAndFinalAssistant(t *testing.T) {
	history := NewSessionHistory("session-files")
	history.Append(UserMessageRecord{SessionBaseRecord: history.NextRecord("user"), Content: "inspect"})
	history.Append(ToolCallRequestRecord{SessionBaseRecord: history.NextRecord("tool_call_request"), CallID: "c1", ToolName: "read_file", Arguments: `{"path":"./pkg/a.go"}`})
	history.Append(ToolCallResultRecord{SessionBaseRecord: history.NextRecord("tool_call_result"), CallID: "c1", ToolName: "read_file", Content: "ok", IsError: false})
	history.Append(ToolCallRequestRecord{SessionBaseRecord: history.NextRecord("tool_call_request"), CallID: "c2", ToolName: "read_file", Arguments: `{"path":"pkg/missing.go"}`})
	history.Append(ToolCallResultRecord{SessionBaseRecord: history.NextRecord("tool_call_result"), CallID: "c2", ToolName: "read_file", Content: "no such file", IsError: true})
	history.Append(ToolCallRequestRecord{SessionBaseRecord: history.NextRecord("tool_call_request"), CallID: "c3", ToolName: "read_symbol", Arguments: `{"path":"pkg/a.go","symbol":"Foo"}`})
	history.Append(ToolCallResultRecord{SessionBaseRecord: history.NextRecord("tool_call_result"), CallID: "c3", ToolName: "read_symbol", Content: "func Foo()", IsError: false})
	history.Append(ToolCallRequestRecord{SessionBaseRecord: history.NextRecord("tool_call_request"), CallID: "c4", ToolName: "list_files", Arguments: `{"path":"pkg"}`})
	history.Append(ToolCallResultRecord{SessionBaseRecord: history.NextRecord("tool_call_result"), CallID: "c4", ToolName: "list_files", Content: "a.go", IsError: false})
	history.Append(AssistantMessageRecord{SessionBaseRecord: history.NextRecord("assistant"), Content: "first draft"})
	history.Append(AssistantMessageRecord{SessionBaseRecord: history.NextRecord("assistant"), Content: "final answer about Foo"})

	stats := ReduceSessionStats(history)
	if stats.FilesRead["pkg/a.go"] != 2 {
		t.Fatalf("files read = %+v, want pkg/a.go counted twice", stats.FilesRead)
	}
	if _, ok := stats.FilesRead["pkg/missing.go"]; ok {
		t.Fatalf("errored read should not count, got %+v", stats.FilesRead)
	}
	if len(stats.FilesRead) != 1 {
		t.Fatalf("files read = %+v, want only pkg/a.go", stats.FilesRead)
	}
	if stats.FinalAssistant != "final answer about Foo" {
		t.Fatalf("final assistant = %q", stats.FinalAssistant)
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

func TestReduceSessionStatsMarksRetriesRecoveredByLaterSuccess(t *testing.T) {
	history := NewSessionHistory("session-104")
	invalid := OutputContractEvaluationRecord{SessionBaseRecord: history.NextRecord("output_contract_evaluation"), StateName: "observe", ValidationStatus: "missing_schema_output"}
	history.Append(invalid)
	history.Append(RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "invalid_state_output", Attempt: 1, DerivedFrom: []string{invalid.RecordID()}})
	history.Append(RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "missing_state_output", Attempt: 2})
	valid := OutputContractEvaluationRecord{SessionBaseRecord: history.NextRecord("output_contract_evaluation"), StateName: "observe", ValidationStatus: "valid"}
	history.Append(valid)
	history.Append(RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "invalid_finalization_output", Attempt: 1})

	stats := ReduceSessionStats(history)
	if stats.Retry.Total != 3 || stats.Retry.Recovered != 2 || stats.Retry.Unrecovered != 1 {
		t.Fatalf("retry stats = %+v, want 2 recovered by later valid output and 1 unrecovered", stats.Retry)
	}
}

func TestReduceSessionStatsMarksArgumentRetryRecoveredByLaterValidToolCall(t *testing.T) {
	history := NewSessionHistory("session-105")
	failed := ToolValidationRecord{SessionBaseRecord: history.NextRecord("tool_validation"), CallID: "call-1", ToolName: "read_file", Valid: false, Reason: "missing_required_arguments", MissingFields: []string{"path"}}
	history.Append(failed)
	history.Append(RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "missing_required_arguments", Attempt: 1, DerivedFrom: []string{failed.RecordID()}})
	otherTool := ToolValidationRecord{SessionBaseRecord: history.NextRecord("tool_validation"), CallID: "call-2", ToolName: "list_files", Valid: true}
	history.Append(otherTool)

	stats := ReduceSessionStats(history)
	if stats.Retry.Recovered != 0 || stats.Retry.Unrecovered != 1 {
		t.Fatalf("retry stats = %+v, want unrecovered while only a different tool validated", stats.Retry)
	}

	fixed := ToolValidationRecord{SessionBaseRecord: history.NextRecord("tool_validation"), CallID: "call-3", ToolName: "read_file", Valid: true}
	history.Append(fixed)
	stats = ReduceSessionStats(history)
	if stats.Retry.Recovered != 1 || stats.Retry.Unrecovered != 0 {
		t.Fatalf("retry stats = %+v, want recovered after same tool validated", stats.Retry)
	}
}

func TestReduceSessionStatsExposesBucketOutputAndFinalizationReasons(t *testing.T) {
	history := NewSessionHistory("session-buckets")
	history.Append(OutputContractEvaluationRecord{SessionBaseRecord: history.NextRecord("output_contract_evaluation"), Chart: "cognitive", StateName: "observe", SchemaName: "cognitive_step_v1", ParseStatus: "valid_json_wrapped", ValidationStatus: "valid"})
	history.Append(OutputContractEvaluationRecord{SessionBaseRecord: history.NextRecord("output_contract_evaluation"), Chart: "workflow", StateName: "triaging", SchemaName: "triage_v1", ParseStatus: "valid_json_wrapped", ValidationStatus: "missing_required_fields", RequiredFields: []string{"decision"}, MissingFields: []string{"decision"}})
	history.Append(CompletionRecord{SessionBaseRecord: history.NextRecord("completion"), Completed: false, StopReason: "finalization_validation_failed", Iteration: 2, FinalizationReason: "cognitive_max_inference_turns+workflow_max_inference_turns"})

	stats := ReduceSessionStats(history)
	if stats.Output.ByChart["cognitive"].Valid != 1 {
		t.Fatalf("cognitive chart stats = %+v, want one valid", stats.Output.ByChart["cognitive"])
	}
	workflow := stats.Output.ByChart["workflow"]
	if workflow.Invalid != 1 || workflow.MissingRequired != 1 || workflow.MissingFieldCounts["decision"] != 1 {
		t.Fatalf("workflow chart stats = %+v, want invalid missing decision", workflow)
	}
	if stats.Finalization.Failures != 1 || stats.Finalization.ByStopReason["finalization_validation_failed"] != 1 {
		t.Fatalf("finalization stats = %+v, want one validation failure", stats.Finalization)
	}
	if stats.Finalization.ByBoundReason["cognitive_max_inference_turns+workflow_max_inference_turns"] != 1 {
		t.Fatalf("finalization bound reasons = %+v, want combined reason", stats.Finalization.ByBoundReason)
	}
}

func TestReduceSessionStatsTracksFinalWorkflowStateFromExit(t *testing.T) {
	history := NewSessionHistory("session-workflow-exit")
	history.Append(StateExitRecord{SessionBaseRecord: history.NextRecord("state_exit"), Chart: "workflow", StateName: "triaging", Reason: "finalized"})

	stats := ReduceSessionStats(history)
	if stats.FinalWorkflowState != "triaging" {
		t.Fatalf("final workflow state = %q, want triaging", stats.FinalWorkflowState)
	}
}

func TestReduceSessionStatsTracksFinalWorkflowStateFromTransition(t *testing.T) {
	history := NewSessionHistory("session-workflow-transition")
	history.Append(StateExitRecord{SessionBaseRecord: history.NextRecord("state_exit"), Chart: "workflow", StateName: "triaging", Reason: "transition"})
	history.Append(StateEnterRecord{SessionBaseRecord: history.NextRecord("state_enter"), Chart: "workflow", StateName: "done"})

	stats := ReduceSessionStats(history)
	if stats.FinalWorkflowState != "done" {
		t.Fatalf("final workflow state = %q, want done", stats.FinalWorkflowState)
	}
}

func TestReduceSessionStatsIgnoresCognitiveStateForFinalWorkflowState(t *testing.T) {
	history := NewSessionHistory("session-cognitive-only")
	history.Append(StateEnterRecord{SessionBaseRecord: history.NextRecord("state_enter"), Chart: "cognitive", StateName: "observe"})
	history.Append(StateExitRecord{SessionBaseRecord: history.NextRecord("state_exit"), Chart: "cognitive", StateName: "observe", Reason: "completed"})

	stats := ReduceSessionStats(history)
	if stats.FinalWorkflowState != "" {
		t.Fatalf("final workflow state = %q, want empty", stats.FinalWorkflowState)
	}
}

// TestReduceSessionStatsCountsWorkflowBoundReason verifies that a workflow-chart
// success exit with BoundReason is counted once in Finalization stats.
func TestReduceSessionStatsCountsWorkflowBoundReason(t *testing.T) {
	history := NewSessionHistory("session-workflow-bound")
	history.Append(StateExitRecord{SessionBaseRecord: history.NextRecord("state_exit"), Chart: "workflow", StateName: "triaging", Reason: "finalized", BoundReason: "workflow_max_tool_calls"})

	stats := ReduceSessionStats(history)
	if stats.Finalization.Completions != 1 {
		t.Fatalf("Finalization.Completions = %d, want 1", stats.Finalization.Completions)
	}
	if stats.Finalization.ByBoundReason["workflow_max_tool_calls"] != 1 {
		t.Fatalf("Finalization.ByBoundReason = %+v, want workflow_max_tool_calls=1", stats.Finalization.ByBoundReason)
	}
}

// TestReduceSessionStatsDedupesCombinedBoundReason verifies that combined
// finalization (cognitive + workflow exits sharing the same BoundReason) is
// counted only once via the workflow exit.
func TestReduceSessionStatsDedupesCombinedBoundReason(t *testing.T) {
	history := NewSessionHistory("session-combined-bound")
	history.Append(StateExitRecord{SessionBaseRecord: history.NextRecord("state_exit"), Chart: "cognitive", StateName: "observe", Reason: "completed", BoundReason: "cognitive_max_inference_turns+workflow_max_inference_turns"})
	history.Append(StateExitRecord{SessionBaseRecord: history.NextRecord("state_exit"), Chart: "workflow", StateName: "triaging", Reason: "finalized", BoundReason: "cognitive_max_inference_turns+workflow_max_inference_turns"})

	stats := ReduceSessionStats(history)
	if stats.Finalization.Completions != 1 {
		t.Fatalf("Finalization.Completions = %d, want 1 (deduped)", stats.Finalization.Completions)
	}
	if stats.Finalization.ByBoundReason["cognitive_max_inference_turns+workflow_max_inference_turns"] != 1 {
		t.Fatalf("Finalization.ByBoundReason = %+v, want combined reason=1", stats.Finalization.ByBoundReason)
	}
}

// TestReduceSessionStatsCountsCognitiveOnlyBoundReason verifies that a
// cognitive-only success exit with BoundReason is counted once.
func TestReduceSessionStatsCountsCognitiveOnlyBoundReason(t *testing.T) {
	history := NewSessionHistory("session-cognitive-bound")
	history.Append(StateExitRecord{SessionBaseRecord: history.NextRecord("state_exit"), Chart: "cognitive", StateName: "observe", Reason: "completed", BoundReason: "cognitive_max_inference_turns"})

	stats := ReduceSessionStats(history)
	if stats.Finalization.Completions != 1 {
		t.Fatalf("Finalization.Completions = %d, want 1", stats.Finalization.Completions)
	}
	if stats.Finalization.ByBoundReason["cognitive_max_inference_turns"] != 1 {
		t.Fatalf("Finalization.ByBoundReason = %+v, want cognitive_max_inference_turns=1", stats.Finalization.ByBoundReason)
	}
}

// TestReduceSessionStatsNoDoubleCountSessionEndingSuccess verifies that a
// session-ending success (exit with BoundReason PLUS CompletionRecord) is
// counted only once.
func TestReduceSessionStatsNoDoubleCountSessionEndingSuccess(t *testing.T) {
	history := NewSessionHistory("session-ending-success")
	history.Append(StateExitRecord{SessionBaseRecord: history.NextRecord("state_exit"), Chart: "cognitive", StateName: "observe", Reason: "completed", BoundReason: "cognitive_max_inference_turns"})
	history.Append(CompletionRecord{SessionBaseRecord: history.NextRecord("completion"), Completed: true, StopReason: "state_finalized", FinalizationReason: "cognitive_max_inference_turns"})

	stats := ReduceSessionStats(history)
	if stats.Finalization.Completions != 1 {
		t.Fatalf("Finalization.Completions = %d, want 1 (not 2)", stats.Finalization.Completions)
	}
	if stats.Finalization.ByBoundReason["cognitive_max_inference_turns"] != 1 {
		t.Fatalf("Finalization.ByBoundReason = %+v, want cognitive_max_inference_turns=1", stats.Finalization.ByBoundReason)
	}
	if stats.Finalization.ByStopReason["state_finalized"] != 1 {
		t.Fatalf("Finalization.ByStopReason = %+v, want state_finalized=1", stats.Finalization.ByStopReason)
	}
}

// TestReduceSessionStatsCountsFailureFromCompletionRecord verifies that a
// finalization failure (no BoundReason exits, only CompletionRecord) is
// counted correctly.
func TestReduceSessionStatsCountsFailureFromCompletionRecord(t *testing.T) {
	history := NewSessionHistory("session-finalization-failure")
	history.Append(CompletionRecord{SessionBaseRecord: history.NextRecord("completion"), Completed: false, StopReason: "finalization_validation_failed", FinalizationReason: "workflow_max_inference_turns"})

	stats := ReduceSessionStats(history)
	if stats.Finalization.Failures != 1 {
		t.Fatalf("Finalization.Failures = %d, want 1", stats.Finalization.Failures)
	}
	if stats.Finalization.ByBoundReason["workflow_max_inference_turns"] != 1 {
		t.Fatalf("Finalization.ByBoundReason = %+v, want workflow_max_inference_turns=1", stats.Finalization.ByBoundReason)
	}
	if stats.Finalization.ByStopReason["finalization_validation_failed"] != 1 {
		t.Fatalf("Finalization.ByStopReason = %+v, want finalization_validation_failed=1", stats.Finalization.ByStopReason)
	}
}
