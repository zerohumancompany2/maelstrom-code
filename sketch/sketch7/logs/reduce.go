package logs

import "sort"

type SessionStats struct {
	SessionID      string                   `json:"session_id"`
	AgentID        string                   `json:"agent_id"`
	RecordCounts   RecordCounts             `json:"record_counts"`
	Output         OutputStats              `json:"output"`
	Tools          ToolStats                `json:"tools"`
	Retry          RetryStats               `json:"retry"`
	Completion     CompletionStats          `json:"completion"`
	ByTool         map[string]PerToolStats  `json:"by_tool"`
	ByState        map[string]PerStateStats `json:"by_state"`
	ModelRefs      map[string]int           `json:"model_refs"`
	ProviderRefs   map[string]int           `json:"provider_refs"`
	StopReasons    map[string]int           `json:"stop_reasons"`
	RetryByReason  map[string]int           `json:"retry_by_reason"`
	OutputStatuses map[string]int           `json:"output_statuses"`
	ParseStatuses  map[string]int           `json:"parse_statuses"`
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
	Total                 int            `json:"total"`
	Valid                 int            `json:"valid"`
	Invalid               int            `json:"invalid"`
	MissingRequired       int            `json:"missing_required"`
	WrongState            int            `json:"wrong_state"`
	ByValidationStatus    map[string]int `json:"by_validation_status"`
	ByParseStatus         map[string]int `json:"by_parse_status"`
	BySchema              map[string]int `json:"by_schema"`
	ByActionType          map[string]int `json:"by_action_type"`
	ByTool                map[string]int `json:"by_tool"`
	MissingFieldCounts    map[string]int `json:"missing_field_counts"`
	CompletionSignalTrue  int            `json:"completion_signal_true"`
	CompletionSignalFalse int            `json:"completion_signal_false"`
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
	Total       int                   `json:"total"`
	Recovered   int                   `json:"recovered"`
	Unrecovered int                   `json:"unrecovered"`
	ByReason    map[string]int        `json:"by_reason"`
	Attribution RetryAttributionStats `json:"attribution"`
}

type RetryAttributionStats struct {
	ByCauseKind map[string]int `json:"by_cause_kind"`
	ByTool      map[string]int `json:"by_tool"`
	ByState     map[string]int `json:"by_state"`
}

type CompletionStats struct {
	Total                int    `json:"total"`
	Completed            int    `json:"completed"`
	Incomplete           int    `json:"incomplete"`
	LatestCompleted      bool   `json:"latest_completed"`
	LatestStopReason     string `json:"latest_stop_reason"`
	LatestIteration      int    `json:"latest_iteration"`
	LatestCognitiveState string `json:"latest_cognitive_state"`
	LatestWorkflowState  string `json:"latest_workflow_state"`
}

type PerToolStats struct {
	Proposed           int            `json:"proposed"`
	ValidProposals     int            `json:"valid_proposals"`
	InvalidProposals   int            `json:"invalid_proposals"`
	Executed           int            `json:"executed"`
	ExecutionSuccess   int            `json:"execution_success"`
	ExecutionFailures  int            `json:"execution_failures"`
	InvalidReasons     map[string]int `json:"invalid_reasons"`
	MissingFieldCounts map[string]int `json:"missing_field_counts"`
}

type PerStateStats struct {
	OutputEvaluations  int            `json:"output_evaluations"`
	Valid              int            `json:"valid"`
	Invalid            int            `json:"invalid"`
	MissingRequired    int            `json:"missing_required"`
	WrongState         int            `json:"wrong_state"`
	ByValidationStatus map[string]int `json:"by_validation_status"`
	ByParseStatus      map[string]int `json:"by_parse_status"`
	MissingFieldCounts map[string]int `json:"missing_field_counts"`
}

type retryAttribution struct {
	CauseKind string
	ToolName  string
	StateName string
}

