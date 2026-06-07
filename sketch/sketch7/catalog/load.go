package catalog

import (
	"fmt"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"gopkg.in/yaml.v3"
)

type documentHeader struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

type modelDocument struct {
	APIVersion   string                  `yaml:"apiVersion"`
	Kind         string                  `yaml:"kind"`
	Name         string                  `yaml:"name"`
	Providers    []modelProviderDocument `yaml:"providers"`
	Limits       modelLimitsDocument     `yaml:"limits"`
	Defaults     modelDefaultsDocument   `yaml:"defaults"`
	Capabilities modelCapsDocument       `yaml:"capabilities"`
}

type modelProviderDocument struct {
	Name     string `yaml:"name"`
	ModelRef string `yaml:"modelRef"`
}

type modelLimitsDocument struct {
	ContextWindow   int `yaml:"contextWindow"`
	MaxOutputTokens int `yaml:"maxOutputTokens"`
}

type modelDefaultsDocument struct {
	Temperature float64 `yaml:"temperature"`
	TopP        float64 `yaml:"topP"`
}

type modelCapsDocument struct {
	Tools      bool `yaml:"tools"`
	Reasoning  bool `yaml:"reasoning"`
	Multimodal bool `yaml:"multimodal"`
	Streaming  bool `yaml:"streaming"`
}

type agentDocument struct {
	APIVersion  string                 `yaml:"apiVersion"`
	Kind        string                 `yaml:"kind"`
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Model       string                 `yaml:"model"`
	Overrides   agentOverridesDocument `yaml:"overrides"`
	Tools       []string               `yaml:"tools"`
	Context     agentContextDocument   `yaml:"context"`
	Cognitive   statechartDocument     `yaml:"cognitive"`
}

type agentOverridesDocument struct {
	Temperature     *float64 `yaml:"temperature"`
	TopP            *float64 `yaml:"topP"`
	MaxOutputTokens *int     `yaml:"maxOutputTokens"`
}

type agentContextDocument struct {
	InputBudget int                  `yaml:"inputBudget"`
	Projections []projectionDocument `yaml:"projections"`
	Chunks      []projectionDocument `yaml:"chunks"`
}

type projectionDocument struct {
	Type               string `yaml:"type"`
	Prompt             string `yaml:"prompt"`
	Name               string `yaml:"name"`
	Chart              string `yaml:"chart"`
	RefreshEveryNTurns *int   `yaml:"refreshEveryNTurns"`
	RetentionMode      string `yaml:"retentionMode"`
}

type workflowDocument struct {
	APIVersion  string             `yaml:"apiVersion"`
	Kind        string             `yaml:"kind"`
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Context     string             `yaml:"context"`
	Statechart  statechartDocument `yaml:"statechart"`
}

type statechartDocument struct {
	InitialState string               `yaml:"initialState"`
	States       []stateDocument      `yaml:"states"`
	Transitions  []transitionDocument `yaml:"transitions"`
}

type stateDocument struct {
	Name            string                  `yaml:"name"`
	Description     string                  `yaml:"description"`
	VisibleTools    []string                `yaml:"visibleTools"`
	EnabledTools    []string                `yaml:"enabledTools"`
	Prompt          string                  `yaml:"prompt"`
	AllowedTriggers []string                `yaml:"allowedTriggers"`
	Inputs          stateInputDocument      `yaml:"inputs"`
	Outputs         stateOutputDocument     `yaml:"outputs"`
	Completion      stateCompletionDocument `yaml:"completion"`
}

type stateInputDocument struct {
	Required []string `yaml:"required"`
	Optional []string `yaml:"optional"`
}

type stateOutputDocument struct {
	SchemaName     string   `yaml:"schema"`
	RequiredFields []string `yaml:"requiredFields"`
	Strict         bool     `yaml:"strict"`
}

type stateCompletionDocument struct {
	SuccessWhen []string `yaml:"successWhen"`
}

