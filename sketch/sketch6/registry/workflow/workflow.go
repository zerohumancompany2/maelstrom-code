package workflowregistry

import (
	"fmt"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch6/registry"
	coreworkflow "github.com/comalice/inference_sketch/sketch/sketch6/workflow"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Document struct {
	APIVersion string             `yaml:"apiVersion"`
	Kind       string             `yaml:"kind"`
	Name       string             `yaml:"name"`
	Description string            `yaml:"description"`
	Context    string             `yaml:"context"`
	Statechart StatechartDocument `yaml:"statechart"`
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

func (Hoister) Hoist(doc Document) (coreworkflow.Definition, error) {
	if strings.TrimSpace(doc.APIVersion) == "" {
		return coreworkflow.Definition{}, fmt.Errorf("workflow apiVersion required")
	}
	if doc.Kind != "Workflow" {
		return coreworkflow.Definition{}, fmt.Errorf("workflow kind must be Workflow")
	}
	if strings.TrimSpace(doc.Name) == "" {
		return coreworkflow.Definition{}, fmt.Errorf("workflow name required")
	}
	return coreworkflow.Definition{
		Name:        strings.TrimSpace(doc.Name),
		Description: strings.TrimSpace(doc.Description),
		Context:     strings.TrimSpace(doc.Context),
		Statechart: coreworkflow.StatechartDefinition{
			InitialState: strings.TrimSpace(doc.Statechart.InitialState),
			States:       hoistStates(doc.Statechart.States),
			Transitions:  hoistTransitions(doc.Statechart.Transitions),
		},
	}, nil
}

func hoistStates(states []StateDocument) []coreworkflow.StateDefinition {
	result := make([]coreworkflow.StateDefinition, 0, len(states))
	for _, state := range states {
		result = append(result, coreworkflow.StateDefinition{
			Name:            strings.TrimSpace(state.Name),
			Description:     strings.TrimSpace(state.Description),
			VisibleTools:    append([]string(nil), state.VisibleTools...),
			EnabledTools:    append([]string(nil), state.EnabledTools...),
			AllowedTriggers: append([]string(nil), state.AllowedTriggers...),
		})
	}
	return result
}

func hoistTransitions(transitions []TransitionDocument) []coreworkflow.TransitionDefinition {
	result := make([]coreworkflow.TransitionDefinition, 0, len(transitions))
	for _, transition := range transitions {
		result = append(result, coreworkflow.TransitionDefinition{
			Trigger: strings.TrimSpace(transition.Trigger),
			From:    strings.TrimSpace(transition.From),
			To:      strings.TrimSpace(transition.To),
		})
	}
	return result
}

func NewIngestor(store registry.Store[coreworkflow.Definition], reg registry.Registry[coreworkflow.Definition]) registry.Ingestor[Document, coreworkflow.Definition] {
	return registry.Ingestor[Document, coreworkflow.Definition]{
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
