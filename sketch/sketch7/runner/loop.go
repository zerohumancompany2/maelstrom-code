package runner

import (
	"fmt"
	"strings"
	"time"

	ctxpkg "github.com/comalice/inference_sketch/sketch/sketch7/context"
	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch7/statecharts"
	"github.com/comalice/inference_sketch/sketch/sketch7/tools"
)

type Loop struct {
	Provider    provider.Provider
	Tools       tools.Executor
	Projections []prompt.Projection
	MaxHistory  int
	StopToken   string
	// Deadline is an optional wall-clock watchdog for the whole run. The
	// check is cooperative (once per iteration), so overshoot is bounded by
	// one provider call plus tool execution. Zero means no deadline.
	Deadline time.Time
	// MaxIterations is a backstop against runaway loops. State bounds and
	// the deadline are the real governors; this only catches sessions that
	// evade both. Zero means the default of 32.
	MaxIterations int
}

const defaultMaxIterations = 32

func (l Loop) maxIterations() int {
	if l.MaxIterations > 0 {
		return l.MaxIterations
	}
	return defaultMaxIterations
}

const hostVersion = "sketch7-dev"
const outputParserVersion = "output-contract-v1"

func (l Loop) Run(agent runtime.Agent, agentDef defs.AgentDefinition, workflowDef *defs.WorkflowDefinition, sessionHistory *logs.SessionHistory, workflowHistory *logs.WorkflowHistory) error {
	assembler := prompt.Assembler{Projections: l.Projections}
	contextBuilder := ctxpkg.Builder{MaxMessages: l.MaxHistory}
	for iteration := 1; ; iteration++ {
		view := BuildSessionView(agent, agentDef, workflowDef, sessionHistory, workflowHistory)
		ensureStateEnterRecords(sessionHistory, view)
		finalizationMode := runtime.ResolveFinalizationMode(view.Cognitive, view.Workflow, sessionHistory)
		if hit, chart, reason := boundStopReason(view, sessionHistory); hit {
			sessionHistory.Append(logs.StateExitRecord{SessionBaseRecord: sessionHistory.NextRecord("state_exit"), Chart: chart, StateName: boundStateName(view, chart), Reason: reason})
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: false, StopReason: reason, Iteration: iteration})
			return fmt.Errorf("%s state bound exceeded: %s", chart, reason)
		}
		if !l.Deadline.IsZero() && time.Now().After(l.Deadline) {
			sessionHistory.Append(logs.StateExitRecord{SessionBaseRecord: sessionHistory.NextRecord("state_exit"), Chart: "cognitive", StateName: view.Cognitive.CurrentState, Reason: "deadline_exceeded"})
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: false, StopReason: "deadline_exceeded", Iteration: iteration})
			return fmt.Errorf("run deadline exceeded")
		}
		finalizing := finalizationMode.IsFinalizing
		payloadID := sessionHistory.NextBundleID()
		inferencePayload := contextBuilder.Build(payloadID, ctxpkg.BuildSections(agentDef, view, sessionHistory, ctxpkg.RepoContextOptions{RootDir: ".", RefreshEveryTurns: 12, MaxFilesToInspect: 2000, MaxTopLevelEntries: 8, MaxExtensionsToShow: 5}), view, sessionHistory, workflowHistory)
		if finalizing {
			inferencePayload.Tools = nil
		}
		inferencePayload = persistContextSnapshots(sessionHistory, inferencePayload)
		assembled, err := assembler.Assemble(prompt.Input{Payload: inferencePayload})
		if err != nil {
			return err
		}

		payload := prompt.BuildPayload(agent, inferencePayload.PayloadID, sessionHistory.SessionID, assembled)
		effectivePolicy := effectiveToolPolicy(view)
		toolsForRequest := toolDefinitions(l.Tools, effectivePolicy.Tools)
		if finalizing {
			toolsForRequest = nil
		}
		request, err := l.Provider.BuildRequest(agent, payload, toolsForRequest)
		if err != nil {
			return err
		}
		if finalizing {
			// Finalization validation requires the wrapped bucket shape, so
			// the enforced schema must request the same shape for every
			// required bucket.
			request.ResponseFormat = finalizationResponseFormat(finalizationMode, view)
		} else {
			request.ResponseFormat = responseFormatForState(view.Cognitive)
		}
		startedAt := time.Now().UnixMilli()
		sessionHistory.Append(logs.InferenceEnvelopeRecord{SessionBaseRecord: sessionHistory.NextRecord("inference_envelope"), PayloadID: inferencePayload.PayloadID, ModelRef: inferencePayload.ModelRef, ProviderRef: agent.ProviderName, StartedAtUnixMilli: startedAt, IncludedContextRecordIDs: contextRecordIDs(inferencePayload.Sections), IncludedTranscriptKinds: messageKinds(inferencePayload.Messages), IncludedToolNames: toolNamesForDefinitions(toolsForRequest)})
		response, err := l.Provider.Send(request)
		if err != nil {
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: false, StopReason: "provider_error", Iteration: iteration})
			return err
		}

		hasToolCalls := false
		var cognitiveEval, workflowEval *logs.OutputContractEvaluationRecord
		for _, output := range response.Outputs {
			records, workflowRecords, err := l.consumeProviderOutput(view, finalizationMode, sessionHistory, workflowHistory, output, finalizing)
			if err != nil {
				sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: false, StopReason: "tool_execution_error", Iteration: iteration})
				return err
			}
			for _, record := range records {
				sessionHistory.Append(record)
				if _, ok := record.(logs.ToolCallRequestRecord); ok {
					hasToolCalls = true
				}
				if eval, ok := record.(logs.OutputContractEvaluationRecord); ok {
					copy := eval
					if eval.Chart == "workflow" {
						workflowEval = &copy
					} else {
						cognitiveEval = &copy
					}
				}
			}
			for _, record := range workflowRecords {
				workflowHistory.Append(record)
			}
		}

		if finalizing {
			cognitiveOK := !finalizationMode.RequireCognitive || evalValid(cognitiveEval)
			workflowOK := !finalizationMode.RequireWorkflow || evalValid(workflowEval)
			if cognitiveOK && workflowOK {
				workflowTransitioned := false
				if finalizationMode.RequireWorkflow {
					workflowTransitioned = appendWorkflowFinalization(sessionHistory, workflowHistory, workflowDef, view, workflowEval, finalizationMode.Reason)
				}
				if finalizationMode.RequireCognitive {
					if transitioned := appendRuntimeCognitiveTransition(sessionHistory, agentDef.Cognitive, view.Cognitive, cognitiveEval, finalizationMode.Reason); transitioned {
						// The cognitive chart moved on; the session continues in
						// the next state. Any workflow finalization above is
						// already durably recorded.
						continue
					}
					sessionHistory.Append(logs.StateExitRecord{SessionBaseRecord: sessionHistory.NextRecord("state_exit"), Chart: "cognitive", StateName: view.Cognitive.CurrentState, Reason: "completed", ParseRecordIDs: []string{cognitiveEval.RecordID()}, CompletionAccepted: true, BoundReason: finalizationMode.Reason})
				} else if workflowTransitioned {
					// Workflow-only finalization advanced the workflow chart;
					// the session continues in the new workflow state with a
					// fresh budget.
					continue
				}
				sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: true, StopReason: finalizationStopReason(finalizationMode), Iteration: iteration, FinalizationReason: finalizationMode.Reason})
				return nil
			}
			retryBudget := runtime.MaxFinalizationRetriesForMode(finalizationMode, view.Cognitive, view.Workflow)
			if runtime.FinalizationRetryCountForMode(finalizationMode, sessionHistory) < retryBudget {
				derived := []string{}
				if finalizationMode.RequireCognitive && !evalValid(cognitiveEval) && cognitiveEval != nil {
					derived = append(derived, cognitiveEval.RecordID())
				}
				if finalizationMode.RequireWorkflow && !evalValid(workflowEval) && workflowEval != nil {
					derived = append(derived, workflowEval.RecordID())
				}
				sessionHistory.Append(logs.RetryRecord{SessionBaseRecord: sessionHistory.NextRecord("retry"), Reason: "invalid_finalization_output", Attempt: runtime.FinalizationRetryCountForMode(finalizationMode, sessionHistory) + 1, Recovered: false, DerivedFrom: derived})
				continue
			}
			if finalizationMode.RequireCognitive && !evalValid(cognitiveEval) {
				sessionHistory.Append(logs.StateExitRecord{SessionBaseRecord: sessionHistory.NextRecord("state_exit"), Chart: "cognitive", StateName: view.Cognitive.CurrentState, Reason: "validation_failed", ParseRecordIDs: evalRecordIDs(cognitiveEval)})
			}
			if finalizationMode.RequireWorkflow && !evalValid(workflowEval) {
				sessionHistory.Append(logs.StateExitRecord{SessionBaseRecord: sessionHistory.NextRecord("state_exit"), Chart: "workflow", StateName: boundStateName(view, "workflow"), Reason: "validation_failed", ParseRecordIDs: evalRecordIDs(workflowEval)})
			}
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: false, StopReason: "finalization_validation_failed", Iteration: iteration, FinalizationReason: finalizationMode.Reason})
			return fmt.Errorf("finalization failed validation")
		}

		if l.shouldStop(sessionHistory) {
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: true, StopReason: "stop_token", Iteration: iteration})
			return nil
		}
		if transitioned := appendRuntimeCognitiveTransition(sessionHistory, agentDef.Cognitive, view.Cognitive, cognitiveEval, ""); transitioned {
			continue
		}
		if hit, chart, reason := boundStopReason(view, sessionHistory); hit {
			sessionHistory.Append(logs.StateExitRecord{SessionBaseRecord: sessionHistory.NextRecord("state_exit"), Chart: chart, StateName: boundStateName(view, chart), Reason: reason})
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: false, StopReason: reason, Iteration: iteration})
			return fmt.Errorf("%s state bound exceeded: %s", chart, reason)
		}
		if cognitiveEval != nil && cognitiveEval.ValidationStatus == "valid" && cognitiveEval.CompletionSignal {
			sessionHistory.Append(logs.StateExitRecord{SessionBaseRecord: sessionHistory.NextRecord("state_exit"), Chart: "cognitive", StateName: view.Cognitive.CurrentState, Reason: "completed", ParseRecordIDs: []string{cognitiveEval.RecordID()}, CompletionAccepted: true})
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: true, StopReason: "state_completed", Iteration: iteration})
			return nil
		}
		if shouldRetryInvalidStateOutput(view.Cognitive, sessionHistory, cognitiveEval) {
			sessionHistory.Append(logs.RetryRecord{SessionBaseRecord: sessionHistory.NextRecord("retry"), Reason: "invalid_state_output", Attempt: logs.CountInferenceTurnsSinceStateEnter(sessionHistory, "cognitive"), Recovered: false, DerivedFrom: []string{cognitiveEval.RecordID()}})
			continue
		}
		if shouldRetryMissingStateOutput(view.Cognitive, cognitiveEval, hasToolCalls) {
			sessionHistory.Append(logs.RetryRecord{SessionBaseRecord: sessionHistory.NextRecord("retry"), Reason: "missing_state_output", Attempt: logs.CountInferenceTurnsSinceStateEnter(sessionHistory, "cognitive"), Recovered: false})
			continue
		}
		if !hasToolCalls {
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: true, StopReason: "assistant_only", Iteration: iteration})
			return nil
		}
		if iteration >= l.maxIterations() {
			sessionHistory.Append(logs.CompletionRecord{SessionBaseRecord: sessionHistory.NextRecord("completion"), Completed: false, StopReason: "loop_guard", Iteration: iteration})
			return fmt.Errorf("loop guard tripped")
		}
	}
}

