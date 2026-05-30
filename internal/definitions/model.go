package definitions

type ModelDefinition struct {
	Name         string             `yaml:"name"`
	Providers    []ModelProviderRef `yaml:"providers,omitempty"`
	Limits       ModelLimits        `yaml:"limits,omitempty"`
	Defaults     ModelDefaults      `yaml:"defaults,omitempty"`
	Capabilities ModelCapabilities  `yaml:"capabilities,omitempty"`
}

type ModelProviderRef struct {
	Name     string `yaml:"name"`
	ModelRef string `yaml:"modelRef"`
}

type ModelLimits struct {
	ContextWindow   int `yaml:"contextWindow,omitempty"`
	MaxOutputTokens int `yaml:"maxOutputTokens,omitempty"`
}

type ModelDefaults struct {
	Temperature float32 `yaml:"temperature,omitempty"`
	TopP        float32 `yaml:"topP,omitempty"`
	MaxTokens   int     `yaml:"maxTokens,omitempty"`
}

type ModelCapabilities struct {
	Tools      bool `yaml:"tools,omitempty"`
	Reasoning  bool `yaml:"reasoning,omitempty"`
	Multimodal bool `yaml:"multimodal,omitempty"`
	Streaming  bool `yaml:"streaming,omitempty"`
}
