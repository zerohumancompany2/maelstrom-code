package prompt

import (
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
)

func BuildProjectionPlan(agentDef defs.AgentDefinition) ([]Projection, int, error) {
	projections := make([]Projection, 0, len(agentDef.Context.Projections))
	maxHistory := 0
	for _, projection := range agentDef.Context.Projections {
		switch projection.Type {
		case "system":
			projections = append(projections, StaticProjection{ProjectionName: projection.Name, Role: "system", Prompt: projection.Prompt})
		case "messages":
			projections = append(projections, RecentHistoryProjection{})
			if maxHistory == 0 {
				maxHistory = 10
			}
		case "cognitive_state":
			projections = append(projections, CognitiveProjection{})
		case "workflow_state":
			projections = append(projections, WorkflowProjection{})
		case "binding":
			projections = append(projections, BindingProjection{})
		case "interaction":
			projections = append(projections, InteractionProjection{})
		default:
			return nil, 0, fmt.Errorf("unknown projection type %q", projection.Type)
		}
	}
	return projections, maxHistory, nil
}
