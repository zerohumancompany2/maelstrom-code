package statecharts

import (
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
)

func TestMachineInitialStateFallsBackToIdle(t *testing.T) {
	machine := Compile("agent", defs.StatechartDefinition{})
	if machine.InitialState() != "idle" {
		t.Fatalf("InitialState = %q, want idle", machine.InitialState())
	}
}

func TestMachineNextReturnsLegalTransition(t *testing.T) {
	machine := Compile("agent", defs.StatechartDefinition{
		InitialState: "observe",
		Transitions: []defs.TransitionDefinition{{
			Trigger: "begin_action",
			From:    "observe",
			To:      "act",
		}},
	})

	next, err := machine.Next("observe", "begin_action")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != "act" {
		t.Fatalf("Next = %q, want act", next)
	}
}

func TestMachineNextUsesInitialStateWhenCurrentEmpty(t *testing.T) {
	machine := Compile("workflow", defs.StatechartDefinition{
		InitialState: "chatting",
		Transitions: []defs.TransitionDefinition{{
			Trigger: "commit_plan",
			From:    "chatting",
			To:      "planning",
		}},
	})

	next, err := machine.Next("", "commit_plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != "planning" {
		t.Fatalf("Next = %q, want planning", next)
	}
}

func TestMachineNextErrorsOnIllegalTrigger(t *testing.T) {
	machine := Compile("agent", defs.StatechartDefinition{
		InitialState: "observe",
		Transitions: []defs.TransitionDefinition{{
			Trigger: "begin_action",
			From:    "observe",
			To:      "act",
		}},
	})

	_, err := machine.Next("observe", "bad_trigger")
	if err == nil {
		t.Fatal("expected error for illegal trigger")
	}
	if !strings.Contains(err.Error(), "cannot fire trigger") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMachineNextErrorsOnUnknownSourceState(t *testing.T) {
	machine := Compile("workflow", defs.StatechartDefinition{
		InitialState: "chatting",
		Transitions: []defs.TransitionDefinition{{
			Trigger: "commit_plan",
			From:    "chatting",
			To:      "planning",
		}},
	})

	_, err := machine.Next("implementing", "commit_plan")
	if err == nil {
		t.Fatal("expected error for unknown source state")
	}
	if !strings.Contains(err.Error(), "has no transitions from state") {
		t.Fatalf("unexpected error: %v", err)
	}
}
