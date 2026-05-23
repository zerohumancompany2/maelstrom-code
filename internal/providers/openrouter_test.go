package providers

import (
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/internal"
	icontext "github.com/comalice/inference_sketch/internal/context"
	"github.com/revrost/go-openrouter"
)

// Fake provider for integration-style tests
type FakeProvider struct {
	LastMessages []icontext.ContextItem
	LastOpts     ProviderOptions
	Response     []internal.SessionItem
}

func (f *FakeProvider) Send(messages []icontext.ContextItem, opts ProviderOptions) (ProviderResponse, error) {
	f.LastMessages = messages
	f.LastOpts = opts
	return fakeResponse{items: f.Response}, nil
}

type fakeResponse struct {
	items []internal.SessionItem
}

func (f fakeResponse) ToSessionItems() []internal.SessionItem { return f.items }

var _ Provider = (*OpenRouterAPI)(nil)
var _ ProviderResponse = (*OpenRouterResponse)(nil)

type unsupportedContextItem struct{ icontext.ContextItem }

func TestConvertItemUserMessage(t *testing.T) {
	item := icontext.ContextUserMessage{Content: "hello"}
	got, err := ConvertItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Role != "user" {
		t.Errorf("Role = %q, want %q", got.Role, "user")
	}
	if got.Content.Text != "hello" {
		t.Errorf("Content = %q, want %q", got.Content.Text, "hello")
	}
}

func TestConvertItemAssistantWithReasoning(t *testing.T) {
	item := icontext.ContextAssistantMessage{Content: "answer", Reasoning: "I thought"}
	got, err := ConvertItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Role != "assistant" {
		t.Errorf("Role = %q, want %q", got.Role, "assistant")
	}
	if got.Content.Text != "answer" {
		t.Errorf("Content = %q, want %q", got.Content.Text, "answer")
	}
	if got.Reasoning == nil || *got.Reasoning != "I thought" {
		t.Errorf("Reasoning = %v, want %q", got.Reasoning, "I thought")
	}
}

func TestConvertItemToolCallRequest(t *testing.T) {
	item := icontext.ContextToolCallRequestMessage{
		ToolCallID: "c1",
		Name:       "weather",
		Arguments:  `{"a":"london"}`,
	}
	got, err := ConvertItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Role != "assistant" {
		t.Errorf("Role = %q, want %q", got.Role, "assistant")
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(got.ToolCalls))
	}
	tc := got.ToolCalls[0]
	if tc.ID != "c1" {
		t.Errorf("ToolCallID = %q, want %q", tc.ID, "c1")
	}
	if tc.Function.Name != "weather" {
		t.Errorf("Function.Name = %q, want %q", tc.Function.Name, "weather")
	}
}

func TestConvertItemToolCallResult(t *testing.T) {
	item := icontext.ContextToolCallResultMessage{
		ToolCallID: "c1",
		Content:    "sunny",
	}
	got, err := ConvertItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Role != "tool" {
		t.Errorf("Role = %q, want %q", got.Role, "tool")
	}
	if got.ToolCallID != "c1" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "c1")
	}
}

func TestToOpenRouterTools(t *testing.T) {
	defs := []internal.ToolDefinition{{Name: "weather", Description: "get weather", Strict: true}}
	got := ToOpenRouterTools(defs...)

	if len(got) != 1 {
		t.Fatalf("got %d tools, want 1", len(got))
	}
	if got[0].Function.Name != "weather" {
		t.Errorf("Name = %q, want %q", got[0].Function.Name, "weather")
	}
	if got[0].Function.Description != "get weather" {
		t.Errorf("Description = %q, want %q", got[0].Function.Description, "get weather")
	}
	if !got[0].Function.Strict {
		t.Error("Strict = false, want true")
	}
}

func TestSerializeArgumentsString(t *testing.T) {
	got := serializeArguments(`{"a":"b"}`)
	if got != `{"a":"b"}` {
		t.Errorf("got %q, want %q", got, `{"a":"b"}`)
	}
}

func TestSerializeArgumentsBytes(t *testing.T) {
	got := serializeArguments([]byte(`{"a":"b"}`))
	if got != `{"a":"b"}` {
		t.Errorf("got %q, want %q", got, `{"a":"b"}`)
	}
}

func TestSerializeArgumentsMap(t *testing.T) {
	got := serializeArguments(map[string]any{"a": "b"})
	if got != `{"a":"b"}` {
		t.Errorf("got %q, want %q", got, `{"a":"b"}`)
	}
}

