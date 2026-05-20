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

func (c *ContextMap) BuildMessages(s session.Session) ([]internal.SessionItem, error) {
	var messages []internal.SessionItem

	for _, chunk := range c.Definition.Chunks {
		builtChunk, err := chunk.Build(s)
		if err != nil {
			return nil, err
		}
		messages = append(messages, builtChunk...)
	}

	return messages, nil
}
