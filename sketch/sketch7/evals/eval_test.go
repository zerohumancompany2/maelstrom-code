package evals

import (
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
)

func TestEvaluateSessionStatsPassesStrictHealthySession(t *testing.T) {
	stats := logs.SessionStats{
		SessionID: "session-healthy",
		AgentID:   "agent-a",
		Completion: logs.CompletionStats{
			LatestCompleted: true,
			Completed:       1,
			Total:           1,
		},
	}
	result := EvaluateSessionStats(stats, StrictRuntimeSessionEval("strict-runtime"))
	if !result.Passed {
		t.Fatalf("expected eval to pass: %+v", result)
	}
	if result.Metadata.SessionID != "session-healthy" || result.Metadata.AgentID != "agent-a" {
		t.Fatalf("unexpected metadata: %+v", result.Metadata)
	}
}

func TestEvaluateSessionStatsFailsOnRuntimeErrors(t *testing.T) {
	stats := logs.SessionStats{
		SessionID: "session-bad",
		Output: logs.OutputStats{
			Invalid:         1,
			MissingRequired: 1,
			WrongState:      1,
		},
		Tools: logs.ToolStats{
			InvalidProposals:  1,
			ExecutionFailures: 1,
		},
		Retry: logs.RetryStats{Unrecovered: 1},
		Completion: logs.CompletionStats{
			LatestCompleted: false,
			Incomplete:      1,
			Total:           1,
		},
	}
	result := EvaluateSessionStats(stats, StrictRuntimeSessionEval("strict-runtime"))
	if result.Passed {
		t.Fatalf("expected eval to fail: %+v", result)
	}
	failed := failedChecks(result)
	for _, name := range []string{"completed", "invalid_outputs", "invalid_tool_proposals", "tool_execution_failures", "unrecovered_retries", "missing_required_outputs", "wrong_state_outputs"} {
		if !failed[name] {
			t.Fatalf("expected failed check %q in %+v", name, result.Checks)
		}
	}
}

func TestEvaluateSessionStatsHonorsThresholds(t *testing.T) {
	stats := logs.SessionStats{
		Output: logs.OutputStats{Invalid: 1},
		Completion: logs.CompletionStats{
			LatestCompleted: false,
			Incomplete:      1,
			Total:           4,
		},
	}
	eval := SessionEvalCase{Name: "thresholded", MaxInvalidOutputs: 1, MaxCompletionFailureRate: 0.25}
	result := EvaluateSessionStats(stats, eval)
	if !result.Passed {
		t.Fatalf("expected thresholded eval to pass: %+v", result)
	}
}

func TestEvaluateAgentAggregatePassesAndFails(t *testing.T) {
	aggregate := logs.AgentAggregate{
		AgentID:          "agent-a",
		SessionCount:     2,
		CompletionRate:   0.5,
		InvalidRate:      0.25,
		ToolFailureRate:  0.1,
		RetryFailureRate: 0.0,
		Combined: logs.SessionStats{
			Output: logs.OutputStats{Invalid: 1},
			Tools:  logs.ToolStats{InvalidProposals: 1},
			Retry:  logs.RetryStats{Unrecovered: 0},
		},
	}
	passing := EvaluateAgentAggregate(aggregate, AggregateEvalCase{Name: "lenient", MinCompletionRate: 0.5, MaxInvalidRate: 0.25, MaxToolFailureRate: 0.1, MaxRetryFailureRate: 0, MaxInvalidOutputs: 1, MaxInvalidToolProposals: 1})
	if !passing.Passed {
		t.Fatalf("expected aggregate eval to pass: %+v", passing)
	}
	failing := EvaluateAgentAggregate(aggregate, StrictAgentAggregateEval("strict"))
	if failing.Passed {
		t.Fatalf("expected strict aggregate eval to fail: %+v", failing)
	}
	if failing.Metadata.AgentID != "agent-a" || failing.Metadata.SessionCount != 2 {
		t.Fatalf("unexpected metadata: %+v", failing.Metadata)
	}
}

func failedChecks(result EvalResult) map[string]bool {
	failed := map[string]bool{}
	for _, check := range result.Checks {
		if !check.Passed {
			failed[check.Name] = true
		}
	}
	return failed
}

func TestEvaluateSessionStatsChecksRequiredFilesRead(t *testing.T) {
	stats := logs.SessionStats{FilesRead: map[string]int{"./pkg/a.go": 1, "pkg/b.go": 2}}
	eval := SessionEvalCase{Name: "files", RequiredFilesRead: []string{"pkg/a.go", "pkg/b.go"}}
	result := EvaluateSessionStats(stats, eval)
	if !result.Passed {
		t.Fatalf("expected pass with cleaned path matching: %+v", result)
	}

	eval.RequiredFilesRead = append(eval.RequiredFilesRead, "pkg/c.go")
	result = EvaluateSessionStats(stats, eval)
	if result.Passed {
		t.Fatalf("expected fail on missing file: %+v", result)
	}
	found := false
	for _, check := range result.Checks {
		if check.Name == "required_files_read" && !check.Passed {
			found = true
			if !strings.Contains(check.Actual, "pkg/c.go") {
				t.Fatalf("actual should name missing file: %q", check.Actual)
			}
		}
	}
	if !found {
		t.Fatalf("missing required_files_read check: %+v", result.Checks)
	}
}

func TestEvaluateSessionStatsChecksFilesReadBudget(t *testing.T) {
	stats := logs.SessionStats{FilesRead: map[string]int{"a.go": 1, "b.go": 1, "c.go": 1}}
	if result := EvaluateSessionStats(stats, SessionEvalCase{MaxFilesRead: 3}); !result.Passed {
		t.Fatalf("expected pass at budget: %+v", result)
	}
	if result := EvaluateSessionStats(stats, SessionEvalCase{MaxFilesRead: 2}); result.Passed {
		t.Fatalf("expected fail over budget: %+v", result)
	}
}

