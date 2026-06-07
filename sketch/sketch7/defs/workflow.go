package defs

type WorkflowDefinition struct {
	Name        string
	Description string
	Context     string
	Statechart  StatechartDefinition
}

type WorkflowInstance struct {
	ID     string
	SpecID string
	Input  string
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
	Prompt          string
	AllowedTriggers []string
	Inputs          StateInputContract
	Outputs         StateOutputContract
	Completion      StateCompletionContract
}

type StateInputContract struct {
	Required []string
	Optional []string
}

type StateOutputContract struct {
	SchemaName     string
	RequiredFields []string
	Strict         bool
}

type StateCompletionContract struct {
	SuccessWhen []string
}

type TransitionDefinition struct {
	Trigger string
	From    string
	To      string
}
