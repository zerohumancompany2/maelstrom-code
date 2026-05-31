package tools

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

func TestBindingToolEmitsSessionAndWorkflowBindRecords(t *testing.T) {
	tool := BindingTool{}
	history := logs.NewSessionHistory("session-005")
	workflowHistory := logs.NewWorkflowHistory("workflow-005")
	request := ExecutionRequest{
		Agent:    runtime.Agent{Name: "builder"},
		History:  history,
		Workflow: workflowHistory,
		Call:     provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-bind-001", ToolName: "bind_workflow", Arguments: map[string]string{"workflow_id": "workflow-005", "binding_id": "bind-005"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if len(result.SessionRecords) != 1 || len(result.WorkflowRecords) != 1 {
		t.Fatalf("got session=%d workflow=%d, want 1 each", len(result.SessionRecords), len(result.WorkflowRecords))
	}
	sessionRecord, ok := result.SessionRecords[0].(logs.SessionWorkflowBindingRecord)
	if !ok {
		t.Fatalf("session record is %T, want SessionWorkflowBindingRecord", result.SessionRecords[0])
	}
	if sessionRecord.Action != "bind" || sessionRecord.WorkflowID != "workflow-005" {
		t.Fatalf("session record = %+v, want bind workflow-005", sessionRecord)
	}
	workflowRecord, ok := result.WorkflowRecords[0].(logs.WorkflowBindingRefRecord)
	if !ok {
		t.Fatalf("workflow record is %T, want WorkflowBindingRefRecord", result.WorkflowRecords[0])
	}
	if workflowRecord.Action != "bind" || workflowRecord.AgentID != "builder" {
		t.Fatalf("workflow record = %+v, want bind by builder", workflowRecord)
	}
}

func TestUnbindToolEmitsSessionAndWorkflowUnbindRecords(t *testing.T) {
	tool := UnbindTool{}
	history := logs.NewSessionHistory("session-006")
	workflowHistory := logs.NewWorkflowHistory("workflow-006")
	request := ExecutionRequest{
		Agent:    runtime.Agent{Name: "builder"},
		Session:  runtime.SessionView{Binding: runtime.BindingView{WorkflowID: "workflow-006", Bound: true}},
		History:  history,
		Workflow: workflowHistory,
		Call:     provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-unbind-001", ToolName: "unbind_workflow", Arguments: map[string]string{"binding_id": "bind-006"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	sessionRecord, ok := result.SessionRecords[0].(logs.SessionWorkflowBindingRecord)
	if !ok {
		t.Fatalf("session record is %T, want SessionWorkflowBindingRecord", result.SessionRecords[0])
	}
	if sessionRecord.Action != "unbind" {
		t.Fatalf("session record = %+v, want unbind", sessionRecord)
	}
	workflowRecord, ok := result.WorkflowRecords[0].(logs.WorkflowBindingRefRecord)
	if !ok {
		t.Fatalf("workflow record is %T, want WorkflowBindingRefRecord", result.WorkflowRecords[0])
	}
	if workflowRecord.Action != "unbind" {
		t.Fatalf("workflow record = %+v, want unbind", workflowRecord)
	}
}

func TestBindingReducerSeesLatestBindAndUnbindRecords(t *testing.T) {
	history := logs.NewSessionHistory("session-007")
	bindTool := BindingTool{}
	unbindTool := UnbindTool{}

	bindResult, err := bindTool.Execute(ExecutionRequest{
		Agent:   runtime.Agent{Name: "builder"},
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-bind-007", ToolName: "bind_workflow", Arguments: map[string]string{"workflow_id": "workflow-007"}}},
	})
	if err != nil {
		t.Fatalf("bind execute: %v", err)
	}
	for _, record := range bindResult.SessionRecords {
		history.Append(record)
	}
	bound := runtime.ReduceBindingState(history)
	if !bound.Bound || bound.WorkflowID != "workflow-007" {
		t.Fatalf("bound view = %+v, want bound workflow-007", bound)
	}

	unbindResult, err := unbindTool.Execute(ExecutionRequest{
		Agent:   runtime.Agent{Name: "builder"},
		Session: runtime.SessionView{Binding: bound},
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-unbind-007", ToolName: "unbind_workflow", Arguments: map[string]string{}}},
	})
	if err != nil {
		t.Fatalf("unbind execute: %v", err)
	}
	for _, record := range unbindResult.SessionRecords {
		history.Append(record)
	}
	unbound := runtime.ReduceBindingState(history)
	if unbound.Bound {
		t.Fatalf("bound view = %+v, want unbound", unbound)
	}
	if unbound.WorkflowID != "workflow-007" {
		t.Fatalf("WorkflowID = %q, want workflow-007", unbound.WorkflowID)
	}
}
