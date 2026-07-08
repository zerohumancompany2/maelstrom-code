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
	// Reason records which bound(s) forced finalization, e.g.
	// "cognitive_max_tool_calls" or
	// "cognitive_max_inference_turns+workflow_max_tool_calls".
	Reason       string
	RetryAttempt int
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
	view.AllowedTriggers = allowedTriggersForState(def.Statechart, view.CurrentState)
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

func allowedTriggersForState(chart defs.StatechartDefinition, name string) []string {
	seen := map[string]bool{}
	triggers := []string{}
	for _, state := range chart.States {
		if state.Name != name {
			continue
		}
		for _, trigger := range state.AllowedTriggers {
			trigger = strings.TrimSpace(trigger)
			if trigger != "" && !seen[trigger] {
				seen[trigger] = true
				triggers = append(triggers, trigger)
			}
		}
		break
	}
	for _, transition := range chart.Transitions {
		if transition.From != name {
			continue
		}
		trigger := strings.TrimSpace(transition.Trigger)
		if trigger != "" && !seen[trigger] {
			seen[trigger] = true
			triggers = append(triggers, trigger)
		}
	}
	return triggers
}

// CognitiveBoundHitReason returns the exhausted cognitive budget
// ("max_inference_turns" or "max_tool_calls"), or "" when no bound is hit.
// A non-empty reason is only reported when cognitive outputs are declared,
// since finalization needs a contract to finalize against; exhausting a
// budget then forces finalization instead of failing the session.
func CognitiveBoundHitReason(view CognitiveView, history *logs.SessionHistory) string {
	return boundHitReason(view.Outputs, view.Bounds, history, "cognitive")
}

// WorkflowBoundHitReason mirrors CognitiveBoundHitReason for the bound
// workflow state, counting budgets since the latest workflow state enter.
func WorkflowBoundHitReason(view WorkflowView, history *logs.SessionHistory) string {
	return boundHitReason(view.Outputs, view.Bounds, history, "workflow")
}

func boundHitReason(outputs defs.StateOutputContract, bounds defs.StateBoundsContract, history *logs.SessionHistory, chart string) string {
	// Outputs must be declared for finalization mode to apply.
	if strings.TrimSpace(outputs.SchemaName) == "" && len(outputs.RequiredFields) == 0 {
		return ""
	}
	if bounds.MaxInferenceTurns > 0 && logs.CountInferenceTurnsSinceStateEnter(history, chart) >= bounds.MaxInferenceTurns {
		return "max_inference_turns"
	}
	if bounds.MaxToolCalls > 0 && logs.CountToolCallsSinceStateEnter(history, chart) >= bounds.MaxToolCalls {
		return "max_tool_calls"
	}
	return ""
}

// IsCognitiveBoundHit checks if the cognitive state's inference-turn or
// tool-call budget has been exhausted.
func IsCognitiveBoundHit(view CognitiveView, history *logs.SessionHistory) bool {
	return CognitiveBoundHitReason(view, history) != ""
}

// ShouldFinalizeCognitive checks if we should enter cognitive finalization mode.
func ShouldFinalizeCognitive(view CognitiveView, history *logs.SessionHistory) bool {
	return IsCognitiveBoundHit(view, history)
}

// ShouldFinalizeWorkflow checks if we should enter workflow finalization mode
// for the bound workflow state.
func ShouldFinalizeWorkflow(view WorkflowView, history *logs.SessionHistory) bool {
	return WorkflowBoundHitReason(view, history) != ""
}

func ResolveFinalizationMode(cognitive CognitiveView, workflow *WorkflowView, history *logs.SessionHistory) FinalizationMode {
	mode := FinalizationMode{}
	reasons := []string{}
	if reason := CognitiveBoundHitReason(cognitive, history); reason != "" {
		mode.IsFinalizing = true
		mode.RequireCognitive = true
		reasons = append(reasons, "cognitive_"+reason)
	}
	if workflow != nil {
		if reason := WorkflowBoundHitReason(*workflow, history); reason != "" {
			mode.IsFinalizing = true
			mode.RequireWorkflow = true
			reasons = append(reasons, "workflow_"+reason)
		}
	}
	if !mode.IsFinalizing {
		return mode
	}
	mode.Reason = strings.Join(reasons, "+")
	mode.RetryAttempt = FinalizationRetryCountForMode(mode, history) + 1
	return mode
}

// FinalizationRetryCountForMode returns the retry count relevant to the active
// mode. Combined finalization re-requests all required buckets together, so it
// uses the maximum count visible in any participating chart window instead of
// implicitly charging only the cognitive chart.
func FinalizationRetryCountForMode(mode FinalizationMode, history *logs.SessionHistory) int {
	count := 0
	if mode.RequireCognitive {
		count = logs.CountFinalizationRetriesSinceStateEnter(history, "cognitive")
	}
	if mode.RequireWorkflow {
		if workflow := logs.CountFinalizationRetriesSinceStateEnter(history, "workflow"); workflow > count {
			count = workflow
		}
	}
	return count
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

// MaxFinalizationRetriesForMode returns the effective retry budget for the
// active finalization mode. Combined finalization retries re-request every
// required bucket, so the budget is the larger of the participating charts'
// budgets.
func MaxFinalizationRetriesForMode(mode FinalizationMode, cognitive CognitiveView, workflow *WorkflowView) int {
	budget := 0
	if mode.RequireCognitive {
		budget = GetMaxFinalizationRetries(cognitive.Bounds)
	}
	if mode.RequireWorkflow && workflow != nil {
		if wf := GetMaxFinalizationRetries(workflow.Bounds); wf > budget {
			budget = wf
		}
	}
	if budget <= 0 {
		budget = 1
	}
	return budget
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
