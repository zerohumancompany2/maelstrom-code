package runtime

import "github.com/comalice/maelstrom/internal/defs"

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
	CurrentState    string
	VisibleTools    []string
	EnabledTools    []string
	AllowedTriggers []string
	Prompt          string
	Inputs          defs.StateInputContract
	Outputs         defs.StateOutputContract
	Completion      defs.StateCompletionContract
	Bounds          defs.StateBoundsContract
}

type WorkflowView struct {
	WorkflowID      string
	CurrentState    string
	VisibleTools    []string
	EnabledTools    []string
	AllowedTriggers []string
	Description     string
	Context         string
	LastBoundAgent  string
	Inputs          defs.StateInputContract
	Outputs         defs.StateOutputContract
	Completion      defs.StateCompletionContract
	Bounds          defs.StateBoundsContract
	Artifacts       []WorkflowArtifactView
}

// WorkflowArtifactView is the reduced projection of a WorkflowArtifactRecord:
// the validated workflow-bucket output a prior agent finalized for a state.
type WorkflowArtifactView struct {
	StateName  string
	SchemaName string
	ByAgent    string
	Content    string
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

type EffectiveToolPolicy struct {
	Applied bool
	Tools   []string
}
