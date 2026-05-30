package workflowregistry

import (
	"fmt"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch6/registry"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Spec struct {
	ID    string
	Steps []string
}

type Document struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Name       string   `yaml:"name"`
	Steps      []string `yaml:"steps"`
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

func (Hoister) Hoist(doc Document) (Spec, error) {
	if strings.TrimSpace(doc.APIVersion) == "" {
		return Spec{}, fmt.Errorf("workflow apiVersion required")
	}
	if doc.Kind != "Workflow" {
		return Spec{}, fmt.Errorf("workflow kind must be Workflow")
	}
	if strings.TrimSpace(doc.Name) == "" {
		return Spec{}, fmt.Errorf("workflow name required")
	}
	return Spec{ID: doc.Name, Steps: append([]string(nil), doc.Steps...)}, nil
}

func NewIngestor(store registry.Store[Spec], reg registry.Registry[Spec]) registry.Ingestor[Document, Spec] {
	return registry.Ingestor[Document, Spec]{
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
