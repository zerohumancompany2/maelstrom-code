package tools

import (
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch6/charts"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
)

type TransitionTool struct {
	Charts          charts.TransitionEngine
	WorkflowHistory *workflow.History
	InitialStates   map[string]string
}

func (TransitionTool) Definition() Definition {
	return Definition{Name: "transition_state", Parameters: map[string]string{"chart": "string", "trigger": "string"}}
}

func (t TransitionTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	chartName := request.Call.Call.Arguments["chart"]
	trigger := request.Call.Call.Arguments["trigger"]
	fromState := charts.BuildSnapshot(request.History).State(chartName)
	if initial, ok := t.InitialStates[chartName]; ok && fromState == "idle" {
		fromState = initial
	}
	if chartName == "workflow" && t.WorkflowHistory != nil {
		workflowState := charts.BuildWorkflowSnapshot(workflow.Definition{Statechart: workflow.StatechartDefinition{InitialState: t.InitialStates["workflow"]}}, t.WorkflowHistory).State("workflow")
		if workflowState != "unknown" {
			fromState = workflowState
		}
	}
	toState, err := t.Charts.Fire(chartName, fromState, trigger)
	if err != nil {
		return ExecutionResult{ToolName: "transition_state", DisplayContent: fmt.Sprintf("transition error: %v", err), IsError: true}, nil
	}
	transition := session.StateTransitionRecord{BaseRecord: request.History.NextRecord("state_transition"), ChartName: chartName, FromState: fromState, ToState: toState, Trigger: trigger, DerivedFromIDs: []string{request.Request.RecordID()}}
	records := []session.Record{transition}
	if chartName == "workflow" && t.WorkflowHistory != nil {
		t.WorkflowHistory.Append(workflow.StateTransitionRecord{BaseRecord: t.WorkflowHistory.NextRecord("workflow_state_transition"), FromState: fromState, ToState: toState, Trigger: trigger, DerivedFromIDs: []string{request.Request.RecordID()}})
	}
	return ExecutionResult{ToolName: "transition_state", DisplayContent: fmt.Sprintf("transitioned %s: %s -> %s via %s", chartName, fromState, toState, trigger), Records: records}, nil
}
