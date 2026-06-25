package prompt

import (
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
)

func BuildProjectionPlan(agentDef defs.AgentDefinition) ([]Projection, int, error) {
	for _, projection := range agentDef.Context.Projections {
		switch projection.Type {
		case "system", "messages", "state_task", "binding", "interaction", "repo_context":
		default:
			return nil, 0, fmt.Errorf("unknown projection type %q", projection.Type)
		}
	}
	projections := make([]Projection, 0, 1)
	maxHistory := 0
	projections = append(projections, ContextProjection{})
	maxHistory = 12
	return projections, maxHistory, nil
}
