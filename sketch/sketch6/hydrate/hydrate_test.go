package hydrate

import (
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/model"
	"github.com/comalice/inference_sketch/sketch/sketch6/registry"
	"github.com/comalice/inference_sketch/sketch/sketch6/tools"
)

func TestHydrateAppliesModelDefaultsAndOverrides(t *testing.T) {
	models := registry.NewMemoryRegistry[model.Definition]()
	err := models.Update(registry.Revision[model.Definition]{
		Key: "gpt-test",
		Value: model.Definition{
			Name:      "gpt-test",
			Providers: []model.ProviderRef{{Name: "openrouter", ModelRef: "openai/test"}},
			Limits:    model.Limits{ContextWindow: 4000, MaxOutputTokens: 900},
			Defaults:  model.Defaults{Temperature: 0.2, TopP: 0.9},
		},
	})
	if err != nil {
		t.Fatalf("update model registry: %v", err)
	}

	temperature := 0.7
	maxOutputTokens := 256
	hydrator := Hydrator{Models: models, Tools: tools.NewToolRegistry(tools.TransitionTool{})}

	runtimeAgent, err := hydrator.Hydrate(agent.Definition{
		Name:  "planner",
		Model: "gpt-test",
		Tools: []string{" transition_state "},
		Overrides: agent.Overrides{
			Temperature:     &temperature,
			MaxOutputTokens: &maxOutputTokens,
		},
		Context:   agent.ContextDefinition{InputBudget: 512, Chunks: []agent.ChunkDefinition{{Type: "system", Prompt: "You are helpful."}, {Type: "messages"}}},
		Cognitive: agent.StatechartDefinition{InitialState: "observe"},
	})
	if err != nil {
		t.Fatalf("hydrate returned error: %v", err)
	}

	if runtimeAgent.ProviderName != "openrouter" || runtimeAgent.ProviderRef != "openai/test" {
		t.Fatalf("provider = %s/%s, want openrouter/openai/test", runtimeAgent.ProviderName, runtimeAgent.ProviderRef)
	}
	if runtimeAgent.Inference.Temperature != 0.7 {
		t.Fatalf("temperature = %v, want 0.7", runtimeAgent.Inference.Temperature)
	}
	if runtimeAgent.Inference.TopP != 0.9 {
		t.Fatalf("topP = %v, want 0.9", runtimeAgent.Inference.TopP)
	}
	if runtimeAgent.Inference.MaxOutputTokens != 256 {
		t.Fatalf("maxOutputTokens = %d, want 256", runtimeAgent.Inference.MaxOutputTokens)
	}
	if len(runtimeAgent.ToolNames) != 1 || runtimeAgent.ToolNames[0] != "transition_state" {
		t.Fatalf("tool names = %#v, want [transition_state]", runtimeAgent.ToolNames)
	}
}

func TestHydrateRejectsUnknownTool(t *testing.T) {
	hydrator := Hydrator{Models: singleModelRegistry(), Tools: tools.NewToolRegistry()}

	_, err := hydrator.Hydrate(validAgentDefinition(func(def *agent.Definition) {
		def.Tools = []string{"missing_tool"}
	}))
	if err == nil || !strings.Contains(err.Error(), `unknown tool "missing_tool"`) {
		t.Fatalf("error = %v, want unknown tool error", err)
	}
}

func TestHydrateRejectsInputBudgetAboveModelLimit(t *testing.T) {
	hydrator := Hydrator{Models: singleModelRegistry(), Tools: tools.NewToolRegistry(tools.TransitionTool{})}

	_, err := hydrator.Hydrate(validAgentDefinition(func(def *agent.Definition) {
		def.Context.InputBudget = 5000
	}))
	if err == nil || !strings.Contains(err.Error(), "exceeds model contextWindow") {
		t.Fatalf("error = %v, want context window validation error", err)
	}
}

func TestHydrateRejectsInvalidStateChunk(t *testing.T) {
	hydrator := Hydrator{Models: singleModelRegistry(), Tools: tools.NewToolRegistry(tools.TransitionTool{})}

	_, err := hydrator.Hydrate(validAgentDefinition(func(def *agent.Definition) {
		def.Context.Chunks = append(def.Context.Chunks, agent.ChunkDefinition{Type: "state"})
	}))
	if err == nil || !strings.Contains(err.Error(), "state chunk chart required") {
		t.Fatalf("error = %v, want state chunk validation error", err)
	}
}

func singleModelRegistry() registry.Registry[model.Definition] {
	models := registry.NewMemoryRegistry[model.Definition]()
	_ = models.Update(registry.Revision[model.Definition]{
		Key: "gpt-test",
		Value: model.Definition{
			Name:      "gpt-test",
			Providers: []model.ProviderRef{{Name: "openrouter", ModelRef: "openai/test"}},
			Limits:    model.Limits{ContextWindow: 2048, MaxOutputTokens: 512},
			Defaults:  model.Defaults{Temperature: 0.1, TopP: 0.95},
		},
	})
	return models
}

func validAgentDefinition(mutator func(*agent.Definition)) agent.Definition {
	def := agent.Definition{
		Name:  "planner",
		Model: "gpt-test",
		Tools: []string{"transition_state"},
		Context: agent.ContextDefinition{
			InputBudget: 512,
			Chunks:      []agent.ChunkDefinition{{Type: "system", Prompt: "You are helpful."}, {Type: "messages"}, {Type: "cognitive_state"}},
		},
		Cognitive: agent.StatechartDefinition{InitialState: "observe"},
	}
	if mutator != nil {
		mutator(&def)
	}
	return def
}
