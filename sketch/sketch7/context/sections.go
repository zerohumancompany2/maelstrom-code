package context

import (
	"fmt"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

func BuildSections(agentDef defs.AgentDefinition, session runtime.SessionView) []Section {
	sections := []Section{}
	for _, projection := range agentDef.Context.Projections {
		switch projection.Type {
		case "system":
			sections = append(sections, Section{Name: projection.Name, Role: "system", Content: projection.Prompt, Sticky: true})
		case "interaction":
			content := fmt.Sprintf("Session interaction mode: %s.", session.Interaction.Mode)
			if strings.TrimSpace(session.Interaction.Reason) != "" {
				content = fmt.Sprintf("Session interaction mode: %s. Reason: %s.", session.Interaction.Mode, session.Interaction.Reason)
			}
			sections = append(sections, Section{Name: "interaction", Role: "system", Content: content, Sticky: true})
		case "cognitive_state":
			sections = append(sections, Section{Name: "cognitive", Role: "system", Content: fmt.Sprintf("Cognitive mode: %s. Prompt: %s. Visible tools: %s. Enabled tools: %s.", session.Cognitive.CurrentState, strings.TrimSpace(session.Cognitive.Prompt), joinList(session.Cognitive.VisibleTools), joinList(session.Cognitive.EnabledTools)), Sticky: true})
		case "workflow_state":
			if session.Workflow != nil {
				sections = append(sections, Section{Name: "workflow", Role: "system", Content: fmt.Sprintf("Workflow %s is in state %s. Description: %s. Context: %s. Visible tools: %s. Enabled tools: %s.", session.Workflow.WorkflowID, session.Workflow.CurrentState, session.Workflow.Description, session.Workflow.Context, joinList(session.Workflow.VisibleTools), joinList(session.Workflow.EnabledTools)), Sticky: true})
			}
		case "binding":
			if session.Binding.Bound {
				sections = append(sections, Section{Name: "binding", Role: "system", Content: fmt.Sprintf("Active binding: workflow=%s.", session.Binding.WorkflowID), Sticky: true})
			}
		}
	}
	return sections
}
