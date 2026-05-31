package tools

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
)

type stubTransitionEngine struct {
	gotChart   string
	gotState   string
	gotTrigger string
	nextState  string
	err        error
}

func (s *stubTransitionEngine) Fire(chartName, currentState, trigger string) (string, error) {
	s.gotChart = chartName
	s.gotState = currentState
	s.gotTrigger = trigger
	return s.nextState, s.err
}

func TestTransitionToolUsesInitialStateWhenSessionHasNoTransitions(t *testing.T) {
	history := session.NewHistory("session-1")
	requestRecord := session.ToolCallRequestRecord{BaseRecord: history.NextRecord("tool_call_request"), CallID: "call-1", ToolName: "transition_state", Arguments: `{"chart":"agent","trigger":"start"}`}
	engine := &stubTransitionEngine{nextState: "observe"}
	tool := TransitionTool{Charts: engine, InitialStates: map[string]string{"agent": "observe_idle"}}

	result, err := tool.Execute(ExecutionRequest{
		History: history,
		Request: requestRecord,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-1", ToolName: "transition_state", Arguments: map[string]string{"chart": "agent", "trigger": "start"}}},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if engine.gotState != "observe_idle" {
		t.Fatalf("engine current state = %q, want %q", engine.gotState, "observe_idle")
	}
	if len(result.Records) != 1 {
		t.Fatalf("records len = %d, want 1", len(result.Records))
	}
	transition, ok := result.Records[0].(session.StateTransitionRecord)
	if !ok {
		t.Fatalf("record type = %T, want session.StateTransitionRecord", result.Records[0])
	}
	if transition.FromState != "observe_idle" || transition.ToState != "observe" || transition.Trigger != "start" {
		t.Fatalf("transition = %+v, want observe_idle -> observe via start", transition)
	}
	if len(transition.DerivedFromIDs) != 1 || transition.DerivedFromIDs[0] != requestRecord.RecordID() {
		t.Fatalf("derivedFrom = %#v, want [%s]", transition.DerivedFromIDs, requestRecord.RecordID())
	}
}

func TestTransitionToolPrefersDurableWorkflowHistory(t *testing.T) {
	history := session.NewHistory("session-1")
	history.Append(session.StateTransitionRecord{BaseRecord: history.NextRecord("state_transition"), ChartName: "workflow", FromState: "idle", ToState: "available", Trigger: "session_only"})
	workflowHistory := workflow.NewHistory("workflow-1")
	workflowHistory.Append(workflow.StateTransitionRecord{BaseRecord: workflowHistory.NextRecord("workflow_state_transition"), FromState: "available", ToState: "lookup_pending", Trigger: "begin_lookup"})
	requestRecord := session.ToolCallRequestRecord{BaseRecord: history.NextRecord("tool_call_request"), CallID: "call-2", ToolName: "transition_state", Arguments: `{"chart":"workflow","trigger":"weather_received"}`}
	engine := &stubTransitionEngine{nextState: "data_ready"}
	tool := TransitionTool{Charts: engine, WorkflowHistory: workflowHistory, InitialStates: map[string]string{"workflow": "available"}}

	result, err := tool.Execute(ExecutionRequest{
		History: history,
		Request: requestRecord,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-2", ToolName: "transition_state", Arguments: map[string]string{"chart": "workflow", "trigger": "weather_received"}}},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if engine.gotState != "lookup_pending" {
		t.Fatalf("engine current state = %q, want %q", engine.gotState, "lookup_pending")
	}
	if len(workflowHistory.Records) != 2 {
		t.Fatalf("workflow history len = %d, want 2", len(workflowHistory.Records))
	}
	durable, ok := workflowHistory.Records[1].(workflow.StateTransitionRecord)
	if !ok {
		t.Fatalf("durable record type = %T, want workflow.StateTransitionRecord", workflowHistory.Records[1])
	}
	if durable.FromState != "lookup_pending" || durable.ToState != "data_ready" {
		t.Fatalf("durable transition = %+v, want lookup_pending -> data_ready", durable)
	}
	if result.DisplayContent != "transitioned workflow: lookup_pending -> data_ready via weather_received" {
		t.Fatalf("display content = %q", result.DisplayContent)
	}
}

func TestTransitionToolReturnsToolErrorResultWhenTransitionFails(t *testing.T) {
	history := session.NewHistory("session-1")
	requestRecord := session.ToolCallRequestRecord{BaseRecord: history.NextRecord("tool_call_request"), CallID: "call-3", ToolName: "transition_state", Arguments: `{"chart":"agent","trigger":"bad"}`}
	engine := &stubTransitionEngine{err: assertErr("bad trigger")}
	tool := TransitionTool{Charts: engine}

	result, err := tool.Execute(ExecutionRequest{
		History: history,
		Request: requestRecord,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-3", ToolName: "transition_state", Arguments: map[string]string{"chart": "agent", "trigger": "bad"}}},
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool result to be marked as error")
	}
	if len(result.Records) != 0 {
		t.Fatalf("records len = %d, want 0", len(result.Records))
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
