package tools

import (
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch6/charts"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
)

type WeatherTool struct{}

func (WeatherTool) Definition() Definition {
	return Definition{Name: "weather", Parameters: map[string]string{"location": "string"}}
}

func (WeatherTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	workflowState := charts.BuildSnapshot(request.History).State("workflow")
	if workflowState != "lookup_pending" {
		return ExecutionResult{ToolName: "weather", DisplayContent: fmt.Sprintf("tool error: workflow must be lookup_pending, got %s", workflowState), IsError: true}, nil
	}
	transition := session.StateTransitionRecord{BaseRecord: request.History.NextRecord("state_transition"), ChartName: "workflow", FromState: workflowState, ToState: "data_ready", Trigger: "weather_received", DerivedFromIDs: []string{request.Request.RecordID()}}
	return ExecutionResult{ToolName: "weather", DisplayContent: fmt.Sprintf("70C, rainy, winds out of SSW in %s", request.Call.Call.Arguments["location"]), Records: []session.Record{transition}}, nil
}
