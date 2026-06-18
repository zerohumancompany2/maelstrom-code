package evals

import (
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
