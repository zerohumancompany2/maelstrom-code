package logs

import (
	"encoding/json"
	"path"
	"sort"
	"strings"
)

type SessionStats struct {
	SessionID          string                   `json:"session_id"`
	AgentID            string                   `json:"agent_id"`
	RecordCounts       RecordCounts             `json:"record_counts"`
	Output             OutputStats              `json:"output"`
	Finalization       FinalizationStats        `json:"finalization"`
	Tools              ToolStats                `json:"tools"`
	Retry              RetryStats               `json:"retry"`
	Completion         CompletionStats          `json:"completion"`
	ByTool             map[string]PerToolStats  `json:"by_tool"`
	ByState            map[string]PerStateStats `json:"by_state"`
	ModelRefs          map[string]int           `json:"model_refs"`
	ProviderRefs       map[string]int           `json:"provider_refs"`
	StopReasons        map[string]int           `json:"stop_reasons"`
	StateExitReasons   map[string]int           `json:"state_exit_reasons"`
	RetryByReason      map[string]int           `json:"retry_by_reason"`
	OutputStatuses     map[string]int           `json:"output_statuses"`
	ParseStatuses      map[string]int           `json:"parse_statuses"`
	FilesRead          map[string]int           `json:"files_read"`
	FinalAssistant     string                   `json:"final_assistant,omitempty"`
	FinalWorkflowState string                   `json:"final_workflow_state,omitempty"`
}

// finalAssistantLimit caps the retained final assistant content so session
// stats stay small while remaining useful for content checks.
const finalAssistantLimit = 4000

// fileReadToolNames are tools whose successful execution counts as reading a
// file for discovery-style evaluations.
var fileReadToolNames = map[string]bool{
	"read_file":         true,
	"get_file_skeleton": true,
	"read_symbol":       true,
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
	Total                 int                          `json:"total"`
	Valid                 int                          `json:"valid"`
	Invalid               int                          `json:"invalid"`
	MissingRequired       int                          `json:"missing_required"`
	WrongState            int                          `json:"wrong_state"`
	ByChart               map[string]BucketOutputStats `json:"by_chart"`
	ByValidationStatus    map[string]int               `json:"by_validation_status"`
	ByParseStatus         map[string]int               `json:"by_parse_status"`
	BySchema              map[string]int               `json:"by_schema"`
	ByActionType          map[string]int               `json:"by_action_type"`
	ByTool                map[string]int               `json:"by_tool"`
	MissingFieldCounts    map[string]int               `json:"missing_field_counts"`
	CompletionSignalTrue  int                          `json:"completion_signal_true"`
	CompletionSignalFalse int                          `json:"completion_signal_false"`
}

type BucketOutputStats struct {
	Total              int            `json:"total"`
	Valid              int            `json:"valid"`
	Invalid            int            `json:"invalid"`
	MissingRequired    int            `json:"missing_required"`
	WrongState         int            `json:"wrong_state"`
	ByValidationStatus map[string]int `json:"by_validation_status"`
	ByParseStatus      map[string]int `json:"by_parse_status"`
	MissingFieldCounts map[string]int `json:"missing_field_counts"`
}

