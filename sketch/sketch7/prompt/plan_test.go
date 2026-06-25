package prompt

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
)

func TestBuildProjectionPlanBuildsKnownProjectionTypes(t *testing.T) {
	agentDef := defs.AgentDefinition{Context: defs.ContextDefinition{Projections: []defs.ProjectionDefinition{
		{Type: "system", Name: "system", Prompt: "You are helpful."},
		{Type: "binding"},
		{Type: "state_task"},
		{Type: "interaction"},
		{Type: "messages"},
	}}}

	projections, maxHistory, err := BuildProjectionPlan(agentDef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projections) != 1 {
		t.Fatalf("got %d projections, want 1 context projection", len(projections))
	}
	if maxHistory != 12 {
		t.Fatalf("MaxHistory = %d, want 12", maxHistory)
	}
}

func TestBuildProjectionPlanRejectsOldStateProjectionTypes(t *testing.T) {
	for _, projectionType := range []string{"cognitive_state", "workflow_state"} {
		agentDef := defs.AgentDefinition{Context: defs.ContextDefinition{Projections: []defs.ProjectionDefinition{{Type: projectionType}}}}
		_, _, err := BuildProjectionPlan(agentDef)
		if err == nil {
			t.Fatalf("expected error for removed projection type %q", projectionType)
		}
	}
}

func TestBuildProjectionPlanRejectsUnknownProjectionType(t *testing.T) {
	agentDef := defs.AgentDefinition{Context: defs.ContextDefinition{Projections: []defs.ProjectionDefinition{{Type: "mystery"}}}}

	_, _, err := BuildProjectionPlan(agentDef)
	if err == nil {
		t.Fatal("expected error for unknown projection type")
	}
}
