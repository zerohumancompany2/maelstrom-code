package providers

import (
	ctx "context"
	"encoding/json"
	"fmt"

	"github.com/comalice/inference_sketch/internal"
	icontext "github.com/comalice/inference_sketch/internal/context"
	"github.com/revrost/go-openrouter"
)

type OpenRouterAPI struct {
	client openrouter.Client
}

type OpenRouterResponse struct {
	Raw *openrouter.ChatCompletionResponse
}

func (o *OpenRouterAPI) Send(messages []icontext.ContextItem, opts ProviderOptions) (ProviderResponse, error) {
	req, err := BuildRequest(messages, opts)
	if err != nil {
		return nil, err
	}

	resp, err := o.client.CreateChatCompletion(ctx.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("openrouter chat completion: %w", err)
	}

	return &OpenRouterResponse{Raw: &resp}, nil
}

// BuildRequest converts messages and options into an openrouter ChatCompletionRequest.
func BuildRequest(messages []icontext.ContextItem, opts ProviderOptions) (openrouter.ChatCompletionRequest, error) {
	converted, err := ConvertMessages(messages)
	if err != nil {
		return openrouter.ChatCompletionRequest{}, err
	}

	return openrouter.ChatCompletionRequest{
		Model:       opts.Model,
		Messages:    converted,
		Tools:       ToOpenRouterTools(opts.Tools...),
		Temperature: 0.7,
	}, nil
}

// ConvertMessages converts shaped context items into openrouter messages.
func ConvertMessages(items []icontext.ContextItem) ([]openrouter.ChatCompletionMessage, error) {
	messages := make([]openrouter.ChatCompletionMessage, 0, len(items))

	for _, item := range items {
		msg, err := ConvertItem(item)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// ConvertItem converts a single context item into an openrouter message.
func ConvertItem(item icontext.ContextItem) (openrouter.ChatCompletionMessage, error) {
	switch msg := item.(type) {
	case icontext.ContextUserMessage:
		return openrouter.UserMessage(msg.Content), nil
	case icontext.ContextSystemMessage:
		return openrouter.SystemMessage(msg.Content), nil
	case icontext.ContextAssistantMessage:
		am := openrouter.AssistantMessage(msg.Content)
		am.Reasoning = &msg.Reasoning
		return am, nil
	case icontext.ContextToolCallRequestMessage:
		return convertToolCallRequest(msg), nil
	case icontext.ContextToolCallResultMessage:
		return openrouter.ToolMessage(msg.ToolCallID, msg.Content), nil
	default:
		return openrouter.ChatCompletionMessage{}, fmt.Errorf("convert context item: unsupported type %T", item)
	}
}

// convertToolCallRequest converts a tool call request into an assistant message with tool_calls.
func convertToolCallRequest(msg icontext.ContextToolCallRequestMessage) openrouter.ChatCompletionMessage {
	am := openrouter.AssistantMessage(msg.Content)

	argsStr := serializeArguments(msg.Arguments)

	am.ToolCalls = append(am.ToolCalls, openrouter.ToolCall{
		ID:   msg.ToolCallID,
		Type: openrouter.ToolTypeFunction,
		Function: openrouter.FunctionCall{
			Name:      msg.Name,
			Arguments: argsStr,
		},
	})
	return am
}

// serializeArguments converts tool arguments to a JSON string.
func serializeArguments(args any) string {
	switch v := args.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf(`{"__serialize_error": failed to marshall arguments: %v"}`, err)
		}
		return string(b)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func (o *OpenRouterResponse) ToSessionItems() []internal.SessionItem {
	if o.Raw == nil || len(o.Raw.Choices) == 0 {
		return nil
	}

	choice := o.Raw.Choices[0]
	msg := choice.Message
	items := []internal.SessionItem{}

	reasoning := ""
	if msg.Reasoning != nil {
		reasoning = *msg.Reasoning
	}
	if msg.Content.Text != "" || len(msg.Content.Multi) > 0 || reasoning != "" {
		items = append(items, internal.AssistantMessage{
			Content:   msg.Content.Text,
			Reasoning: reasoning,
		})
	}

	for _, tc := range msg.ToolCalls {
		toolCallMsg := internal.ToolCallRequestMessage{
			ToolCallID: tc.ID,
			Name:       tc.Function.Name,
			Arguments:  tc.Function.Arguments,
		}
		items = append(items, toolCallMsg)
	}

	return items
}

func ToOpenRouterTools(defs ...internal.ToolDefinition) []openrouter.Tool {
	tools := make([]openrouter.Tool, len(defs))
	for i, def := range defs {
		tools[i] = openrouter.Tool{
			Type: openrouter.ToolTypeFunction,
			Function: &openrouter.FunctionDefinition{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.Parameters,
				Strict:      def.Strict,
			},
		}
	}

	return tools
}

func NewOpenRouter(apiKey string) (OpenRouterAPI, error) {
	if apiKey == "" {
		return OpenRouterAPI{}, fmt.Errorf("missing OpenRouter API key")
	}
	return OpenRouterAPI{
		client: *openrouter.NewClient(
			apiKey,
			openrouter.WithXTitle("MaelstromCode"),
			openrouter.WithHTTPReferer("https://maelstrom.dev"),
		),
	}, nil
}
