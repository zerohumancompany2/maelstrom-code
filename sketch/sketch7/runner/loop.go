package runner

import (
	"encoding/json"
	"fmt"
	"strings"

	ctxpkg "github.com/comalice/inference_sketch/sketch/sketch7/context"
	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch7/tools"
)

type Loop struct {
	Provider    provider.Provider
	Tools       tools.Executor
	Projections []prompt.Projection
	MaxHistory  int
	StopToken   string
}

func (l Loop) Run(agent runtime.Agent, agentDef defs.AgentDefinition, workflowDef *defs.WorkflowDefinition, sessionHistory *logs.SessionHistory, workflowHistory *logs.WorkflowHistory) error {
	assembler := prompt.Assembler{Projections: l.Projections}
	contextBuilder := ctxpkg.Builder{MaxMessages: l.MaxHistory}
	for iteration := 1; ; iteration++ {
		view := BuildSessionView(agent, agentDef, workflowDef, sessionHistory, workflowHistory)
		ensureStateEnterRecords(sessionHistory, view)
		payloadID := sessionHistory.NextBundleID()
		inferencePayload := contextBuilder.Build(payloadID, ctxpkg.BuildSections(agentDef, view, sessionHistory, ctxpkg.RepoContextOptions{RootDir: ".", RefreshEveryTurns: 12, MaxFilesToInspect: 2000, MaxTopLevelEntries: 8, MaxExtensionsToShow: 5}), view, sessionHistory, workflowHistory)
		inferencePayload = persistContextSnapshots(sessionHistory, inferencePayload)
		assembled, err := assembler.Assemble(prompt.Input{Payload: inferencePayload})
		if err != nil {
			return err
		}

		payload := prompt.BuildPayload(agent, inferencePayload.PayloadID, sessionHistory.SessionID, assembled)
		request, err := l.Provider.BuildRequest(agent, payload, toolDefinitions(l.Tools))
		if err != nil {
			return err
		}
		sessionHistory.Append(logs.InferenceEnvelopeRecord{SessionBaseRecord: sessionHistory.NextRecord("inference_envelope"), PayloadID: inferencePayload.PayloadID, ModelRef: inferencePayload.ModelRef, ProviderRef: agent.ProviderName, IncludedContextRecordIDs: contextRecordIDs(inferencePayload.Sections), IncludedTranscriptKinds: messageKinds(inferencePayload.Messages), IncludedToolNames: inferencePayload.Tools})
		response, err := l.Provider.Send(request)
		if err != nil {
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: false, StopReason: "provider_error", Iteration: iteration})
			return err
		}

		hasToolCalls := false
		for _, output := range response.Outputs {
			records, workflowRecords, err := l.consumeProviderOutput(view, sessionHistory, workflowHistory, output)
			if err != nil {
				sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: false, StopReason: "tool_execution_error", Iteration: iteration})
				return err
			}
			for _, record := range records {
				sessionHistory.Append(record)
				if _, ok := record.(logs.ToolCallRequestRecord); ok {
					hasToolCalls = true
				}
			}
			for _, record := range workflowRecords {
				workflowHistory.Append(record)
			}
		}

		if l.shouldStop(sessionHistory) {
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: true, StopReason: "stop_token", Iteration: iteration})
			return nil
		}
		if !hasToolCalls {
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: true, StopReason: "assistant_only", Iteration: iteration})
			return nil
		}
		if iteration == 8 {
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: false, StopReason: "loop_guard", Iteration: iteration})
			return fmt.Errorf("loop guard tripped")
		}
	}
}

func ensureStateEnterRecords(history *logs.SessionHistory, view runtime.SessionView) {
	if history == nil {
		return
	}
	ensureStateEnterRecord(history, "cognitive", view.Cognitive.CurrentState)
	if view.Workflow != nil {
		ensureStateEnterRecord(history, "workflow", view.Workflow.CurrentState)
	}
}

func ensureStateEnterRecord(history *logs.SessionHistory, chart, stateName string) {
	if strings.TrimSpace(chart) == "" || strings.TrimSpace(stateName) == "" {
		return
	}
	for i := len(history.Records) - 1; i >= 0; i-- {
		record := history.Records[i]
		enter, ok := record.(logs.StateEnterRecord)
		if ok && enter.Chart == chart {
			if enter.StateName == stateName {
				return
			}
			break
		}
		enterPtr, ok := record.(*logs.StateEnterRecord)
		if ok && enterPtr.Chart == chart {
			if enterPtr.StateName == stateName {
				return
			}
			break
		}
		exit, ok := record.(logs.StateExitRecord)
		if ok && exit.Chart == chart {
			break
		}
		exitPtr, ok := record.(*logs.StateExitRecord)
		if ok && exitPtr.Chart == chart {
			break
		}
	}
	history.Append(logs.StateEnterRecord{SessionBaseRecord: history.NextRecord("state_enter"), Chart: chart, StateName: stateName})
}

