package defs

type AgentDefinition struct {
	Name        string
	Description string
	Model       string
	Overrides   AgentOverrides
	Tools       []string
	Context     ContextDefinition
	Cognitive   StatechartDefinition
}

type AgentOverrides struct {
	Temperature     *float64
	TopP            *float64
	MaxOutputTokens *int
}

type ContextDefinition struct {
	InputBudget int
	Projections []ProjectionDefinition
}

type ProjectionDefinition struct {
	Type               string
	Prompt             string
	Name               string
	Chart              string
	RefreshEveryNTurns *int
	RetentionMode      string
}
