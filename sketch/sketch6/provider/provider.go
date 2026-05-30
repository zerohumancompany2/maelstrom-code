package provider

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/assembly"
)

type Output interface{ output() }

type AssistantOutput struct{ Content string }

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

type Request struct {
	PayloadID string
	Provider  string
	ModelName string
	Lines     []string
}

type Response struct {
	Outputs []Output
}

type Provider interface {
	BuildRequest(spec agent.Spec, payload assembly.InferencePayload) (Request, error)
	ParseResponse(request Request) (Response, error)
}

type Stub struct{}

func (Stub) BuildRequest(spec agent.Spec, payload assembly.InferencePayload) (Request, error) {
	lines := make([]string, 0, len(payload.Segments))
	for _, segment := range payload.Segments {
		switch v := segment.(type) {
		case assembly.PromptSegment:
			lines = append(lines, fmt.Sprintf("role=%s content=%q", v.Role, v.Content))
		case assembly.StateSegment:
			lines = append(lines, fmt.Sprintf("state=%s value=%q", v.ChartName, v.State))
		default:
			lines = append(lines, fmt.Sprintf("segment=%s content=%q", segment.SegmentKind(), segment.TokenText()))
		}
	}
	return Request{PayloadID: payload.PayloadID, Provider: spec.Model.Provider, ModelName: spec.Model.Name, Lines: lines}, nil
}

func (Stub) ParseResponse(request Request) (Response, error) {
	agentState := extractState(request.Lines, "agent")
	workflowState := extractState(request.Lines, "workflow")
	switch {
	case agentState == "idle":
		return Response{Outputs: []Output{mustToolRequest("call_agent_1", "transition_state", map[string]string{"chart": "agent", "trigger": "start_research"})}}, nil
	case workflowState == "idle":
		return Response{Outputs: []Output{mustToolRequest("call_workflow_1", "transition_state", map[string]string{"chart": "workflow", "trigger": "begin_lookup"})}}, nil
	case workflowState == "lookup_pending":
		return Response{Outputs: []Output{mustToolRequest("call_weather_1", "weather", map[string]string{"location": "Paris"})}}, nil
	case workflowState == "data_ready" && agentState == "researching":
		return Response{Outputs: []Output{mustToolRequest("call_agent_2", "transition_state", map[string]string{"chart": "agent", "trigger": "draft_answer"})}}, nil
	case workflowState == "data_ready" && agentState == "answering":
		return Response{Outputs: []Output{AssistantOutput{Content: "It is 70C, rainy, with winds out of the SSW in Paris."}}}, nil
	default:
		return Response{Outputs: []Output{AssistantOutput{Content: fmt.Sprintf("unexpected state combination: agent=%s workflow=%s", agentState, workflowState)}}}, nil
	}
}

func mustToolRequest(callID, toolName string, args map[string]string) ToolRequestOutput {
	raw, _ := json.Marshal(args)
	normalized := make(map[string]string, len(args))
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		normalized[key] = args[key]
	}
	return ToolRequestOutput{Call: ToolCall{CallID: callID, ToolName: toolName, Arguments: normalized, RawArgs: raw}}
}

func extractState(lines []string, chart string) string {
	prefix := fmt.Sprintf("state=%s value=", chart)
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(line[len(prefix):], `"`)
		}
	}
	return ""
}
