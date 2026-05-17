package yaml

import (
	"fmt"

	"github.com/comalice/inference_sketch/internal/agent"
	"gopkg.in/yaml.v3"
)

func validateAgentDefinition(*agent.Agent) (string, error) {
	return "", nil
}

func UnmarshalYAMLToAgent(s string) (*agent.Agent, error) {
	var agent agent.Agent
	err := yaml.Unmarshal([]byte(s), &agent)
	if err != nil {
		return nil, fmt.Errorf("Unable to unmarshal agent definition: %w", err)
	}
	return &agent, nil
}