func ReduceSessionStats(history *SessionHistory) SessionStats {
	stats := SessionStats{
		ByTool:         map[string]PerToolStats{},
		ByState:        map[string]PerStateStats{},
		ModelRefs:      map[string]int{},
		ProviderRefs:   map[string]int{},
		StopReasons:    map[string]int{},
		RetryByReason:  map[string]int{},
		OutputStatuses: map[string]int{},
		ParseStatuses:  map[string]int{},
		Output: OutputStats{
			ByValidationStatus: map[string]int{},
			ByParseStatus:      map[string]int{},
			BySchema:           map[string]int{},
			ByActionType:       map[string]int{},
			ByTool:             map[string]int{},
			MissingFieldCounts: map[string]int{},
		},
		Tools: ToolStats{ByReason: map[string]int{}},
		Retry: RetryStats{ByReason: map[string]int{}, Attribution: RetryAttributionStats{ByCauseKind: map[string]int{}, ByTool: map[string]int{}, ByState: map[string]int{}}},
	}
	if history == nil {
		return stats
	}
	stats.SessionID = history.SessionID
	stats.AgentID = history.AgentID
	index := buildRecordIndex(history)
	latestCognitiveState := ""
	latestWorkflowState := ""
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
			stats = reduceRetry(stats, v, index)
		case *RetryRecord:
			stats.RecordCounts.Retries++
			stats = reduceRetry(stats, *v, index)
		case CompletionRecord:
			stats.RecordCounts.Completions++
			stats = reduceCompletion(stats, v, latestCognitiveState, latestWorkflowState)
		case *CompletionRecord:
			stats.RecordCounts.Completions++
			stats = reduceCompletion(stats, *v, latestCognitiveState, latestWorkflowState)
		case CognitiveTransitionRecord:
			latestCognitiveState = v.ToState
		case *CognitiveTransitionRecord:
			latestCognitiveState = v.ToState
		case WorkflowTransitionRefRecord:
			latestWorkflowState = v.ToState
		case *WorkflowTransitionRefRecord:
			latestWorkflowState = v.ToState
		case InferenceEnvelopeRecord:
			incrementIfPresent(stats.ModelRefs, v.ModelRef)
			incrementIfPresent(stats.ProviderRefs, v.ProviderRef)
		case *InferenceEnvelopeRecord:
			incrementIfPresent(stats.ModelRefs, v.ModelRef)
			incrementIfPresent(stats.ProviderRefs, v.ProviderRef)
		}
	}
	return stats
}

func reduceOutput(stats SessionStats, record OutputContractEvaluationRecord) SessionStats {
	stats.Output.Total++
	incrementIfPresent(stats.Output.ByValidationStatus, record.ValidationStatus)
	incrementIfPresent(stats.Output.ByParseStatus, record.ParseStatus)
	incrementIfPresent(stats.Output.BySchema, record.SchemaName)
	incrementIfPresent(stats.Output.ByActionType, record.ActionType)
	incrementIfPresent(stats.Output.ByTool, record.ToolName)
	incrementIfPresent(stats.OutputStatuses, record.ValidationStatus)
	incrementIfPresent(stats.ParseStatuses, record.ParseStatus)
	if record.CompletionSignal {
		stats.Output.CompletionSignalTrue++
	} else {
		stats.Output.CompletionSignalFalse++
	}
	stateStats := stats.ByState[record.StateName]
	ensurePerStateMaps(&stateStats)
	stateStats.OutputEvaluations++
	incrementIfPresent(stateStats.ByValidationStatus, record.ValidationStatus)
	incrementIfPresent(stateStats.ByParseStatus, record.ParseStatus)
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
		for _, field := range record.MissingFields {
			incrementIfPresent(stats.Output.MissingFieldCounts, field)
			incrementIfPresent(stateStats.MissingFieldCounts, field)
		}
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
	ensurePerToolMaps(&toolStats)
	toolStats.Proposed++
	if record.Valid {
		stats.Tools.ValidProposals++
		toolStats.ValidProposals++
	} else {
		stats.Tools.InvalidProposals++
		toolStats.InvalidProposals++
		incrementIfPresent(stats.Tools.ByReason, record.Reason)
		incrementIfPresent(toolStats.InvalidReasons, record.Reason)
		for _, field := range record.MissingFields {
			incrementIfPresent(toolStats.MissingFieldCounts, field)
		}
	}
	stats.ByTool[record.ToolName] = toolStats
	return stats
}

