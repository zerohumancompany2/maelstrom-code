package provider

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch6/assembly"
	"github.com/comalice/inference_sketch/sketch/sketch6/runtime"
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
	ModelRef  string
	Lines     []string
}

type Response struct {
	Outputs []Output
}

type Provider interface {
	BuildRequest(agent runtime.Agent, payload assembly.InferencePayload) (Request, error)
	ParseResponse(request Request) (Response, error)
}

type Stub struct{}

func (Stub) BuildRequest(agent runtime.Agent, payload assembly.InferencePayload) (Request, error) {
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
	return Request{PayloadID: payload.PayloadID, Provider: agent.ProviderName, ModelRef: agent.ProviderRef, Lines: lines}, nil
}

func (Stub) ParseResponse(request Request) (Response, error) {
	cognitive := extractQuoted(request.Lines, "Cognitive mode: ")
	workflowState := extractQuoted(request.Lines, "Workflow workflow-001 is in state ")
	switch {
	case strings.HasPrefix(cognitive, "observe"):
		return Response{Outputs: []Output{mustToolRequest("call_agent_1", "transition_state", map[string]string{"chart": "agent", "trigger": "start_research"})}}, nil
	case strings.HasPrefix(cognitive, "orient") && strings.HasPrefix(workflowState, "available"):
		return Response{Outputs: []Output{mustToolRequest("call_workflow_1", "transition_state", map[string]string{"chart": "workflow", "trigger": "begin_lookup"})}}, nil
	case strings.HasPrefix(cognitive, "orient") && strings.HasPrefix(workflowState, "lookup_pending"):
		return Response{Outputs: []Output{mustToolRequest("call_agent_2", "transition_state", map[string]string{"chart": "agent", "trigger": "begin_action"})}}, nil
	case strings.HasPrefix(cognitive, "act") && strings.HasPrefix(workflowState, "lookup_pending"):
		return Response{Outputs: []Output{mustToolRequest("call_weather_1", "weather", map[string]string{"location": "Paris"})}}, nil
	case strings.HasPrefix(cognitive, "act") && strings.HasPrefix(workflowState, "data_ready"):
		return Response{Outputs: []Output{mustToolRequest("call_agent_3", "transition_state", map[string]string{"chart": "agent", "trigger": "draft_answer"})}}, nil
	case strings.HasPrefix(cognitive, "observe") && strings.HasPrefix(workflowState, "data_ready"):
		return Response{Outputs: []Output{AssistantOutput{Content: "It is 70C, rainy, with winds out of the SSW in Paris."}}}, nil
	default:
		return Response{Outputs: []Output{AssistantOutput{Content: fmt.Sprintf("unexpected state combination: cognitive=%s workflow=%s", cognitive, workflowState)}}}, nil
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

func extractQuoted(lines []string, prefix string) string {
	for _, line := range lines {
		if !strings.Contains(line, prefix) {
			continue
		}
		idx := strings.Index(line, prefix)
		value := line[idx+len(prefix):]
		value = strings.TrimSpace(value)
		value = strings.TrimPrefix(value, "\"")
		if quote := strings.Index(value, "\""); quote >= 0 {
			value = value[:quote]
		}
		return value
	}
	return ""
}
