package tools

import (
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch6/charts"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
)

type TransitionTool struct {
	Charts charts.TransitionEngine
}

func (TransitionTool) Definition() Definition {
	return Definition{Name: "transition_state", Parameters: map[string]string{"chart": "string", "trigger": "string"}}
}

func (t TransitionTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	chartName := request.Call.Call.Arguments["chart"]
	trigger := request.Call.Call.Arguments["trigger"]
	fromState := charts.BuildSnapshot(request.History).State(chartName)
	toState, err := t.Charts.Fire(chartName, fromState, trigger)
	if err != nil {
		return ExecutionResult{ToolName: "transition_state", DisplayContent: fmt.Sprintf("transition error: %v", err), IsError: true}, nil
	}
	transition := session.StateTransitionRecord{BaseRecord: request.History.NextRecord("state_transition"), ChartName: chartName, FromState: fromState, ToState: toState, Trigger: trigger, DerivedFromIDs: []string{request.Request.RecordID()}}
	return ExecutionResult{ToolName: "transition_state", DisplayContent: fmt.Sprintf("transitioned %s: %s -> %s via %s", chartName, fromState, toState, trigger), Records: []session.Record{transition}}, nil
}
