package runner

import (
	"github.com/comalice/maelstrom/internal/defs"
	"github.com/comalice/maelstrom/internal/logs"
	"github.com/comalice/maelstrom/internal/runtime"
)

func BuildSessionView(agent runtime.Agent, agentDef defs.AgentDefinition, workflowDef *defs.WorkflowDefinition, sessionHistory *logs.SessionHistory, workflowHistory *logs.WorkflowHistory) runtime.SessionView {
	binding := runtime.ReduceBindingState(sessionHistory)
	interaction := runtime.ReduceInteractionMode(sessionHistory)
	cognitive := runtime.ReduceCognitiveState(sessionHistory, agentDef.Cognitive.InitialState, agentDef.Cognitive)

	var workflowView *runtime.WorkflowView
	if binding.Bound && workflowDef != nil && workflowHistory != nil {
		view := runtime.ReduceWorkflowState(workflowHistory, *workflowDef, agent.Name)
		workflowView = &view
	}

	return runtime.SessionView{
		Agent:       agent,
		Cognitive:   cognitive,
		Binding:     binding,
		Workflow:    workflowView,
		Interaction: interaction,
	}
}
