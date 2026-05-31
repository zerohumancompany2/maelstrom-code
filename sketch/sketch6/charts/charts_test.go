package charts

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
)

func TestBuildSnapshotUsesLatestTransitionPerChart(t *testing.T) {
	history := session.NewHistory("session-1")
	history.Append(session.UserMessageRecord{BaseRecord: history.NextRecord("user_message"), Content: "hello"})
	history.Append(session.StateTransitionRecord{BaseRecord: history.NextRecord("state_transition"), ChartName: "agent", FromState: "idle", ToState: "observe", Trigger: "start"})
	history.Append(session.StateTransitionRecord{BaseRecord: history.NextRecord("state_transition"), ChartName: "workflow", FromState: "idle", ToState: "available", Trigger: "bind"})
	history.Append(session.StateTransitionRecord{BaseRecord: history.NextRecord("state_transition"), ChartName: "agent", FromState: "observe", ToState: "act", Trigger: "begin_action"})

	snapshot := BuildSnapshot(history)

	if got := snapshot.State("agent"); got != "act" {
		t.Fatalf("agent state = %q, want %q", got, "act")
	}
	if got := snapshot.State("workflow"); got != "available" {
		t.Fatalf("workflow state = %q, want %q", got, "available")
	}
	if got := snapshot.State("missing"); got != "unknown" {
		t.Fatalf("missing state = %q, want %q", got, "unknown")
	}
}

func TestBuildWorkflowSnapshotFallsBackToDefinitionInitialState(t *testing.T) {
	snapshot := BuildWorkflowSnapshot(workflow.Definition{Statechart: workflow.StatechartDefinition{InitialState: "available"}}, workflow.NewHistory("workflow-1"))

	if got := snapshot.State("workflow"); got != "available" {
		t.Fatalf("workflow state = %q, want %q", got, "available")
	}
}

func TestSetFireUsesAuthoredTransitions(t *testing.T) {
	set := NewSet(
		agent.Definition{Cognitive: agent.StatechartDefinition{InitialState: "observe", Transitions: []agent.TransitionDefinition{{From: "observe", Trigger: "begin_action", To: "act"}}}},
		workflow.Definition{Statechart: workflow.StatechartDefinition{InitialState: "available", Transitions: []workflow.TransitionDefinition{{From: "available", Trigger: "begin_lookup", To: "lookup_pending"}}}},
	)

	next, err := set.Fire("agent", "observe", "begin_action")
	if err != nil {
		t.Fatalf("agent transition returned error: %v", err)
	}
	if next != "act" {
		t.Fatalf("agent next state = %q, want %q", next, "act")
	}

	next, err = set.Fire("workflow", "available", "begin_lookup")
	if err != nil {
		t.Fatalf("workflow transition returned error: %v", err)
	}
	if next != "lookup_pending" {
		t.Fatalf("workflow next state = %q, want %q", next, "lookup_pending")
	}
}

func TestSetFireReturnsErrorForUnknownChart(t *testing.T) {
	set := NewSet(agent.Definition{}, workflow.Definition{})

	_, err := set.Fire("missing", "idle", "noop")
	if err == nil {
		t.Fatal("expected error for unknown chart")
	}
}
