package runner

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

// evaluateSingleOutput asserts exactly one evaluation record and returns it.
func evaluateSingleOutput(t *testing.T, history *logs.SessionHistory, session runtime.SessionView, mode runtime.FinalizationMode, content string) logs.OutputContractEvaluationRecord {
	t.Helper()
	records := evaluateAssistantOutput(history, session, mode, provider.AssistantOutput{Content: content}, "assistant-1")
	if len(records) != 1 {
		t.Fatalf("got %d evaluation records, want 1: %+v", len(records), records)
	}
	return records[0]
}

func evaluationByChart(t *testing.T, records []logs.OutputContractEvaluationRecord, chart string) logs.OutputContractEvaluationRecord {
	t.Helper()
	for _, record := range records {
		if record.Chart == chart {
			return record
		}
	}
	t.Fatalf("no evaluation record for chart %q in %+v", chart, records)
	return logs.OutputContractEvaluationRecord{}
}

func TestEvaluateAssistantOutputRejectsUnknownStrictField(t *testing.T) {
	history := logs.NewSessionHistory("session-unknown-strict-field")
	view := runtime.CognitiveView{
		CurrentState: "observe",
		Outputs: defs.StateOutputContract{
			SchemaName:     "cognitive_step_v1",
			RequiredFields: []string{"summary", "completion_signal"},
			OptionalFields: []string{"evidence"},
			Strict:         true,
		},
	}
	record := evaluateSingleOutput(t, history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, `{"summary":"done","completion_signal":true,"unexpected":"x"}`)
	if record.ValidationStatus != "unknown_fields" {
		t.Fatalf("ValidationStatus = %q, want unknown_fields", record.ValidationStatus)
	}
}

func TestEvaluateAssistantOutputParsesTransitionSignal(t *testing.T) {
	history := logs.NewSessionHistory("session-transition-signal")
	view := runtime.CognitiveView{
		CurrentState:    "observe",
		AllowedTriggers: []string{"observed"},
		Outputs: defs.StateOutputContract{
			SchemaName:     "cognitive_step_v1",
			RequiredFields: []string{"summary", "transition"},
		},
	}
	record := evaluateSingleOutput(t, history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, `{"summary":"done","transition":"observed"}`)
	if record.ValidationStatus != "valid" {
		t.Fatalf("ValidationStatus = %q, want valid", record.ValidationStatus)
	}
	if record.TransitionTrigger != "observed" {
		t.Fatalf("TransitionTrigger = %q, want observed", record.TransitionTrigger)
	}
}

func TestEvaluateAssistantOutputRejectsInvalidTransitionSignal(t *testing.T) {
	history := logs.NewSessionHistory("session-invalid-transition-signal")
	view := runtime.CognitiveView{
		CurrentState:    "observe",
		AllowedTriggers: []string{"observed"},
		Outputs: defs.StateOutputContract{
			SchemaName:     "cognitive_step_v1",
			RequiredFields: []string{"summary", "transition"},
		},
	}
	record := evaluateSingleOutput(t, history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, `{"summary":"done","transition":"decided"}`)
	if record.ValidationStatus != "invalid_transition_signal" {
		t.Fatalf("ValidationStatus = %q, want invalid_transition_signal", record.ValidationStatus)
	}
}

func TestEvaluateAssistantOutputAcceptsWrappedCognitiveBucket(t *testing.T) {
	history := logs.NewSessionHistory("session-wrapped-cognitive")
	view := runtime.CognitiveView{
		CurrentState: "observe",
		Outputs: defs.StateOutputContract{
			SchemaName:     "cognitive_step_v1",
			RequiredFields: []string{"summary", "completion_signal"},
			Strict:         true,
		},
	}
	record := evaluateSingleOutput(t, history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, `{"cognitive":{"summary":"done","completion_signal":true},"workflow":{"note":"ignored for now"}}`)
	if record.ParseStatus != "valid_json_wrapped" {
		t.Fatalf("ParseStatus = %q, want valid_json_wrapped", record.ParseStatus)
	}
	if record.ValidationStatus != "valid" {
		t.Fatalf("ValidationStatus = %q, want valid", record.ValidationStatus)
	}
	if !record.CompletionSignal {
		t.Fatal("expected completion_signal=true")
	}
}

