package tools

import (
	"encoding/json"
	"testing"

	"github.com/comalice/maelstrom/internal/defs"
	"github.com/comalice/maelstrom/internal/logs"
	"github.com/comalice/maelstrom/internal/provider"
	"github.com/comalice/maelstrom/internal/runtime"
	"github.com/comalice/maelstrom/internal/statecharts"
)

func TestRegistryExecutesRegisteredTool(t *testing.T) {
	tool := TransitionTool{
		AgentChart: statecharts.Compile("agent", defs.StatechartDefinition{InitialState: "observe", Transitions: []defs.TransitionDefinition{{Trigger: "begin_action", From: "observe", To: "act"}}}),
	}
	registry := NewRegistry(tool)
	request := ExecutionRequest{
		Session: runtime.SessionView{Cognitive: runtime.CognitiveView{CurrentState: "observe"}},
		History: logs.NewSessionHistory("session-001"),
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-001", ToolName: "transition_state", Arguments: map[string]string{"chart": "agent", "trigger": "begin_action"}}},
	}

	result, err := registry.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToolName != "transition_state" {
		t.Fatalf("ToolName = %q, want transition_state", result.ToolName)
	}
	if len(result.SessionRecords) != 3 {
		t.Fatalf("got %d session records, want 3", len(result.SessionRecords))
	}
}

func TestTransitionToolTransitionsAgentState(t *testing.T) {
	tool := TransitionTool{
		AgentChart: statecharts.Compile("agent", defs.StatechartDefinition{InitialState: "observe", Transitions: []defs.TransitionDefinition{{Trigger: "begin_action", From: "observe", To: "act"}}}),
	}
	history := logs.NewSessionHistory("session-002")
	raw, _ := json.Marshal(map[string]string{"chart": "agent", "trigger": "begin_action"})
	request := ExecutionRequest{
		Session: runtime.SessionView{Cognitive: runtime.CognitiveView{CurrentState: "observe"}},
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-002", ToolName: "transition_state", Arguments: map[string]string{"chart": "agent", "trigger": "begin_action"}, RawArgs: raw}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	exit, ok := result.SessionRecords[0].(logs.StateExitRecord)
	if !ok {
		t.Fatalf("first record is %T, want StateExitRecord", result.SessionRecords[0])
	}
	if exit.Chart != "cognitive" || exit.StateName != "observe" || exit.Reason != "transition" {
		t.Fatalf("exit record = %+v, want cognitive observe transition exit", exit)
	}
	transition, ok := result.SessionRecords[1].(logs.CognitiveTransitionRecord)
	if !ok {
		t.Fatalf("second record is %T, want CognitiveTransitionRecord", result.SessionRecords[1])
	}
	if transition.ToState != "act" {
		t.Fatalf("ToState = %q, want act", transition.ToState)
	}
	enter, ok := result.SessionRecords[2].(logs.StateEnterRecord)
	if !ok {
		t.Fatalf("third record is %T, want StateEnterRecord", result.SessionRecords[2])
	}
	if enter.Chart != "cognitive" || enter.StateName != "act" {
		t.Fatalf("enter record = %+v, want cognitive act enter", enter)
	}
}

func TestTransitionToolTransitionsWorkflowState(t *testing.T) {
	tool := TransitionTool{
		WorkflowChart: statecharts.Compile("workflow", defs.StatechartDefinition{InitialState: "planning", Transitions: []defs.TransitionDefinition{{Trigger: "start_implementation", From: "planning", To: "implementing"}}}),
	}
	workflowHistory := logs.NewWorkflowHistory("workflow-001")
	request := ExecutionRequest{
		Session:  runtime.SessionView{Workflow: &runtime.WorkflowView{WorkflowID: "workflow-001", CurrentState: "planning"}},
		History:  logs.NewSessionHistory("session-003"),
		Workflow: workflowHistory,
		Call:     provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-003", ToolName: "transition_state", Arguments: map[string]string{"chart": "workflow", "trigger": "start_implementation"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if len(result.SessionRecords) != 3 {
		t.Fatalf("got %d session records, want 3", len(result.SessionRecords))
	}
	if len(result.WorkflowRecords) != 1 {
		t.Fatalf("got %d workflow records, want 1", len(result.WorkflowRecords))
	}
	exit, ok := result.SessionRecords[0].(logs.StateExitRecord)
	if !ok {
		t.Fatalf("first session record is %T, want StateExitRecord", result.SessionRecords[0])
	}
	if exit.Chart != "workflow" || exit.StateName != "planning" || exit.Reason != "transition" {
		t.Fatalf("exit record = %+v, want workflow planning transition exit", exit)
	}
	sessionRef, ok := result.SessionRecords[1].(logs.WorkflowTransitionRefRecord)
	if !ok {
		t.Fatalf("second session record is %T, want WorkflowTransitionRefRecord", result.SessionRecords[1])
	}
	if sessionRef.ToState != "implementing" {
		t.Fatalf("session ref ToState = %q, want implementing", sessionRef.ToState)
	}
	transition, ok := result.WorkflowRecords[0].(logs.WorkflowTransitionRecord)
	if !ok {
		t.Fatalf("record is %T, want WorkflowTransitionRecord", result.WorkflowRecords[0])
	}
	if transition.ToState != "implementing" {
		t.Fatalf("ToState = %q, want implementing", transition.ToState)
	}
	if sessionRef.WorkflowRecordID != transition.RecordID() {
		t.Fatalf("WorkflowRecordID = %q, want %q", sessionRef.WorkflowRecordID, transition.RecordID())
	}
	enter, ok := result.SessionRecords[2].(logs.StateEnterRecord)
	if !ok {
		t.Fatalf("third session record is %T, want StateEnterRecord", result.SessionRecords[2])
	}
	if enter.Chart != "workflow" || enter.StateName != "implementing" {
		t.Fatalf("enter record = %+v, want workflow implementing enter", enter)
	}
}

func TestTransitionToolReturnsErrorResultForUnknownChart(t *testing.T) {
	tool := TransitionTool{}
	request := ExecutionRequest{
		History: logs.NewSessionHistory("session-004"),
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-004", ToolName: "transition_state", Arguments: map[string]string{"chart": "bogus", "trigger": "noop"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}
