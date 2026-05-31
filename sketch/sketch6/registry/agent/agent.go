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
	APIVersion  string             `yaml:"apiVersion"`
	Kind        string             `yaml:"kind"`
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Model       string             `yaml:"model"`
	Overrides   OverridesDoc       `yaml:"overrides"`
	ToolNames   []string           `yaml:"tools"`
	Context     ContextDoc         `yaml:"context"`
	Cognitive   StatechartDocument `yaml:"cognitive"`
}

type OverridesDoc struct {
	Temperature     *float64 `yaml:"temperature"`
	TopP            *float64 `yaml:"topP"`
	MaxOutputTokens *int     `yaml:"maxOutputTokens"`
}

type ContextDoc struct {
	InputBudget int        `yaml:"inputBudget"`
	Chunks      []ChunkDoc `yaml:"chunks"`
}

type ChunkDoc struct {
	Type      string  `yaml:"type"`
	Prompt    string  `yaml:"prompt"`
	Chart     string  `yaml:"chart"`
	Source    string  `yaml:"source"`
	Flexible  bool    `yaml:"flexible"`
	Policy    string  `yaml:"policy"`
	Priority  int     `yaml:"priority"`
	BudgetPct float64 `yaml:"budgetPct"`
}

type StatechartDocument struct {
	InitialState string               `yaml:"initialState"`
	States       []StateDocument      `yaml:"states"`
	Transitions  []TransitionDocument `yaml:"transitions"`
}

type StateDocument struct {
	Name            string   `yaml:"name"`
	Description     string   `yaml:"description"`
	VisibleTools    []string `yaml:"visibleTools"`
	EnabledTools    []string `yaml:"enabledTools"`
	Prompt          string   `yaml:"prompt"`
	AllowedTriggers []string `yaml:"allowedTriggers"`
}

type TransitionDocument struct {
	Trigger string `yaml:"trigger"`
	From    string `yaml:"from"`
	To      string `yaml:"to"`
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

func (Hoister) Hoist(doc Document) (coreagent.Definition, error) {
	if strings.TrimSpace(doc.APIVersion) == "" {
		return coreagent.Definition{}, fmt.Errorf("agent apiVersion required")
	}
	if doc.Kind != "Agent" {
		return coreagent.Definition{}, fmt.Errorf("agent kind must be Agent")
	}
	if strings.TrimSpace(doc.Name) == "" {
		return coreagent.Definition{}, fmt.Errorf("agent name required")
	}
	if strings.TrimSpace(doc.Model) == "" {
		return coreagent.Definition{}, fmt.Errorf("agent model required")
	}
	return coreagent.Definition{
		Name:        strings.TrimSpace(doc.Name),
		Description: strings.TrimSpace(doc.Description),
		Model:       strings.TrimSpace(doc.Model),
		Overrides: coreagent.Overrides{
			Temperature:     doc.Overrides.Temperature,
			TopP:            doc.Overrides.TopP,
			MaxOutputTokens: doc.Overrides.MaxOutputTokens,
		},
		Tools: append([]string(nil), doc.ToolNames...),
		Context: coreagent.ContextDefinition{
			InputBudget: doc.Context.InputBudget,
			Chunks:      hoistChunks(doc.Context.Chunks),
		},
		Cognitive: coreagent.StatechartDefinition{
			InitialState: strings.TrimSpace(doc.Cognitive.InitialState),
			States:       hoistStates(doc.Cognitive.States),
			Transitions:  hoistTransitions(doc.Cognitive.Transitions),
		},
	}, nil
}

func hoistChunks(chunks []ChunkDoc) []coreagent.ChunkDefinition {
	result := make([]coreagent.ChunkDefinition, 0, len(chunks))
	for _, chunk := range chunks {
		result = append(result, coreagent.ChunkDefinition{
			Type:      strings.TrimSpace(chunk.Type),
			Prompt:    chunk.Prompt,
			Chart:     strings.TrimSpace(chunk.Chart),
			Source:    strings.TrimSpace(chunk.Source),
			Flexible:  chunk.Flexible,
			Policy:    strings.TrimSpace(chunk.Policy),
			Priority:  chunk.Priority,
			BudgetPct: chunk.BudgetPct,
		})
	}
	return result
}

func hoistStates(states []StateDocument) []coreagent.StateDefinition {
	result := make([]coreagent.StateDefinition, 0, len(states))
	for _, state := range states {
		result = append(result, coreagent.StateDefinition{
			Name:            strings.TrimSpace(state.Name),
			Description:     strings.TrimSpace(state.Description),
			VisibleTools:    append([]string(nil), state.VisibleTools...),
			EnabledTools:    append([]string(nil), state.EnabledTools...),
			Prompt:          state.Prompt,
			AllowedTriggers: append([]string(nil), state.AllowedTriggers...),
		})
	}
	return result
}

func hoistTransitions(transitions []TransitionDocument) []coreagent.TransitionDefinition {
	result := make([]coreagent.TransitionDefinition, 0, len(transitions))
	for _, transition := range transitions {
		result = append(result, coreagent.TransitionDefinition{
			Trigger: strings.TrimSpace(transition.Trigger),
			From:    strings.TrimSpace(transition.From),
			To:      strings.TrimSpace(transition.To),
		})
	}
	return result
}

func NewIngestor(store registry.Store[coreagent.Definition], reg registry.Registry[coreagent.Definition]) registry.Ingestor[Document, coreagent.Definition] {
	return registry.Ingestor[Document, coreagent.Definition]{
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
	Ingestor registry.Ingestor[Document, coreagent.Definition]
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
