package provider

import (
	"encoding/json"
	"strings"
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
		Tools: []ToolDefinition{{Name: "read_file", Description: "Read a file", Parameters: map[string]string{"path": "string"}, Required: []string{"path"}}},
		ResponseFormat: &StructuredOutputFormat{
			Name:           "reader-answer-v1",
			RequiredFields: []string{"summary", "evidence", "completion_signal"},
			FieldEnums:     map[string][]string{"transition": []string{"observed"}},
			Strict:         true,
		},
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
	templateKwargs, ok := decoded["chat_template_kwargs"].(map[string]any)
	if !ok || templateKwargs["enable_thinking"] != false {
		t.Fatalf("chat_template_kwargs = %#v, want enable_thinking=false", decoded["chat_template_kwargs"])
	}
	tools, ok := decoded["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want 1 tool", decoded["tools"])
	}
	stop, ok := decoded["stop"].([]any)
	if !ok || len(stop) != 1 || stop[0] != "</tool_call>" {
		t.Fatalf("stop = %#v, want [</tool_call>]", decoded["stop"])
	}
	responseFormat := decoded["response_format"].(map[string]any)
	if responseFormat["type"] != "json_object" {
		t.Fatalf("response_format = %#v, want json_object when tools are exposed", responseFormat)
	}
	toolSchema := tools[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
	required, ok := toolSchema["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "path" {
		t.Fatalf("required = %#v, want [path]", toolSchema["required"])
	}
}

func TestOpenAICompatibleProviderBuildsJSONSchemaResponseFormatWithoutTools(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	body, err := p.buildHTTPBody(Request{
		ModelRef: "local-model",
		Lines: []RequestLine{
			{Kind: "prompt", Role: "system", Content: "Return JSON only."},
			{Kind: "prompt", Role: "user", Content: "Summarize."},
		},
		ResponseFormat: &StructuredOutputFormat{
			Name:           "reader-answer-v1",
			RequiredFields: []string{"summary", "evidence", "transition"},
			FieldEnums:     map[string][]string{"transition": []string{"observed"}},
			Strict:         true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	responseFormat := decoded["response_format"].(map[string]any)
	if responseFormat["type"] != "json_schema" {
		t.Fatalf("response_format = %#v, want json_schema", responseFormat)
	}
	jsonSchema := responseFormat["json_schema"].(map[string]any)
	if jsonSchema["name"] != "reader_answer_v1" || jsonSchema["strict"] != true {
		t.Fatalf("json_schema = %#v, want sanitized strict schema", jsonSchema)
	}
	schema := jsonSchema["schema"].(map[string]any)
	if schema["additionalProperties"] != false {
		t.Fatalf("schema = %#v, want additionalProperties=false", schema)
	}
	properties := schema["properties"].(map[string]any)
	transition := properties["transition"].(map[string]any)
	if transition["type"] != "string" {
		t.Fatalf("transition schema = %#v, want conservative string schema", transition)
	}
	enumValues, ok := transition["enum"].([]any)
	if !ok || len(enumValues) != 1 || enumValues[0] != "observed" {
		t.Fatalf("transition enum = %#v, want [observed]", transition["enum"])
	}
}

func TestOpenAICompatibleProviderOmitsStopWhenNoToolsExposed(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	body, err := p.buildHTTPBody(Request{
		ModelRef: "local-model",
		Lines:    []RequestLine{{Kind: "prompt", Role: "system", Content: "Return JSON only."}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if _, ok := decoded["stop"]; ok {
		t.Fatalf("stop = %#v, want omitted when no tools are exposed", decoded["stop"])
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
	if len(resp.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(resp.Outputs))
	}
	tool, ok := resp.Outputs[0].(ToolRequestOutput)
	if !ok {
		t.Fatalf("first output = %#v, want tool request", resp.Outputs[0])
	}
	if tool.Call.ToolName != "read_file" || tool.Call.Arguments["path"] != "foo.go" {
		t.Fatalf("tool call = %+v, want read_file foo.go", tool.Call)
	}
}

func TestParseOpenAIResponseKeepsPseudoXMLToolCallsAsAssistantText(t *testing.T) {
	content := "<tool_call>\n<function=list_files>\n<parameter=path>\n.\n</parameter>\n<parameter=recursive>\nfalse\n</parameter>\n</function>\n"
	resp, err := parseOpenAIResponse(openAIChatResponse{
		Choices: []struct {
			Message openAIMessage `json:"message"`
		}{
			{
				Message: openAIMessage{
					Role:    "assistant",
					Content: content,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(resp.Outputs))
	}
	assistant, ok := resp.Outputs[0].(AssistantOutput)
	if !ok {
		t.Fatalf("first output = %#v, want assistant text, not executable tool request", resp.Outputs[0])
	}
	if assistant.Content != content {
		t.Fatalf("assistant content = %q, want pseudo tool text preserved", assistant.Content)
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

func TestOpenAICompatibleProviderAppendsUserContinuationAfterAssistantTail(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	body, err := p.buildHTTPBody(Request{
		ModelRef: "local-model",
		Lines: []RequestLine{
			{Kind: "prompt", Role: "user", Content: "Do task."},
			{Kind: "prompt", Role: "assistant", Content: `{"summary":"partial"}`},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	messages := decoded["messages"].([]any)
	last := messages[len(messages)-1].(map[string]any)
	if last["role"] != "user" {
		t.Fatalf("last message = %#v, want user continuation", last)
	}
	if !strings.Contains(last["content"].(string), "current task frame") {
		t.Fatalf("last message = %#v, want task-frame continuation", last)
	}
}