func TestEvaluateSessionStatsChecksFinalOutputContains(t *testing.T) {
	stats := logs.SessionStats{FinalAssistant: "The Payload is assembled from Sections."}
	eval := SessionEvalCase{FinalOutputContains: []string{"payload", "sections"}}
	if result := EvaluateSessionStats(stats, eval); !result.Passed {
		t.Fatalf("expected case-insensitive pass: %+v", result)
	}
	eval.FinalOutputContains = []string{"payload", "transcript"}
	result := EvaluateSessionStats(stats, eval)
	if result.Passed {
		t.Fatalf("expected fail on missing term: %+v", result)
	}
}

func TestEvaluateSessionStatsChecksInvalidWorkflowOutputs(t *testing.T) {
	stats := logs.SessionStats{Output: logs.OutputStats{ByChart: map[string]logs.BucketOutputStats{"workflow": {Invalid: 2}}}}
	eval := SessionEvalCase{MaxInvalidWorkflowOutputs: 2}
	if result := EvaluateSessionStats(stats, eval); !result.Passed {
		t.Fatalf("expected pass at threshold: %+v", result)
	}
	eval.MaxInvalidWorkflowOutputs = 1
	result := EvaluateSessionStats(stats, eval)
	if result.Passed {
		t.Fatalf("expected fail over threshold: %+v", result)
	}
}

func TestEvaluateSessionStatsChecksValidWorkflowOutputs(t *testing.T) {
	stats := logs.SessionStats{Output: logs.OutputStats{ByChart: map[string]logs.BucketOutputStats{"workflow": {Valid: 3}}}}
	eval := SessionEvalCase{MinValidWorkflowOutputs: 3}
	if result := EvaluateSessionStats(stats, eval); !result.Passed {
		t.Fatalf("expected pass at threshold: %+v", result)
	}
	eval.MinValidWorkflowOutputs = 4
	result := EvaluateSessionStats(stats, eval)
	if result.Passed {
		t.Fatalf("expected fail below threshold: %+v", result)
	}
	// Check not added when zero
	eval = SessionEvalCase{MinValidWorkflowOutputs: 0}
	result = EvaluateSessionStats(stats, eval)
	for _, check := range result.Checks {
		if check.Name == "valid_workflow_outputs" {
			t.Fatalf("valid_workflow_outputs check should not be added when MinValidWorkflowOutputs is zero")
		}
	}
}

func TestEvaluateSessionStatsChecksStopReason(t *testing.T) {
	stats := logs.SessionStats{Completion: logs.CompletionStats{LatestStopReason: "state_finalized"}}
	eval := SessionEvalCase{RequiredStopReason: "state_finalized"}
	if result := EvaluateSessionStats(stats, eval); !result.Passed {
		t.Fatalf("expected pass on matching stop reason: %+v", result)
	}
	eval.RequiredStopReason = "loop_guard"
	result := EvaluateSessionStats(stats, eval)
	if result.Passed {
		t.Fatalf("expected fail on mismatched stop reason: %+v", result)
	}
	// Check not added when empty
	eval = SessionEvalCase{RequiredStopReason: ""}
	result = EvaluateSessionStats(stats, eval)
	for _, check := range result.Checks {
		if check.Name == "stop_reason" {
			t.Fatalf("stop_reason check should not be added when RequiredStopReason is empty")
		}
	}
}

func TestEvaluateSessionStatsChecksFinalizationReason(t *testing.T) {
	stats := logs.SessionStats{Finalization: logs.FinalizationStats{ByBoundReason: map[string]int{"cognitive_max_turns": 1}}}
	eval := SessionEvalCase{RequiredFinalizationReason: "cognitive_max_turns"}
	if result := EvaluateSessionStats(stats, eval); !result.Passed {
		t.Fatalf("expected pass on matching finalization reason: %+v", result)
	}
	eval.RequiredFinalizationReason = "workflow_max_turns"
	result := EvaluateSessionStats(stats, eval)
	if result.Passed {
		t.Fatalf("expected fail on missing finalization reason: %+v", result)
	}
	// Check not added when empty
	eval = SessionEvalCase{RequiredFinalizationReason: ""}
	result = EvaluateSessionStats(stats, eval)
	for _, check := range result.Checks {
		if check.Name == "finalization_reason" {
			t.Fatalf("finalization_reason check should not be added when RequiredFinalizationReason is empty")
		}
	}
}

func TestEvaluateSessionStatsChecksFinalWorkflowState(t *testing.T) {
	stats := logs.SessionStats{FinalWorkflowState: "done"}
	eval := SessionEvalCase{RequiredFinalWorkflowState: "done"}
	if result := EvaluateSessionStats(stats, eval); !result.Passed {
		t.Fatalf("expected pass on matching final workflow state: %+v", result)
	}
	eval.RequiredFinalWorkflowState = "triaging"
	result := EvaluateSessionStats(stats, eval)
	if result.Passed {
		t.Fatalf("expected fail on mismatched final workflow state: %+v", result)
	}
	// Check not added when empty
	eval = SessionEvalCase{RequiredFinalWorkflowState: ""}
	result = EvaluateSessionStats(stats, eval)
	for _, check := range result.Checks {
		if check.Name == "final_workflow_state" {
			t.Fatalf("final_workflow_state check should not be added when RequiredFinalWorkflowState is empty")
		}
	}
}
