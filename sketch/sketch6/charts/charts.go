package charts

import (
	"context"
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch6/session"
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

func NewSet() Set {
	return Set{charts: map[string]*Definition{
		"agent":    buildAgentChart(),
		"workflow": buildWorkflowChart(),
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
	name        string
	transitions map[string]map[string]string
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

func buildAgentChart() *Definition {
	return &Definition{name: "agent", transitions: map[string]map[string]string{
		"idle":        {"start_research": "researching"},
		"researching": {"draft_answer": "answering"},
	}}
}

func buildWorkflowChart() *Definition {
	return &Definition{name: "workflow", transitions: map[string]map[string]string{
		"idle":           {"begin_lookup": "lookup_pending"},
		"lookup_pending": {"weather_received": "data_ready"},
	}}
}
