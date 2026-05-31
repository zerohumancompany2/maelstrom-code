package assembly

import (
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/charts"
	"github.com/comalice/inference_sketch/sketch/sketch6/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
)

func TestBuildPlanIncludesNamedChunksAndHistoryBudget(t *testing.T) {
	plan, err := BuildPlan(agent.Definition{Context: agent.ContextDefinition{Chunks: []agent.ChunkDefinition{{Type: "system", Prompt: "system"}, {Type: "messages"}, {Type: "cognitive_state"}, {Type: "workflow_state"}, {Type: "binding"}, {Type: "state", Chart: "agent"}}}})
	if err != nil {
		t.Fatalf("build plan returned error: %v", err)
	}

	if plan.MaxHistoryItems != 10 {
		t.Fatalf("maxHistoryItems = %d, want 10", plan.MaxHistoryItems)
	}
	if got := len(plan.Assembler.Chunks); got != 6 {
		t.Fatalf("chunk count = %d, want 6", got)
	}
	if got := plan.Assembler.Chunks[2].Name(); got != "cognitive-state" {
		t.Fatalf("chunk[2] name = %q, want cognitive-state", got)
	}
	if got := plan.Assembler.Chunks[3].Name(); got != "workflow-state" {
		t.Fatalf("chunk[3] name = %q, want workflow-state", got)
	}
	if got := plan.Assembler.Chunks[4].Name(); got != "binding" {
		t.Fatalf("chunk[4] name = %q, want binding", got)
	}
}

func TestBuildPlanRejectsUnknownChunkType(t *testing.T) {
	_, err := BuildPlan(agent.Definition{Context: agent.ContextDefinition{Chunks: []agent.ChunkDefinition{{Type: "mystery"}}}})
	if err == nil {
		t.Fatal("expected error for unknown chunk type")
	}
	if _, ok := err.(ErrUnknownChunkType); !ok {
		t.Fatalf("error type = %T, want ErrUnknownChunkType", err)
	}
}

func TestNamedChunkProjectionCarriesDetailedState(t *testing.T) {
	input := Input{
		RuntimeView: runtime.RuntimeView{
			Cognitive: runtime.CognitiveSnapshot{CurrentState: "act", Prompt: "Use tools carefully.", VisibleTools: []string{"transition_state", "weather"}, EnabledTools: []string{"weather"}},
			Binding:   runtime.BindingSnapshot{WorkflowID: "workflow-001", Bound: true},
			Workflow:  &runtime.WorkflowSnapshot{WorkflowID: "workflow-001", CurrentState: "lookup_pending", Description: "Look up weather", Context: "Paris", VisibleTools: []string{"weather"}, EnabledTools: []string{"weather"}},
		},
		History: session.NewHistory("session-1"),
		Charts:  charts.Snapshot{States: map[string]string{"agent": "act"}},
	}

	cognitive, err := (CognitiveStateChunk{}).Build(input)
	if err != nil {
		t.Fatalf("build cognitive chunk: %v", err)
	}
	workflowChunk, err := (WorkflowStateChunk{}).Build(input)
	if err != nil {
		t.Fatalf("build workflow chunk: %v", err)
	}
	binding, err := (BindingChunk{}).Build(input)
	if err != nil {
		t.Fatalf("build binding chunk: %v", err)
	}

	cognitiveText := cognitive.Segments[0].TokenText()
	if !strings.Contains(cognitiveText, "Prompt: Use tools carefully.") || !strings.Contains(cognitiveText, "Visible tools: transition_state, weather") || !strings.Contains(cognitiveText, "Enabled tools: weather") {
		t.Fatalf("cognitive text = %q, want prompt and tool details", cognitiveText)
	}
	workflowText := workflowChunk.Segments[0].TokenText()
	if !strings.Contains(workflowText, "Workflow workflow-001 is in state lookup_pending") || !strings.Contains(workflowText, "Context: Paris") {
		t.Fatalf("workflow text = %q, want workflow details", workflowText)
	}
	bindingText := binding.Segments[0].TokenText()
	if bindingText != "Active binding: workflow=workflow-001." {
		t.Fatalf("binding text = %q, want active binding text", bindingText)
	}
}
