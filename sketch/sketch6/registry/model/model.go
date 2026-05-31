package modelregistry

import (
	"fmt"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch6/model"
	"github.com/comalice/inference_sketch/sketch/sketch6/registry"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Document struct {
	APIVersion   string          `yaml:"apiVersion"`
	Kind         string          `yaml:"kind"`
	Name         string          `yaml:"name"`
	Providers    []ProviderDoc   `yaml:"providers"`
	Limits       LimitsDoc       `yaml:"limits"`
	Defaults     DefaultsDoc     `yaml:"defaults"`
	Capabilities CapabilitiesDoc `yaml:"capabilities"`
}

type ProviderDoc struct {
	Name     string `yaml:"name"`
	ModelRef string `yaml:"modelRef"`
}

type LimitsDoc struct {
	ContextWindow   int `yaml:"contextWindow"`
	MaxOutputTokens int `yaml:"maxOutputTokens"`
}

type DefaultsDoc struct {
	Temperature float64 `yaml:"temperature"`
	TopP        float64 `yaml:"topP"`
}

type CapabilitiesDoc struct {
	Tools      bool `yaml:"tools"`
	Reasoning  bool `yaml:"reasoning"`
	Multimodal bool `yaml:"multimodal"`
	Streaming  bool `yaml:"streaming"`
}

type Decoder struct{}

func (Decoder) Decode(data []byte) (Document, error) {
	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

type Hoister struct{}

func (Hoister) Hoist(doc Document) (model.Definition, error) {
	if strings.TrimSpace(doc.APIVersion) == "" {
		return model.Definition{}, fmt.Errorf("model apiVersion required")
	}
	if doc.Kind != "Model" {
		return model.Definition{}, fmt.Errorf("model kind must be Model")
	}
	if strings.TrimSpace(doc.Name) == "" {
		return model.Definition{}, fmt.Errorf("model name required")
	}
	if len(doc.Providers) == 0 {
		return model.Definition{}, fmt.Errorf("model providers required")
	}
	providers := make([]model.ProviderRef, 0, len(doc.Providers))
	for _, provider := range doc.Providers {
		name := strings.TrimSpace(provider.Name)
		modelRef := strings.TrimSpace(provider.ModelRef)
		if name == "" || modelRef == "" {
			return model.Definition{}, fmt.Errorf("model providers require name and modelRef")
		}
		providers = append(providers, model.ProviderRef{Name: name, ModelRef: modelRef})
	}
	return model.Definition{
		Name:      strings.TrimSpace(doc.Name),
		Providers: providers,
		Limits: model.Limits{
			ContextWindow:   doc.Limits.ContextWindow,
			MaxOutputTokens: doc.Limits.MaxOutputTokens,
		},
		Defaults: model.Defaults{
			Temperature: doc.Defaults.Temperature,
			TopP:        doc.Defaults.TopP,
		},
		Capabilities: model.Capabilities{
			Tools:      doc.Capabilities.Tools,
			Reasoning:  doc.Capabilities.Reasoning,
			Multimodal: doc.Capabilities.Multimodal,
			Streaming:  doc.Capabilities.Streaming,
		},
	}, nil
}

func NewIngestor(store registry.Store[model.Definition], reg registry.Registry[model.Definition]) registry.Ingestor[Document, model.Definition] {
	return registry.Ingestor[Document, model.Definition]{
		Decoder:  Decoder{},
		Hoister:  Hoister{},
		Store:    store,
		Registry: reg,
		Hooks:    []registry.Preprocessor{registry.IdentityHook{}},
		NewID: func(key string) string {
			return uuid.NewString()
		},
	}
}