func responseFormatForState(view runtime.CognitiveView) *provider.StructuredOutputFormat {
	outputs := view.Outputs
	if strings.TrimSpace(outputs.SchemaName) == "" || len(outputs.RequiredFields) == 0 {
		return nil
	}
	fieldEnums := map[string][]string{}
	fieldTypes := map[string]string{}
	if len(view.AllowedTriggers) > 0 {
		fieldEnums["transition"] = append([]string(nil), view.AllowedTriggers...)
		fieldEnums["next_step_signal"] = append([]string(nil), view.AllowedTriggers...)
	}
	fieldTypes["completion_signal"] = "boolean"
	return &provider.StructuredOutputFormat{
		Name:           outputs.SchemaName,
		RequiredFields: append([]string(nil), outputs.RequiredFields...),
		OptionalFields: append([]string(nil), outputs.OptionalFields...),
		FieldTypes:     fieldTypes,
		FieldEnums:     fieldEnums,
		Strict:         outputs.Strict,
	}
}

func shouldRetryMissingStateOutput(view runtime.CognitiveView, eval *logs.OutputContractEvaluationRecord, hasToolCalls bool) bool {
	if eval != nil || hasToolCalls || view.Bounds.MaxInferenceTurns <= 0 {
		return false
	}
	return strings.TrimSpace(view.Outputs.SchemaName) != "" || len(view.Outputs.RequiredFields) > 0
}

