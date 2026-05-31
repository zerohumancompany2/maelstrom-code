package model

type Definition struct {
	Name         string
	Providers    []ProviderRef
	Limits       Limits
	Defaults     Defaults
	Capabilities Capabilities
}

type ProviderRef struct {
	Name     string
	ModelRef string
}

type Limits struct {
	ContextWindow   int
	MaxOutputTokens int
}

type Defaults struct {
	Temperature float64
	TopP        float64
}

type Capabilities struct {
	Tools      bool
	Reasoning  bool
	Multimodal bool
	Streaming  bool
}
