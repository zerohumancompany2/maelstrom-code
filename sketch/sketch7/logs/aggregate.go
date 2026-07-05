package logs

import (
	"sort"
	"strings"
)

type AgentAggregate struct {
	AgentID                 string                   `json:"agent_id"`
	SessionCount            int                      `json:"session_count"`
	Combined                SessionStats             `json:"combined"`
	SessionIDs              []string                 `json:"session_ids"`
	TopInvalidSessions      []TopContributingSession `json:"top_invalid_sessions"`
	TopToolFailureSessions  []TopContributingSession `json:"top_tool_failure_sessions"`
	TopRetryFailureSessions []TopContributingSession `json:"top_retry_failure_sessions"`
	IncompleteSessions      []TopContributingSession `json:"incomplete_sessions"`
	CompletionRate          float64                  `json:"completion_rate"`
	InvalidRate             float64                  `json:"invalid_rate"`
	ToolFailureRate         float64                  `json:"tool_failure_rate"`
	RetryFailureRate        float64                  `json:"retry_failure_rate"`
}

type TopContributingSession struct {
	SessionID             string `json:"session_id"`
	AgentID               string `json:"agent_id"`
	InvalidOutputs        int    `json:"invalid_outputs"`
	ToolFailures          int    `json:"tool_failures"`
	UnrecoveredRetries    int    `json:"unrecovered_retries"`
	IncompleteCompletions int    `json:"incomplete_completions"`
}

type SessionAggregateReport struct {
	SessionDir string                    `json:"session_dir"`
	Agents     map[string]AgentAggregate `json:"agents"`
}

func AggregateSessionStatsByAgent(sessionDir string) (SessionAggregateReport, error) {
	return AggregateSessionStatsByAgentStore(FileSessionStore{SessionDir: sessionDir}, sessionDir)
}

func AggregateSessionStatsByAgentStore(store SessionStore, sessionDir string) (SessionAggregateReport, error) {
	report := SessionAggregateReport{SessionDir: sessionDir, Agents: map[string]AgentAggregate{}}
	sessionIDs, err := store.ListSessions()
	if err != nil {
		return report, err
	}
	for _, sessionID := range sessionIDs {
		session, _, err := store.LoadSession(sessionID)
		if err != nil {
			return report, err
		}
		agentID := session.AgentID
		if agentID == "" {
			agentID = "unknown"
		}
		stats := ReduceSessionStats(session)
		agg := report.Agents[agentID]
		agg.AgentID = agentID
		agg.SessionCount++
		agg.SessionIDs = append(agg.SessionIDs, session.SessionID)
		agg.Combined = combineSessionStats(agg.Combined, stats)
		contributor := topContributingSession(session.AgentID, stats)
		agg.TopInvalidSessions = append(agg.TopInvalidSessions, contributor)
		agg.TopToolFailureSessions = append(agg.TopToolFailureSessions, contributor)
		agg.TopRetryFailureSessions = append(agg.TopRetryFailureSessions, contributor)
		agg.IncompleteSessions = append(agg.IncompleteSessions, contributor)
		report.Agents[agentID] = agg
	}
	for agentID, agg := range report.Agents {
		agg.TopInvalidSessions = topSessions(agg.TopInvalidSessions, func(item TopContributingSession) int { return item.InvalidOutputs })
		agg.TopToolFailureSessions = topSessions(agg.TopToolFailureSessions, func(item TopContributingSession) int { return item.ToolFailures })
		agg.TopRetryFailureSessions = topSessions(agg.TopRetryFailureSessions, func(item TopContributingSession) int { return item.UnrecoveredRetries })
		agg.IncompleteSessions = topSessions(agg.IncompleteSessions, func(item TopContributingSession) int { return item.IncompleteCompletions })
		agg.CompletionRate = ratio(agg.Combined.Completion.Completed, agg.Combined.Completion.Total)
		agg.InvalidRate = ratio(agg.Combined.Output.Invalid, agg.Combined.Output.Total)
		agg.ToolFailureRate = ratio(agg.Combined.Tools.ExecutionFailures, agg.Combined.Tools.Executed)
		agg.RetryFailureRate = ratio(agg.Combined.Retry.Unrecovered, agg.Combined.Retry.Total)
		report.Agents[agentID] = agg
	}
	return report, nil
}

