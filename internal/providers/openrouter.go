package providers

import (
	ctx "context"
	"encoding/json"
	"fmt"

	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/context"
	"github.com/revrost/go-openrouter"
)

type OpenRouterToolCallRequest struct{}

type OpenRouterAPI struct {
	client openrouter.Client
}

type OpenRouterResponse struct {
	Raw *openrouter.ChatCompletionResponse
}

func (o *OpenRouterAPI) Send(i *context.InferenceBundle) (ProviderResponse, error) {
	req := BuildRequest(i)

	resp, err := o.client.CreateChatCompletion(ctx.Background(), req)
	if err != nil {
		fmt.Printf("ChatCompletion error: %v\n", err)
		return nil, err
	}

	return &OpenRouterResponse{Raw: &resp}, nil
}

// BuildRequest converts an InferenceBundle into an openrouter ChatCompletionRequest.
func BuildRequest(i *context.InferenceBundle) openrouter.ChatCompletionRequest {
	return openrouter.ChatCompletionRequest{
		Model:       i.Model,
		Messages:    ConvertMessages(i.Messages),
		Tools:       ToOpenRouterTools(i.Tools...),
		Temperature: 0.7,
	}
}

// ConvertMessages converts internal session items into openrouter messages.
func ConvertMessages(items []internal.SessionItem) []openrouter.ChatCompletionMessage {
	messages := []openrouter.ChatCompletionMessage{
		openrouter.SystemMessage("You are a helpful assistant."),
	}

	for _, item := range items {
		messages = append(messages, ConvertItem(item))
	}

	return messages
}

// ConvertItem converts a single session item into an openrouter message.
func ConvertItem(item internal.SessionItem) openrouter.ChatCompletionMessage {
	switch msg := item.(type) {
	case internal.UserMessage:
		return openrouter.UserMessage(msg.Content)
	case internal.SystemMessage:
		return openrouter.SystemMessage(msg.Content)
	case internal.AssistantMessage:
		am := openrouter.AssistantMessage(msg.Content)
		am.Reasoning = &msg.Reasoning
		return am
	case internal.ToolCallRequestMessage:
		return convertToolCallRequest(msg)
	case internal.ToolCallResultMessage:
		return openrouter.ToolMessage(msg.ToolCallID, msg.Content)
	default:
		return openrouter.UserMessage("")
	}
}

// convertToolCallRequest converts a tool call request into an assistant message with tool_calls.
func convertToolCallRequest(msg internal.ToolCallRequestMessage) openrouter.ChatCompletionMessage {
	am := openrouter.AssistantMessage("")

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
		b, _ := json.Marshal(v)
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

	// Add assistant message if it has content or reasoning
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

	// Add tool calls if present
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

func NewOpenRouter(apiKey string) OpenRouterAPI {
	return OpenRouterAPI{
		client: *openrouter.NewClient(
			apiKey,
			openrouter.WithXTitle("MaelstromCode"),
			openrouter.WithHTTPReferer("https://maelstrom.dev"),
		),
	}
}
