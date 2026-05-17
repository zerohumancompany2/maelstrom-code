package yaml_test

import (
	"testing"

	"github.com/comalice/inference_sketch/internal/yaml"
)

func TestCanUnmarshalAgentName(t *testing.T) {
	def := `name: agent_name`

	agent, err := yaml.UnmarshalYAMLToAgent(def)
	if err != nil {
		t.Fatalf("could not unmarshal definition %v", def)
	}
	if agent.Name != "agent_name" {
		t.Fatalf("got %v, want %v", def, agent.Name)
	}
}