func shouldRetryInvalidStateOutput(view runtime.CognitiveView, history *logs.SessionHistory, eval *logs.OutputContractEvaluationRecord) bool {
	if eval == nil || eval.ValidationStatus == "valid" || view.Bounds.MaxInferenceTurns <= 0 {
		return false
	}
	if strings.TrimSpace(view.Outputs.SchemaName) == "" && len(view.Outputs.RequiredFields) == 0 {
		return false
	}
	return true
}

func appendRuntimeCognitiveTransition(history *logs.SessionHistory, chart defs.StatechartDefinition, view runtime.CognitiveView, eval *logs.OutputContractEvaluationRecord, boundReason string) bool {
	if history == nil || eval == nil || eval.ValidationStatus != "valid" || strings.TrimSpace(eval.TransitionTrigger) == "" {
		return false
	}
	trigger := strings.TrimSpace(eval.TransitionTrigger)
	if len(view.AllowedTriggers) > 0 && !contains(view.AllowedTriggers, trigger) {
		return false
	}
	machine := statecharts.Compile("agent", chart)
	next, err := machine.Next(view.CurrentState, trigger)
	if err != nil {
		return false
	}
	history.Append(logs.StateExitRecord{SessionBaseRecord: history.NextRecord("state_exit"), Chart: "cognitive", StateName: view.CurrentState, Reason: "transition", DerivedFromIDs: []string{eval.RecordID()}, ParseRecordIDs: []string{eval.RecordID()}, CompletionAccepted: true, BoundReason: boundReason})
	history.Append(logs.CognitiveTransitionRecord{SessionBaseRecord: history.NextRecord("cognitive_transition"), FromState: view.CurrentState, ToState: next, Trigger: trigger, DerivedFromIDs: []string{eval.RecordID()}})
	history.Append(logs.StateEnterRecord{SessionBaseRecord: history.NextRecord("state_enter"), Chart: "cognitive", StateName: next, DerivedFromIDs: []string{eval.RecordID()}})
	return true
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

func toolDefinitions(executor tools.Executor, allowedNames []string) []provider.ToolDefinition {
	registry, ok := executor.(tools.Registry)
	if !ok || len(allowedNames) == 0 {
		return nil
	}
	allowed := map[string]bool{}
	for _, name := range allowedNames {
		allowed[name] = true
	}
	defs := registry.Definitions()
	converted := make([]provider.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		if len(allowed) > 0 && !allowed[def.Name] {
			continue
		}
		converted = append(converted, provider.ToolDefinition{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
			Required:    append([]string(nil), def.Required...),
		})
	}
	return converted
}

func (l Loop) consumeProviderOutput(view runtime.SessionView, finalizationMode runtime.FinalizationMode, history *logs.SessionHistory, workflowHistory *logs.WorkflowHistory, output provider.Output, finalizing bool) ([]logs.SessionRecord, []logs.WorkflowRecord, error) {
	switch v := output.(type) {
	case provider.AssistantOutput:
		assistant := logs.AssistantMessageRecord{SessionBaseRecord: history.NextRecord("assistant"), Content: v.Content, Reasoning: v.Reasoning}
		records := []logs.SessionRecord{assistant}
		for _, evaluation := range evaluateAssistantOutput(history, view, finalizationMode, v, assistant.RecordID()) {
			records = append(records, evaluation)
		}
		return records, nil, nil
	case provider.ToolRequestOutput:
		validation := validateToolRequest(history, view, v.Call)
		requestRecord := logs.ToolCallRequestRecord{SessionBaseRecord: history.NextRecord("tool_call_request"), CallID: v.Call.CallID, ToolName: v.Call.ToolName, Arguments: string(v.Call.RawArgs)}
		records := []logs.SessionRecord{validation, requestRecord}
		// Do not execute if finalizing (no-tools mode) or if tool is not enabled
		if finalizing {
			return records, nil, nil
		}
		if !validation.Valid {
			if validation.Reason == "missing_required_arguments" {
				return append(records, logs.RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: validation.Reason, Attempt: 1, Recovered: false, DerivedFrom: []string{validation.RecordID()}}), nil, nil
			}
			return records, nil, nil
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

func validateToolRequest(history *logs.SessionHistory, view runtime.SessionView, call provider.ToolCall) logs.ToolValidationRecord {
	required := requiredFieldsForTool(view.Agent.ToolNames, call.ToolName)
	missing := missingStringMapFields(call.Arguments, required)
	policy := effectiveToolPolicy(view)
	allowedTools := policy.Tools
	knownTool := len(view.Agent.ToolNames) == 0 || contains(view.Agent.ToolNames, call.ToolName)
	enabledTool := contains(allowedTools, call.ToolName)
	if len(allowedTools) == 0 && !policy.Applied && len(view.Agent.ToolNames) == 0 {
		enabledTool = true
	}
	valid := knownTool && enabledTool && len(missing) == 0
	reason := ""
	if !knownTool {
		reason = "unknown_tool"
	} else if !enabledTool {
		reason = "tool_not_enabled"
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

func effectiveToolPolicy(view runtime.SessionView) runtime.EffectiveToolPolicy {
	allowed := append([]string(nil), view.Agent.ToolNames...)
	policyApplied := false
	if len(view.Cognitive.EnabledTools) > 0 || len(view.Cognitive.VisibleTools) > 0 {
		policyApplied = true
		if len(allowed) == 0 {
			allowed = append([]string(nil), view.Cognitive.EnabledTools...)
		} else {
			allowed = intersectPreservingOrder(allowed, view.Cognitive.EnabledTools)
		}
	}
	if view.Workflow != nil && (len(view.Workflow.EnabledTools) > 0 || len(view.Workflow.VisibleTools) > 0) {
		policyApplied = true
		if len(allowed) == 0 {
			allowed = append([]string(nil), view.Workflow.EnabledTools...)
		} else {
			allowed = intersectPreservingOrder(allowed, view.Workflow.EnabledTools)
		}
	}
	if len(allowed) == 0 && !policyApplied {
		return runtime.EffectiveToolPolicy{Applied: false, Tools: append([]string(nil), view.Agent.ToolNames...)}
	}
	return runtime.EffectiveToolPolicy{Applied: policyApplied, Tools: allowed}
}

func toolNamesForDefinitions(defs []provider.ToolDefinition) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}

// boundStopReason reports a hard-stop bound violation for either chart.
// Charts whose bound exhaustion can be absorbed by finalization (an output
// contract is declared) are skipped here; finalization handles them instead.
func boundStopReason(view runtime.SessionView, history *logs.SessionHistory) (bool, string, string) {
	if history == nil {
		return false, "", ""
	}
	cognitive := view.Cognitive
	if cognitive.Bounds.MaxToolCalls > 0 && logs.CountToolCallsSinceStateEnter(history, "cognitive") >= cognitive.Bounds.MaxToolCalls {
		if !runtime.ShouldFinalizeCognitive(cognitive, history) {
			return true, "cognitive", "max_tool_calls"
		}
	}
	if cognitive.Bounds.MaxWallTimeSeconds > 0 {
		start := latestStateStartMillis(history, "cognitive")
		if start > 0 && time.Now().UnixMilli()-start >= int64(cognitive.Bounds.MaxWallTimeSeconds)*1000 {
			if !runtime.ShouldFinalizeCognitive(cognitive, history) {
				return true, "cognitive", "max_wall_time"
			}
		}
	}
	if view.Workflow == nil {
		return false, "", ""
	}
	workflow := *view.Workflow
	if workflow.Bounds.MaxToolCalls > 0 && logs.CountToolCallsSinceStateEnter(history, "workflow") >= workflow.Bounds.MaxToolCalls {
		if !runtime.ShouldFinalizeWorkflow(workflow, history) {
			return true, "workflow", "workflow_max_tool_calls"
		}
	}
	if workflow.Bounds.MaxWallTimeSeconds > 0 {
		start := latestStateStartMillis(history, "workflow")
		if start > 0 && time.Now().UnixMilli()-start >= int64(workflow.Bounds.MaxWallTimeSeconds)*1000 {
			if !runtime.ShouldFinalizeWorkflow(workflow, history) {
				return true, "workflow", "workflow_max_wall_time"
			}
		}
	}
	return false, "", ""
}

func boundStateName(view runtime.SessionView, chart string) string {
	if chart == "workflow" && view.Workflow != nil {
		return view.Workflow.CurrentState
	}
	return view.Cognitive.CurrentState
}

func evalValid(eval *logs.OutputContractEvaluationRecord) bool {
	return eval != nil && eval.ValidationStatus == "valid"
}

func evalRecordIDs(eval *logs.OutputContractEvaluationRecord) []string {
	if eval == nil {
		return []string{}
	}
	return []string{eval.RecordID()}
}

func finalizationStopReason(mode runtime.FinalizationMode) string {
	switch {
	case mode.RequireCognitive && mode.RequireWorkflow:
		return "combined_state_finalized"
	case mode.RequireWorkflow:
		return "workflow_state_finalized"
	default:
		return "state_finalized"
	}
}

// finalizationResponseFormat builds the wrapper schema covering every bucket
// required by the finalization mode.
func finalizationResponseFormat(mode runtime.FinalizationMode, view runtime.SessionView) *provider.StructuredOutputFormat {
	buckets := make([]provider.BucketFormat, 0, 2)
	names := make([]string, 0, 2)
	strict := false
	if mode.RequireCognitive {
		outputs := view.Cognitive.Outputs
		fieldEnums := map[string][]string{}
		if len(view.Cognitive.AllowedTriggers) > 0 {
			fieldEnums["transition"] = append([]string(nil), view.Cognitive.AllowedTriggers...)
			fieldEnums["next_step_signal"] = append([]string(nil), view.Cognitive.AllowedTriggers...)
		}
		buckets = append(buckets, provider.BucketFormat{
			Name:           "cognitive",
			RequiredFields: append([]string(nil), outputs.RequiredFields...),
			OptionalFields: append([]string(nil), outputs.OptionalFields...),
			FieldTypes:     map[string]string{"completion_signal": "boolean"},
			FieldEnums:     fieldEnums,
			Strict:         outputs.Strict,
		})
		if name := strings.TrimSpace(outputs.SchemaName); name != "" {
			names = append(names, name)
		}
		strict = strict || outputs.Strict
	}
	if mode.RequireWorkflow && view.Workflow != nil {
		outputs := view.Workflow.Outputs
		fieldEnums := map[string][]string{}
		if len(view.Workflow.AllowedTriggers) > 0 {
			fieldEnums["transition"] = append([]string(nil), view.Workflow.AllowedTriggers...)
			fieldEnums["next_step_signal"] = append([]string(nil), view.Workflow.AllowedTriggers...)
		}
		buckets = append(buckets, provider.BucketFormat{
			Name:           "workflow",
			RequiredFields: append([]string(nil), outputs.RequiredFields...),
			OptionalFields: append([]string(nil), outputs.OptionalFields...),
			FieldTypes:     map[string]string{"completion_signal": "boolean"},
			FieldEnums:     fieldEnums,
			Strict:         outputs.Strict,
		})
		if name := strings.TrimSpace(outputs.SchemaName); name != "" {
			names = append(names, name)
		}
		strict = strict || outputs.Strict
	}
	if len(buckets) == 0 {
		return nil
	}
	name := strings.Join(names, "__")
	if name == "" {
		name = "finalization"
	}
	return &provider.StructuredOutputFormat{Name: name, Strict: strict, Buckets: buckets}
}

// appendWorkflowFinalization records a validated workflow finalization: a
// session-side workflow state exit plus mirrored workflow-history records so
// the workflow lifecycle is reconstructable without the session log. A valid
// transition trigger advances the workflow chart; otherwise the state exit is
// recorded as finalized in place.
func appendWorkflowFinalization(sessionHistory *logs.SessionHistory, workflowHistory *logs.WorkflowHistory, workflowDef *defs.WorkflowDefinition, view runtime.SessionView, eval *logs.OutputContractEvaluationRecord, boundReason string) bool {
	if view.Workflow == nil || eval == nil {
		return false
	}
	current := view.Workflow.CurrentState
	trigger := strings.TrimSpace(eval.TransitionTrigger)
	next := ""
	if trigger != "" && workflowDef != nil {
		machine := statecharts.Compile("workflow", workflowDef.Statechart)
		if candidate, err := machine.Next(current, trigger); err == nil {
			next = candidate
		}
	}
	reason := "finalized"
	if next != "" {
		reason = "transition"
	}
	sessionHistory.Append(logs.StateExitRecord{SessionBaseRecord: sessionHistory.NextRecord("state_exit"), Chart: "workflow", StateName: current, Reason: reason, ParseRecordIDs: []string{eval.RecordID()}, CompletionAccepted: true, BoundReason: boundReason})
	if workflowHistory != nil {
		workflowHistory.Append(logs.WorkflowStateExitRecord{WorkflowBaseRecord: workflowHistory.NextRecord("workflow_state_exit"), StateName: current, Reason: reason, ByAgent: view.Agent.Name, DerivedFromIDs: []string{eval.RecordID()}, BoundReason: boundReason})
		if next != "" {
			workflowHistory.Append(logs.WorkflowTransitionRecord{WorkflowBaseRecord: workflowHistory.NextRecord("workflow_transition"), FromState: current, ToState: next, Trigger: trigger, DerivedFromIDs: []string{eval.RecordID()}})
		}
	}
	if next != "" {
		sessionHistory.Append(logs.StateEnterRecord{SessionBaseRecord: sessionHistory.NextRecord("state_enter"), Chart: "workflow", StateName: next, DerivedFromIDs: []string{eval.RecordID()}})
		return true
	}
	return false
}

func latestStateStartMillis(history *logs.SessionHistory, chart string) int64 {
	if history == nil {
		return 0
	}
	enterIndex := -1
	for i := len(history.Records) - 1; i >= 0; i-- {
		switch v := history.Records[i].(type) {
		case logs.StateEnterRecord:
			if v.Chart == chart {
				enterIndex = i
			}
		case *logs.StateEnterRecord:
			if v.Chart == chart {
				enterIndex = i
			}
		}
		if enterIndex != -1 {
			break
		}
	}
	if enterIndex == -1 {
		return 0
	}
	for i := enterIndex + 1; i < len(history.Records); i++ {
		switch v := history.Records[i].(type) {
		case logs.InferenceEnvelopeRecord:
			if v.StartedAtUnixMilli > 0 {
				return v.StartedAtUnixMilli
			}
		case *logs.InferenceEnvelopeRecord:
			if v.StartedAtUnixMilli > 0 {
				return v.StartedAtUnixMilli
			}
		}
	}
	return 0
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

func truncatePreview(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 200 {
		return content
	}
	return content[:200]
}
