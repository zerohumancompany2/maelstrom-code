package context

import (
	"github.com/comalice/inference_sketch/internal"
)

type ContextDefinition struct {
	Model string
	Tools []internal.ToolDefinition
}
