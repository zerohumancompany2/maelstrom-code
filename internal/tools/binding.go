package tools

import (
	"fmt"

	"github.com/comalice/maelstrom/internal/logs"
)

type BindingTool struct{}

func (BindingTool) Definition() Definition {
	return Definition{
		Name:        "bind_workflow",
		Description: "Bind the current session to a workflow instance",
		Parameters:  map[string]string{"workflow_id": "string", "binding_id": "string"},
	}
}

func (BindingTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	workflowID := request.Call.Call.Arguments["workflow_id"]
	bindingID := request.Call.Call.Arguments["binding_id"]
	if workflowID == "" {
		return ExecutionResult{ToolName: "bind_workflow", DisplayContent: "binding error: workflow_id is required", IsError: true}, nil
	}
	if bindingID == "" {
		bindingID = request.Call.Call.CallID
	}
	sessionRecord := logs.SessionWorkflowBindingRecord{
		SessionBaseRecord: request.History.NextRecord("workflow_binding_ref"),
		BindingID:         bindingID,
		WorkflowID:        workflowID,
		Action:            "bind",
	}
	result := ExecutionResult{
		ToolName:       "bind_workflow",
		DisplayContent: fmt.Sprintf("bound workflow %s", workflowID),
		SessionRecords: []logs.SessionRecord{sessionRecord},
	}
	if request.Workflow != nil {
		workflowRecord := logs.WorkflowBindingRefRecord{
			WorkflowBaseRecord: request.Workflow.NextRecord("workflow_binding_ref"),
			BindingID:          bindingID,
			AgentID:            request.Agent.Name,
			Action:             "bind",
		}
		result.WorkflowRecords = []logs.WorkflowRecord{workflowRecord}
	}
	return result, nil
}

type UnbindTool struct{}

func (UnbindTool) Definition() Definition {
	return Definition{
		Name:        "unbind_workflow",
		Description: "Unbind the current session from its workflow instance",
		Parameters:  map[string]string{"binding_id": "string"},
	}
}

func (UnbindTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	if !request.Session.Binding.Bound {
		return ExecutionResult{ToolName: "unbind_workflow", DisplayContent: "binding error: session is not bound", IsError: true}, nil
	}
	bindingID := request.Call.Call.Arguments["binding_id"]
	if bindingID == "" {
		bindingID = request.Call.Call.CallID
	}
	sessionRecord := logs.SessionWorkflowBindingRecord{
		SessionBaseRecord: request.History.NextRecord("workflow_binding_ref"),
		BindingID:         bindingID,
		WorkflowID:        request.Session.Binding.WorkflowID,
		Action:            "unbind",
	}
	result := ExecutionResult{
		ToolName:       "unbind_workflow",
		DisplayContent: fmt.Sprintf("unbound workflow %s", request.Session.Binding.WorkflowID),
		SessionRecords: []logs.SessionRecord{sessionRecord},
	}
	if request.Workflow != nil {
		workflowRecord := logs.WorkflowBindingRefRecord{
			WorkflowBaseRecord: request.Workflow.NextRecord("workflow_binding_ref"),
			BindingID:          bindingID,
			AgentID:            request.Agent.Name,
			Action:             "unbind",
		}
		result.WorkflowRecords = []logs.WorkflowRecord{workflowRecord}
	}
	return result, nil
}
