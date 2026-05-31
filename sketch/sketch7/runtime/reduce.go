package runtime

import (
	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
)

func ReduceCognitiveState(history *logs.SessionHistory, initialState string, chart defs.StatechartDefinition) CognitiveView {
	current := initialState
	if current == "" {
		current = "idle"
	}
	for i := len(history.Records) - 1; i >= 0; i-- {
		transition, ok := history.Records[i].(logs.CognitiveTransitionRecord)
		if !ok {
			continue
		}
		current = transition.ToState
		break
	}
	return cognitiveViewForState(chart, current)
}

func ReduceWorkflowState(history *logs.WorkflowHistory, def defs.WorkflowDefinition, baseAgent string) WorkflowView {
	current := def.Statechart.InitialState
	if current == "" {
		current = "idle"
	}
	resolvedState := false
	view := WorkflowView{
		WorkflowID:     history.WorkflowID,
		CurrentState:   current,
		Description:    def.Description,
		Context:        def.Context,
		LastBoundAgent: baseAgent,
	}
	for i := len(history.Records) - 1; i >= 0; i-- {
		switch v := history.Records[i].(type) {
		case logs.WorkflowTransitionRecord:
			if !resolvedState {
				view.CurrentState = v.ToState
				resolvedState = true
			}
		case logs.WorkflowBindingRefRecord:
			if view.LastBoundAgent == "" && v.Action == "bind" {
				view.LastBoundAgent = v.AgentID
			}
		}
	}
	view.VisibleTools, view.EnabledTools = toolPolicyForState(def.Statechart, view.CurrentState)
	return view
}

func ReduceBindingState(history *logs.SessionHistory) BindingView {
	for i := len(history.Records) - 1; i >= 0; i-- {
		binding, ok := history.Records[i].(logs.SessionWorkflowBindingRecord)
		if !ok {
			continue
		}
		return BindingView{WorkflowID: binding.WorkflowID, Bound: binding.Action == "bind"}
	}
	return BindingView{}
}

func ReduceInteractionMode(history *logs.SessionHistory) InteractionView {
	for i := len(history.Records) - 1; i >= 0; i-- {
		switch v := history.Records[i].(type) {
		case logs.ResumeRecord:
			return InteractionView{Mode: InteractionResumable, Reason: v.Reason}
		case logs.InterruptRecord:
			return InteractionView{Mode: InteractionInterrupted, Reason: v.Reason}
		case logs.UserMessageRecord:
			return InteractionView{Mode: InteractionInteractive}
		case logs.ToolCallRequestRecord, logs.ToolCallResultRecord:
			return InteractionView{Mode: InteractionRunning}
		}
	}
	return InteractionView{Mode: InteractionInteractive}
}

func cognitiveViewForState(chart defs.StatechartDefinition, name string) CognitiveView {
	for _, state := range chart.States {
		if state.Name != name {
			continue
		}
		return CognitiveView{
			CurrentState: state.Name,
			VisibleTools: append([]string(nil), state.VisibleTools...),
			EnabledTools: append([]string(nil), state.EnabledTools...),
			Prompt:       state.Prompt,
		}
	}
	return CognitiveView{CurrentState: name}
}

func toolPolicyForState(chart defs.StatechartDefinition, name string) ([]string, []string) {
	for _, state := range chart.States {
		if state.Name != name {
			continue
		}
		return append([]string(nil), state.VisibleTools...), append([]string(nil), state.EnabledTools...)
	}
	return nil, nil
}
