package tools

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
)

func TestWeatherToolUsesDurableWorkflowStateAndAppendsWorkflowHistory(t *testing.T) {
	history := session.NewHistory("session-1")
	history.Append(session.StateTransitionRecord{BaseRecord: history.NextRecord("state_transition"), ChartName: "workflow", FromState: "idle", ToState: "available", Trigger: "session_only"})
	workflowHistory := workflow.NewHistory("workflow-1")
	workflowHistory.Append(workflow.StateTransitionRecord{BaseRecord: workflowHistory.NextRecord("workflow_state_transition"), FromState: "available", ToState: "lookup_pending", Trigger: "begin_lookup"})
	requestRecord := session.ToolCallRequestRecord{BaseRecord: history.NextRecord("tool_call_request"), CallID: "call-weather", ToolName: "weather", Arguments: `{"location":"Paris"}`}
	tool := WeatherTool{WorkflowHistory: workflowHistory, InitialState: "available"}

	result, err := tool.Execute(ExecutionRequest{
		History: history,
		Request: requestRecord,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-weather", ToolName: "weather", Arguments: map[string]string{"location": "Paris"}}},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error content %q", result.DisplayContent)
	}
	if len(result.Records) != 1 {
		t.Fatalf("records len = %d, want 1", len(result.Records))
	}
	transition, ok := result.Records[0].(session.StateTransitionRecord)
	if !ok {
		t.Fatalf("record type = %T, want session.StateTransitionRecord", result.Records[0])
	}
	if transition.FromState != "lookup_pending" || transition.ToState != "data_ready" {
		t.Fatalf("session transition = %+v, want lookup_pending -> data_ready", transition)
	}
	if len(workflowHistory.Records) != 2 {
		t.Fatalf("workflow history len = %d, want 2", len(workflowHistory.Records))
	}
	durable, ok := workflowHistory.Records[1].(workflow.StateTransitionRecord)
	if !ok {
		t.Fatalf("durable record type = %T, want workflow.StateTransitionRecord", workflowHistory.Records[1])
	}
	if durable.FromState != "lookup_pending" || durable.ToState != "data_ready" || durable.Trigger != "weather_received" {
		t.Fatalf("durable transition = %+v, want lookup_pending -> data_ready via weather_received", durable)
	}
	if len(durable.DerivedFromIDs) != 1 || durable.DerivedFromIDs[0] != requestRecord.RecordID() {
		t.Fatalf("durable derivedFrom = %#v, want [%s]", durable.DerivedFromIDs, requestRecord.RecordID())
	}
}

func TestWeatherToolRejectsWrongWorkflowState(t *testing.T) {
	history := session.NewHistory("session-1")
	requestRecord := session.ToolCallRequestRecord{BaseRecord: history.NextRecord("tool_call_request"), CallID: "call-weather", ToolName: "weather", Arguments: `{"location":"Paris"}`}
	tool := WeatherTool{InitialState: "available"}

	result, err := tool.Execute(ExecutionRequest{
		History: history,
		Request: requestRecord,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-weather", ToolName: "weather", Arguments: map[string]string{"location": "Paris"}}},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected weather tool error result")
	}
	if len(result.Records) != 0 {
		t.Fatalf("records len = %d, want 0", len(result.Records))
	}
	if result.DisplayContent != "tool error: workflow must be lookup_pending, got idle" {
		t.Fatalf("display content = %q", result.DisplayContent)
	}
}
