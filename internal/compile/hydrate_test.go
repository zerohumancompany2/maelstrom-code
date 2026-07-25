package compile

import (
	"testing"

	"github.com/comalice/maelstrom/internal/defs"
	"github.com/comalice/maelstrom/internal/tools"
)

func TestHydrateAgentAppliesModelDefaultsAndOverrides(t *testing.T) {
	temperature := 0.4
	modelDef := defs.ModelDefinition{
		Name:      "glm-4.5-air",
		Providers: []defs.ProviderRef{{Name: "openrouter", ModelRef: "z-ai/glm-4.5-air:free"}},
		Limits:    defs.ModelLimits{ContextWindow: 32768, MaxOutputTokens: 8192},
		Defaults:  defs.ModelDefaults{Temperature: 0.7, TopP: 1.0},
	}
	agentDef := defs.AgentDefinition{
		Name:      "builder",
		Model:     "glm-4.5-air",
		Tools:     []string{"bind_workflow"},
		Overrides: defs.AgentOverrides{Temperature: &temperature},
		Context:   defs.ContextDefinition{InputBudget: 1000},
		Cognitive: defs.StatechartDefinition{InitialState: "observe"},
	}
	registry := tools.NewRegistry(tools.BindingTool{})

	agent, err := HydrateAgent(agentDef, modelDef, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.ProviderName != "openrouter" {
		t.Fatalf("ProviderName = %q, want openrouter", agent.ProviderName)
	}
	if agent.Inference.Temperature != 0.4 {
		t.Fatalf("Temperature = %v, want 0.4", agent.Inference.Temperature)
	}
	if len(agent.ToolNames) != 1 || agent.ToolNames[0] != "bind_workflow" {
		t.Fatalf("ToolNames = %v, want [bind_workflow]", agent.ToolNames)
	}
}

func TestHydrateAgentRejectsUnknownTool(t *testing.T) {
	modelDef := defs.ModelDefinition{
		Name:      "glm-4.5-air",
		Providers: []defs.ProviderRef{{Name: "openrouter", ModelRef: "z-ai/glm-4.5-air:free"}},
		Limits:    defs.ModelLimits{ContextWindow: 32768, MaxOutputTokens: 8192},
		Defaults:  defs.ModelDefaults{Temperature: 0.7, TopP: 1.0},
	}
	agentDef := defs.AgentDefinition{
		Name:      "builder",
		Model:     "glm-4.5-air",
		Tools:     []string{"missing_tool"},
		Context:   defs.ContextDefinition{InputBudget: 1000},
		Cognitive: defs.StatechartDefinition{InitialState: "observe"},
	}

	_, err := HydrateAgent(agentDef, modelDef, tools.NewRegistry())
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}
