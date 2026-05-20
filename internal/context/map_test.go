package context

import (
	"testing"

	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/session"
)

// 3.1: Empty session → empty messages
func TestBuildMessagesEmpty(t *testing.T) {
	cm := ContextMap{Model: "test-model"}
	s := session.New()

	messages, err := cm.BuildMessages(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("got %d messages, want 0", len(messages))
	}
}

// 3.2: All session items appear in messages
func TestBuildMessagesFidelity(t *testing.T) {
	cm := ContextMap{
		Model: "test-model",
		Definition: ContextDefinition{
			Chunks: []ContextChunk{
				MessagesChunk{},
			},
		},
	}
	s := session.New()
	s.Append(session.NewUserMessage("first"))
	s.Append(session.NewUserMessage("second"))

	messages, err := cm.BuildMessages(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(messages))
	}
}

// 3.3: NewFromDefinition copies, doesn't alias
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
