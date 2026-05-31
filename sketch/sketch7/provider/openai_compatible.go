package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

type OpenAICompatibleProvider struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Headers    map[string]string
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []openAITool    `json:"tools,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAITool struct {
	Type     string               `json:"type"`
	Function openAIFunctionSchema `json:"function"`
}

type openAIFunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
}

func (p *OpenAICompatibleProvider) BuildRequest(agent runtime.Agent, payload prompt.Payload, tools []ToolDefinition) (Request, error) {
	return BuildRequest(agent, payload, tools)
}

func (p *OpenAICompatibleProvider) Send(request Request) (Response, error) {
	body, err := p.buildHTTPBody(request)
	if err != nil {
		return Response{}, err
	}
	endpoint := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	for k, v := range p.Headers {
		httpReq.Header.Set(k, v)
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("openai-compatible request: %w", err)
	}
	defer httpResp.Body.Close()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("openai-compatible status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return Response{}, err
	}
	return parseOpenAIResponse(chatResp)
}

func (p *OpenAICompatibleProvider) buildHTTPBody(request Request) ([]byte, error) {
	messages := make([]openAIMessage, 0, len(request.Lines))
	for _, line := range request.Lines {
		switch line.Role {
		case "assistant_tool_call":
			messages = append(messages, openAIMessage{
				Role: "assistant",
				ToolCalls: []openAIToolCall{{
					ID:   line.CallID,
					Type: "function",
					Function: openAIFunctionCall{
						Name:      line.Name,
						Arguments: line.Content,
					},
				}},
			})
		case "tool":
			messages = append(messages, openAIMessage{Role: "tool", Content: line.Content, ToolCallID: line.CallID})
		default:
			messages = append(messages, openAIMessage{Role: line.Role, Content: line.Content})
		}
	}
	tools := make([]openAITool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, openAITool{
			Type: "function",
			Function: openAIFunctionSchema{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  toJSONSchema(tool.Parameters),
			},
		})
	}
	chatReq := openAIChatRequest{
		Model:    request.ModelRef,
		Messages: messages,
		Tools:    tools,
	}
	return json.Marshal(chatReq)
}

func parseOpenAIResponse(resp openAIChatResponse) (Response, error) {
	if len(resp.Choices) == 0 {
		return Response{}, nil
	}
	msg := resp.Choices[0].Message
	outputs := []Output{}
	if strings.TrimSpace(msg.Content) != "" {
		outputs = append(outputs, AssistantOutput{Content: msg.Content})
	}
	for _, toolCall := range msg.ToolCalls {
		arguments := map[string]string{}
		if strings.TrimSpace(toolCall.Function.Arguments) != "" {
			var decoded map[string]any
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &decoded); err == nil {
				for k, v := range decoded {
					arguments[k] = fmt.Sprintf("%v", v)
				}
			}
		}
		outputs = append(outputs, ToolRequestOutput{Call: ToolCall{
			CallID:    toolCall.ID,
			ToolName:  toolCall.Function.Name,
			Arguments: arguments,
			RawArgs:   json.RawMessage(toolCall.Function.Arguments),
		}})
	}
	return Response{Outputs: outputs}, nil
}

func toJSONSchema(parameters map[string]string) map[string]any {
	props := map[string]any{}
	required := []string{}
	for name, typ := range parameters {
		props[name] = map[string]any{"type": normalizeJSONType(typ)}
		required = append(required, name)
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

func normalizeJSONType(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "integer", "int":
		return "integer"
	case "boolean", "bool":
		return "boolean"
	default:
		return "string"
	}
}