func reduceToolResult(stats SessionStats, toolName string, isError bool) SessionStats {
	stats.Tools.Executed++
	toolStats := stats.ByTool[toolName]
	ensurePerToolMaps(&toolStats)
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

func reduceRetry(stats SessionStats, record RetryRecord, index map[string]SessionRecord) SessionStats {
	stats.Retry.Total++
	incrementIfPresent(stats.Retry.ByReason, record.Reason)
	incrementIfPresent(stats.RetryByReason, record.Reason)
	attribution := resolveRetryAttribution(record, index)
	incrementIfPresent(stats.Retry.Attribution.ByCauseKind, attribution.CauseKind)
	incrementIfPresent(stats.Retry.Attribution.ByTool, attribution.ToolName)
	incrementIfPresent(stats.Retry.Attribution.ByState, attribution.StateName)
	if record.Recovered {
		stats.Retry.Recovered++
	} else {
		stats.Retry.Unrecovered++
	}
	return stats
}

func reduceCompletion(stats SessionStats, record CompletionRecord, latestCognitiveState, latestWorkflowState string) SessionStats {
	stats.Completion.Total++
	incrementIfPresent(stats.StopReasons, record.StopReason)
	stats.Completion.LatestCompleted = record.Completed
	stats.Completion.LatestStopReason = record.StopReason
	stats.Completion.LatestIteration = record.Iteration
	stats.Completion.LatestCognitiveState = latestCognitiveState
	stats.Completion.LatestWorkflowState = latestWorkflowState
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

func buildRecordIndex(history *SessionHistory) map[string]SessionRecord {
	index := map[string]SessionRecord{}
	if history == nil {
		return index
	}
	for _, record := range history.Records {
		if record == nil {
			continue
		}
		index[record.RecordID()] = record
	}
	return index
}

func resolveRetryAttribution(record RetryRecord, index map[string]SessionRecord) retryAttribution {
	for _, id := range record.DerivedFrom {
		if id == "" {
			continue
		}
		attribution := attributionFromRecord(index[id])
		if attribution.CauseKind != "" {
			return attribution
		}
	}
	return retryAttribution{CauseKind: "unknown"}
}

func attributionFromRecord(record SessionRecord) retryAttribution {
	switch v := record.(type) {
	case ToolValidationRecord:
		return retryAttribution{CauseKind: "tool_validation", ToolName: v.ToolName}
	case *ToolValidationRecord:
		return retryAttribution{CauseKind: "tool_validation", ToolName: v.ToolName}
	case OutputContractEvaluationRecord:
		return retryAttribution{CauseKind: "output_contract_evaluation", ToolName: v.ToolName, StateName: v.StateName}
	case *OutputContractEvaluationRecord:
		return retryAttribution{CauseKind: "output_contract_evaluation", ToolName: v.ToolName, StateName: v.StateName}
	case ToolCallResultRecord:
		return retryAttribution{CauseKind: "tool_call_result", ToolName: v.ToolName}
	case *ToolCallResultRecord:
		return retryAttribution{CauseKind: "tool_call_result", ToolName: v.ToolName}
	default:
		return retryAttribution{}
	}
}

func ensurePerToolMaps(stats *PerToolStats) {
	if stats.InvalidReasons == nil {
		stats.InvalidReasons = map[string]int{}
	}
	if stats.MissingFieldCounts == nil {
		stats.MissingFieldCounts = map[string]int{}
	}
}

func ensurePerStateMaps(stats *PerStateStats) {
	if stats.ByValidationStatus == nil {
		stats.ByValidationStatus = map[string]int{}
	}
	if stats.ByParseStatus == nil {
		stats.ByParseStatus = map[string]int{}
	}
	if stats.MissingFieldCounts == nil {
		stats.MissingFieldCounts = map[string]int{}
	}
}

func incrementIfPresent(items map[string]int, key string) {
	if key == "" {
		return
	}
	items[key]++
}
