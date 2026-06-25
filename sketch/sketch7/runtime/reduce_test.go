package runtime

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
)

func TestReduceCognitiveStateUsesInitialStateWhenHistoryEmpty(t *testing.T) {
	history := logs.NewSessionHistory("session-001")
	chart := defs.StatechartDefinition{
		InitialState: "observe",
		States: []defs.StateDefinition{{
			Name:         "observe",
			VisibleTools: []string{"transition_state"},
			EnabledTools: []string{"transition_state"},
			Prompt:       "Observe first.",
		}},
	}

	view := ReduceCognitiveState(history, chart.InitialState, chart)
	if view.CurrentState != "observe" {
		t.Fatalf("CurrentState = %q, want observe", view.CurrentState)
	}
	if view.Prompt != "Observe first." {
		t.Fatalf("Prompt = %q, want %q", view.Prompt, "Observe first.")
	}
}

func TestReduceCognitiveStateCarriesBounds(t *testing.T) {
	history := logs.NewSessionHistory("session-bounds-cognitive")
	chart := defs.StatechartDefinition{
		InitialState: "observe",
		States: []defs.StateDefinition{{
			Name: "observe",
			Bounds: defs.StateBoundsContract{
				MaxInferenceTurns:      3,
				MaxToolCalls:           6,
				MaxWallTimeSeconds:     120,
				MaxFinalizationRetries: 1,
			},
		}},
	}

	view := ReduceCognitiveState(history, chart.InitialState, chart)
	if view.Bounds.MaxInferenceTurns != 3 || view.Bounds.MaxToolCalls != 6 || view.Bounds.MaxWallTimeSeconds != 120 || view.Bounds.MaxFinalizationRetries != 1 {
		t.Fatalf("Bounds = %+v, want configured bounds", view.Bounds)
	}
}

func TestReduceCognitiveStateUsesLatestTransition(t *testing.T) {
	history := logs.NewSessionHistory("session-002")
	history.Append(logs.CognitiveTransitionRecord{
		SessionBaseRecord: history.NextRecord("cognitive_transition"),
		FromState:         "observe",
		ToState:           "act",
		Trigger:           "begin_action",
	})
	chart := defs.StatechartDefinition{
		InitialState: "observe",
		States: []defs.StateDefinition{
			{Name: "observe", Prompt: "Observe first."},
			{Name: "act", Prompt: "Act now.", VisibleTools: []string{"edit_file"}, EnabledTools: []string{"edit_file"}},
		},
	}

	view := ReduceCognitiveState(history, chart.InitialState, chart)
	if view.CurrentState != "act" {
		t.Fatalf("CurrentState = %q, want act", view.CurrentState)
	}
	if view.Prompt != "Act now." {
		t.Fatalf("Prompt = %q, want %q", view.Prompt, "Act now.")
	}
	if len(view.EnabledTools) != 1 || view.EnabledTools[0] != "edit_file" {
		t.Fatalf("EnabledTools = %v, want [edit_file]", view.EnabledTools)
	}
}

func TestReduceWorkflowStateUsesInitialStateWhenHistoryEmpty(t *testing.T) {
	history := logs.NewWorkflowHistory("workflow-001")
	def := defs.WorkflowDefinition{
		Name:        "conversation_to_execution",
		Description: "Conversation to execution",
		Context:     "Implement the requested change.",
		Statechart: defs.StatechartDefinition{
			InitialState: "chatting",
			States: []defs.StateDefinition{{
				Name:         "chatting",
				VisibleTools: []string{"bind_workflow"},
				EnabledTools: []string{"bind_workflow"},
			}},
		},
	}

	view := ReduceWorkflowState(history, def, "weather-agent")
	if view.CurrentState != "chatting" {
		t.Fatalf("CurrentState = %q, want chatting", view.CurrentState)
	}
	if view.LastBoundAgent != "weather-agent" {
		t.Fatalf("LastBoundAgent = %q, want weather-agent", view.LastBoundAgent)
	}
	if len(view.EnabledTools) != 1 || view.EnabledTools[0] != "bind_workflow" {
		t.Fatalf("EnabledTools = %v, want [bind_workflow]", view.EnabledTools)
	}
}

func TestReduceWorkflowStateCarriesBounds(t *testing.T) {
	history := logs.NewWorkflowHistory("workflow-bounds")
	def := defs.WorkflowDefinition{Statechart: defs.StatechartDefinition{
		InitialState: "validating",
		States: []defs.StateDefinition{{
			Name: "validating",
			Bounds: defs.StateBoundsContract{
				MaxInferenceTurns:      2,
				MaxToolCalls:           4,
				MaxWallTimeSeconds:     90,
				MaxFinalizationRetries: 2,
			},
		}},
	}}

	view := ReduceWorkflowState(history, def, "builder")
	if view.Bounds.MaxInferenceTurns != 2 || view.Bounds.MaxToolCalls != 4 || view.Bounds.MaxWallTimeSeconds != 90 || view.Bounds.MaxFinalizationRetries != 2 {
		t.Fatalf("Bounds = %+v, want configured workflow bounds", view.Bounds)
	}
}

