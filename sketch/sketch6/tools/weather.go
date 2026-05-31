package tools

import (
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch6/charts"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
)

type WeatherTool struct {
	WorkflowHistory *workflow.History
	InitialState    string
}

func (WeatherTool) Definition() Definition {
	return Definition{Name: "weather", Parameters: map[string]string{"location": "string"}}
}

func (w WeatherTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	workflowState := charts.BuildSnapshot(request.History).State("workflow")
	if w.WorkflowHistory != nil {
		derived := charts.BuildWorkflowSnapshot(workflow.Definition{Statechart: workflow.StatechartDefinition{InitialState: w.InitialState}}, w.WorkflowHistory).State("workflow")
		if derived != "unknown" {
			workflowState = derived
		}
	}
	if workflowState != "lookup_pending" {
		return ExecutionResult{ToolName: "weather", DisplayContent: fmt.Sprintf("tool error: workflow must be lookup_pending, got %s", workflowState), IsError: true}, nil
	}
	transition := session.StateTransitionRecord{BaseRecord: request.History.NextRecord("state_transition"), ChartName: "workflow", FromState: workflowState, ToState: "data_ready", Trigger: "weather_received", DerivedFromIDs: []string{request.Request.RecordID()}}
	if w.WorkflowHistory != nil {
		w.WorkflowHistory.Append(workflow.StateTransitionRecord{BaseRecord: w.WorkflowHistory.NextRecord("workflow_state_transition"), FromState: workflowState, ToState: "data_ready", Trigger: "weather_received", DerivedFromIDs: []string{request.Request.RecordID()}})
	}
	return ExecutionResult{ToolName: "weather", DisplayContent: fmt.Sprintf("70C, rainy, winds out of SSW in %s", request.Call.Call.Arguments["location"]), Records: []session.Record{transition}}, nil
}