func combineSessionStats(a, b SessionStats) SessionStats {
	combined := SessionStats{
		SessionID:        a.SessionID,
		AgentID:          firstNonEmpty(a.AgentID, b.AgentID),
		RecordCounts:     combineRecordCounts(a.RecordCounts, b.RecordCounts),
		Output:           combineOutputStats(a.Output, b.Output),
		Tools:            combineToolStats(a.Tools, b.Tools),
		Retry:            combineRetryStats(a.Retry, b.Retry),
		Completion:       combineCompletionStats(a.Completion, b.Completion),
		ByTool:           combinePerToolStats(a.ByTool, b.ByTool),
		ByState:          combinePerStateStats(a.ByState, b.ByState),
		ModelRefs:        combineCountMaps(a.ModelRefs, b.ModelRefs),
		ProviderRefs:     combineCountMaps(a.ProviderRefs, b.ProviderRefs),
		StopReasons:      combineCountMaps(a.StopReasons, b.StopReasons),
		StateExitReasons: combineCountMaps(a.StateExitReasons, b.StateExitReasons),
		RetryByReason:    combineCountMaps(a.RetryByReason, b.RetryByReason),
		OutputStatuses:   combineCountMaps(a.OutputStatuses, b.OutputStatuses),
		ParseStatuses:    combineCountMaps(a.ParseStatuses, b.ParseStatuses),
		FilesRead:        combineCountMaps(a.FilesRead, b.FilesRead),
		FinalAssistant:   firstNonEmpty(b.FinalAssistant, a.FinalAssistant),
	}
	if combined.SessionID == "" {
		combined.SessionID = b.SessionID
	}
	return combined
}

func combineRecordCounts(a, b RecordCounts) RecordCounts {
	return RecordCounts{
		Total:             a.Total + b.Total,
		Users:             a.Users + b.Users,
		Assistants:        a.Assistants + b.Assistants,
		ToolRequests:      a.ToolRequests + b.ToolRequests,
		ToolResults:       a.ToolResults + b.ToolResults,
		OutputEvaluations: a.OutputEvaluations + b.OutputEvaluations,
		ToolValidations:   a.ToolValidations + b.ToolValidations,
		Retries:           a.Retries + b.Retries,
		Completions:       a.Completions + b.Completions,
	}
}

func combineOutputStats(a, b OutputStats) OutputStats {
	return OutputStats{
		Total:                 a.Total + b.Total,
		Valid:                 a.Valid + b.Valid,
		Invalid:               a.Invalid + b.Invalid,
		MissingRequired:       a.MissingRequired + b.MissingRequired,
		WrongState:            a.WrongState + b.WrongState,
		ByValidationStatus:    combineCountMaps(a.ByValidationStatus, b.ByValidationStatus),
		ByParseStatus:         combineCountMaps(a.ByParseStatus, b.ByParseStatus),
		BySchema:              combineCountMaps(a.BySchema, b.BySchema),
		ByActionType:          combineCountMaps(a.ByActionType, b.ByActionType),
		ByTool:                combineCountMaps(a.ByTool, b.ByTool),
		MissingFieldCounts:    combineCountMaps(a.MissingFieldCounts, b.MissingFieldCounts),
		CompletionSignalTrue:  a.CompletionSignalTrue + b.CompletionSignalTrue,
		CompletionSignalFalse: a.CompletionSignalFalse + b.CompletionSignalFalse,
	}
}

func combineToolStats(a, b ToolStats) ToolStats {
	return ToolStats{
		Proposed:          a.Proposed + b.Proposed,
		ValidProposals:    a.ValidProposals + b.ValidProposals,
		InvalidProposals:  a.InvalidProposals + b.InvalidProposals,
		Executed:          a.Executed + b.Executed,
		ExecutionSuccess:  a.ExecutionSuccess + b.ExecutionSuccess,
		ExecutionFailures: a.ExecutionFailures + b.ExecutionFailures,
		ByReason:          combineCountMaps(a.ByReason, b.ByReason),
	}
}

