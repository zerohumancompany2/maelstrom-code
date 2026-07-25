package tools

import (
	"fmt"

	"github.com/comalice/maelstrom/internal/logs"
	"github.com/comalice/maelstrom/internal/statecharts"
)

type TransitionTool struct {
	AgentChart    statecharts.Machine
	WorkflowChart statecharts.Machine
}

func (TransitionTool) Definition() Definition {
	return Definition{
		Name:        "transition_state",
		Description: "Transition an agent or workflow statechart",
		Parameters:  map[string]string{"chart": "string", "trigger": "string"},
	}
}

func (t TransitionTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	chartName := request.Call.Call.Arguments["chart"]
	trigger := request.Call.Call.Arguments["trigger"]
	if chartName == "" || trigger == "" {
		return ExecutionResult{ToolName: "transition_state", DisplayContent: "transition error: chart and trigger are required", IsError: true}, nil
	}

	switch chartName {
	case "agent":
		toState, err := t.AgentChart.Next(request.Session.Cognitive.CurrentState, trigger)
		if err != nil {
			return ExecutionResult{ToolName: "transition_state", DisplayContent: fmt.Sprintf("transition error: %v", err), IsError: true}, nil
		}
		fromState := request.Session.Cognitive.CurrentState
		if fromState == "" {
			fromState = t.AgentChart.InitialState()
		}
		exitRecord := logs.StateExitRecord{
			SessionBaseRecord: request.History.NextRecord("state_exit"),
			Chart:             "cognitive",
			StateName:         fromState,
			Reason:            "transition",
			DerivedFromIDs:    []string{request.Call.Call.CallID},
		}
		transition := logs.CognitiveTransitionRecord{
			SessionBaseRecord: request.History.NextRecord("cognitive_transition"),
			FromState:         fromState,
			ToState:           toState,
			Trigger:           trigger,
			DerivedFromIDs:    []string{request.Call.Call.CallID},
		}
		enterRecord := logs.StateEnterRecord{
			SessionBaseRecord: request.History.NextRecord("state_enter"),
			Chart:             "cognitive",
			StateName:         toState,
			DerivedFromIDs:    []string{request.Call.Call.CallID},
		}
		return ExecutionResult{
			ToolName:       "transition_state",
			DisplayContent: fmt.Sprintf("transitioned agent: %s -> %s via %s", fromState, toState, trigger),
			SessionRecords: []logs.SessionRecord{exitRecord, transition, enterRecord},
		}, nil
	case "workflow":
		if request.Session.Workflow == nil || request.Workflow == nil {
			return ExecutionResult{ToolName: "transition_state", DisplayContent: "transition error: workflow is not bound", IsError: true}, nil
		}
		toState, err := t.WorkflowChart.Next(request.Session.Workflow.CurrentState, trigger)
		if err != nil {
			return ExecutionResult{ToolName: "transition_state", DisplayContent: fmt.Sprintf("transition error: %v", err), IsError: true}, nil
		}
		fromState := request.Session.Workflow.CurrentState
		if fromState == "" {
			fromState = t.WorkflowChart.InitialState()
		}
		exitRecord := logs.StateExitRecord{
			SessionBaseRecord: request.History.NextRecord("state_exit"),
			Chart:             "workflow",
			StateName:         fromState,
			Reason:            "transition",
			DerivedFromIDs:    []string{request.Call.Call.CallID},
		}
		workflowTransition := logs.WorkflowTransitionRecord{
			WorkflowBaseRecord: request.Workflow.NextRecord("workflow_transition"),
			FromState:          fromState,
			ToState:            toState,
			Trigger:            trigger,
			DerivedFromIDs:     []string{request.Call.Call.CallID},
		}
		enterRecord := logs.StateEnterRecord{
			SessionBaseRecord: request.History.NextRecord("state_enter"),
			Chart:             "workflow",
			StateName:         toState,
			DerivedFromIDs:    []string{request.Call.Call.CallID},
		}
		sessionTransition := logs.WorkflowTransitionRefRecord{
			SessionBaseRecord: request.History.NextRecord("workflow_transition_ref"),
			WorkflowID:        request.Session.Workflow.WorkflowID,
			FromState:         fromState,
			ToState:           toState,
			Trigger:           trigger,
			DerivedFromIDs:    []string{request.Call.Call.CallID},
			WorkflowRecordID:  workflowTransition.RecordID(),
		}
		return ExecutionResult{
			ToolName:        "transition_state",
			DisplayContent:  fmt.Sprintf("transitioned workflow: %s -> %s via %s", fromState, toState, trigger),
			SessionRecords:  []logs.SessionRecord{exitRecord, sessionTransition, enterRecord},
			WorkflowRecords: []logs.WorkflowRecord{workflowTransition},
		}, nil
	default:
		return ExecutionResult{ToolName: "transition_state", DisplayContent: fmt.Sprintf("transition error: unknown chart %q", chartName), IsError: true}, nil
	}
}