func (l Loop) shouldStop(history *logs.SessionHistory) bool {
	if history == nil || strings.TrimSpace(l.StopToken) == "" || len(history.Records) == 0 {
		return false
	}
	last, ok := history.Records[len(history.Records)-1].(logs.AssistantMessageRecord)
	if !ok {
		return false
	}
	return strings.Contains(last.Content, l.StopToken)
}

func persistContextSnapshots(history *logs.SessionHistory, payload ctxpkg.Payload) ctxpkg.Payload {
	turn := ctxpkg.InteractionTurnCount(history)
	for i, section := range payload.Sections {
		if section.LogicalKey == "" || section.SourceKind == "static" {
			continue
		}
		latest := latestContextSnapshotRecord(history, section.LogicalKey)
		if latest != nil && latest.ContentHash == ctxpkg.HashContent(section.Content) {
			payload.Sections[i].RecordID = latest.RecordID()
			continue
		}
		record := logs.ContextSnapshotRecord{SessionBaseRecord: history.NextRecord("context_snapshot"), PayloadID: payload.PayloadID, LogicalKey: section.LogicalKey, SectionName: section.Name, SectionType: section.SectionType, SourceKind: section.SourceKind, Content: section.Content, ContentHash: ctxpkg.HashContent(section.Content), GeneratedAtTurn: turn, RefreshEveryNTurns: section.RefreshEveryTurns, RetentionMode: section.RetentionMode}
		if latest != nil {
			record.SupersedesRecordID = latest.RecordID()
		}
		history.Append(record)
		payload.Sections[i].RecordID = record.RecordID()
	}
	return payload
}

func latestContextSnapshotRecord(history *logs.SessionHistory, logicalKey string) *logs.ContextSnapshotRecord {
	for i := len(history.Records) - 1; i >= 0; i-- {
		rec, ok := history.Records[i].(logs.ContextSnapshotRecord)
		if ok && rec.LogicalKey == logicalKey {
			copy := rec
			return &copy
		}
	}
	return nil
}

func contextRecordIDs(sections []ctxpkg.Section) []string {
	ids := []string{}
	for _, section := range sections {
		if section.RecordID != "" {
			ids = append(ids, section.RecordID)
		}
	}
	return ids
}

func messageKinds(messages []ctxpkg.Message) []string {
	kinds := make([]string, 0, len(messages))
	for _, message := range messages {
		kinds = append(kinds, message.Kind)
	}
	return kinds
}

func toolDefinitions(executor tools.Executor) []provider.ToolDefinition {
	registry, ok := executor.(tools.Registry)
	if !ok {
		return nil
	}
	defs := registry.Definitions()
	converted := make([]provider.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		converted = append(converted, provider.ToolDefinition{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
			Required:    append([]string(nil), def.Required...),
		})
	}
	return converted
}

func (l Loop) consumeProviderOutput(view runtime.SessionView, history *logs.SessionHistory, workflowHistory *logs.WorkflowHistory, output provider.Output) ([]logs.SessionRecord, []logs.WorkflowRecord, error) {
	switch v := output.(type) {
	case provider.AssistantOutput:
		record := evaluateAssistantOutput(history, view.Cognitive, v)
		records := []logs.SessionRecord{}
		if record != nil {
			records = append(records, *record)
		}
		records = append(records, logs.AssistantMessageRecord{SessionBaseRecord: history.NextRecord("assistant"), Content: v.Content, Reasoning: v.Reasoning})
		return records, nil, nil
	case provider.ToolRequestOutput:
		validation := validateToolRequest(history, view, v.Call)
		requestRecord := logs.ToolCallRequestRecord{SessionBaseRecord: history.NextRecord("tool_call_request"), CallID: v.Call.CallID, ToolName: v.Call.ToolName, Arguments: string(v.Call.RawArgs)}
		records := []logs.SessionRecord{validation, requestRecord}
		if !validation.Valid && validation.Reason == "missing_required_arguments" {
			return append(records, logs.RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: validation.Reason, Attempt: 1, Recovered: false, DerivedFrom: []string{validation.RecordID()}}), nil, nil
		}
		result, err := l.Tools.Execute(tools.ExecutionRequest{Agent: view.Agent, Session: view, Call: v, History: history, Workflow: workflowHistory})
		if err != nil {
			return nil, nil, err
		}
		records = append(records, result.SessionRecords...)
		records = append(records, logs.ToolCallResultRecord{SessionBaseRecord: history.NextRecord("tool_call_result"), CallID: v.Call.CallID, ToolName: result.ToolName, Content: result.DisplayContent, IsError: result.IsError})
		return records, result.WorkflowRecords, nil
	default:
		return nil, nil, fmt.Errorf("unknown provider output %T", output)
	}
}

