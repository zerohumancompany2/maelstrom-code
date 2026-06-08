package logs

import "sort"

type SessionStats struct {
	SessionID      string                    `json:"session_id"`
	RecordCounts   RecordCounts              `json:"record_counts"`
	Output         OutputStats               `json:"output"`
	Tools          ToolStats                 `json:"tools"`
	Retry          RetryStats                `json:"retry"`
	Completion     CompletionStats           `json:"completion"`
	ByTool         map[string]PerToolStats   `json:"by_tool"`
	ByState        map[string]PerStateStats  `json:"by_state"`
	StopReasons    map[string]int            `json:"stop_reasons"`
	RetryByReason  map[string]int            `json:"retry_by_reason"`
	OutputStatuses map[string]int            `json:"output_statuses"`
	ParseStatuses  map[string]int            `json:"parse_statuses"`
}

type RecordCounts struct {
	Total             int `json:"total"`
	Users             int `json:"users"`
	Assistants        int `json:"assistants"`
	ToolRequests      int `json:"tool_requests"`
	ToolResults       int `json:"tool_results"`
	OutputEvaluations int `json:"output_evaluations"`
	ToolValidations   int `json:"tool_validations"`
	Retries           int `json:"retries"`
	Completions       int `json:"completions"`
}

type OutputStats struct {
	Total               int            `json:"total"`
	Valid               int            `json:"valid"`
	Invalid             int            `json:"invalid"`
	MissingRequired     int            `json:"missing_required"`
	WrongState          int            `json:"wrong_state"`
	ByValidationStatus  map[string]int `json:"by_validation_status"`
	ByParseStatus       map[string]int `json:"by_parse_status"`
}

type ToolStats struct {
	Proposed          int            `json:"proposed"`
	ValidProposals    int            `json:"valid_proposals"`
	InvalidProposals  int            `json:"invalid_proposals"`
	Executed          int            `json:"executed"`
	ExecutionSuccess  int            `json:"execution_success"`
	ExecutionFailures int            `json:"execution_failures"`
	ByReason          map[string]int `json:"by_reason"`
}

type RetryStats struct {
	Total       int            `json:"total"`
	Recovered   int            `json:"recovered"`
	Unrecovered int            `json:"unrecovered"`
	ByReason    map[string]int `json:"by_reason"`
}

type CompletionStats struct {
	Total            int    `json:"total"`
	Completed        int    `json:"completed"`
	Incomplete       int    `json:"incomplete"`
	LatestCompleted  bool   `json:"latest_completed"`
	LatestStopReason string `json:"latest_stop_reason"`
}

type PerToolStats struct {
	Proposed          int `json:"proposed"`
	ValidProposals    int `json:"valid_proposals"`
	InvalidProposals  int `json:"invalid_proposals"`
	Executed          int `json:"executed"`
	ExecutionSuccess  int `json:"execution_success"`
	ExecutionFailures int `json:"execution_failures"`
}

type PerStateStats struct {
	OutputEvaluations int `json:"output_evaluations"`
	Valid             int `json:"valid"`
	Invalid           int `json:"invalid"`
	MissingRequired   int `json:"missing_required"`
	WrongState        int `json:"wrong_state"`
}

