package prompt

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
)

func TestBuildProjectionPlanBuildsKnownProjectionTypes(t *testing.T) {
	agentDef := defs.AgentDefinition{Context: defs.ContextDefinition{Projections: []defs.ProjectionDefinition{
		{Type: "system", Name: "system", Prompt: "You are helpful."},
		{Type: "binding"},
		{Type: "workflow_state"},
		{Type: "cognitive_state"},
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

func TestBuildProjectionPlanRejectsUnknownProjectionType(t *testing.T) {
	agentDef := defs.AgentDefinition{Context: defs.ContextDefinition{Projections: []defs.ProjectionDefinition{{Type: "mystery"}}}}

	_, _, err := BuildProjectionPlan(agentDef)
	if err == nil {
		t.Fatal("expected error for unknown projection type")
	}
}