func TestEvaluateAssistantOutputRejectsMissingCognitiveBucket(t *testing.T) {
	history := logs.NewSessionHistory("session-missing-cognitive-bucket")
	view := runtime.CognitiveView{
		CurrentState: "observe",
		Outputs: defs.StateOutputContract{
			SchemaName:     "cognitive_step_v1",
			RequiredFields: []string{"summary"},
		},
	}
	record := evaluateSingleOutput(t, history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, `{"workflow":{"summary":"done"}}`)
	if record.ParseStatus != "valid_json_wrapped" {
		t.Fatalf("ParseStatus = %q, want valid_json_wrapped", record.ParseStatus)
	}
	if record.ValidationStatus != "missing_cognitive_bucket" {
		t.Fatalf("ValidationStatus = %q, want missing_cognitive_bucket", record.ValidationStatus)
	}
}

func TestEvaluateAssistantOutputRejectsUnknownBucket(t *testing.T) {
	history := logs.NewSessionHistory("session-unknown-bucket")
	view := runtime.CognitiveView{
		CurrentState: "observe",
		Outputs: defs.StateOutputContract{
			SchemaName:     "cognitive_step_v1",
			RequiredFields: []string{"summary"},
		},
	}
	record := evaluateSingleOutput(t, history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, `{"cognitive":{"summary":"done"},"meta":{"trace":"x"}}`)
	if record.ValidationStatus != "unknown_bucket" {
		t.Fatalf("ValidationStatus = %q, want unknown_bucket", record.ValidationStatus)
	}
}

func TestEvaluateAssistantOutputRequiresBucketsInCognitiveFinalizationMode(t *testing.T) {
	history := logs.NewSessionHistory("session-cognitive-finalization-flat")
	view := runtime.CognitiveView{CurrentState: "observe", Outputs: defs.StateOutputContract{SchemaName: "cognitive_step_v1", RequiredFields: []string{"summary"}}}
	mode := runtime.FinalizationMode{IsFinalizing: true, RequireCognitive: true}
	record := evaluateSingleOutput(t, history, runtime.SessionView{Cognitive: view}, mode, `{"summary":"done"}`)
	if record.ParseStatus != "valid_json" {
		t.Fatalf("ParseStatus = %q, want valid_json", record.ParseStatus)
	}
	if record.ValidationStatus != "missing_required_buckets" {
		t.Fatalf("ValidationStatus = %q, want missing_required_buckets", record.ValidationStatus)
	}
}

func combinedFinalizationSession() runtime.SessionView {
	return runtime.SessionView{
		Cognitive: runtime.CognitiveView{
			CurrentState: "observe",
			Outputs:      defs.StateOutputContract{SchemaName: "cognitive_step_v1", RequiredFields: []string{"summary"}},
		},
		Workflow: &runtime.WorkflowView{
			WorkflowID:   "workflow-eval",
			CurrentState: "triaging",
			Outputs:      defs.StateOutputContract{SchemaName: "triage_v1", RequiredFields: []string{"decision"}},
		},
	}
}

func TestEvaluateAssistantOutputCombinedFinalizationEmitsPerBucketRecords(t *testing.T) {
	history := logs.NewSessionHistory("session-combined-finalization")
	mode := runtime.FinalizationMode{IsFinalizing: true, RequireCognitive: true, RequireWorkflow: true}
	records := evaluateAssistantOutput(history, combinedFinalizationSession(), mode, provider.AssistantOutput{Content: `{"cognitive":{"summary":"done"},"workflow":{"decision":"ship it"}}`}, "assistant-1")
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	cognitive := evaluationByChart(t, records, "cognitive")
	if cognitive.ValidationStatus != "valid" || cognitive.SchemaName != "cognitive_step_v1" || cognitive.StateName != "observe" {
		t.Fatalf("cognitive record = %+v, want valid cognitive_step_v1 in observe", cognitive)
	}
	workflow := evaluationByChart(t, records, "workflow")
	if workflow.ValidationStatus != "valid" || workflow.SchemaName != "triage_v1" || workflow.StateName != "triaging" {
		t.Fatalf("workflow record = %+v, want valid triage_v1 in triaging", workflow)
	}
}