func ReduceSessionStats(history *SessionHistory) SessionStats {
	stats := SessionStats{
		ByTool:         map[string]PerToolStats{},
		ByState:        map[string]PerStateStats{},
		StopReasons:    map[string]int{},
		RetryByReason:  map[string]int{},
		OutputStatuses: map[string]int{},
		ParseStatuses:  map[string]int{},
		Output: OutputStats{
			ByValidationStatus: map[string]int{},
			ByParseStatus:      map[string]int{},
		},
		Tools: ToolStats{ByReason: map[string]int{}},
		Retry: RetryStats{ByReason: map[string]int{}},
	}
	if history == nil {
		return stats
	}
	stats.SessionID = history.SessionID
	for _, record := range history.Records {
		stats.RecordCounts.Total++
		switch v := record.(type) {
		case UserMessageRecord:
			stats.RecordCounts.Users++
		case *UserMessageRecord:
			stats.RecordCounts.Users++
		case AssistantMessageRecord:
			stats.RecordCounts.Assistants++
		case *AssistantMessageRecord:
			stats.RecordCounts.Assistants++
		case ToolCallRequestRecord:
			stats.RecordCounts.ToolRequests++
		case *ToolCallRequestRecord:
			stats.RecordCounts.ToolRequests++
		case ToolCallResultRecord:
			stats.RecordCounts.ToolResults++
			stats = reduceToolResult(stats, v.ToolName, v.IsError)
		case *ToolCallResultRecord:
			stats.RecordCounts.ToolResults++
			stats = reduceToolResult(stats, v.ToolName, v.IsError)
		case OutputContractEvaluationRecord:
			stats.RecordCounts.OutputEvaluations++
			stats = reduceOutput(stats, v)
		case *OutputContractEvaluationRecord:
			stats.RecordCounts.OutputEvaluations++
			stats = reduceOutput(stats, *v)
		case ToolValidationRecord:
			stats.RecordCounts.ToolValidations++
			stats = reduceToolValidation(stats, v)
		case *ToolValidationRecord:
			stats.RecordCounts.ToolValidations++
			stats = reduceToolValidation(stats, *v)
		case RetryRecord:
			stats.RecordCounts.Retries++
			stats = reduceRetry(stats, v)
		case *RetryRecord:
			stats.RecordCounts.Retries++
			stats = reduceRetry(stats, *v)
		case CompletionRecord:
			stats.RecordCounts.Completions++
			stats = reduceCompletion(stats, v)
		case *CompletionRecord:
			stats.RecordCounts.Completions++
			stats = reduceCompletion(stats, *v)
		}
	}
	return stats
}

func reduceOutput(stats SessionStats, record OutputContractEvaluationRecord) SessionStats {
	stats.Output.Total++
	stats.Output.ByValidationStatus[record.ValidationStatus]++
	stats.Output.ByParseStatus[record.ParseStatus]++
	stats.OutputStatuses[record.ValidationStatus]++
	stats.ParseStatuses[record.ParseStatus]++
	stateStats := stats.ByState[record.StateName]
	stateStats.OutputEvaluations++
	switch record.ValidationStatus {
	case "valid":
		stats.Output.Valid++
		stateStats.Valid++
	default:
		stats.Output.Invalid++
		stateStats.Invalid++
	}
	if len(record.MissingFields) > 0 {
		stats.Output.MissingRequired++
		stateStats.MissingRequired++
	}
	if record.WrongState {
		stats.Output.WrongState++
		stateStats.WrongState++
	}
	stats.ByState[record.StateName] = stateStats
	return stats
}

func reduceToolValidation(stats SessionStats, record ToolValidationRecord) SessionStats {
	stats.Tools.Proposed++
	toolStats := stats.ByTool[record.ToolName]
	toolStats.Proposed++
	if record.Valid {
		stats.Tools.ValidProposals++
		toolStats.ValidProposals++
	} else {
		stats.Tools.InvalidProposals++
		toolStats.InvalidProposals++
		stats.Tools.ByReason[record.Reason]++
	}
	stats.ByTool[record.ToolName] = toolStats
	return stats
}

func reduceToolResult(stats SessionStats, toolName string, isError bool) SessionStats {
	stats.Tools.Executed++
	toolStats := stats.ByTool[toolName]
	toolStats.Executed++
	if isError {
		stats.Tools.ExecutionFailures++
		toolStats.ExecutionFailures++
	} else {
		stats.Tools.ExecutionSuccess++
		toolStats.ExecutionSuccess++
	}
	stats.ByTool[toolName] = toolStats
	return stats
}

func reduceRetry(stats SessionStats, record RetryRecord) SessionStats {
	stats.Retry.Total++
	stats.Retry.ByReason[record.Reason]++
	stats.RetryByReason[record.Reason]++
	if record.Recovered {
		stats.Retry.Recovered++
	} else {
		stats.Retry.Unrecovered++
	}
	return stats
}

func reduceCompletion(stats SessionStats, record CompletionRecord) SessionStats {
	stats.Completion.Total++
	stats.StopReasons[record.StopReason]++
	stats.Completion.LatestCompleted = record.Completed
	stats.Completion.LatestStopReason = record.StopReason
	if record.Completed {
		stats.Completion.Completed++
	} else {
		stats.Completion.Incomplete++
	}
	return stats
}

func SortedMapKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
