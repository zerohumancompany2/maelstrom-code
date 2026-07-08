package runtime

import (
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
)

// FinalizationMode indicates the current finalization state.
type FinalizationMode struct {
	IsFinalizing     bool
	RequireCognitive bool
	RequireWorkflow  bool
	RetryAttempt     int
}

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
	view.Inputs, view.Outputs, view.Completion, view.Bounds = stateContractsForState(def.Statechart, view.CurrentState)
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
			CurrentState:    state.Name,
			VisibleTools:    append([]string(nil), state.VisibleTools...),
			EnabledTools:    append([]string(nil), state.EnabledTools...),
			AllowedTriggers: append([]string(nil), state.AllowedTriggers...),
			Prompt:          state.Prompt,
			Inputs: defs.StateInputContract{
				Required: append([]string(nil), state.Inputs.Required...),
				Optional: append([]string(nil), state.Inputs.Optional...),
			},
			Outputs: defs.StateOutputContract{
				SchemaName:     state.Outputs.SchemaName,
				RequiredFields: append([]string(nil), state.Outputs.RequiredFields...),
				OptionalFields: append([]string(nil), state.Outputs.OptionalFields...),
				Strict:         state.Outputs.Strict,
			},
			Completion: defs.StateCompletionContract{
				SuccessWhen: append([]string(nil), state.Completion.SuccessWhen...),
			},
			Bounds: defs.StateBoundsContract{
				MaxInferenceTurns:      state.Bounds.MaxInferenceTurns,
				MaxToolCalls:           state.Bounds.MaxToolCalls,
				MaxWallTimeSeconds:     state.Bounds.MaxWallTimeSeconds,
				MaxFinalizationRetries: state.Bounds.MaxFinalizationRetries,
			},
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

// IsCognitiveBoundHit checks if the cognitive state's inference-turn or
// tool-call budget has been exhausted. Returns true only when cognitive
// outputs are declared, since finalization needs a contract to finalize
// against; exhausting a budget then forces finalization instead of failing
// the session.
func IsCognitiveBoundHit(view CognitiveView, history *logs.SessionHistory) bool {
	// Cognitive outputs must be declared for finalization mode to apply.
	if view.Outputs.SchemaName == "" && len(view.Outputs.RequiredFields) == 0 {
		return false
	}
	if view.Bounds.MaxInferenceTurns > 0 && logs.CountInferenceTurnsSinceStateEnter(history, "cognitive") >= view.Bounds.MaxInferenceTurns {
		return true
	}
	if view.Bounds.MaxToolCalls > 0 && logs.CountToolCallsSinceStateEnter(history, "cognitive") >= view.Bounds.MaxToolCalls {
		return true
	}
	return false
}

// ShouldFinalizeCognitive checks if we should enter cognitive finalization mode.
func ShouldFinalizeCognitive(view CognitiveView, history *logs.SessionHistory) bool {
	return IsCognitiveBoundHit(view, history)
}

func ResolveFinalizationMode(cognitive CognitiveView, workflow *WorkflowView, history *logs.SessionHistory) FinalizationMode {
	mode := FinalizationMode{}
	if ShouldFinalizeCognitive(cognitive, history) {
		mode.IsFinalizing = true
		mode.RequireCognitive = true
		mode.RetryAttempt = logs.CountFinalizationRetriesSinceStateEnter(history, "cognitive") + 1
	}
	if workflow == nil {
		return mode
	}
	if workflowRequiresFinalization(*workflow) {
		mode.IsFinalizing = true
		mode.RequireWorkflow = true
	}
	return mode
}

func workflowRequiresFinalization(view WorkflowView) bool {
	return strings.TrimSpace(view.Outputs.SchemaName) != "" && len(view.Outputs.RequiredFields) > 0
}

// GetMaxFinalizationRetries returns the max finalization retries, defaulting to 1 if not set.
func GetMaxFinalizationRetries(bounds defs.StateBoundsContract) int {
	if bounds.MaxFinalizationRetries <= 0 {
		return 1
	}
	return bounds.MaxFinalizationRetries
}

// HasFinalizationRetriesExceeded checks if finalization retries have been exhausted.
func HasFinalizationRetriesExceeded(view CognitiveView, history *logs.SessionHistory) bool {
	maxRetries := GetMaxFinalizationRetries(view.Bounds)
	retries := logs.CountFinalizationRetriesSinceStateEnter(history, "cognitive")
	return retries >= maxRetries
}

func stateContractsForState(chart defs.StatechartDefinition, name string) (defs.StateInputContract, defs.StateOutputContract, defs.StateCompletionContract, defs.StateBoundsContract) {
	for _, state := range chart.States {
		if state.Name != name {
			continue
		}
		return defs.StateInputContract{
				Required: append([]string(nil), state.Inputs.Required...),
				Optional: append([]string(nil), state.Inputs.Optional...),
			}, defs.StateOutputContract{
				SchemaName:     state.Outputs.SchemaName,
				RequiredFields: append([]string(nil), state.Outputs.RequiredFields...),
				OptionalFields: append([]string(nil), state.Outputs.OptionalFields...),
				Strict:         state.Outputs.Strict,
			}, defs.StateCompletionContract{
				SuccessWhen: append([]string(nil), state.Completion.SuccessWhen...),
			}, defs.StateBoundsContract{
				MaxInferenceTurns:      state.Bounds.MaxInferenceTurns,
				MaxToolCalls:           state.Bounds.MaxToolCalls,
				MaxWallTimeSeconds:     state.Bounds.MaxWallTimeSeconds,
				MaxFinalizationRetries: state.Bounds.MaxFinalizationRetries,
			}
	}
	return defs.StateInputContract{}, defs.StateOutputContract{}, defs.StateCompletionContract{}, defs.StateBoundsContract{}
}
