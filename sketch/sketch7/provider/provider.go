package provider

import (
	"encoding/json"
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

type RequestLine struct {
	Kind    string
	Role    string
	Name    string
	Content string
	CallID  string
}

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]string
}

type Request struct {
	PayloadID string
	Provider  string
	ModelRef  string
	Lines     []RequestLine
	Tools     []ToolDefinition
}

type Output interface{ output() }

type AssistantOutput struct {
	Content   string
	Reasoning string
}

func (AssistantOutput) output() {}

type ToolCall struct {
	CallID    string
	ToolName  string
	Arguments map[string]string
	RawArgs   json.RawMessage
}

type ToolRequestOutput struct {
	Call ToolCall
}

func (ToolRequestOutput) output() {}

type Response struct {
	Outputs []Output
}

type Provider interface {
	BuildRequest(agent runtime.Agent, payload prompt.Payload, tools []ToolDefinition) (Request, error)
	Send(request Request) (Response, error)
}

func BuildRequest(agent runtime.Agent, payload prompt.Payload, tools []ToolDefinition) (Request, error) {
	lines := make([]RequestLine, 0, len(payload.Segments))
	for _, segment := range payload.Segments {
		switch v := segment.(type) {
		case prompt.PromptSegment:
			line := RequestLine{Kind: "prompt", Role: v.Role, Name: v.Name, Content: v.Content, CallID: v.CallID}
			lines = append(lines, line)
		case prompt.StateSegment:
			lines = append(lines, RequestLine{Kind: "state", Name: v.Name, Content: v.State})
		default:
			return Request{}, fmt.Errorf("unsupported segment type %T", segment)
		}
	}
	return Request{
		PayloadID: payload.PayloadID,
		Provider:  agent.ProviderName,
		ModelRef:  agent.ProviderRef,
		Lines:     lines,
		Tools:     append([]ToolDefinition(nil), tools...),
	}, nil
}
