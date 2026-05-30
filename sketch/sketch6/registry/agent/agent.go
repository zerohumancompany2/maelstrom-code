package agentregistry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	coreagent "github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/registry"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Document struct {
	APIVersion   string   `yaml:"apiVersion"`
	Kind         string   `yaml:"kind"`
	Name         string   `yaml:"name"`
	Version      string   `yaml:"version"`
	Provider     string   `yaml:"provider"`
	Model        string   `yaml:"model"`
	MaxHistory   int      `yaml:"max_history"`
	ToolNames    []string `yaml:"tools"`
	Temperature  float64  `yaml:"temperature"`
	ContextLimit int      `yaml:"context_limit"`
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

func (Hoister) Hoist(doc Document) (coreagent.Spec, error) {
	if strings.TrimSpace(doc.APIVersion) == "" {
		return coreagent.Spec{}, fmt.Errorf("agent apiVersion required")
	}
	if doc.Kind != "Agent" {
		return coreagent.Spec{}, fmt.Errorf("agent kind must be Agent")
	}
	if strings.TrimSpace(doc.Name) == "" {
		return coreagent.Spec{}, fmt.Errorf("agent name required")
	}
	if strings.TrimSpace(doc.Model) == "" {
		return coreagent.Spec{}, fmt.Errorf("agent model required")
	}
	return coreagent.Spec{
		ID:        doc.Name,
		Version:   doc.Version,
		Model:     coreagent.ModelSpec{Provider: doc.Provider, Name: doc.Model, Temperature: doc.Temperature, ContextLimit: doc.ContextLimit},
		Context:   coreagent.ContextSpec{MaxHistoryItems: doc.MaxHistory},
		ToolNames: append([]string(nil), doc.ToolNames...),
	}, nil
}

func NewIngestor(store registry.Store[coreagent.Spec], reg registry.Registry[coreagent.Spec]) registry.Ingestor[Document, coreagent.Spec] {
	return registry.Ingestor[Document, coreagent.Spec]{
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

type FileIngestor struct {
	Ingestor registry.Ingestor[Document, coreagent.Spec]
}

func (f FileIngestor) IngestFile(ctx context.Context, path string) error {
	if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	key := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	_, err = f.Ingestor.Ingest(ctx, key, path, data)
	return err
}
