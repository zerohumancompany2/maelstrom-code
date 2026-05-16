package context

import (
	"testing"

	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/session"
)

// 3.1: Empty session → empty message bundle
func TestBuildInferenceBundleEmpty(t *testing.T) {
	cm := ContextMap{Model: "test-model"}
	s := session.New()

	bundle, err := cm.BuildInferenceBundle(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bundle.Messages) != 0 {
		t.Errorf("got %d messages, want 0", len(bundle.Messages))
	}
}

// 3.2: All session items appear in bundle
func TestBuildInferenceBundleFidelity(t *testing.T) {
	cm := ContextMap{Model: "test-model"}
	s := session.New()
	s.Append(session.NewUserMessage("first"))
	s.Append(session.NewUserMessage("second"))

	bundle, err := cm.BuildInferenceBundle(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bundle.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(bundle.Messages))
	}
}

// 3.3: Bundle.Model and Bundle.Tools propagate
func TestBuildInferenceBundlePropagatesConfig(t *testing.T) {
	tool := internal.ToolDefinition{Name: "weather"}
	cm := ContextMap{Model: "my-model", Tools: []internal.ToolDefinition{tool}}
	s := session.New()

	bundle, err := cm.BuildInferenceBundle(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bundle.Model != "my-model" {
		t.Errorf("Model = %q, want %q", bundle.Model, "my-model")
	}
	if len(bundle.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(bundle.Tools))
	}
	if bundle.Tools[0].Name != "weather" {
		t.Errorf("Tools[0].Name = %q, want %q", bundle.Tools[0].Name, "weather")
	}
}

// 3.4: NewFromDefinition copies, doesn't alias
func TestNewFromDefinitionNoAlias(t *testing.T) {
	cd := ContextDefinition{
		Model: "original",
		Tools: []internal.ToolDefinition{{Name: "weather"}},
	}
	cm := NewFromDefinition(cd)

	// Mutate original
	cd.Model = "mutated"
	cd.Tools = append(cd.Tools, internal.ToolDefinition{Name: "extra"})

	if cm.Model != "original" {
		t.Errorf("Model = %q, want %q (aliasing original)", cm.Model, "original")
	}
	if len(cm.Tools) != 1 {
		t.Errorf("got %d tools, want 1 (aliasing original)", len(cm.Tools))
	}
}
