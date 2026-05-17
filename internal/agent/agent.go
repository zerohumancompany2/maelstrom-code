package agent

import (
	"github.com/comalice/inference_sketch/internal/context"
	"github.com/comalice/inference_sketch/internal/tools"
)

// TODO move to models.go
type Model struct {
	Name          string  `yaml:"name"`
	Provider      string  `yaml:"provider"`
	ContextLength int     `yaml:"contextLength"`
	Temperature   float32 `yaml:"temperature"`
	TopP          float32 `yaml:"topP,omitempty"`
	MaxTokens     int     `yaml:"maxTokens,omitempty"`
}

// Agent - model specs, cognitive states, default tools, contexxt map
type Agent struct {
	ID           string                    // UUID assigned on ingestion
	Name         string                    `yaml:"name"`                  // plaintext model name
	Description  string                    `yaml:"description,omitempty"` // description of agent, personality, cognitive states, etc.
	Version      string                    // TODO populate from definition registry
	Model        Model                     `yaml:"model"` // Model specification, TODO make literal OR named model spec
	SystemPrompt string                    `yaml:"systemPrompt,omitempty"`
	Context      context.ContextDefinition `yaml:"-"`
}

func (a *Agent) BuildContextDefinition(r *tools.Registry) context.ContextDefinition {
	return context.ContextDefinition{
		Model: a.Model.Name,
		Tools: r.Definitions(),
		Chunks: []context.ContextChunk{
			context.SystemChunk{Prompt: a.SystemPrompt},
			context.MessagesChunk{},
		},
	}
}
