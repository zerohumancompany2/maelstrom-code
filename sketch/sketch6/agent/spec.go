package agent

type Spec struct {
	ID        string
	Version   string
	Model     ModelSpec
	Context   ContextSpec
	ToolNames []string
}

type ModelSpec struct {
	Provider     string
	Name         string
	Temperature  float64
	ContextLimit int
}

type ContextSpec struct {
	MaxHistoryItems int
}