type transitionDocument struct {
	Trigger string `yaml:"trigger"`
	From    string `yaml:"from"`
	To      string `yaml:"to"`
}

func LoadIntoMemory(memory *Memory, raw []byte) error {
	header, err := decodeHeader(raw)
	if err != nil {
		return err
	}
	switch header.Kind {
	case "Model":
		def, err := LoadModel(raw)
		if err != nil {
			return err
		}
		memory.PutModel(def)
		return nil
	case "Agent":
		def, err := LoadAgent(raw)
		if err != nil {
			return err
		}
		memory.PutAgent(def)
		return nil
	case "Workflow":
		def, err := LoadWorkflow(raw)
		if err != nil {
			return err
		}
		memory.PutWorkflow(def)
		return nil
	default:
		return fmt.Errorf("unsupported definition kind %q", header.Kind)
	}
}

func LoadModel(raw []byte) (defs.ModelDefinition, error) {
	var doc modelDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return defs.ModelDefinition{}, err
	}
	if strings.TrimSpace(doc.Name) == "" {
		return defs.ModelDefinition{}, fmt.Errorf("model name required")
	}
	providers := make([]defs.ProviderRef, 0, len(doc.Providers))
	for _, provider := range doc.Providers {
		providers = append(providers, defs.ProviderRef{Name: strings.TrimSpace(provider.Name), ModelRef: strings.TrimSpace(provider.ModelRef)})
	}
	return defs.ModelDefinition{
		Name:      strings.TrimSpace(doc.Name),
		Providers: providers,
		Limits: defs.ModelLimits{
			ContextWindow:   doc.Limits.ContextWindow,
			MaxOutputTokens: doc.Limits.MaxOutputTokens,
		},
		Defaults: defs.ModelDefaults{Temperature: doc.Defaults.Temperature, TopP: doc.Defaults.TopP},
		Capabilities: defs.ModelCapabilities{
			Tools:      doc.Capabilities.Tools,
			Reasoning:  doc.Capabilities.Reasoning,
			Multimodal: doc.Capabilities.Multimodal,
			Streaming:  doc.Capabilities.Streaming,
		},
	}, nil
}

func LoadAgent(raw []byte) (defs.AgentDefinition, error) {
	var doc agentDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return defs.AgentDefinition{}, err
	}
	if strings.TrimSpace(doc.Name) == "" {
		return defs.AgentDefinition{}, fmt.Errorf("agent name required")
	}
	projections := doc.Context.Projections
	if len(projections) == 0 {
		projections = doc.Context.Chunks
	}
	converted, err := toProjectionDefinitions(projections)
	if err != nil {
		return defs.AgentDefinition{}, err
	}
	return defs.AgentDefinition{
		Name:        strings.TrimSpace(doc.Name),
		Description: strings.TrimSpace(doc.Description),
		Model:       strings.TrimSpace(doc.Model),
		Overrides: defs.AgentOverrides{
			Temperature:     doc.Overrides.Temperature,
			TopP:            doc.Overrides.TopP,
			MaxOutputTokens: doc.Overrides.MaxOutputTokens,
		},
		Tools: append([]string(nil), doc.Tools...),
		Context: defs.ContextDefinition{
			InputBudget: doc.Context.InputBudget,
			Projections: converted,
		},
		Cognitive: toStatechartDefinition(doc.Cognitive),
	}, nil
}

func LoadWorkflow(raw []byte) (defs.WorkflowDefinition, error) {
	var doc workflowDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return defs.WorkflowDefinition{}, err
	}
	if strings.TrimSpace(doc.Name) == "" {
		return defs.WorkflowDefinition{}, fmt.Errorf("workflow name required")
	}
	return defs.WorkflowDefinition{
		Name:        strings.TrimSpace(doc.Name),
		Description: strings.TrimSpace(doc.Description),
		Context:     strings.TrimSpace(doc.Context),
		Statechart:  toStatechartDefinition(doc.Statechart),
	}, nil
}

