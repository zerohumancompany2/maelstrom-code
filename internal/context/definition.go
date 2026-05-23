package context

import (
	"github.com/comalice/inference_sketch/internal"
)

type ContextDefinition struct {
	Model        string
	Tools        []internal.ToolDefinition
	Chunks       []ChunkSpec
	ContextLimit int
}

// NewFromDefinition copies top-level configuration slices into a new ContextMap.
//
// Chunk and policy implementations are expected to be immutable runtime configuration.
// They are reloaded from source definitions rather than mutated in place.
func NewFromDefinition(cd ContextDefinition) ContextMap {
	tools := append([]internal.ToolDefinition(nil), cd.Tools...)
	chunks := append([]ChunkSpec(nil), cd.Chunks...)
	return ContextMap{
		Model:        cd.Model,
		Tools:        tools,
		Definition:   ContextDefinition{Model: cd.Model, Tools: tools, Chunks: chunks, ContextLimit: cd.ContextLimit},
		ContextLimit: cd.ContextLimit,
	}
}
