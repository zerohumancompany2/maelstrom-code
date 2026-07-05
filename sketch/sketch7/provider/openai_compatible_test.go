package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			OptionalFields: []string{"risks"},
			FieldTypes:     map[string]string{"completion_signal": "boolean"},
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
	if _, ok := decoded["stop"]; ok {
		t.Fatalf("stop = %#v, want omitted so native tool-call generation is not truncated", decoded["stop"])
	}
	if responseFormat, ok := decoded["response_format"]; ok {
		t.Fatalf("response_format = %#v, want omitted when tools are exposed so tool calling is not suppressed", responseFormat)
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
			OptionalFields: []string{"completion_signal"},
			FieldTypes:     map[string]string{"completion_signal": "boolean"},
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
	completionSignal := properties["completion_signal"].(map[string]any)
	if completionSignal["type"] != "boolean" {
		t.Fatalf("completion_signal schema = %#v, want boolean type", completionSignal)
	}
}

func TestOpenAICompatibleProviderWrapsSchemaInBucket(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	body, err := p.buildHTTPBody(Request{
		ModelRef: "local-model",
		Lines:    []RequestLine{{Kind: "prompt", Role: "user", Content: "Finalize."}},
		ResponseFormat: &StructuredOutputFormat{
			Name:           "reader-answer-v1",
			RequiredFields: []string{"summary", "completion_signal"},
			FieldTypes:     map[string]string{"completion_signal": "boolean"},
			Strict:         true,
			WrapBucket:     "cognitive",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	schema := decoded["response_format"].(map[string]any)["json_schema"].(map[string]any)["schema"].(map[string]any)
	required, ok := schema["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "cognitive" {
		t.Fatalf("outer required = %#v, want [cognitive]", schema["required"])
	}
	inner := schema["properties"].(map[string]any)["cognitive"].(map[string]any)
	innerProps := inner["properties"].(map[string]any)
	if _, ok := innerProps["summary"]; !ok {
		t.Fatalf("inner schema = %#v, want flat contract nested under cognitive", inner)
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

func TestToJSONSchemaOmitsRequiredWhenEmpty(t *testing.T) {
	raw, err := json.Marshal(toJSONSchema(map[string]string{"path": "string"}, nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if value, ok := decoded["required"]; ok {
		t.Fatalf("required = %#v, want omitted so backends do not reject required:null", value)
	}
	raw, err = json.Marshal(toJSONSchema(map[string]string{"path": "string"}, []string{"path"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	required, ok := decoded["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "path" {
		t.Fatalf("required = %#v, want [path]", decoded["required"])
	}
}

func TestOpenAICompatibleProviderSurfacesInBodyErrorEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":{"message":"Provider returned an empty response","code":502}}`))
	}))
	defer server.Close()
	p := &OpenAICompatibleProvider{BaseURL: server.URL}
	_, err := p.Send(Request{ModelRef: "test-model", Lines: []RequestLine{{Kind: "prompt", Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "Provider returned an empty response") {
		t.Fatalf("err = %v, want in-body provider error surfaced", err)
	}
}

func TestParseOpenAIResponsePromotesReasoningWhenContentEmpty(t *testing.T) {
	resp, err := parseOpenAIResponse(openAIChatResponse{
		Choices: []struct {
			Message openAIMessage `json:"message"`
		}{
			{Message: openAIMessage{Role: "assistant", Content: "", Reasoning: `{"cognitive":{"summary":"done"}}`}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Outputs) != 1 {
		t.Fatalf("outputs = %d, want 1", len(resp.Outputs))
	}
	assistant, ok := resp.Outputs[0].(AssistantOutput)
	if !ok || assistant.Content != `{"cognitive":{"summary":"done"}}` {
		t.Fatalf("output = %#v, want reasoning promoted to content", resp.Outputs[0])
	}
}

func TestParseOpenAIResponseUsesReasoningContentFallback(t *testing.T) {
	resp, err := parseOpenAIResponse(openAIChatResponse{
		Choices: []struct {
			Message openAIMessage `json:"message"`
		}{
			{Message: openAIMessage{Role: "assistant", Content: "", ReasoningContent: "local-style reasoning"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Outputs) != 1 {
		t.Fatalf("outputs = %d, want 1", len(resp.Outputs))
	}
	assistant := resp.Outputs[0].(AssistantOutput)
	if assistant.Content != "local-style reasoning" {
		t.Fatalf("content = %q, want reasoning_content fallback", assistant.Content)
	}
}

func TestParseOpenAIResponseKeepsToolCallsOverReasoning(t *testing.T) {
	resp, err := parseOpenAIResponse(openAIChatResponse{
		Choices: []struct {
			Message openAIMessage `json:"message"`
		}{
			{Message: openAIMessage{Role: "assistant", Content: "", Reasoning: "thinking...", ToolCalls: []openAIToolCall{{ID: "c1", Type: "function", Function: openAIFunctionCall{Name: "read_file", Arguments: `{"path":"a.go"}`}}}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Outputs) != 1 {
		t.Fatalf("outputs = %d, want tool call only", len(resp.Outputs))
	}
	if _, ok := resp.Outputs[0].(ToolRequestOutput); !ok {
		t.Fatalf("output = %#v, want ToolRequestOutput", resp.Outputs[0])
	}
}
