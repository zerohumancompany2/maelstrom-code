package provider

import (
	"encoding/json"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

func TestBuildRequestConvertsPromptPayloadToStructuredLines(t *testing.T) {
	agent := runtime.Agent{ProviderName: "openrouter", ProviderRef: "gpt-test"}
	step := prompt.ProvenanceStep{ProjectionName: "system", Operation: "project-static"}
	payload := prompt.Payload{
		PayloadID: "payload-001",
		Segments: []prompt.Segment{
			prompt.PromptSegment{Role: "system", Content: "You are a coding agent.", Step: step},
			prompt.PromptSegment{Role: "user", Content: "Fix the test.", Step: step},
		},
	}
	tools := []ToolDefinition{{Name: "run_tests", Description: "Run test suite", Parameters: map[string]string{"package": "string"}}}

	request, err := BuildRequest(agent, payload, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Provider != "openrouter" {
		t.Fatalf("Provider = %q, want openrouter", request.Provider)
	}
	if request.ModelRef != "gpt-test" {
		t.Fatalf("ModelRef = %q, want gpt-test", request.ModelRef)
	}
	if len(request.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(request.Lines))
	}
	if request.Lines[0].Kind != "prompt" || request.Lines[0].Role != "system" {
		t.Fatalf("first line = %+v, want system prompt", request.Lines[0])
	}
	if len(request.Tools) != 1 || request.Tools[0].Name != "run_tests" {
		t.Fatalf("Tools = %+v, want run_tests tool", request.Tools)
	}
}

func TestBuildRequestConvertsStateSegment(t *testing.T) {
	agent := runtime.Agent{ProviderName: "openrouter", ProviderRef: "gpt-test"}
	step := prompt.ProvenanceStep{ProjectionName: "state", Operation: "project-state"}
	payload := prompt.Payload{PayloadID: "payload-002", Segments: []prompt.Segment{
		prompt.StateSegment{Name: "workflow", State: "planning", Step: step},
	}}

	request, err := BuildRequest(agent, payload, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(request.Lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(request.Lines))
	}
	if request.Lines[0].Kind != "state" || request.Lines[0].Name != "workflow" || request.Lines[0].Content != "planning" {
		t.Fatalf("line = %+v, want workflow state planning", request.Lines[0])
	}
}

func TestFakeProviderSendReturnsConfiguredResponse(t *testing.T) {
	fake := &FakeProvider{Response: Response{Outputs: []Output{AssistantOutput{Content: "done"}}}}
	request := Request{PayloadID: "payload-003"}

	response, err := fake.Send(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.SentRequest.PayloadID != "payload-003" {
		t.Fatalf("SentRequest.PayloadID = %q, want payload-003", fake.SentRequest.PayloadID)
	}
	if len(response.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(response.Outputs))
	}
	if output, ok := response.Outputs[0].(AssistantOutput); !ok || output.Content != "done" {
		t.Fatalf("unexpected output = %#v", response.Outputs[0])
	}
}

func TestToolRequestOutputCarriesStructuredArguments(t *testing.T) {
	raw, err := json.Marshal(map[string]string{"path": "./..."})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	response := Response{Outputs: []Output{ToolRequestOutput{Call: ToolCall{CallID: "call-001", ToolName: "run_tests", Arguments: map[string]string{"path": "./..."}, RawArgs: raw}}}}

	if len(response.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(response.Outputs))
	}
	tool, ok := response.Outputs[0].(ToolRequestOutput)
	if !ok {
		t.Fatalf("output is %T, want ToolRequestOutput", response.Outputs[0])
	}
	if tool.Call.ToolName != "run_tests" {
		t.Fatalf("ToolName = %q, want run_tests", tool.Call.ToolName)
	}
	if tool.Call.Arguments["path"] != "./..." {
		t.Fatalf("Arguments[path] = %q, want ./...", tool.Call.Arguments["path"])
	}
}
