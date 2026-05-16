package providers

import (
	"testing"

	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/context"
	"github.com/revrost/go-openrouter"
)

// Fake provider for integration-style tests
type FakeProvider struct {
	LastBundle *context.InferenceBundle
	Response   []internal.SessionItem
}

func (f *FakeProvider) Send(b *context.InferenceBundle) (ProviderResponse, error) {
	f.LastBundle = b
	return fakeResponse{items: f.Response}, nil
}

type fakeResponse struct {
	items []internal.SessionItem
}

func (f fakeResponse) ToSessionItems() []internal.SessionItem { return f.items }

// Compile-time: OpenRouterAPI implements Provider
var _ Provider = (*OpenRouterAPI)(nil)

// Compile-time: *OpenRouterResponse implements ProviderResponse
var _ ProviderResponse = (*OpenRouterResponse)(nil)

// ─── Seam 4: Send conversion ───

// 4.1: ConvertItem converts UserMessage
func TestConvertItemUserMessage(t *testing.T) {
	item := internal.UserMessage{Content: "hello"}
	got := ConvertItem(item)

	if got.Role != "user" {
		t.Errorf("Role = %q, want %q", got.Role, "user")
	}
	if got.Content.Text != "hello" {
		t.Errorf("Content = %q, want %q", got.Content.Text, "hello")
	}
}

// 4.2: ConvertItem converts AssistantMessage with reasoning
func TestConvertItemAssistantWithReasoning(t *testing.T) {
	item := internal.AssistantMessage{Content: "answer", Reasoning: "I thought"}
	got := ConvertItem(item)

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

// 4.3: ConvertItem converts ToolCallRequestMessage
func TestConvertItemToolCallRequest(t *testing.T) {
	item := internal.ToolCallRequestMessage{
		ToolCallID: "c1",
		Name:       "weather",
		Arguments:  `{"a":"london"}`,
	}
	got := ConvertItem(item)

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

// 4.4: ConvertItem converts ToolCallResultMessage
func TestConvertItemToolCallResult(t *testing.T) {
	item := internal.ToolCallResultMessage{
		ToolCallID: "c1",
		Content:    "sunny",
	}
	got := ConvertItem(item)

	if got.Role != "tool" {
		t.Errorf("Role = %q, want %q", got.Role, "tool")
	}
	if got.ToolCallID != "c1" {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, "c1")
	}
}

// 4.5: ToOpenRouterTools converts definitions
func TestToOpenRouterTools(t *testing.T) {
	defs := []internal.ToolDefinition{
		{Name: "weather", Description: "get weather", Strict: true},
	}
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

// 4.6a: serializeArguments handles string
func TestSerializeArgumentsString(t *testing.T) {
	got := serializeArguments(`{"a":"b"}`)
	if got != `{"a":"b"}` {
		t.Errorf("got %q, want %q", got, `{"a":"b"}`)
	}
}

// 4.6b: serializeArguments handles []byte
func TestSerializeArgumentsBytes(t *testing.T) {
	got := serializeArguments([]byte(`{"a":"b"}`))
	if got != `{"a":"b"}` {
		t.Errorf("got %q, want %q", got, `{"a":"b"}`)
	}
}

// 4.6c: serializeArguments handles map
func TestSerializeArgumentsMap(t *testing.T) {
	got := serializeArguments(map[string]any{"a": "b"})
	if got != `{"a":"b"}` {
		t.Errorf("got %q, want %q", got, `{"a":"b"}`)
	}
}

// 4.7: BuildRequest propagates model name
func TestBuildRequestModel(t *testing.T) {
	bundle := &context.InferenceBundle{
		Model:    "my-model",
		Messages: []internal.SessionItem{},
	}
	req := BuildRequest(bundle)

	if req.Model != "my-model" {
		t.Errorf("Model = %q, want %q", req.Model, "my-model")
	}
}

// ─── Seam 5: Response to SessionItems ───

// Helper: build a minimal OpenRouterResponse with controlled fields
func newTestResponse(content string, reasoning *string, toolCalls []openrouter.ToolCall) OpenRouterResponse {
	msg := openrouter.ChatCompletionMessage{
		Content:   openrouter.Content{Text: content},
		Reasoning: reasoning,
		ToolCalls: toolCalls,
	}
	return OpenRouterResponse{
		Raw: &openrouter.ChatCompletionResponse{
			Choices: []openrouter.ChatCompletionChoice{
				{Message: msg},
			},
		},
	}
}

// 5.1: Nil response → nil items
func TestToSessionItemsNilResponse(t *testing.T) {
	resp := OpenRouterResponse{}
	items := resp.ToSessionItems()
	if items != nil {
		t.Errorf("got %d items, want nil", len(items))
	}
}

// 5.2: Empty choices → nil items
func TestToSessionItemsEmptyChoices(t *testing.T) {
	resp := OpenRouterResponse{Raw: &openrouter.ChatCompletionResponse{}}
	items := resp.ToSessionItems()
	if items != nil {
		t.Errorf("got %d items, want nil", len(items))
	}
}

// 5.3: Plain text → single AssistantMessage
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

// 5.4: Text + reasoning → AssistantMessage with both fields
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

// 5.5: Tool calls → ToolCallRequestMessages
func TestToSessionItemsToolCalls(t *testing.T) {
	resp := newTestResponse("", nil, []openrouter.ToolCall{
		{ID: "c1", Function: openrouter.FunctionCall{Name: "weather", Arguments: "{}"}},
	})
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

// 5.6: Tool calls only, no text → no empty AssistantMessage
func TestToSessionItemsToolCallsNoText(t *testing.T) {
	resp := newTestResponse("", nil, []openrouter.ToolCall{
		{ID: "c1", Function: openrouter.FunctionCall{Name: "weather", Arguments: "{}"}},
	})
	items := resp.ToSessionItems()

	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (no phantom assistant message)", len(items))
	}
	_, ok := items[0].(internal.ToolCallRequestMessage)
	if !ok {
		t.Errorf("items[0] is %T, want ToolCallRequestMessage", items[0])
	}
}

// 5.7: Multiple tool calls → correct count
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
