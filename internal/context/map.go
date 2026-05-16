package context

import (
	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/session"
)

type ContextMap struct {
	Model string // placeholder until we get proper definitions
	Tools []internal.ToolDefinition
}

func (c *ContextMap) BuildInferenceBundle(s session.Session) (InferenceBundle, error) {
	chunks := []internal.SessionItem{} // lift to ContextChunk

	for _, chunk := range s.Items {
		chunks = append(chunks, chunk)
	}

	return InferenceBundle{
		Model:    c.Model,
		Messages: chunks,
		Tools:    c.Tools,
	}, nil
}

func NewFromDefinition(cd ContextDefinition) ContextMap {
	return ContextMap{Model: cd.Model, Tools: cd.Tools}
}