func combineRetryStats(a, b RetryStats) RetryStats {
	return RetryStats{
		Total:       a.Total + b.Total,
		Recovered:   a.Recovered + b.Recovered,
		Unrecovered: a.Unrecovered + b.Unrecovered,
		ByReason:    combineCountMaps(a.ByReason, b.ByReason),
		Attribution: RetryAttributionStats{
			ByCauseKind: combineCountMaps(a.Attribution.ByCauseKind, b.Attribution.ByCauseKind),
			ByTool:      combineCountMaps(a.Attribution.ByTool, b.Attribution.ByTool),
			ByState:     combineCountMaps(a.Attribution.ByState, b.Attribution.ByState),
		},
	}
}

func combineCompletionStats(a, b CompletionStats) CompletionStats {
	result := CompletionStats{
		Total:      a.Total + b.Total,
		Completed:  a.Completed + b.Completed,
		Incomplete: a.Incomplete + b.Incomplete,
	}
	if stringsTrimNonEmpty(b.LatestStopReason) {
		result.LatestStopReason = b.LatestStopReason
		result.LatestCompleted = b.LatestCompleted
		result.LatestIteration = b.LatestIteration
		result.LatestCognitiveState = b.LatestCognitiveState
		result.LatestWorkflowState = b.LatestWorkflowState
	} else {
		result.LatestStopReason = a.LatestStopReason
		result.LatestCompleted = a.LatestCompleted
		result.LatestIteration = a.LatestIteration
		result.LatestCognitiveState = a.LatestCognitiveState
		result.LatestWorkflowState = a.LatestWorkflowState
	}
	return result
}

func combinePerToolStats(a, b map[string]PerToolStats) map[string]PerToolStats {
	result := map[string]PerToolStats{}
	for key, value := range a {
		result[key] = value
	}
	for key, value := range b {
		current := result[key]
		current.Proposed += value.Proposed
		current.ValidProposals += value.ValidProposals
		current.InvalidProposals += value.InvalidProposals
		current.Executed += value.Executed
		current.ExecutionSuccess += value.ExecutionSuccess
		current.ExecutionFailures += value.ExecutionFailures
		current.InvalidReasons = combineCountMaps(current.InvalidReasons, value.InvalidReasons)
		current.MissingFieldCounts = combineCountMaps(current.MissingFieldCounts, value.MissingFieldCounts)
		result[key] = current
	}
	return result
}

func combinePerStateStats(a, b map[string]PerStateStats) map[string]PerStateStats {
	result := map[string]PerStateStats{}
	for key, value := range a {
		result[key] = value
	}
	for key, value := range b {
		current := result[key]
		current.OutputEvaluations += value.OutputEvaluations
		current.Valid += value.Valid
		current.Invalid += value.Invalid
		current.MissingRequired += value.MissingRequired
		current.WrongState += value.WrongState
		current.ByValidationStatus = combineCountMaps(current.ByValidationStatus, value.ByValidationStatus)
		current.ByParseStatus = combineCountMaps(current.ByParseStatus, value.ByParseStatus)
		current.MissingFieldCounts = combineCountMaps(current.MissingFieldCounts, value.MissingFieldCounts)
		result[key] = current
	}
	return result
}

func combineCountMaps(a, b map[string]int) map[string]int {
	result := map[string]int{}
	for key, value := range a {
		result[key] = value
	}
	for key, value := range b {
		result[key] += value
	}
	return result
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func stringsTrimNonEmpty(value string) bool {
	return strings.TrimSpace(value) != ""
}

func topContributingSession(agentID string, stats SessionStats) TopContributingSession {
	return TopContributingSession{
		SessionID:             stats.SessionID,
		AgentID:               agentID,
		InvalidOutputs:        stats.Output.Invalid,
		ToolFailures:          stats.Tools.ExecutionFailures,
		UnrecoveredRetries:    stats.Retry.Unrecovered,
		IncompleteCompletions: stats.Completion.Incomplete,
	}
}

func topSessions(items []TopContributingSession, score func(TopContributingSession) int) []TopContributingSession {
	filtered := make([]TopContributingSession, 0, len(items))
	for _, item := range items {
		if score(item) > 0 {
			filtered = append(filtered, item)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left := score(filtered[i])
		right := score(filtered[j])
		if left == right {
			return filtered[i].SessionID < filtered[j].SessionID
		}
		return left > right
	})
	if len(filtered) > 5 {
		return filtered[:5]
	}
	return filtered
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
