package agent

import (
	"testing"

	"github.com/comalice/inference_sketch/internal/context"
	"github.com/comalice/inference_sketch/internal/tools"
)

func TestBuildContextDefinitionSetsBudgetedChunks(t *testing.T) {
	a := Agent{
		Model:        Model{Name: "test-model", ContextLength: 2048},
		SystemPrompt: "be nice",
	}

	def := a.BuildContextDefinition(tools.NewRegistry(tools.WeatherTool{}))

	if def.ContextLimit != 2048 {
		t.Fatalf("ContextLimit = %d, want 2048", def.ContextLimit)
	}
	if len(def.Chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(def.Chunks))
	}
	if _, ok := def.Chunks[0].Policy.(*context.FailTruncate); !ok {
		t.Fatalf("expected system chunk to use FailTruncate, got %T", def.Chunks[0].Policy)
	}
	if _, ok := def.Chunks[1].Policy.(*context.HardTruncate); !ok {
		t.Fatalf("expected messages chunk to use HardTruncate, got %T", def.Chunks[1].Policy)
	}
	if !def.Chunks[1].Flexible {
		t.Fatal("expected messages chunk to be flexible")
	}
}