func TestBuildRequestModel(t *testing.T) {
	req, err := BuildRequest([]icontext.ContextItem{}, ProviderOptions{Model: "my-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Model != "my-model" {
		t.Errorf("Model = %q, want %q", req.Model, "my-model")
	}
}

func TestConvertItemUnsupportedType(t *testing.T) {
	_, err := ConvertItem(unsupportedContextItem{})
	if err == nil {
		t.Fatal("expected error for unsupported context item type")
	}
}

func TestConvertMessagesUnsupportedType(t *testing.T) {
	_, err := ConvertMessages([]icontext.ContextItem{unsupportedContextItem{}})
	if err == nil {
		t.Fatal("expected error for unsupported context item slice")
	}
}

func TestBuildRequestUnsupportedType(t *testing.T) {
	_, err := BuildRequest([]icontext.ContextItem{unsupportedContextItem{}}, ProviderOptions{Model: "my-model"})
	if err == nil {
		t.Fatal("expected error for unsupported context item during request build")
	}
}

func TestOpenRouterSendReturnsBuildRequestError(t *testing.T) {
	api := OpenRouterAPI{}

	_, err := api.Send([]icontext.ContextItem{unsupportedContextItem{}}, ProviderOptions{Model: "my-model"})
	if err == nil {
		t.Fatal("expected Send to return build-request error")
	}
	if !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newTestResponse(content string, reasoning *string, toolCalls []openrouter.ToolCall) OpenRouterResponse {
	msg := openrouter.ChatCompletionMessage{
		Content:   openrouter.Content{Text: content},
		Reasoning: reasoning,
		ToolCalls: toolCalls,
	}
	return OpenRouterResponse{Raw: &openrouter.ChatCompletionResponse{Choices: []openrouter.ChatCompletionChoice{{Message: msg}}}}
}

func TestToSessionItemsNilResponse(t *testing.T) {
	resp := OpenRouterResponse{}
	items := resp.ToSessionItems()
	if items != nil {
		t.Errorf("got %d items, want nil", len(items))
	}
}

func TestToSessionItemsEmptyChoices(t *testing.T) {
	resp := OpenRouterResponse{Raw: &openrouter.ChatCompletionResponse{}}
	items := resp.ToSessionItems()
	if items != nil {
		t.Errorf("got %d items, want nil", len(items))
	}
}

func TestToSessionItemsPlainText(t *testing.T) {
	resp := newTestResponse("hello world", nil, nil)
	items := resp.ToSessionItems()

	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	am, ok := items[0].(internal.AssistantMessage)
	if !ok {
		t.Fatalf("items[0] is %T, want AssistantMessage", items[0])
	}
	if am.Content != "hello world" {
		t.Errorf("Content = %q, want %q", am.Content, "hello world")
	}
}

func TestToSessionItemsWithReasoning(t *testing.T) {
	reasoning := "I thought about it"
	resp := newTestResponse("answer", &reasoning, nil)
	items := resp.ToSessionItems()

	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	am, ok := items[0].(internal.AssistantMessage)
	if !ok {
		t.Fatalf("items[0] is %T, want AssistantMessage", items[0])
	}
	if am.Reasoning != "I thought about it" {
		t.Errorf("Reasoning = %q, want %q", am.Reasoning, "I thought about it")
	}
}

func TestToSessionItemsToolCalls(t *testing.T) {
	resp := newTestResponse("", nil, []openrouter.ToolCall{{ID: "c1", Function: openrouter.FunctionCall{Name: "weather", Arguments: "{}"}}})
	items := resp.ToSessionItems()

	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	tc, ok := items[0].(internal.ToolCallRequestMessage)
	if !ok {
		t.Fatalf("items[0] is %T, want ToolCallRequestMessage", items[0])
	}
	if tc.Name != "weather" {
		t.Errorf("Name = %q, want %q", tc.Name, "weather")
	}
}

func TestToSessionItemsToolCallsNoText(t *testing.T) {
	resp := newTestResponse("", nil, []openrouter.ToolCall{{ID: "c1", Function: openrouter.FunctionCall{Name: "weather", Arguments: "{}"}}})
	items := resp.ToSessionItems()

	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (no phantom assistant message)", len(items))
	}
	_, ok := items[0].(internal.ToolCallRequestMessage)
	if !ok {
		t.Errorf("items[0] is %T, want ToolCallRequestMessage", items[0])
	}
}

func TestToSessionItemsMultipleToolCalls(t *testing.T) {
	resp := newTestResponse("", nil, []openrouter.ToolCall{
		{ID: "c1", Function: openrouter.FunctionCall{Name: "weather", Arguments: "{}"}},
		{ID: "c2", Function: openrouter.FunctionCall{Name: "time", Arguments: "{}"}},
	})
	items := resp.ToSessionItems()

	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
}
