package evals

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type TaskDeck struct {
	Name           string     `yaml:"name" json:"name"`
	Description    string     `yaml:"description" json:"description"`
	Models         []string   `yaml:"models,omitempty" json:"models,omitempty"`
	Agents         []string   `yaml:"agents,omitempty" json:"agents,omitempty"`
	TimeoutSeconds int        `yaml:"timeoutSeconds,omitempty" json:"timeout_seconds,omitempty"`
	Cases          []TaskCase `yaml:"cases" json:"cases"`
}

type TaskCase struct {
	ID             string          `yaml:"id" json:"id"`
	Prompt         string          `yaml:"prompt" json:"prompt"`
	AgentPath      string          `yaml:"agent" json:"agent"`
	WorkflowPath   string          `yaml:"workflow,omitempty" json:"workflow,omitempty"`
	ModelPath      string          `yaml:"model,omitempty" json:"model,omitempty"`
	Repeats        int             `yaml:"repeats,omitempty" json:"repeats,omitempty"`
	TimeoutSeconds int             `yaml:"timeoutSeconds,omitempty" json:"timeout_seconds,omitempty"`
	Eval           SessionEvalCase `yaml:"eval" json:"eval"`
	Stages         []TaskStage     `yaml:"stages,omitempty" json:"stages,omitempty"`
	// Sandbox opts the case into the write-enabled tier: the run executes
	// against a disposable copy of the runner root, and only sandboxed cases
	// get write-capable tools (replace_text, run_command). For staged cases
	// one sandbox spans all stages of a (model, repeat) so later stages see
	// earlier stages' edits.
	Sandbox         bool     `yaml:"sandbox,omitempty" json:"sandbox,omitempty"`
	SandboxExcludes []string `yaml:"sandboxExcludes,omitempty" json:"sandbox_excludes,omitempty"`
	// CommandAllowlist, when set, runs run_command in its safe tier: no
	// shell, prefix-matched commands only, workdir clamped to the sandbox.
	CommandAllowlist []string `yaml:"commandAllowlist,omitempty" json:"command_allowlist,omitempty"`
}

// TaskStage describes one ordered (agent, prompt) stage of a staged case. The
// harness runs stages sequentially against ONE persisted workflow instance:
// stage N+1's agent binds to the workflow containing stage N's finalized
// artifacts. The runtime renders those artifacts into the task frame; the
// evals/ layer owns only the sequencing (bind/unbind records and stage run IDs).
type TaskStage struct {
	ID     string          `yaml:"id" json:"id"`
	Agent  string          `yaml:"agent" json:"agent"`
	Prompt string          `yaml:"prompt" json:"prompt"`
	Model  string          `yaml:"model,omitempty" json:"model,omitempty"`
	Eval   SessionEvalCase `yaml:"eval" json:"eval"`
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
		tc := &deck.Cases[i]
		if strings.TrimSpace(tc.ID) == "" {
			return TaskDeck{}, fmt.Errorf("case %d missing id", i)
		}
		if len(tc.Stages) > 0 {
			// Staged cases: the workflow path is mandatory (stages share one
			// persisted workflow instance) and the case-level prompt/agent are
			// forbidden (each stage carries its own agent and prompt).
			if strings.TrimSpace(tc.WorkflowPath) == "" {
				return TaskDeck{}, fmt.Errorf("case %q with stages must set a workflow path", tc.ID)
			}
			if strings.TrimSpace(tc.Prompt) != "" {
				return TaskDeck{}, fmt.Errorf("case %q with stages must not set a case-level prompt", tc.ID)
			}
			if strings.TrimSpace(tc.AgentPath) != "" {
				return TaskDeck{}, fmt.Errorf("case %q with stages must not set a case-level agent", tc.ID)
			}
			for j := range tc.Stages {
				stage := &tc.Stages[j]
				if strings.TrimSpace(stage.Agent) == "" {
					return TaskDeck{}, fmt.Errorf("case %q stage %d missing agent", tc.ID, j)
				}
				if strings.TrimSpace(stage.Prompt) == "" {
					return TaskDeck{}, fmt.Errorf("case %q stage %d missing prompt", tc.ID, j)
				}
				if strings.TrimSpace(stage.ID) == "" {
					stage.ID = fmt.Sprintf("stage-%d", j+1)
				}
				if strings.TrimSpace(stage.Eval.Name) == "" {
					stage.Eval.Name = tc.ID + ":" + stage.ID
				}
			}
		} else {
			// Non-staged cases keep the legacy validation: prompt is required
			// and an agent must be resolvable (case-level or deck-level).
			if strings.TrimSpace(tc.Prompt) == "" {
				return TaskDeck{}, fmt.Errorf("case %q missing prompt", tc.ID)
			}
			if strings.TrimSpace(tc.AgentPath) == "" && len(deck.Agents) == 0 {
				return TaskDeck{}, fmt.Errorf("case %q missing agent path", tc.ID)
			}
		}
		if len(tc.CommandAllowlist) > 0 && !tc.Sandbox {
			return TaskDeck{}, fmt.Errorf("case %q sets a command allowlist without sandbox: true; run_command is only available to sandboxed cases", tc.ID)
		}
		if tc.Repeats <= 0 {
			tc.Repeats = 1
		}
		if tc.TimeoutSeconds <= 0 {
			tc.TimeoutSeconds = deck.TimeoutSeconds
		}
		if strings.TrimSpace(tc.Eval.Name) == "" {
			tc.Eval.Name = tc.ID
		}
	}
	return deck, nil
}
