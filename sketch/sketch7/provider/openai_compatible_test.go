package provider

import (
	"encoding/json"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

func TestOpenAICompatibleProviderBuildsChatCompletionsBody(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	body, err := p.buildHTTPBody(Request{
		ModelRef: "local-model",
		Lines: []RequestLine{
			{Kind: "prompt", Role: "system", Content: "You are a coding agent."},
			{Kind: "prompt", Role: "user", Content: "Fix the bug."},
		},
		Tools: []ToolDefinition{{Name: "read_file", Description: "Read a file", Parameters: map[string]string{"path": "string"}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if decoded["model"] != "local-model" {
		t.Fatalf("model = %v, want local-model", decoded["model"])
	}
	tools, ok := decoded["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want 1 tool", decoded["tools"])
	}
}

func TestParseOpenAIResponseConvertsAssistantAndToolCalls(t *testing.T) {
	resp, err := parseOpenAIResponse(openAIChatResponse{
		Choices: []struct {
			Message openAIMessage `json:"message"`
		}{
			{
				Message: openAIMessage{
					Role:    "assistant",
					Content: "I'll inspect the file.",
					ToolCalls: []openAIToolCall{{
						ID:   "call-1",
						Type: "function",
						Function: openAIFunctionCall{
							Name:      "read_file",
							Arguments: `{"path":"foo.go"}`,
						},
					}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Outputs) != 2 {
		t.Fatalf("got %d outputs, want 2", len(resp.Outputs))
	}
	if assistant, ok := resp.Outputs[0].(AssistantOutput); !ok || assistant.Content != "I'll inspect the file." {
		t.Fatalf("first output = %#v, want assistant output", resp.Outputs[0])
	}
	tool, ok := resp.Outputs[1].(ToolRequestOutput)
	if !ok {
		t.Fatalf("second output = %#v, want tool request", resp.Outputs[1])
	}
	if tool.Call.ToolName != "read_file" || tool.Call.Arguments["path"] != "foo.go" {
		t.Fatalf("tool call = %+v, want read_file foo.go", tool.Call)
	}
}

func TestOpenAICompatibleProviderImplementsProviderContractBuildRequest(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	request, err := p.BuildRequest(runtime.Agent{ProviderName: "openai", ProviderRef: "local-model"}, prompt.Payload{PayloadID: "p1", Segments: []prompt.Segment{prompt.PromptSegment{Role: "system", Content: "hello"}}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.ModelRef != "local-model" {
		t.Fatalf("ModelRef = %q, want local-model", request.ModelRef)
	}
}

func TestOpenAICompatibleProviderBuildsToolReplayMessages(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	body, err := p.buildHTTPBody(Request{
		ModelRef: "local-model",
		Lines: []RequestLine{
			{Kind: "prompt", Role: "user", Content: "Find replace_text."},
			{Kind: "prompt", Role: "assistant_tool_call", Name: "search_files", CallID: "call-123", Content: `{"pattern":"replace_text"}`},
			{Kind: "prompt", Role: "tool", Name: "search_files", CallID: "call-123", Content: "Backend: rg\nsketch/sketch7/tools/replace_text.go:16: Name: replace_text"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	messages, ok := decoded["messages"].([]any)
	if !ok || len(messages) != 3 {
		t.Fatalf("messages = %#v, want 3 messages", decoded["messages"])
	}
	assistantMsg := messages[1].(map[string]any)
	if assistantMsg["role"] != "assistant" {
		t.Fatalf("assistant replay role = %v, want assistant", assistantMsg["role"])
	}
	toolCalls, ok := assistantMsg["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("tool_calls = %#v, want 1 call", assistantMsg["tool_calls"])
	}
	toolMsg := messages[2].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "call-123" {
		t.Fatalf("tool replay message = %#v, want tool role with call id", toolMsg)
	}
}
