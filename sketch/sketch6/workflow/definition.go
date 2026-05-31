package workflow

type Definition struct {
	Name        string
	Description string
	Context     string
	Statechart  StatechartDefinition
}

type StatechartDefinition struct {
	InitialState string
	States       []StateDefinition
	Transitions  []TransitionDefinition
}

type StateDefinition struct {
	Name            string
	Description     string
	VisibleTools    []string
	EnabledTools    []string
	AllowedTriggers []string
}

type TransitionDefinition struct {
	Trigger string
	From    string
	To      string
}

type Instance struct {
	ID     string
	SpecID string
	Input  string
}