func decodeHeader(raw []byte) (documentHeader, error) {
	var header documentHeader
	if err := yaml.Unmarshal(raw, &header); err != nil {
		return documentHeader{}, err
	}
	return header, nil
}

func toProjectionDefinitions(items []projectionDocument) ([]defs.ProjectionDefinition, error) {
	result := make([]defs.ProjectionDefinition, 0, len(items))
	for _, item := range items {
		def := defs.ProjectionDefinition{
			Type:               strings.TrimSpace(item.Type),
			Prompt:             item.Prompt,
			Name:               strings.TrimSpace(item.Name),
			Chart:              strings.TrimSpace(item.Chart),
			RefreshEveryNTurns: item.RefreshEveryNTurns,
			RetentionMode:      strings.TrimSpace(item.RetentionMode),
		}
		if err := validateProjectionDefinition(def); err != nil {
			return nil, err
		}
		result = append(result, def)
	}
	return result, nil
}

func validateProjectionDefinition(def defs.ProjectionDefinition) error {
	switch def.Type {
	case "system":
		if def.RefreshEveryNTurns != nil {
			return fmt.Errorf("projection type %q does not support refreshEveryNTurns", def.Type)
		}
		if def.RetentionMode != "" {
			return fmt.Errorf("projection type %q does not support retentionMode", def.Type)
		}
	case "repo_context":
		if def.RetentionMode != "" && def.RetentionMode != "latest_effective" {
			return fmt.Errorf("projection type %q only supports retentionMode=latest_effective", def.Type)
		}
	case "messages":
		if def.RefreshEveryNTurns != nil {
			return fmt.Errorf("projection type %q does not support refreshEveryNTurns", def.Type)
		}
		if def.RetentionMode != "" && def.RetentionMode != "coherent_tail" {
			return fmt.Errorf("projection type %q only supports retentionMode=coherent_tail", def.Type)
		}
	case "interaction", "cognitive_state", "workflow_state", "binding":
		if def.RefreshEveryNTurns != nil {
			return fmt.Errorf("projection type %q does not support refreshEveryNTurns", def.Type)
		}
		if def.RetentionMode != "" {
			return fmt.Errorf("projection type %q does not support retentionMode", def.Type)
		}
	}
	return nil
}

func toStatechartDefinition(doc statechartDocument) defs.StatechartDefinition {
	states := make([]defs.StateDefinition, 0, len(doc.States))
	for _, state := range doc.States {
		states = append(states, defs.StateDefinition{
			Name:            strings.TrimSpace(state.Name),
			Description:     strings.TrimSpace(state.Description),
			VisibleTools:    append([]string(nil), state.VisibleTools...),
			EnabledTools:    append([]string(nil), state.EnabledTools...),
			Prompt:          state.Prompt,
			AllowedTriggers: append([]string(nil), state.AllowedTriggers...),
			Inputs: defs.StateInputContract{
				Required: append([]string(nil), state.Inputs.Required...),
				Optional: append([]string(nil), state.Inputs.Optional...),
			},
			Outputs: defs.StateOutputContract{
				SchemaName:     strings.TrimSpace(state.Outputs.SchemaName),
				RequiredFields: append([]string(nil), state.Outputs.RequiredFields...),
				Strict:         state.Outputs.Strict,
			},
			Completion: defs.StateCompletionContract{
				SuccessWhen: append([]string(nil), state.Completion.SuccessWhen...),
			},
		})
	}
	transitions := make([]defs.TransitionDefinition, 0, len(doc.Transitions))
	for _, transition := range doc.Transitions {
		transitions = append(transitions, defs.TransitionDefinition{
			Trigger: strings.TrimSpace(transition.Trigger),
			From:    strings.TrimSpace(transition.From),
			To:      strings.TrimSpace(transition.To),
		})
	}
	return defs.StatechartDefinition{
		InitialState: strings.TrimSpace(doc.InitialState),
		States:       states,
		Transitions:  transitions,
	}
}
