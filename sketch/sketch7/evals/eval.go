package evals

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
)

type SessionEvalCase struct {
	Name                     string   `yaml:"name" json:"name"`
	RequireCompleted         bool     `yaml:"require_completed" json:"require_completed"`
	MaxInvalidOutputs        int      `yaml:"max_invalid_outputs" json:"max_invalid_outputs"`
	MaxInvalidToolProposals  int      `yaml:"max_invalid_tool_proposals" json:"max_invalid_tool_proposals"`
	MaxToolExecutionFailures int      `yaml:"max_tool_execution_failures" json:"max_tool_execution_failures"`
	MaxUnrecoveredRetries    int      `yaml:"max_unrecovered_retries" json:"max_unrecovered_retries"`
	MaxMissingRequired       int      `yaml:"max_missing_required" json:"max_missing_required"`
	MaxWrongState            int      `yaml:"max_wrong_state" json:"max_wrong_state"`
	MaxCompletionFailureRate float64  `yaml:"max_completion_failure_rate" json:"max_completion_failure_rate"`
	RequiredFilesRead        []string `yaml:"required_files_read" json:"required_files_read,omitempty"`
	MaxFilesRead             int      `yaml:"max_files_read" json:"max_files_read,omitempty"`
	FinalOutputContains      []string `yaml:"final_output_contains" json:"final_output_contains,omitempty"`
}

type AggregateEvalCase struct {
	Name                    string  `json:"name"`
	MinCompletionRate       float64 `json:"min_completion_rate"`
	MaxInvalidRate          float64 `json:"max_invalid_rate"`
	MaxToolFailureRate      float64 `json:"max_tool_failure_rate"`
	MaxRetryFailureRate     float64 `json:"max_retry_failure_rate"`
	MaxInvalidOutputs       int     `json:"max_invalid_outputs"`
	MaxInvalidToolProposals int     `json:"max_invalid_tool_proposals"`
	MaxUnrecoveredRetries   int     `json:"max_unrecovered_retries"`
}

type EvalResult struct {
	Name     string       `json:"name"`
	Passed   bool         `json:"passed"`
	Checks   []EvalCheck  `json:"checks"`
	Metadata EvalMetadata `json:"metadata"`
}