func evaluateAssistantOutput(history *logs.SessionHistory, view runtime.CognitiveView, output provider.AssistantOutput) *logs.OutputContractEvaluationRecord {
	if strings.TrimSpace(view.Outputs.SchemaName) == "" && len(view.Outputs.RequiredFields) == 0 {
		return nil
	}
	record := logs.OutputContractEvaluationRecord{
		SessionBaseRecord: history.NextRecord("output_contract_evaluation"),
		StateName:         view.CurrentState,
		SchemaName:        view.Outputs.SchemaName,
		ParseStatus:       "plain_text",
		ValidationStatus:  "missing_schema_output",
		RequiredFields:    append([]string(nil), view.Outputs.RequiredFields...),
		MissingFields:     append([]string(nil), view.Outputs.RequiredFields...),
		RawContentPreview: truncatePreview(output.Content),
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output.Content), &payload); err != nil {
		return &record
	}
	record.ParseStatus = "valid_json"
	record.ValidationStatus = "valid"
	if state, ok := payload["state"].(string); ok {
		if state != "" && state != view.CurrentState {
			record.WrongState = true
			record.ValidationStatus = "wrong_state"
		}
	}
	if actionType, ok := payload["action_type"].(string); ok {
		record.ActionType = actionType
	}
	if completion, ok := payload["completion_signal"].(bool); ok {
		record.CompletionSignal = completion
	}
	missing := missingFields(payload, view.Outputs.RequiredFields)
	record.MissingFields = missing
	if len(missing) > 0 {
		record.ValidationStatus = "missing_required_fields"
	}
	if toolPayload, ok := payload["tool"].(map[string]any); ok {
		if name, ok := toolPayload["name"].(string); ok {
			record.ToolName = name
		}
	}
	if record.ValidationStatus == "valid" && view.Outputs.Strict {
		for key := range payload {
			if !allowedOutputField(key) {
				record.ValidationStatus = "unknown_fields"
				break
			}
		}
	}
	return &record
}

func validateToolRequest(history *logs.SessionHistory, view runtime.SessionView, call provider.ToolCall) logs.ToolValidationRecord {
	required := requiredFieldsForTool(view.Agent.ToolNames, call.ToolName)
	missing := missingStringMapFields(call.Arguments, required)
	allowedTools := effectiveAllowedTools(view)
	knownTool := len(view.Agent.ToolNames) == 0 || contains(view.Agent.ToolNames, call.ToolName)
	valid := knownTool && len(missing) == 0
	reason := ""
	if !knownTool {
		reason = "unknown_tool"
	} else if len(missing) > 0 {
		reason = "missing_required_arguments"
	}
	return logs.ToolValidationRecord{
		SessionBaseRecord: history.NextRecord("tool_validation"),
		CallID:            call.CallID,
		ToolName:          call.ToolName,
		Valid:             valid,
		Reason:            reason,
		EnabledTools:      append([]string(nil), allowedTools...),
		RequiredFields:    append([]string(nil), required...),
		MissingFields:     missing,
	}
}

func effectiveAllowedTools(view runtime.SessionView) []string {
	allowed := append([]string(nil), view.Agent.ToolNames...)
	if len(view.Cognitive.EnabledTools) > 0 {
		allowed = intersectPreservingOrder(allowed, view.Cognitive.EnabledTools)
	}
	if view.Workflow != nil && len(view.Workflow.EnabledTools) > 0 {
		allowed = intersectPreservingOrder(allowed, view.Workflow.EnabledTools)
	}
	if len(allowed) == 0 {
		return append([]string(nil), view.Agent.ToolNames...)
	}
	return allowed
}

func intersectPreservingOrder(base []string, filter []string) []string {
	result := make([]string, 0)
	for _, item := range base {
		if contains(filter, item) {
			result = append(result, item)
		}
	}
	return result
}

func missingFields(payload map[string]any, required []string) []string {
	missing := make([]string, 0)
	for _, field := range required {
		if _, ok := payload[field]; !ok {
			missing = append(missing, field)
		}
	}
	return missing
}

func missingStringMapFields(payload map[string]string, required []string) []string {
	missing := make([]string, 0)
	for _, field := range required {
		if strings.TrimSpace(payload[field]) == "" {
			missing = append(missing, field)
		}
	}
	return missing
}

func requiredFieldsForTool(_ []string, toolName string) []string {
	switch toolName {
	case "read_file":
		return []string{"path"}
	case "replace_text":
		return []string{"path", "old_text", "new_text"}
	case "run_command":
		return []string{"command"}
	case "search_files":
		return []string{"pattern"}
	case "get_file_skeleton":
		return []string{"path"}
	case "find_references":
		return []string{"symbol"}
	case "read_symbol":
		return []string{"path", "symbol"}
	case "transition_state":
		return []string{"chart", "trigger"}
	case "bind_workflow":
		return []string{"workflow_id", "binding_id"}
	case "interrupt_session":
		return []string{"reason", "requested_by"}
	case "resume_session":
		return []string{"reason"}
	default:
		return nil
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func allowedOutputField(field string) bool {
	switch field {
	case "state", "action_type", "summary", "completion_signal", "tool", "final_response":
		return true
	default:
		return false
	}
}

func truncatePreview(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 200 {
		return content
	}
	return content[:200]
}
