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

func (o *OpenRouterAPI) Send(i *context.InferenceBundle) (OpenRouterResponse, error) {
	// lift user messages
	messages := []openrouter.ChatCompletionMessage{
		openrouter.SystemMessage("You are a helpful assistant."),
	}
	for _, msg := range i.Messages {
		switch msg := msg.(type) {
		case internal.UserMessage:
			messages = append(messages, openrouter.UserMessage(msg.Content))
		case internal.SystemMessage:
			messages = append(messages, openrouter.SystemMessage(msg.Content))
		case internal.AssistantMessage:
			am := openrouter.AssistantMessage(msg.Content)
			am.Reasoning = &msg.Reasoning
			messages = append(messages, am)
		case internal.ToolCallRequestMessage:
			// Reconstruct proper assistant message WITH tool_calls
			am := openrouter.AssistantMessage("")

			argsStr := ""
			switch v := msg.Arguments.(type) {
			case string:
				argsStr = v
			case []byte:
				argsStr = string(v)
			case map[string]any:
				b, _ := json.Marshal(v) // need import "encoding/json"
				argsStr = string(b)
			default:
				b, _ := json.Marshal(v)
				argsStr = string(b)
			}

			am.ToolCalls = append(am.ToolCalls, openrouter.ToolCall{
				ID:   msg.ToolCallID,
				Type: openrouter.ToolTypeFunction,
				Function: openrouter.FunctionCall{ // or Function depending on exact lib version
					Name:      msg.Name,
					Arguments: argsStr,
				},
			})
			messages = append(messages, am)
		case internal.ToolCallResultMessage:
			messages = append(messages, openrouter.ToolMessage(msg.ToolCallID, msg.Content))
		default:
			messages = append(messages, openrouter.UserMessage(msg.(internal.UserMessage).Content))
		}
	}

	// send
	resp, err := o.client.CreateChatCompletion(
		ctx.Background(),
		openrouter.ChatCompletionRequest{
			Model:       i.Model,
			Messages:    messages,
			Tools:       ToOpenRouterTools(i.Tools...),
			Temperature: 0.7,
		},
	)

	if err != nil {
		fmt.Printf("ChatCompletion error: %v\n", err)
		return OpenRouterResponse{}, err
	}

	return OpenRouterResponse{Raw: &resp}, nil
}

func (o *OpenRouterResponse) ToSessionItem() internal.SessionItem {

	return internal.UserMessage{}
}

func (o *OpenRouterResponse) ToSessionItems() []internal.SessionItem {
	if o.Raw == nil || len(o.Raw.Choices) == 0 {
		return nil // should return error
	}

	choice := o.Raw.Choices[0]
	msg := choice.Message
	items := []internal.SessionItem{}

	// Add assistant message if it has content
	if msg.Content.Text != "" || len(msg.Content.Multi) > 0 {
		items = append(items, internal.AssistantMessage{Content: msg.Content.Text})
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
