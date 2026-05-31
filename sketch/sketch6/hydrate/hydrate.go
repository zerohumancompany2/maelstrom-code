package hydrate

import (
	"fmt"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/assembly"
	"github.com/comalice/inference_sketch/sketch/sketch6/model"
	"github.com/comalice/inference_sketch/sketch/sketch6/registry"
	"github.com/comalice/inference_sketch/sketch/sketch6/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch6/tools"
)

type Hydrator struct {
	Models registry.Registry[model.Definition]
	Tools  tools.Registry
}

func (h Hydrator) Hydrate(def agent.Definition) (runtime.Agent, error) {
	modelRevision, ok := h.Models.Get(def.Model)
	if !ok {
		return runtime.Agent{}, fmt.Errorf("unknown logical model %q", def.Model)
	}
	modelDef := modelRevision.Value
	if err := validateAgentDefinition(def, modelDef, h.Tools); err != nil {
		return runtime.Agent{}, err
	}
	provider := modelDef.Providers[0]
	settings := runtime.InferenceSettings{
		Temperature:     modelDef.Defaults.Temperature,
		TopP:            modelDef.Defaults.TopP,
		MaxOutputTokens: modelDef.Limits.MaxOutputTokens,
	}
	if def.Overrides.Temperature != nil {
		settings.Temperature = *def.Overrides.Temperature
	}
	if def.Overrides.TopP != nil {
		settings.TopP = *def.Overrides.TopP
	}
	if def.Overrides.MaxOutputTokens != nil {
		settings.MaxOutputTokens = *def.Overrides.MaxOutputTokens
	}
	toolNames := make([]string, 0, len(def.Tools))
	for _, toolName := range def.Tools {
		toolNames = append(toolNames, strings.TrimSpace(toolName))
	}
	return runtime.Agent{
		Name:         def.Name,
		Description:  def.Description,
		LogicalModel: modelDef.Name,
		ProviderName: provider.Name,
		ProviderRef:  provider.ModelRef,
		Inference:    settings,
		Limits:       modelDef.Limits,
		Capabilities: modelDef.Capabilities,
		ToolNames:    toolNames,
	}, nil
}

func validateAgentDefinition(def agent.Definition, modelDef model.Definition, toolRegistry tools.Registry) error {
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("agent name required")
	}
	if strings.TrimSpace(def.Model) == "" {
		return fmt.Errorf("agent model required")
	}
	if def.Context.InputBudget <= 0 {
		return fmt.Errorf("agent context inputBudget must be positive")
	}
	if modelDef.Limits.ContextWindow > 0 && def.Context.InputBudget > modelDef.Limits.ContextWindow {
		return fmt.Errorf("agent inputBudget %d exceeds model contextWindow %d", def.Context.InputBudget, modelDef.Limits.ContextWindow)
	}
	if _, err := assembly.BuildPlan(def); err != nil {
		return err
	}
	for _, toolName := range def.Tools {
		if _, ok := toolRegistry.Lookup(strings.TrimSpace(toolName)); !ok {
			return fmt.Errorf("unknown tool %q", toolName)
		}
	}
	for _, chunk := range def.Context.Chunks {
		switch chunk.Type {
		case "system":
			if strings.TrimSpace(chunk.Prompt) == "" {
				return fmt.Errorf("system chunk prompt required")
			}
		case "messages":
		case "state":
			if strings.TrimSpace(chunk.Chart) == "" {
				return fmt.Errorf("state chunk chart required")
			}
		default:
			return fmt.Errorf("unknown context chunk type %q", chunk.Type)
		}
	}
	return nil
}
