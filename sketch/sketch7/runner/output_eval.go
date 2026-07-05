package runner

import (
	"encoding/json"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

func evaluateAssistantOutput(history *logs.SessionHistory, session runtime.SessionView, mode runtime.FinalizationMode, output provider.AssistantOutput, sourceRecordID string) *logs.OutputContractEvaluationRecord {
	view := session.Cognitive
	if strings.TrimSpace(view.Outputs.SchemaName) == "" && len(view.Outputs.RequiredFields) == 0 {
		return nil
	}
	record := logs.OutputContractEvaluationRecord{
		SessionBaseRecord: history.NextRecord("output_contract_evaluation"),
		StateName:         view.CurrentState,
		Chart:             "cognitive",
		SchemaName:        view.Outputs.SchemaName,
		SourceRecordID:    sourceRecordID,
		HostVersion:       hostVersion,
		ParserVersion:     outputParserVersion,
		ParseStatus:       "plain_text",
		ValidationStatus:  "missing_schema_output",
		RequiredFields:    append([]string(nil), view.Outputs.RequiredFields...),
		MissingFields:     append([]string(nil), view.Outputs.RequiredFields...),
		RawContentPreview: truncatePreview(output.Content),
	}
	payload, parseStatus, validationStatus, ok := parseOutputPayload(output.Content, mode)
	if !ok {
		if parseStatus != "" {
			record.ParseStatus = parseStatus
		}
		if validationStatus != "" {
			record.ValidationStatus = validationStatus
		}
		return &record
	}
	record.ParseStatus = parseStatus
	record.ValidationStatus = "valid"
	applyStateValidation(&record, view, payload)
	applyActionFields(&record, payload)
	applyTransitionValidation(&record, view, payload)
	applyCompletionSignal(&record, payload)
	applyRequiredFieldValidation(&record, view.Outputs.RequiredFields, payload)
	applyToolAttribution(&record, payload)
	applyStrictFieldValidation(&record, view.Outputs, payload)
	return &record
}

func parseOutputPayload(content string, mode runtime.FinalizationMode) (map[string]any, string, string, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return nil, "plain_text", "missing_schema_output", false
	}
	if wrapped, status, ok := unwrapBuckets(payload, mode); ok {
		return wrapped, "valid_json_wrapped", "", true
	} else if status != "" {
		return nil, "valid_json_wrapped", status, false
	}
	return payload, "valid_json", "", true
}

func unwrapBuckets(payload map[string]any, mode runtime.FinalizationMode) (map[string]any, string, bool) {
	if !hasAnyBucketKeys(payload) {
		if mode.IsFinalizing && (mode.RequireCognitive || mode.RequireWorkflow) {
			return nil, "missing_required_buckets", false
		}
		return nil, "", false
	}
	for key := range payload {
		if key != "cognitive" && key != "workflow" {
			return nil, "unknown_bucket", false
		}
	}
	if mode.RequireWorkflow {
		workflowRaw, ok := payload["workflow"]
		if !ok {
			return nil, "missing_workflow_bucket", false
		}
		if _, ok := workflowRaw.(map[string]any); !ok {
			return nil, "invalid_workflow_bucket", false
		}
	}
	cognitiveRaw, ok := payload["cognitive"]
	if !ok {
		if mode.RequireCognitive {
			return nil, "missing_cognitive_bucket", false
		}
		return nil, "missing_cognitive_bucket", false
	}
	cognitive, ok := cognitiveRaw.(map[string]any)
	if !ok {
		return nil, "invalid_cognitive_bucket", false
	}
	return cognitive, "", true
}

func hasAnyBucketKeys(payload map[string]any) bool {
	_, hasCognitive := payload["cognitive"]
	_, hasWorkflow := payload["workflow"]
	return hasCognitive || hasWorkflow
}

func applyStateValidation(record *logs.OutputContractEvaluationRecord, view runtime.CognitiveView, payload map[string]any) {
	state, ok := payload["state"].(string)
	if !ok || state == "" {
		return
	}
	if state != view.CurrentState {
		record.WrongState = true
		record.ValidationStatus = "wrong_state"
	}
}

func applyActionFields(record *logs.OutputContractEvaluationRecord, payload map[string]any) {
	if actionType, ok := payload["action_type"].(string); ok {
		record.ActionType = actionType
	}
}

func applyTransitionValidation(record *logs.OutputContractEvaluationRecord, view runtime.CognitiveView, payload map[string]any) {
	if transition, ok := payload["transition"].(string); ok {
		record.TransitionTrigger = transition
	}
	if transition, ok := payload["next_step_signal"].(string); ok && record.TransitionTrigger == "" {
		record.TransitionTrigger = transition
	}
	if record.TransitionTrigger != "" && len(view.AllowedTriggers) > 0 && !contains(view.AllowedTriggers, record.TransitionTrigger) {
		record.ValidationStatus = "invalid_transition_signal"
	}
}

func applyCompletionSignal(record *logs.OutputContractEvaluationRecord, payload map[string]any) {
	if completion, ok := payload["completion_signal"].(bool); ok {
		record.CompletionSignal = completion
		return
	}
	if completion, ok := payload["completion_signal"].(string); ok {
		record.CompletionSignal = strings.EqualFold(strings.TrimSpace(completion), "true")
	}
}

func applyRequiredFieldValidation(record *logs.OutputContractEvaluationRecord, required []string, payload map[string]any) {
	missing := missingFields(payload, required)
	record.MissingFields = missing
	if len(missing) > 0 {
		record.ValidationStatus = "missing_required_fields"
	}
}

func applyToolAttribution(record *logs.OutputContractEvaluationRecord, payload map[string]any) {
	if toolPayload, ok := payload["tool"].(map[string]any); ok {
		if name, ok := toolPayload["name"].(string); ok {
			record.ToolName = name
		}
	}
}

func applyStrictFieldValidation(record *logs.OutputContractEvaluationRecord, contract defs.StateOutputContract, payload map[string]any) {
	if record.ValidationStatus != "valid" || !contract.Strict {
		return
	}
	allowedFields := allowedOutputFields(contract)
	for key := range payload {
		if !contains(allowedFields, key) && !allowedOutputField(key) {
			record.ValidationStatus = "unknown_fields"
			return
		}
	}
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

func allowedOutputField(field string) bool {
	switch field {
	case "state", "action_type", "summary", "evidence", "decision", "risks", "transition", "next_step_signal", "completion_signal", "tool", "final_response":
		return true
	default:
		return false
	}
}

func allowedOutputFields(contract defs.StateOutputContract) []string {
	fields := make([]string, 0, len(contract.RequiredFields)+len(contract.OptionalFields))
	fields = append(fields, contract.RequiredFields...)
	fields = append(fields, contract.OptionalFields...)
	return fields
}
