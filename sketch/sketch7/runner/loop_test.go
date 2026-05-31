package runner

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch7/statecharts"
	"github.com/comalice/inference_sketch/sketch/sketch7/tools"
)

type scriptedProvider struct {
	responses []provider.Response
	requests  []provider.Request
	builds    []provider.Request
	index     int
}

func (s *scriptedProvider) BuildRequest(agent runtime.Agent, payload prompt.Payload, tools []provider.ToolDefinition) (provider.Request, error) {
	request, err := provider.BuildRequest(agent, payload, tools)
	if err != nil {
		return provider.Request{}, err
	}
	s.builds = append(s.builds, request)
	return request, nil
}

func (s *scriptedProvider) Send(request provider.Request) (provider.Response, error) {
	s.requests = append(s.requests, request)
	if s.index >= len(s.responses) {
		return provider.Response{}, fmt.Errorf("no scripted response for request %d", s.index)
	}
	response := s.responses[s.index]
	s.index++
	return response, nil
}

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

func TestLoopRunStopsWhenProviderReturnsOnlyAssistantOutput(t *testing.T) {
	fakeProvider := &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{
		provider.AssistantOutput{Content: "All done."},
	}}}
	loop := Loop{
		Provider: fakeProvider,
		Tools:    tools.NewRegistry(),
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 10,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model"}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe"}}
	sessionHistory := logs.NewSessionHistory("session-003")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Summarize the current task."})

	if err := loop.Run(agent, agentDef, nil, sessionHistory, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	foundAssistant := false
	for _, record := range sessionHistory.Records {
		if msg, ok := record.(logs.AssistantMessageRecord); ok && msg.Content == "All done." {
			foundAssistant = true
		}
	}
	if !foundAssistant {
		t.Fatalf("expected assistant message in session history, got %#v", sessionHistory.Records)
	}
}

func TestLoopRunProcessesWorkflowTransitionWhenBound(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"chart": "workflow", "trigger": "start_implementation"})
	fakeProvider := &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{
		provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-004", ToolName: "transition_state", Arguments: map[string]string{"chart": "workflow", "trigger": "start_implementation"}, RawArgs: raw}},
		provider.AssistantOutput{Content: "Workflow advanced."},
	}}}
	toolRegistry := tools.NewRegistry(tools.TransitionTool{
		WorkflowChart: statecharts.Compile("workflow", defs.StatechartDefinition{InitialState: "planning", Transitions: []defs.TransitionDefinition{{Trigger: "start_implementation", From: "planning", To: "implementing"}}}),
	})
	loop := Loop{
		Provider: fakeProvider,
		Tools:    toolRegistry,
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.WorkflowProjection{},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 10,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model"}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe"}}
	workflowDef := defs.WorkflowDefinition{Description: "Conversation flow", Context: "Implement feature", Statechart: defs.StatechartDefinition{InitialState: "planning", States: []defs.StateDefinition{{Name: "planning"}, {Name: "implementing"}}}}
	sessionHistory := logs.NewSessionHistory("session-004")
	sessionHistory.Append(logs.SessionWorkflowBindingRecord{SessionBaseRecord: sessionHistory.NextRecord("workflow_binding_ref"), BindingID: "bind-004", WorkflowID: "workflow-004", Action: "bind"})
	workflowHistory := logs.NewWorkflowHistory("workflow-004")

	err := loop.Run(agent, agentDef, &workflowDef, sessionHistory, workflowHistory)
	if err == nil {
		foundSessionRef := false
		foundWorkflowTransition := false
		for _, record := range sessionHistory.Records {
			if ref, ok := record.(logs.WorkflowTransitionRefRecord); ok && ref.ToState == "implementing" {
				foundSessionRef = true
			}
		}
		for _, record := range workflowHistory.Records {
			if tr, ok := record.(logs.WorkflowTransitionRecord); ok && tr.ToState == "implementing" {
				foundWorkflowTransition = true
			}
		}
		if !foundSessionRef || !foundWorkflowTransition {
			t.Fatalf("expected linked workflow transition records, got session=%#v workflow=%#v", sessionHistory.Records, workflowHistory.Records)
		}
		return
	}
	if err.Error() != "loop guard tripped" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoopRunScriptedBindInterruptResumeFlow(t *testing.T) {
	bindRaw, _ := json.Marshal(map[string]string{"workflow_id": "workflow-010", "binding_id": "bind-010"})
	interruptRaw, _ := json.Marshal(map[string]string{"reason": "need clarification", "requested_by": "user"})
	resumeRaw, _ := json.Marshal(map[string]string{"reason": "user clarified"})
	providerScript := &scriptedProvider{responses: []provider.Response{
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-bind-010", ToolName: "bind_workflow", Arguments: map[string]string{"workflow_id": "workflow-010", "binding_id": "bind-010"}, RawArgs: bindRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-interrupt-010", ToolName: "interrupt_session", Arguments: map[string]string{"reason": "need clarification", "requested_by": "user"}, RawArgs: interruptRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-resume-010", ToolName: "resume_session", Arguments: map[string]string{"reason": "user clarified"}, RawArgs: resumeRaw}},
		}},
		{Outputs: []provider.Output{
			provider.AssistantOutput{Content: "Resumed successfully."},
		}},
	}}
	toolRegistry := tools.NewRegistry(tools.BindingTool{}, tools.InterruptTool{}, tools.ResumeTool{})
	loop := Loop{
		Provider: providerScript,
		Tools:    toolRegistry,
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.BindingProjection{},
			prompt.InteractionProjection{},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 10,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model"}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe"}}
	workflowDef := defs.WorkflowDefinition{Description: "Conversation flow", Context: "Implement feature", Statechart: defs.StatechartDefinition{InitialState: "chatting"}}
	sessionHistory := logs.NewSessionHistory("session-010")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Start working, but I may interrupt."})
	workflowHistory := logs.NewWorkflowHistory("workflow-010")

	if err := loop.Run(agent, agentDef, &workflowDef, sessionHistory, workflowHistory); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundBind := false
	foundInterrupt := false
	foundResume := false
	foundAssistant := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.SessionWorkflowBindingRecord:
			if v.Action == "bind" && v.WorkflowID == "workflow-010" {
				foundBind = true
			}
		case logs.InterruptRecord:
			if v.Reason == "need clarification" {
				foundInterrupt = true
			}
		case logs.ResumeRecord:
			if v.Reason == "user clarified" {
				foundResume = true
			}
		case logs.AssistantMessageRecord:
			if v.Content == "Resumed successfully." {
				foundAssistant = true
			}
		}
	}
	if !foundBind || !foundInterrupt || !foundResume || !foundAssistant {
		t.Fatalf("expected bind/interrupt/resume/assistant records, got %#v", sessionHistory.Records)
	}
	if len(providerScript.requests) != 4 {
		t.Fatalf("got %d provider requests, want 4", len(providerScript.requests))
	}
}