func TestReduceWorkflowStateUsesLatestTransitionAndBindingRef(t *testing.T) {
	history := logs.NewWorkflowHistory("workflow-002")
	history.Append(logs.WorkflowBindingRefRecord{
		WorkflowBaseRecord: history.NextRecord("workflow_binding_ref"),
		BindingID:          "bind-001",
		AgentID:            "builder-agent",
		Action:             "bind",
	})
	history.Append(logs.WorkflowTransitionRecord{
		WorkflowBaseRecord: history.NextRecord("workflow_transition"),
		FromState:          "planning",
		ToState:            "implementing",
		Trigger:            "start_implementation",
	})
	def := defs.WorkflowDefinition{
		Statechart: defs.StatechartDefinition{
			InitialState: "chatting",
			States: []defs.StateDefinition{
				{Name: "chatting"},
				{Name: "implementing", VisibleTools: []string{"edit_file", "run_tests"}, EnabledTools: []string{"edit_file", "run_tests"}},
			},
		},
	}

	view := ReduceWorkflowState(history, def, "")
	if view.CurrentState != "implementing" {
		t.Fatalf("CurrentState = %q, want implementing", view.CurrentState)
	}
	if view.LastBoundAgent != "builder-agent" {
		t.Fatalf("LastBoundAgent = %q, want builder-agent", view.LastBoundAgent)
	}
	if len(view.VisibleTools) != 2 {
		t.Fatalf("VisibleTools = %v, want 2 tools", view.VisibleTools)
	}
}

func TestReduceBindingStateUsesLatestSessionBinding(t *testing.T) {
	history := logs.NewSessionHistory("session-003")
	history.Append(logs.SessionWorkflowBindingRecord{
		SessionBaseRecord: history.NextRecord("workflow_binding_ref"),
		BindingID:         "bind-001",
		WorkflowID:        "workflow-003",
		Action:            "bind",
	})
	history.Append(logs.SessionWorkflowBindingRecord{
		SessionBaseRecord: history.NextRecord("workflow_binding_ref"),
		BindingID:         "bind-002",
		WorkflowID:        "workflow-003",
		Action:            "unbind",
	})

	view := ReduceBindingState(history)
	if view.Bound {
		t.Fatalf("Bound = true, want false")
	}
	if view.WorkflowID != "workflow-003" {
		t.Fatalf("WorkflowID = %q, want workflow-003", view.WorkflowID)
	}
}

func TestReduceInteractionModeDefaultsToInteractive(t *testing.T) {
	history := logs.NewSessionHistory("session-004")
	view := ReduceInteractionMode(history)
	if view.Mode != InteractionInteractive {
		t.Fatalf("Mode = %q, want %q", view.Mode, InteractionInteractive)
	}
}

func TestReduceInteractionModeRunningWhenLatestRecordIsToolActivity(t *testing.T) {
	history := logs.NewSessionHistory("session-005")
	history.Append(logs.ToolCallRequestRecord{
		SessionBaseRecord: history.NextRecord("tool_call_request"),
		CallID:            "call-001",
		ToolName:          "run_tests",
		Arguments:         `{}`,
	})

	view := ReduceInteractionMode(history)
	if view.Mode != InteractionRunning {
		t.Fatalf("Mode = %q, want %q", view.Mode, InteractionRunning)
	}
}

func TestReduceInteractionModeInterruptedWhenLatestRecordIsInterrupt(t *testing.T) {
	history := logs.NewSessionHistory("session-006")
	history.Append(logs.InterruptRecord{
		SessionBaseRecord: history.NextRecord("interrupt"),
		Reason:            "user pressed escape",
		RequestedBy:       "user",
	})

	view := ReduceInteractionMode(history)
	if view.Mode != InteractionInterrupted {
		t.Fatalf("Mode = %q, want %q", view.Mode, InteractionInterrupted)
	}
	if view.Reason != "user pressed escape" {
		t.Fatalf("Reason = %q, want %q", view.Reason, "user pressed escape")
	}
}

func TestReduceInteractionModeResumableWhenLatestRecordIsResume(t *testing.T) {
	history := logs.NewSessionHistory("session-007")
	history.Append(logs.InterruptRecord{
		SessionBaseRecord: history.NextRecord("interrupt"),
		Reason:            "user wants to redirect",
		RequestedBy:       "user",
	})
	history.Append(logs.ResumeRecord{
		SessionBaseRecord: history.NextRecord("resume"),
		Reason:            "user finished guidance",
	})

	view := ReduceInteractionMode(history)
	if view.Mode != InteractionResumable {
		t.Fatalf("Mode = %q, want %q", view.Mode, InteractionResumable)
	}
	if view.Reason != "user finished guidance" {
		t.Fatalf("Reason = %q, want %q", view.Reason, "user finished guidance")
	}
}
