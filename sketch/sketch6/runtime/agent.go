package runtime

import (
	"github.com/comalice/inference_sketch/sketch/sketch6/model"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
)

type Agent struct {
	Name         string
	Description  string
	LogicalModel string

	ProviderName string
	ProviderRef  string

	Inference    InferenceSettings
	Limits       model.Limits
	Capabilities model.Capabilities

	ToolNames []string
}

type InferenceSettings struct {
	Temperature     float64
	TopP            float64
	MaxOutputTokens int
}

type CognitiveSnapshot struct {
	CurrentState string
	VisibleTools []string
	EnabledTools []string
	Prompt       string
}

type WorkflowSnapshot struct {
	WorkflowID     string
	CurrentState   string
	VisibleTools   []string
	EnabledTools   []string
	Description    string
	Context        string
	LastBoundAgent string
}

type BindingSnapshot struct {
	WorkflowID string
	Bound      bool
}

type RuntimeView struct {
	Agent     Agent
	Cognitive CognitiveSnapshot
	Binding   BindingSnapshot
	Workflow  *WorkflowSnapshot
}

func ReduceCognitiveState(definitionState string, states map[string]CognitiveSnapshot) CognitiveSnapshot {
	if snapshot, ok := states[definitionState]; ok {
		return snapshot
	}
	return CognitiveSnapshot{CurrentState: definitionState}
}

func ReduceCognitiveStateFromHistory(history *session.History, initialState string, states map[string]CognitiveSnapshot) CognitiveSnapshot {
	current := initialState
	if current == "" {
		current = "idle"
	}
	for i := len(history.Records) - 1; i >= 0; i-- {
		transition, ok := history.Records[i].(session.StateTransitionRecord)
		if !ok || transition.ChartName != "agent" {
			continue
		}
		current = transition.ToState
		break
	}
	return ReduceCognitiveState(current, states)
}

func ReduceWorkflowState(snapshot WorkflowSnapshot, binding BindingSnapshot) *WorkflowSnapshot {
	if !binding.Bound {
		return nil
	}
	result := snapshot
	return &result
}

func ReduceWorkflowStateFromHistory(history *workflow.History, base *WorkflowSnapshot) WorkflowSnapshot {
	if base == nil {
		return WorkflowSnapshot{}
	}
	result := *base
	for i := len(history.Records) - 1; i >= 0; i-- {
		switch v := history.Records[i].(type) {
		case workflow.StateTransitionRecord:
			result.CurrentState = v.ToState
			return result
		case workflow.BindingRefRecord:
			if result.LastBoundAgent == "" && v.Action == "bind" {
				result.LastBoundAgent = v.AgentID
			}
		}
	}
	return result
}

func LatestWorkflowBinding(history []workflow.Record) BindingSnapshot {
	for i := len(history) - 1; i >= 0; i-- {
		binding, ok := history[i].(workflow.BindingRefRecord)
		if !ok {
			continue
		}
		return BindingSnapshot{WorkflowID: binding.BindingID, Bound: binding.Action == "bind"}
	}
	return BindingSnapshot{}
}

func ReduceBindingState(history []workflow.Record) BindingSnapshot {
	for i := len(history) - 1; i >= 0; i-- {
		binding, ok := history[i].(workflow.BindingRefRecord)
		if !ok {
			continue
		}
		return BindingSnapshot{WorkflowID: binding.BindingID, Bound: binding.Action == "bind"}
	}
	return BindingSnapshot{}
}
