package definitions_test

import (
	"testing"

	"github.com/comalice/inference_sketch/internal/definitions"
	"gopkg.in/yaml.v3"
)

func TestCanUnmarshalAgentDefinitionSketch(t *testing.T) {
	def := `name: weather-assistant
model: glm-4.5-air
tools:
  - weather
context:
  inputBudget: 24000
  chunks:
    - type: system
      prompt: hello
      budgetPct: 0.05
      policy: fail
      priority: 10
    - type: messages
      flexible: true
      policy: hard
      priority: 5
`

	var agent definitions.AgentDefinition
	if err := yaml.Unmarshal([]byte(def), &agent); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if agent.Model != "glm-4.5-air" {
		t.Fatalf("got model %q, want glm-4.5-air", agent.Model)
	}
	if agent.Context.InputBudget != 24000 {
		t.Fatalf("got input budget %d, want 24000", agent.Context.InputBudget)
	}
	if len(agent.Context.Chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(agent.Context.Chunks))
	}
}

func TestCanUnmarshalModelDefinitionSketch(t *testing.T) {
	def := `name: glm-4.5-air
providers:
  - name: openrouter
    modelRef: z-ai/glm-4.5-air:free
limits:
  contextWindow: 32768
  maxOutputTokens: 8192
defaults:
  temperature: 0.7
  topP: 1.0
capabilities:
  tools: true
  reasoning: true
  streaming: true
`

	var model definitions.ModelDefinition
	if err := yaml.Unmarshal([]byte(def), &model); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if model.Name != "glm-4.5-air" {
		t.Fatalf("got name %q, want glm-4.5-air", model.Name)
	}
	if len(model.Providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(model.Providers))
	}
	if model.Limits.ContextWindow != 32768 {
		t.Fatalf("got context window %d, want 32768", model.Limits.ContextWindow)
	}
}