type EvalMetadata struct {
	SessionID    string `json:"session_id,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	SessionCount int    `json:"session_count,omitempty"`
}

type EvalCheck struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

func EvaluateSessionStats(stats logs.SessionStats, eval SessionEvalCase) EvalResult {
	result := EvalResult{Name: eval.Name, Passed: true, Metadata: EvalMetadata{SessionID: stats.SessionID, AgentID: stats.AgentID}}
	if eval.RequireCompleted {
		result.addCheck("completed", stats.Completion.LatestCompleted, "completed=true", boolString(stats.Completion.LatestCompleted))
	}
	result.addCheck("invalid_outputs", stats.Output.Invalid <= eval.MaxInvalidOutputs, lessOrEqual(eval.MaxInvalidOutputs), intString(stats.Output.Invalid))
	result.addCheck("invalid_tool_proposals", stats.Tools.InvalidProposals <= eval.MaxInvalidToolProposals, lessOrEqual(eval.MaxInvalidToolProposals), intString(stats.Tools.InvalidProposals))
	result.addCheck("tool_execution_failures", stats.Tools.ExecutionFailures <= eval.MaxToolExecutionFailures, lessOrEqual(eval.MaxToolExecutionFailures), intString(stats.Tools.ExecutionFailures))
	result.addCheck("unrecovered_retries", stats.Retry.Unrecovered <= eval.MaxUnrecoveredRetries, lessOrEqual(eval.MaxUnrecoveredRetries), intString(stats.Retry.Unrecovered))
	result.addCheck("missing_required_outputs", stats.Output.MissingRequired <= eval.MaxMissingRequired, lessOrEqual(eval.MaxMissingRequired), intString(stats.Output.MissingRequired))
	result.addCheck("wrong_state_outputs", stats.Output.WrongState <= eval.MaxWrongState, lessOrEqual(eval.MaxWrongState), intString(stats.Output.WrongState))
	if eval.MaxCompletionFailureRate > 0 {
		failureRate := ratio(stats.Completion.Incomplete, stats.Completion.Total)
		result.addCheck("completion_failure_rate", failureRate <= eval.MaxCompletionFailureRate, lessOrEqualFloat(eval.MaxCompletionFailureRate), floatString(failureRate))
	}
	if len(eval.RequiredFilesRead) > 0 {
		missing := missingFiles(stats.FilesRead, eval.RequiredFilesRead)
		total := len(eval.RequiredFilesRead)
		actual := fmt.Sprintf("%d/%d read", total-len(missing), total)
		if len(missing) > 0 {
			actual += " (missing: " + strings.Join(missing, ", ") + ")"
		}
		result.addCheck("required_files_read", len(missing) == 0, fmt.Sprintf("%d/%d read", total, total), actual)
	}
	if eval.MaxFilesRead > 0 {
		result.addCheck("files_read_budget", len(stats.FilesRead) <= eval.MaxFilesRead, lessOrEqual(eval.MaxFilesRead), intString(len(stats.FilesRead)))
	}
	if len(eval.FinalOutputContains) > 0 {
		missing := missingTerms(stats.FinalAssistant, eval.FinalOutputContains)
		total := len(eval.FinalOutputContains)
		actual := fmt.Sprintf("%d/%d terms", total-len(missing), total)
		if len(missing) > 0 {
			actual += " (missing: " + strings.Join(missing, ", ") + ")"
		}
		result.addCheck("final_output_contains", len(missing) == 0, fmt.Sprintf("%d/%d terms", total, total), actual)
	}
	return result
}

// missingFiles returns the required paths not present in the files-read map.
// Paths are cleaned on both sides so "./a/b.go" matches "a/b.go".
func missingFiles(filesRead map[string]int, required []string) []string {
	normalized := map[string]bool{}
	for filePath := range filesRead {
		normalized[path.Clean(filePath)] = true
	}
	missing := []string{}
	for _, filePath := range required {
		if !normalized[path.Clean(strings.TrimSpace(filePath))] {
			missing = append(missing, filePath)
		}
	}
	return missing
}

// missingTerms returns the terms not found (case-insensitive) in content.
func missingTerms(content string, terms []string) []string {
	lowered := strings.ToLower(content)
	missing := []string{}
	for _, term := range terms {
		if !strings.Contains(lowered, strings.ToLower(strings.TrimSpace(term))) {
			missing = append(missing, term)
		}
	}
	return missing
}

func EvaluateAgentAggregate(aggregate logs.AgentAggregate, eval AggregateEvalCase) EvalResult {
	result := EvalResult{Name: eval.Name, Passed: true, Metadata: EvalMetadata{AgentID: aggregate.AgentID, SessionCount: aggregate.SessionCount}}
	result.addCheck("completion_rate", aggregate.CompletionRate >= eval.MinCompletionRate, greaterOrEqualFloat(eval.MinCompletionRate), floatString(aggregate.CompletionRate))
	result.addCheck("invalid_rate", aggregate.InvalidRate <= eval.MaxInvalidRate, lessOrEqualFloat(eval.MaxInvalidRate), floatString(aggregate.InvalidRate))
	result.addCheck("tool_failure_rate", aggregate.ToolFailureRate <= eval.MaxToolFailureRate, lessOrEqualFloat(eval.MaxToolFailureRate), floatString(aggregate.ToolFailureRate))
	result.addCheck("retry_failure_rate", aggregate.RetryFailureRate <= eval.MaxRetryFailureRate, lessOrEqualFloat(eval.MaxRetryFailureRate), floatString(aggregate.RetryFailureRate))
	result.addCheck("invalid_outputs", aggregate.Combined.Output.Invalid <= eval.MaxInvalidOutputs, lessOrEqual(eval.MaxInvalidOutputs), intString(aggregate.Combined.Output.Invalid))
	result.addCheck("invalid_tool_proposals", aggregate.Combined.Tools.InvalidProposals <= eval.MaxInvalidToolProposals, lessOrEqual(eval.MaxInvalidToolProposals), intString(aggregate.Combined.Tools.InvalidProposals))
	result.addCheck("unrecovered_retries", aggregate.Combined.Retry.Unrecovered <= eval.MaxUnrecoveredRetries, lessOrEqual(eval.MaxUnrecoveredRetries), intString(aggregate.Combined.Retry.Unrecovered))
	return result
}

func (r *EvalResult) addCheck(name string, passed bool, expected, actual string) {
	r.Checks = append(r.Checks, EvalCheck{Name: name, Passed: passed, Expected: expected, Actual: actual})
	if !passed {
		r.Passed = false
	}
}

func StrictRuntimeSessionEval(name string) SessionEvalCase {
	return SessionEvalCase{Name: name, RequireCompleted: true}
}

func StrictAgentAggregateEval(name string) AggregateEvalCase {
	return AggregateEvalCase{Name: name, MinCompletionRate: 1.0}
}

func intString(value int) string {
	return strconv.Itoa(value)
}

func boolString(value bool) string {
	return strconv.FormatBool(value)
}

func floatString(value float64) string {
	return fmt.Sprintf("%.4f", value)
}

func lessOrEqual(value int) string {
	return "<= " + intString(value)
}

func greaterOrEqualFloat(value float64) string {
	return ">= " + floatString(value)
}

func lessOrEqualFloat(value float64) string {
	return "<= " + floatString(value)
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
