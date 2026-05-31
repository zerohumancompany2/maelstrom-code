package runner

import (
	"encoding/json"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch7/statecharts"
	"github.com/comalice/inference_sketch/sketch/sketch7/tools"
)

func TestBuildSessionViewReducesBoundWorkflowState(t *testing.T) {
	agent := runtime.Agent{Name: "builder"}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", Prompt: "Observe first."}}}}
	workflowDef := defs.WorkflowDefinition{Description: "Conversation flow", Context: "Implement feature", Statechart: defs.StatechartDefinition{InitialState: "planning", States: []defs.StateDefinition{{Name: "planning"}, {Name: "implementing"}}}}
	sessionHistory := logs.NewSessionHistory("session-001")
	sessionHistory.Append(logs.SessionWorkflowBindingRecord{SessionBaseRecord: sessionHistory.NextRecord("workflow_binding_ref"), BindingID: "bind-001", WorkflowID: "workflow-001", Action: "bind"})
	workflowHistory := logs.NewWorkflowHistory("workflow-001")
	workflowHistory.Append(logs.WorkflowTransitionRecord{WorkflowBaseRecord: workflowHistory.NextRecord("workflow_transition"), FromState: "planning", ToState: "implementing", Trigger: "start_implementation"})

	view := BuildSessionView(agent, agentDef, &workflowDef, sessionHistory, workflowHistory)
	if !view.Binding.Bound {
		t.Fatal("expected binding to be active")
	}
	if view.Workflow == nil || view.Workflow.CurrentState != "implementing" {
		t.Fatalf("workflow state = %+v, want implementing", view.Workflow)
	}
}

func TestLoopRunProcessesToolCallAndAssistantResponse(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"chart": "agent", "trigger": "begin_action"})
	fakeProvider := &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{
		provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-001", ToolName: "transition_state", Arguments: map[string]string{"chart": "agent", "trigger": "begin_action"}, RawArgs: raw}},
		provider.AssistantOutput{Content: "Done."},
	}}}
	toolRegistry := tools.NewRegistry(tools.TransitionTool{AgentChart: statecharts.Compile("agent", defs.StatechartDefinition{InitialState: "observe", Transitions: []defs.TransitionDefinition{{Trigger: "begin_action", From: "observe", To: "act"}}})})
	loop := Loop{
		Provider: fakeProvider,
		Tools:    toolRegistry,
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.CognitiveProjection{},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 10,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model"}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", Prompt: "Observe first."}, {Name: "act", Prompt: "Act now."}}}}
	sessionHistory := logs.NewSessionHistory("session-002")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Do the thing."})

	err := loop.Run(agent, agentDef, nil, sessionHistory, nil)
	if err == nil {
		foundAssistant := false
		foundTransition := false
		for _, record := range sessionHistory.Records {
			switch v := record.(type) {
			case logs.AssistantMessageRecord:
				if v.Content == "Done." {
					foundAssistant = true
				}
			case logs.CognitiveTransitionRecord:
				if v.ToState == "act" {
					foundTransition = true
				}
			}
		}
		if !foundAssistant || !foundTransition {
			t.Fatalf("expected assistant and transition records, got %#v", sessionHistory.Records)
		}
		return
	}
	if err.Error() != "loop guard tripped" {
		t.Fatalf("unexpected error: %v", err)
	}
}
