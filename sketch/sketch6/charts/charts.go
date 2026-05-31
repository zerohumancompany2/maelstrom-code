package charts

import (
	"context"
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
	"github.com/qmuntal/stateless"
)

type Snapshot struct {
	States map[string]string
}

func BuildSnapshot(history *session.History) Snapshot {
	states := map[string]string{
		"agent":    "idle",
		"workflow": "idle",
	}
	for _, record := range history.Records {
		transition, ok := record.(session.StateTransitionRecord)
		if !ok {
			continue
		}
		states[transition.ChartName] = transition.ToState
	}
	return Snapshot{States: states}
}

func BuildWorkflowSnapshot(def workflow.Definition, history *workflow.History) Snapshot {
	initial := def.Statechart.InitialState
	if initial == "" {
		initial = "idle"
	}
	states := map[string]string{"workflow": initial}
	for _, record := range history.Records {
		transition, ok := record.(workflow.StateTransitionRecord)
		if !ok {
			continue
		}
		states["workflow"] = transition.ToState
	}
	return Snapshot{States: states}
}

func (s Snapshot) State(chartName string) string {
	if state, ok := s.States[chartName]; ok {
		return state
	}
	return "unknown"
}

type TransitionEngine interface {
	Fire(chartName, currentState, trigger string) (string, error)
}

type Set struct {
	charts map[string]*Definition
}

func NewSet(agentDef agent.Definition, workflowDef workflow.Definition) Set {
	return Set{charts: map[string]*Definition{
		"agent":    FromAgentStatechart(agentDef.Cognitive),
		"workflow": FromWorkflowStatechart(workflowDef.Statechart),
	}}
}

func (s Set) Fire(chartName, currentState, trigger string) (string, error) {
	chart, ok := s.charts[chartName]
	if !ok {
		return "", fmt.Errorf("unknown chart %q", chartName)
	}
	return chart.Fire(currentState, trigger)
}

type Definition struct {
	name         string
	initialState string
	transitions  map[string]map[string]string
}

func (d *Definition) Fire(currentState, trigger string) (string, error) {
	machine := stateless.NewStateMachine(currentState)
	for from, triggers := range d.transitions {
		config := machine.Configure(from)
		for trig, to := range triggers {
			config.Permit(stateless.Trigger(trig), to)
		}
	}
	if err := machine.Fire(stateless.Trigger(trigger)); err != nil {
		return "", err
	}
	state, err := machine.State(context.Background())
	if err != nil {
		return "", err
	}
	result, ok := state.(string)
	if !ok {
		return "", fmt.Errorf("chart %s produced non-string state %T", d.name, state)
	}
	return result, nil
}

func FromAgentStatechart(def agent.StatechartDefinition) *Definition {
	transitions := map[string]map[string]string{}
	for _, transition := range def.Transitions {
		if _, ok := transitions[transition.From]; !ok {
			transitions[transition.From] = map[string]string{}
		}
		transitions[transition.From][transition.Trigger] = transition.To
	}
	return &Definition{name: "agent", initialState: def.InitialState, transitions: transitions}
}

func FromWorkflowStatechart(def workflow.StatechartDefinition) *Definition {
	transitions := map[string]map[string]string{}
	for _, transition := range def.Transitions {
		if _, ok := transitions[transition.From]; !ok {
			transitions[transition.From] = map[string]string{}
		}
		transitions[transition.From][transition.Trigger] = transition.To
	}
	return &Definition{name: "workflow", initialState: def.InitialState, transitions: transitions}
}
