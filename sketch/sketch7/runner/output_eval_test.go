package runner

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

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
	record := evaluateAssistantOutput(history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, provider.AssistantOutput{Content: `{"summary":"done","completion_signal":true,"unexpected":"x"}`}, "assistant-1")
	if record == nil {
		t.Fatal("expected output evaluation record")
	}
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
	record := evaluateAssistantOutput(history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, provider.AssistantOutput{Content: `{"summary":"done","transition":"observed"}`}, "assistant-1")
	if record == nil {
		t.Fatal("expected output evaluation record")
	}
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
	record := evaluateAssistantOutput(history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, provider.AssistantOutput{Content: `{"summary":"done","transition":"decided"}`}, "assistant-1")
	if record == nil {
		t.Fatal("expected output evaluation record")
	}
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
	record := evaluateAssistantOutput(history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, provider.AssistantOutput{Content: `{"cognitive":{"summary":"done","completion_signal":true},"workflow":{"note":"ignored for now"}}`}, "assistant-1")
	if record == nil {
		t.Fatal("expected output evaluation record")
	}
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
	record := evaluateAssistantOutput(history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, provider.AssistantOutput{Content: `{"workflow":{"summary":"done"}}`}, "assistant-1")
	if record == nil {
		t.Fatal("expected output evaluation record")
	}
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
	record := evaluateAssistantOutput(history, runtime.SessionView{Cognitive: view}, runtime.FinalizationMode{}, provider.AssistantOutput{Content: `{"cognitive":{"summary":"done"},"meta":{"trace":"x"}}`}, "assistant-1")
	if record == nil {
		t.Fatal("expected output evaluation record")
	}
	if record.ValidationStatus != "unknown_bucket" {
		t.Fatalf("ValidationStatus = %q, want unknown_bucket", record.ValidationStatus)
	}
}

func TestEvaluateAssistantOutputRequiresWorkflowBucketInCombinedFinalizationMode(t *testing.T) {
	history := logs.NewSessionHistory("session-combined-finalization-missing-workflow")
	view := runtime.CognitiveView{CurrentState: "observe", Outputs: defs.StateOutputContract{SchemaName: "cognitive_step_v1", RequiredFields: []string{"summary"}}}
	mode := runtime.FinalizationMode{IsFinalizing: true, RequireCognitive: true, RequireWorkflow: true}
	record := evaluateAssistantOutput(history, runtime.SessionView{Cognitive: view}, mode, provider.AssistantOutput{Content: `{"cognitive":{"summary":"done"}}`}, "assistant-1")
	if record == nil {
		t.Fatal("expected output evaluation record")
	}
	if record.ValidationStatus != "missing_workflow_bucket" {
		t.Fatalf("ValidationStatus = %q, want missing_workflow_bucket", record.ValidationStatus)
	}
}

func TestEvaluateAssistantOutputRequiresBucketsInCognitiveFinalizationMode(t *testing.T) {
	history := logs.NewSessionHistory("session-cognitive-finalization-flat")
	view := runtime.CognitiveView{CurrentState: "observe", Outputs: defs.StateOutputContract{SchemaName: "cognitive_step_v1", RequiredFields: []string{"summary"}}}
	mode := runtime.FinalizationMode{IsFinalizing: true, RequireCognitive: true}
	record := evaluateAssistantOutput(history, runtime.SessionView{Cognitive: view}, mode, provider.AssistantOutput{Content: `{"summary":"done"}`}, "assistant-1")
	if record == nil {
		t.Fatal("expected output evaluation record")
	}
	if record.ValidationStatus != "missing_required_buckets" {
		t.Fatalf("ValidationStatus = %q, want missing_required_buckets", record.ValidationStatus)
	}
}