func TestEvaluateAssistantOutputCombinedFinalizationDistinguishesMissingBucketFromMissingField(t *testing.T) {
	history := logs.NewSessionHistory("session-combined-partial")
	mode := runtime.FinalizationMode{IsFinalizing: true, RequireCognitive: true, RequireWorkflow: true}

	// Workflow bucket absent entirely.
	records := evaluateAssistantOutput(history, combinedFinalizationSession(), mode, provider.AssistantOutput{Content: `{"cognitive":{"summary":"done"}}`}, "assistant-1")
	workflow := evaluationByChart(t, records, "workflow")
	if workflow.ValidationStatus != "missing_workflow_bucket" {
		t.Fatalf("ValidationStatus = %q, want missing_workflow_bucket", workflow.ValidationStatus)
	}
	cognitive := evaluationByChart(t, records, "cognitive")
	if cognitive.ValidationStatus != "valid" {
		t.Fatalf("cognitive ValidationStatus = %q, want valid (attribution preserved)", cognitive.ValidationStatus)
	}

	// Workflow bucket present but missing a required field.
	records = evaluateAssistantOutput(history, combinedFinalizationSession(), mode, provider.AssistantOutput{Content: `{"cognitive":{"summary":"done"},"workflow":{"note":"no decision"}}`}, "assistant-2")
	workflow = evaluationByChart(t, records, "workflow")
	if workflow.ValidationStatus != "missing_required_fields" {
		t.Fatalf("ValidationStatus = %q, want missing_required_fields", workflow.ValidationStatus)
	}
	if len(workflow.MissingFields) != 1 || workflow.MissingFields[0] != "decision" {
		t.Fatalf("MissingFields = %v, want [decision]", workflow.MissingFields)
	}
}

func TestEvaluateAssistantOutputCombinedFinalizationRejectsInvalidWorkflowBucket(t *testing.T) {
	history := logs.NewSessionHistory("session-combined-invalid-workflow")
	mode := runtime.FinalizationMode{IsFinalizing: true, RequireCognitive: true, RequireWorkflow: true}
	records := evaluateAssistantOutput(history, combinedFinalizationSession(), mode, provider.AssistantOutput{Content: `{"cognitive":{"summary":"done"},"workflow":"not an object"}`}, "assistant-1")
	workflow := evaluationByChart(t, records, "workflow")
	if workflow.ValidationStatus != "invalid_workflow_bucket" {
		t.Fatalf("ValidationStatus = %q, want invalid_workflow_bucket", workflow.ValidationStatus)
	}
}

func TestEvaluateAssistantOutputWorkflowOnlyFinalizationValidatesWorkflowContract(t *testing.T) {
	history := logs.NewSessionHistory("session-workflow-only-finalization")
	session := combinedFinalizationSession()
	mode := runtime.FinalizationMode{IsFinalizing: true, RequireWorkflow: true}
	records := evaluateAssistantOutput(history, session, mode, provider.AssistantOutput{Content: `{"workflow":{"decision":"escalate","completion_signal":true}}`}, "assistant-1")
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	workflow := records[0]
	if workflow.Chart != "workflow" || workflow.ValidationStatus != "valid" {
		t.Fatalf("record = %+v, want valid workflow evaluation", workflow)
	}
	if !workflow.CompletionSignal {
		t.Fatal("expected completion_signal=true on workflow record")
	}
}

func TestEvaluateAssistantOutputWorkflowStrictRejectsUnknownField(t *testing.T) {
	history := logs.NewSessionHistory("session-workflow-strict")
	session := combinedFinalizationSession()
	session.Workflow.Outputs.Strict = true
	mode := runtime.FinalizationMode{IsFinalizing: true, RequireWorkflow: true}
	records := evaluateAssistantOutput(history, session, mode, provider.AssistantOutput{Content: `{"workflow":{"decision":"ship it","unexpected":"x"}}`}, "assistant-1")
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].ValidationStatus != "unknown_fields" {
		t.Fatalf("ValidationStatus = %q, want unknown_fields", records[0].ValidationStatus)
	}
}

func TestEvaluateAssistantOutputFinalizationMalformedJSONRecordsBothBuckets(t *testing.T) {
	history := logs.NewSessionHistory("session-combined-malformed")
	mode := runtime.FinalizationMode{IsFinalizing: true, RequireCognitive: true, RequireWorkflow: true}
	records := evaluateAssistantOutput(history, combinedFinalizationSession(), mode, provider.AssistantOutput{Content: `not json at all`}, "assistant-1")
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	for _, record := range records {
		if record.ParseStatus != "plain_text" || record.ValidationStatus != "missing_schema_output" {
			t.Fatalf("record = %+v, want plain_text/missing_schema_output", record)
		}
	}
}
