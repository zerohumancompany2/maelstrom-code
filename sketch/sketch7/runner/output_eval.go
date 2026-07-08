package runner

import (
	"encoding/json"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

// evaluateAssistantOutput evaluates assistant plaintext against the active
// output contracts. During finalization it produces one evaluation record per
// required bucket (cognitive and/or workflow); outside finalization it keeps
// the observation-only cognitive evaluation.
func evaluateAssistantOutput(history *logs.SessionHistory, session runtime.SessionView, mode runtime.FinalizationMode, output provider.AssistantOutput, sourceRecordID string) []logs.OutputContractEvaluationRecord {
	if mode.IsFinalizing && (mode.RequireCognitive || mode.RequireWorkflow) {
		return evaluateFinalizationOutput(history, session, mode, output, sourceRecordID)
	}
	record := evaluateObservationOutput(history, session, output, sourceRecordID)
	if record == nil {
		return nil
	}
	return []logs.OutputContractEvaluationRecord{*record}
}

// evaluateObservationOutput is the non-finalization path: it evaluates the
// cognitive output contract against a flat payload or a voluntarily wrapped
// cognitive bucket.
func evaluateObservationOutput(history *logs.SessionHistory, session runtime.SessionView, output provider.AssistantOutput, sourceRecordID string) *logs.OutputContractEvaluationRecord {
	view := session.Cognitive
	if !hasOutputContract(view.Outputs) {
		return nil
	}
	record := newBucketEvaluationRecord(history, "cognitive", view.CurrentState, view.Outputs, output, sourceRecordID)
	var payload map[string]any
	if err := json.Unmarshal([]byte(output.Content), &payload); err != nil {
		return &record
	}
	if hasAnyBucketKeys(payload) {
		record.ParseStatus = "valid_json_wrapped"
		bucket, status := extractBucket(payload, "cognitive")
		if status != "" {
			record.ValidationStatus = status
			return &record
		}
		payload = bucket
	} else {
		record.ParseStatus = "valid_json"
	}
	record.ValidationStatus = "valid"
	validateCognitiveBucket(&record, view, payload)
	return &record
}

// evaluateFinalizationOutput parses the assistant plaintext once and produces
// an evaluation record for each bucket required by the finalization mode.
func evaluateFinalizationOutput(history *logs.SessionHistory, session runtime.SessionView, mode runtime.FinalizationMode, output provider.AssistantOutput, sourceRecordID string) []logs.OutputContractEvaluationRecord {
	var payload map[string]any
	parseErr := json.Unmarshal([]byte(output.Content), &payload)

	records := make([]logs.OutputContractEvaluationRecord, 0, 2)
	if mode.RequireCognitive {
		record := newBucketEvaluationRecord(history, "cognitive", session.Cognitive.CurrentState, session.Cognitive.Outputs, output, sourceRecordID)
		if bucket, done := resolveFinalizationBucket(&record, payload, parseErr, "cognitive"); !done {
			record.ValidationStatus = "valid"
			validateCognitiveBucket(&record, session.Cognitive, bucket)
		}
		records = append(records, record)
	}
	if mode.RequireWorkflow {
		workflow := runtime.WorkflowView{}
		if session.Workflow != nil {
			workflow = *session.Workflow
		}
		record := newBucketEvaluationRecord(history, "workflow", workflow.CurrentState, workflow.Outputs, output, sourceRecordID)
		if bucket, done := resolveFinalizationBucket(&record, payload, parseErr, "workflow"); !done {
			record.ValidationStatus = "valid"
			validateWorkflowBucket(&record, workflow, bucket)
		}
		records = append(records, record)
	}
	return records
}

// resolveFinalizationBucket applies parse- and wrapper-level checks for one
// required bucket. It returns the bucket payload when validation should
// continue, or done=true when a terminal parse/wrapper status was recorded.
func resolveFinalizationBucket(record *logs.OutputContractEvaluationRecord, payload map[string]any, parseErr error, bucketName string) (map[string]any, bool) {
	if parseErr != nil {
		// Defaults already record plain_text / missing_schema_output.
		return nil, true
	}
	if !hasAnyBucketKeys(payload) {
		record.ParseStatus = "valid_json"
		record.ValidationStatus = "missing_required_buckets"
		return nil, true
	}
	record.ParseStatus = "valid_json_wrapped"
	bucket, status := extractBucket(payload, bucketName)
	if status != "" {
		record.ValidationStatus = status
		return nil, true
	}
	return bucket, false
}

// extractBucket returns the named bucket object from a wrapper payload, or a
// terminal validation status: unknown_bucket when the wrapper carries keys
// other than cognitive/workflow, missing_<name>_bucket when the requested
// bucket is absent, invalid_<name>_bucket when it is not an object.
func extractBucket(payload map[string]any, bucketName string) (map[string]any, string) {
	for key := range payload {
		if key != "cognitive" && key != "workflow" {
			return nil, "unknown_bucket"
		}
	}
	raw, ok := payload[bucketName]
	if !ok {
		return nil, "missing_" + bucketName + "_bucket"
	}
	bucket, ok := raw.(map[string]any)
	if !ok {
		return nil, "invalid_" + bucketName + "_bucket"
	}
	return bucket, ""
}

func hasAnyBucketKeys(payload map[string]any) bool {
	_, hasCognitive := payload["cognitive"]
	_, hasWorkflow := payload["workflow"]
	return hasCognitive || hasWorkflow
}

func hasOutputContract(contract defs.StateOutputContract) bool {
	return strings.TrimSpace(contract.SchemaName) != "" || len(contract.RequiredFields) > 0
}

func newBucketEvaluationRecord(history *logs.SessionHistory, chart, stateName string, contract defs.StateOutputContract, output provider.AssistantOutput, sourceRecordID string) logs.OutputContractEvaluationRecord {
	return logs.OutputContractEvaluationRecord{
		SessionBaseRecord: history.NextRecord("output_contract_evaluation"),
		StateName:         stateName,
		Chart:             chart,
		SchemaName:        contract.SchemaName,
		SourceRecordID:    sourceRecordID,
		HostVersion:       hostVersion,
		ParserVersion:     outputParserVersion,
		ParseStatus:       "plain_text",
		ValidationStatus:  "missing_schema_output",
		RequiredFields:    append([]string(nil), contract.RequiredFields...),
		MissingFields:     append([]string(nil), contract.RequiredFields...),
		RawContentPreview: truncatePreview(output.Content),
	}
}

func validateCognitiveBucket(record *logs.OutputContractEvaluationRecord, view runtime.CognitiveView, payload map[string]any) {
	applyStateValidation(record, view, payload)
	applyActionFields(record, payload)
	applyTransitionValidation(record, view.AllowedTriggers, payload)
	applyCompletionSignal(record, payload)
	applyRequiredFieldValidation(record, view.Outputs.RequiredFields, payload)
	applyToolAttribution(record, payload)
	applyStrictFieldValidation(record, view.Outputs, payload)
}

// validateWorkflowBucket validates the workflow bucket against the workflow
// state's output contract. Transition triggers are recorded but not checked
// against a trigger list here; the workflow statechart itself arbitrates
// transitions when the loop applies them.
func validateWorkflowBucket(record *logs.OutputContractEvaluationRecord, view runtime.WorkflowView, payload map[string]any) {
	applyStateValidation(record, runtime.CognitiveView{CurrentState: view.CurrentState}, payload)
	applyActionFields(record, payload)
	applyTransitionValidation(record, nil, payload)
	applyCompletionSignal(record, payload)
	applyRequiredFieldValidation(record, view.Outputs.RequiredFields, payload)
	applyToolAttribution(record, payload)
	applyStrictFieldValidation(record, view.Outputs, payload)
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

func applyTransitionValidation(record *logs.OutputContractEvaluationRecord, allowedTriggers []string, payload map[string]any) {
	if transition, ok := payload["transition"].(string); ok {
		record.TransitionTrigger = transition
	}
	if transition, ok := payload["next_step_signal"].(string); ok && record.TransitionTrigger == "" {
		record.TransitionTrigger = transition
	}
	if record.TransitionTrigger != "" && len(allowedTriggers) > 0 && !contains(allowedTriggers, record.TransitionTrigger) {
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
