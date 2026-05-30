package definitions

type AgentDefinition struct {
	ID          string              `yaml:"id,omitempty"`
	Name        string              `yaml:"name"`
	Description string              `yaml:"description,omitempty"`
	Version     string              `yaml:"version,omitempty"`
	Model       string              `yaml:"model"`
	Overrides   AgentModelOverrides `yaml:"overrides,omitempty"`
	Tools       []string            `yaml:"tools,omitempty"`
	Context     ContextDefinition   `yaml:"context"`
	Cognitive   CognitiveDefinition `yaml:"cognitive,omitempty"`
}

type AgentModelOverrides struct {
	Temperature float32 `yaml:"temperature,omitempty"`
	TopP        float32 `yaml:"topP,omitempty"`
	MaxTokens   int     `yaml:"maxTokens,omitempty"`
}

type ContextDefinition struct {
	InputBudget int               `yaml:"inputBudget,omitempty"`
	Chunks      []ChunkDefinition `yaml:"chunks,omitempty"`
}

type ChunkDefinition struct {
	Type      string  `yaml:"type"`
	Prompt    string  `yaml:"prompt,omitempty"`
	BudgetPct float64 `yaml:"budgetPct,omitempty"`
	Priority  int     `yaml:"priority,omitempty"`
	Flexible  bool    `yaml:"flexible,omitempty"`
	Policy    string  `yaml:"policy,omitempty"`
}

type CognitiveDefinition struct {
	InitialState string                     `yaml:"initialState,omitempty"`
	States       []CognitiveStateDefinition `yaml:"states,omitempty"`
}

type CognitiveStateDefinition struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}
