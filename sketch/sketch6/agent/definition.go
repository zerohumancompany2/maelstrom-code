package agent

type Definition struct {
	Name        string
	Description string
	Model       string
	Overrides   Overrides
	Tools       []string
	Context     ContextDefinition
	Cognitive   StatechartDefinition
}

type Overrides struct {
	Temperature     *float64
	TopP            *float64
	MaxOutputTokens *int
}

type ContextDefinition struct {
	InputBudget int
	Chunks      []ChunkDefinition
}

type ChunkDefinition struct {
	Type      string
	Prompt    string
	Chart     string
	Source    string
	Flexible  bool
	Policy    string
	Priority  int
	BudgetPct float64
}

type StatechartDefinition struct {
	InitialState string
	States       []StateDefinition
	Transitions  []TransitionDefinition
}

type StateDefinition struct {
	Name             string
	Description      string
	VisibleTools     []string
	EnabledTools     []string
	Prompt           string
	AllowedTriggers  []string
}

type TransitionDefinition struct {
	Trigger string
	From    string
	To      string
}
