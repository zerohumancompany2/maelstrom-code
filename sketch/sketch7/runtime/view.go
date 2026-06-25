package runtime

import "github.com/comalice/inference_sketch/sketch/sketch7/defs"

type Agent struct {
	Name         string
	Description  string
	LogicalModel string

	ProviderName string
	ProviderRef  string

	Inference    InferenceSettings
	Limits       defs.ModelLimits
	Capabilities defs.ModelCapabilities

	ToolNames []string
}

type InferenceSettings struct {
	Temperature     float64
	TopP            float64
	MaxOutputTokens int
}

type CognitiveView struct {
	CurrentState string
	VisibleTools []string
	EnabledTools []string
	Prompt       string
	Inputs       defs.StateInputContract
	Outputs      defs.StateOutputContract
	Completion   defs.StateCompletionContract
	Bounds       defs.StateBoundsContract
}

type WorkflowView struct {
	WorkflowID     string
	CurrentState   string
	VisibleTools   []string
	EnabledTools   []string
	Description    string
	Context        string
	LastBoundAgent string
	Inputs         defs.StateInputContract
	Outputs        defs.StateOutputContract
	Completion     defs.StateCompletionContract
	Bounds         defs.StateBoundsContract
}

type BindingView struct {
	WorkflowID string
	Bound      bool
}

type InteractionMode string

const (
	InteractionInteractive InteractionMode = "interactive"
	InteractionRunning     InteractionMode = "running"
	InteractionInterrupted InteractionMode = "interrupted"
	InteractionResumable   InteractionMode = "resumable"
)

type InteractionView struct {
	Mode   InteractionMode
	Reason string
}

type SessionView struct {
	Agent       Agent
	Cognitive   CognitiveView
	Binding     BindingView
	Workflow    *WorkflowView
	Interaction InteractionView
}
