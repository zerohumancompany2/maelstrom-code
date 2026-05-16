package context

import "github.com/comalice/inference_sketch/internal"

type InferenceBundle struct {
	Model    string
	Messages []internal.SessionItem
	Tools    []internal.ToolDefinition
}
