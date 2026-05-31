package agent

type Definition struct {
	Name        string
	Description string
	Model       string
	Overrides   Overrides
	Tools       []string
	Context     ContextDefinition
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
	Flexible  bool
	Policy    string
	Priority  int
	BudgetPct float64
}
