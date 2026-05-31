package compile

import (
	"fmt"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch7/tools"
)

func HydrateAgent(agentDef defs.AgentDefinition, modelDef defs.ModelDefinition, toolRegistry tools.Registry) (runtime.Agent, error) {
	if strings.TrimSpace(agentDef.Name) == "" {
		return runtime.Agent{}, fmt.Errorf("agent name required")
	}
	if strings.TrimSpace(agentDef.Model) == "" {
		return runtime.Agent{}, fmt.Errorf("agent model required")
	}
	if len(modelDef.Providers) == 0 {
		return runtime.Agent{}, fmt.Errorf("model providers required")
	}
	if agentDef.Context.InputBudget <= 0 {
		return runtime.Agent{}, fmt.Errorf("agent context inputBudget must be positive")
	}
	if modelDef.Limits.ContextWindow > 0 && agentDef.Context.InputBudget > modelDef.Limits.ContextWindow {
		return runtime.Agent{}, fmt.Errorf("agent inputBudget %d exceeds model contextWindow %d", agentDef.Context.InputBudget, modelDef.Limits.ContextWindow)
	}
	if strings.TrimSpace(agentDef.Cognitive.InitialState) == "" {
		return runtime.Agent{}, fmt.Errorf("agent cognitive initialState required")
	}
	for _, toolName := range agentDef.Tools {
		if _, ok := toolRegistry.Lookup(strings.TrimSpace(toolName)); !ok {
			return runtime.Agent{}, fmt.Errorf("unknown tool %q", toolName)
		}
	}
	provider := modelDef.Providers[0]
	settings := runtime.InferenceSettings{
		Temperature:     modelDef.Defaults.Temperature,
		TopP:            modelDef.Defaults.TopP,
		MaxOutputTokens: modelDef.Limits.MaxOutputTokens,
	}
	if agentDef.Overrides.Temperature != nil {
		settings.Temperature = *agentDef.Overrides.Temperature
	}
	if agentDef.Overrides.TopP != nil {
		settings.TopP = *agentDef.Overrides.TopP
	}
	if agentDef.Overrides.MaxOutputTokens != nil {
		settings.MaxOutputTokens = *agentDef.Overrides.MaxOutputTokens
	}
	toolNames := make([]string, 0, len(agentDef.Tools))
	for _, toolName := range agentDef.Tools {
		toolNames = append(toolNames, strings.TrimSpace(toolName))
	}
	return runtime.Agent{
		Name:         agentDef.Name,
		Description:  agentDef.Description,
		LogicalModel: modelDef.Name,
		ProviderName: provider.Name,
		ProviderRef:  provider.ModelRef,
		Inference:    settings,
		Limits:       modelDef.Limits,
		Capabilities: modelDef.Capabilities,
		ToolNames:    toolNames,
	}, nil
}
