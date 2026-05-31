package provider

import (
	"encoding/json"
	"fmt"
	"strings"

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
	lastToolCallID := ""
	lastToolName := ""
	for _, segment := range payload.Segments {
		switch v := segment.(type) {
		case prompt.PromptSegment:
			line := RequestLine{Kind: "prompt", Role: v.Role, Content: v.Content}
			if v.Role == "assistant_tool_call" {
				callName, callArgs := parseAssistantToolCallText(v.Content)
				lastToolCallID = firstRecordID(v.RecordIDs)
				if lastToolCallID == "" {
					lastToolCallID = callName + "-call"
				}
				lastToolName = callName
				line.Name = callName
				line.Content = callArgs
				line.CallID = lastToolCallID
			}
			if v.Role == "tool" {
				line.CallID = lastToolCallID
				line.Name = lastToolName
			}
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

func parseAssistantToolCallText(content string) (string, string) {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "tool call ")
	open := strings.Index(trimmed, "(")
	close := strings.LastIndex(trimmed, ")")
	if open == -1 || close == -1 || close < open {
		return trimmed, "{}"
	}
	name := strings.TrimSpace(trimmed[:open])
	args := strings.TrimSpace(trimmed[open+1 : close])
	if args == "" {
		args = "{}"
	}
	return name, args
}

func firstRecordID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}
