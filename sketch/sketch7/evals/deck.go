package evals

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type TaskDeck struct {
	Name        string     `yaml:"name" json:"name"`
	Description string     `yaml:"description" json:"description"`
	Cases       []TaskCase `yaml:"cases" json:"cases"`
}

type TaskCase struct {
	ID           string          `yaml:"id" json:"id"`
	Prompt       string          `yaml:"prompt" json:"prompt"`
	AgentPath    string          `yaml:"agent" json:"agent"`
	WorkflowPath string          `yaml:"workflow,omitempty" json:"workflow,omitempty"`
	ModelPath    string          `yaml:"model,omitempty" json:"model,omitempty"`
	Repeats      int             `yaml:"repeats,omitempty" json:"repeats,omitempty"`
	Eval         SessionEvalCase `yaml:"eval" json:"eval"`
}

func LoadTaskDeck(path string) (TaskDeck, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return TaskDeck{}, err
	}
	var deck TaskDeck
	if err := yaml.Unmarshal(raw, &deck); err != nil {
		return TaskDeck{}, err
	}
	if strings.TrimSpace(deck.Name) == "" {
		return TaskDeck{}, fmt.Errorf("deck name required")
	}
	if len(deck.Cases) == 0 {
		return TaskDeck{}, fmt.Errorf("deck must contain at least one case")
	}
	for i := range deck.Cases {
		if strings.TrimSpace(deck.Cases[i].ID) == "" {
			return TaskDeck{}, fmt.Errorf("case %d missing id", i)
		}
		if strings.TrimSpace(deck.Cases[i].Prompt) == "" {
			return TaskDeck{}, fmt.Errorf("case %q missing prompt", deck.Cases[i].ID)
		}
		if strings.TrimSpace(deck.Cases[i].AgentPath) == "" {
			return TaskDeck{}, fmt.Errorf("case %q missing agent path", deck.Cases[i].ID)
		}
		if deck.Cases[i].Repeats <= 0 {
			deck.Cases[i].Repeats = 1
		}
		if strings.TrimSpace(deck.Cases[i].Eval.Name) == "" {
			deck.Cases[i].Eval.Name = deck.Cases[i].ID
		}
	}
	return deck, nil
}
