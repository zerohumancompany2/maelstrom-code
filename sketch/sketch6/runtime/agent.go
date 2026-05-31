package runtime

import "github.com/comalice/inference_sketch/sketch/sketch6/model"

type Agent struct {
	Name         string
	Description  string
	LogicalModel string

	ProviderName string
	ProviderRef  string

	Inference    InferenceSettings
	Limits       model.Limits
	Capabilities model.Capabilities

	ToolNames       []string
	MaxHistoryItems int
}

type InferenceSettings struct {
	Temperature     float64
	TopP            float64
	MaxOutputTokens int
}
