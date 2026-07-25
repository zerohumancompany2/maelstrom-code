package statecharts

import (
	"fmt"

	"github.com/comalice/maelstrom/internal/defs"
)

type Machine struct {
	name        string
	initial     string
	transitions map[string]map[string]string
}

func Compile(name string, def defs.StatechartDefinition) Machine {
	transitions := map[string]map[string]string{}
	for _, transition := range def.Transitions {
		if _, ok := transitions[transition.From]; !ok {
			transitions[transition.From] = map[string]string{}
		}
		transitions[transition.From][transition.Trigger] = transition.To
	}
	return Machine{name: name, initial: def.InitialState, transitions: transitions}
}

func (m Machine) InitialState() string {
	if m.initial == "" {
		return "idle"
	}
	return m.initial
}

func (m Machine) Next(currentState, trigger string) (string, error) {
	from := currentState
	if from == "" {
		from = m.InitialState()
	}
	triggers, ok := m.transitions[from]
	if !ok {
		return "", fmt.Errorf("chart %s has no transitions from state %q", m.name, from)
	}
	next, ok := triggers[trigger]
	if !ok {
		return "", fmt.Errorf("chart %s cannot fire trigger %q from state %q", m.name, trigger, from)
	}
	return next, nil
}
