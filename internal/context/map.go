package context

import (
	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/session"
)

type ContextMap struct {
	Model      string // placeholder until we get proper definitions
	Tools      []internal.ToolDefinition
	Definition ContextDefinition
}

func (c *ContextMap) BuildInferenceBundle(s session.Session) (InferenceBundle, error) {
	var messages []internal.SessionItem

	for _, chunk := range c.Definition.Chunks {
		builtChunk, err := chunk.Build(s)
		if err != nil {
			return InferenceBundle{}, err
		}
		messages = append(messages, builtChunk...)
	}

	return InferenceBundle{
		Model:    c.Model,
		Messages: messages,
		Tools:    c.Tools,
	}, nil
}
