package defs

type ModelDefinition struct {
	Name         string
	Providers    []ProviderRef
	Limits       ModelLimits
	Defaults     ModelDefaults
	Capabilities ModelCapabilities
}

type ProviderRef struct {
	Name     string
	ModelRef string
}

type ModelLimits struct {
	ContextWindow   int
	MaxOutputTokens int
}

type ModelDefaults struct {
	Temperature float64
	TopP        float64
}

type ModelCapabilities struct {
	Tools      bool
	Reasoning  bool
	Multimodal bool
	Streaming  bool
}