type FinalizationStats struct {
	Completions   int            `json:"completions"`
	Failures      int            `json:"failures"`
	ByStopReason  map[string]int `json:"by_stop_reason"`
	ByBoundReason map[string]int `json:"by_bound_reason"`
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
		ByTool:           map[string]PerToolStats{},
		ByState:          map[string]PerStateStats{},
		ModelRefs:        map[string]int{},
		ProviderRefs:     map[string]int{},
		StopReasons:      map[string]int{},
		StateExitReasons: map[string]int{},
		RetryByReason:    map[string]int{},
		OutputStatuses:   map[string]int{},
		ParseStatuses:    map[string]int{},
		FilesRead:        map[string]int{},
		Output: OutputStats{
			ByChart:            map[string]BucketOutputStats{},
			ByValidationStatus: map[string]int{},
			ByParseStatus:      map[string]int{},
			BySchema:           map[string]int{},
			ByActionType:       map[string]int{},
			ByTool:             map[string]int{},
			MissingFieldCounts: map[string]int{},
		},
		Finalization: FinalizationStats{ByStopReason: map[string]int{}, ByBoundReason: map[string]int{}},
		Tools:        ToolStats{ByReason: map[string]int{}},
		Retry:        RetryStats{ByReason: map[string]int{}, Attribution: RetryAttributionStats{ByCauseKind: map[string]int{}, ByTool: map[string]int{}, ByState: map[string]int{}}},
	}
	if history == nil {
		return stats
	}
	stats.SessionID = history.SessionID
	stats.AgentID = history.AgentID
	index := buildRecordIndex(history)
	recoveredRetries := computeRecoveredRetries(history, index)
	latestCognitiveState := ""
	latestWorkflowState := ""
	finalAssistant := ""
	pendingReads := map[string]string{}
	for _, record := range history.Records {
		stats.RecordCounts.Total++
		switch v := record.(type) {
		case UserMessageRecord:
			stats.RecordCounts.Users++
		case *UserMessageRecord:
			stats.RecordCounts.Users++
		case AssistantMessageRecord:
			stats.RecordCounts.Assistants++
			finalAssistant = v.Content
		case *AssistantMessageRecord:
			stats.RecordCounts.Assistants++
			finalAssistant = v.Content
		case ToolCallRequestRecord:
			stats.RecordCounts.ToolRequests++
			trackFileReadRequest(pendingReads, v)
		case *ToolCallRequestRecord:
			stats.RecordCounts.ToolRequests++
			trackFileReadRequest(pendingReads, *v)
		case ToolCallResultRecord:
			stats.RecordCounts.ToolResults++
			stats = reduceToolResult(stats, v.ToolName, v.IsError)
			resolveFileRead(stats.FilesRead, pendingReads, v.CallID, v.IsError)
		case *ToolCallResultRecord:
			stats.RecordCounts.ToolResults++
			stats = reduceToolResult(stats, v.ToolName, v.IsError)
			resolveFileRead(stats.FilesRead, pendingReads, v.CallID, v.IsError)
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
			stats = reduceRetry(stats, v, index, recoveredRetries[v.RecordID()])
		case *RetryRecord:
			stats.RecordCounts.Retries++
			stats = reduceRetry(stats, *v, index, recoveredRetries[v.RecordID()])
		case CompletionRecord:
			stats.RecordCounts.Completions++
			stats = reduceCompletion(stats, v, latestCognitiveState, latestWorkflowState)
		case *CompletionRecord:
			stats.RecordCounts.Completions++
			stats = reduceCompletion(stats, *v, latestCognitiveState, latestWorkflowState)
		case StateExitRecord:
			incrementIfPresent(stats.StateExitReasons, v.Reason)
			if v.Chart == "workflow" {
				stats.FinalWorkflowState = v.StateName
			}
			// Count success finalization events from exits with BoundReason.
			// Dedupe rule: workflow exits count; cognitive exits count only if
			// their BoundReason doesn't contain "workflow_" (combined events are
			// counted at the workflow exit and skipped at the cognitive exit).
			if v.BoundReason != "" {
				if v.Chart == "workflow" || (v.Chart == "cognitive" && !strings.Contains(v.BoundReason, "workflow_")) {
					stats.Finalization.Completions++
					incrementIfPresent(stats.Finalization.ByBoundReason, v.BoundReason)
				}
			}
		case *StateExitRecord:
			incrementIfPresent(stats.StateExitReasons, v.Reason)
			if v.Chart == "workflow" {
				stats.FinalWorkflowState = v.StateName
			}
			// Count success finalization events from exits with BoundReason.
			// Dedupe rule: workflow exits count; cognitive exits count only if
			// their BoundReason doesn't contain "workflow_" (combined events are
			// counted at the workflow exit and skipped at the cognitive exit).
			if v.BoundReason != "" {
				if v.Chart == "workflow" || (v.Chart == "cognitive" && !strings.Contains(v.BoundReason, "workflow_")) {
					stats.Finalization.Completions++
					incrementIfPresent(stats.Finalization.ByBoundReason, v.BoundReason)
				}
			}
		case StateEnterRecord:
			if v.Chart == "workflow" {
				stats.FinalWorkflowState = v.StateName
			}
		case *StateEnterRecord:
			if v.Chart == "workflow" {
				stats.FinalWorkflowState = v.StateName
			}
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
	if len(finalAssistant) > finalAssistantLimit {
		finalAssistant = finalAssistant[:finalAssistantLimit]
	}
	stats.FinalAssistant = finalAssistant
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
	chartStats := stats.Output.ByChart[record.Chart]
	ensureBucketOutputMaps(&chartStats)
	chartStats.Total++
	incrementIfPresent(chartStats.ByValidationStatus, record.ValidationStatus)
	incrementIfPresent(chartStats.ByParseStatus, record.ParseStatus)
	if record.ValidationStatus == "valid" {
		chartStats.Valid++
	} else {
		chartStats.Invalid++
	}
	if len(record.MissingFields) > 0 {
		chartStats.MissingRequired++
		for _, field := range record.MissingFields {
			incrementIfPresent(chartStats.MissingFieldCounts, field)
		}
	}
	if record.WrongState {
		chartStats.WrongState++
	}
	if strings.TrimSpace(record.Chart) != "" {
		stats.Output.ByChart[record.Chart] = chartStats
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

func reduceRetry(stats SessionStats, record RetryRecord, index map[string]SessionRecord, recovered bool) SessionStats {
	stats.Retry.Total++
	incrementIfPresent(stats.Retry.ByReason, record.Reason)
	incrementIfPresent(stats.RetryByReason, record.Reason)
	attribution := resolveRetryAttribution(record, index)
	incrementIfPresent(stats.Retry.Attribution.ByCauseKind, attribution.CauseKind)
	incrementIfPresent(stats.Retry.Attribution.ByTool, attribution.ToolName)
	incrementIfPresent(stats.Retry.Attribution.ByState, attribution.StateName)
	if record.Recovered || recovered {
		stats.Retry.Recovered++
	} else {
		stats.Retry.Unrecovered++
	}
	return stats
}

// computeRecoveredRetries pairs each retry with a later success signal. The
// runtime appends retries before knowing the outcome, so recovery is derived
// at reduce time: output-contract retries recover when a later evaluation
// validates, and missing-argument retries recover when the same tool later
// passes validation.
func computeRecoveredRetries(history *SessionHistory, index map[string]SessionRecord) map[string]bool {
	recovered := map[string]bool{}
	if history == nil {
		return recovered
	}
	pendingOutput := []string{}
	pendingByTool := map[string][]string{}
	markTool := func(name string) {
		for _, id := range pendingByTool[name] {
			recovered[id] = true
		}
		delete(pendingByTool, name)
	}
	handleRetry := func(record RetryRecord) {
		switch record.Reason {
		case "invalid_state_output", "missing_state_output", "invalid_finalization_output", "missing_required_fields", "invalid_json":
			pendingOutput = append(pendingOutput, record.RecordID())
		case "missing_required_arguments":
			tool := resolveRetryAttribution(record, index).ToolName
			pendingByTool[tool] = append(pendingByTool[tool], record.RecordID())
		}
	}
	handleEval := func(record OutputContractEvaluationRecord) {
		if record.ValidationStatus != "valid" {
			return
		}
		for _, id := range pendingOutput {
			recovered[id] = true
		}
		pendingOutput = pendingOutput[:0]
	}
	handleToolValidation := func(record ToolValidationRecord) {
		if !record.Valid {
			return
		}
		markTool(record.ToolName)
		markTool("")
	}
	for _, record := range history.Records {
		switch v := record.(type) {
		case RetryRecord:
			handleRetry(v)
		case *RetryRecord:
			handleRetry(*v)
		case OutputContractEvaluationRecord:
			handleEval(v)
		case *OutputContractEvaluationRecord:
			handleEval(*v)
		case ToolValidationRecord:
			handleToolValidation(v)
		case *ToolValidationRecord:
			handleToolValidation(*v)
		}
	}
	return recovered
}

// reduceCompletion handles CompletionRecords. For finalization stop reasons,
// ByStopReason is always incremented (tracking how sessions ended). Failures
// (!Completed) increment Failures and ByBoundReason. Successes are NOT counted
// here — they're counted at their StateExitRecords with BoundReason, avoiding
// double-count for session-ending successes that append both exit and completion.
func reduceCompletion(stats SessionStats, record CompletionRecord, latestCognitiveState, latestWorkflowState string) SessionStats {
	stats.Completion.Total++
	incrementIfPresent(stats.StopReasons, record.StopReason)
	if isFinalizationStopReason(record.StopReason) {
		incrementIfPresent(stats.Finalization.ByStopReason, record.StopReason)
		if !record.Completed {
			stats.Finalization.Failures++
			incrementIfPresent(stats.Finalization.ByBoundReason, record.FinalizationReason)
		}
	}
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

func isFinalizationStopReason(reason string) bool {
	switch reason {
	case "state_finalized", "workflow_state_finalized", "combined_state_finalized", "finalization_validation_failed":
		return true
	default:
		return false
	}
}

func SortedMapKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// CountInferenceTurnsSinceStateEnter counts InferenceEnvelopeRecords since the latest StateEnterRecord for the given chart.
// Returns 0 if no StateEnterRecord is found for the chart.
func CountInferenceTurnsSinceStateEnter(history *SessionHistory, chart string) int {
	if history == nil {
		return 0
	}
	// Find the latest state enter record for this chart
	enterIndex := -1
	for i := len(history.Records) - 1; i >= 0; i-- {
		enter, ok := history.Records[i].(StateEnterRecord)
		if ok && enter.Chart == chart {
			enterIndex = i
			break
		}
		enterPtr, ok := history.Records[i].(*StateEnterRecord)
		if ok && enterPtr.Chart == chart {
			enterIndex = i
			break
		}
	}
	if enterIndex == -1 {
		return 0
	}
	// Count inference envelopes after the state enter
	count := 0
	for i := enterIndex + 1; i < len(history.Records); i++ {
		switch history.Records[i].(type) {
		case InferenceEnvelopeRecord:
			count++
		case *InferenceEnvelopeRecord:
			count++
		}
	}
	return count
}

// CountFinalizationRetriesSinceStateEnter counts retry attempts for finalization failures since state enter.
func CountFinalizationRetriesSinceStateEnter(history *SessionHistory, chart string) int {
	if history == nil {
		return 0
	}
	// Find the latest state enter record for this chart
	enterIndex := -1
	for i := len(history.Records) - 1; i >= 0; i-- {
		enter, ok := history.Records[i].(StateEnterRecord)
		if ok && enter.Chart == chart {
			enterIndex = i
			break
		}
		enterPtr, ok := history.Records[i].(*StateEnterRecord)
		if ok && enterPtr.Chart == chart {
			enterIndex = i
			break
		}
	}
	if enterIndex == -1 {
		return 0
	}
	// Count retry records related to output contract evaluation failures
	count := 0
	for i := enterIndex + 1; i < len(history.Records); i++ {
		switch v := history.Records[i].(type) {
		case RetryRecord:
			if v.Reason == "invalid_finalization_output" || v.Reason == "missing_required_fields" || v.Reason == "invalid_json" {
				count++
			}
		case *RetryRecord:
			if v.Reason == "invalid_finalization_output" || v.Reason == "missing_required_fields" || v.Reason == "invalid_json" {
				count++
			}
		}
	}
	return count
}

func CountToolCallsSinceStateEnter(history *SessionHistory, chart string) int {
	if history == nil {
		return 0
	}
	enterIndex := -1
	for i := len(history.Records) - 1; i >= 0; i-- {
		enter, ok := history.Records[i].(StateEnterRecord)
		if ok && enter.Chart == chart {
			enterIndex = i
			break
		}
		enterPtr, ok := history.Records[i].(*StateEnterRecord)
		if ok && enterPtr.Chart == chart {
			enterIndex = i
			break
		}
	}
	if enterIndex == -1 {
		return 0
	}
	count := 0
	for i := enterIndex + 1; i < len(history.Records); i++ {
		switch history.Records[i].(type) {
		case ToolCallRequestRecord:
			count++
		case *ToolCallRequestRecord:
			count++
		}
	}
	return count
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

func ensureBucketOutputMaps(stats *BucketOutputStats) {
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

// trackFileReadRequest remembers the target path of a file-reading tool call
// so a later successful result can be attributed to that file.
func trackFileReadRequest(pending map[string]string, record ToolCallRequestRecord) {
	if !fileReadToolNames[record.ToolName] || record.CallID == "" {
		return
	}
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(record.Arguments), &args); err != nil {
		return
	}
	cleaned := path.Clean(strings.TrimSpace(args.Path))
	if cleaned == "" || cleaned == "." {
		return
	}
	pending[record.CallID] = cleaned
}

// resolveFileRead counts a file as read when its tool call succeeded.
func resolveFileRead(filesRead map[string]int, pending map[string]string, callID string, isError bool) {
	filePath, ok := pending[callID]
	if !ok {
		return
	}
	delete(pending, callID)
	if isError {
		return
	}
	filesRead[filePath]++
}

func incrementIfPresent(items map[string]int, key string) {
	if key == "" {
		return
	}
	items[key]++
}
