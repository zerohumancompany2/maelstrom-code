package context

import (
	"github.com/comalice/inference_sketch/internal"
)

type ContextDefinition struct {
	Model  string
	Tools  []internal.ToolDefinition
	Chunks []ContextChunk // TODO make map[string]chunk?
}

func NewFromDefinition(cd ContextDefinition) ContextMap {
	return ContextMap{Model: cd.Model, Tools: cd.Tools, Definition: cd}
}
