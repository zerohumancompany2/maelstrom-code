package session

import (
	"testing"

	"github.com/comalice/inference_sketch/internal"
)

// 2.1: New() produces empty session
func TestNewEmpty(t *testing.T) {
	s := New()
	if len(s.Items) != 0 {
		t.Errorf("New() produced %d items, want 0", len(s.Items))
	}
}

// 2.2: Append() preserves insertion order
func TestAppendPreservesOrder(t *testing.T) {
	s := New()
	s.Append(NewUserMessage("first"))
	s.Append(NewUserMessage("second"))
	s.Append(NewUserMessage("third"))

	if len(s.Items) != 3 {
		t.Fatalf("got %d items, want 3", len(s.Items))
	}
	if s.Items[0].(internal.UserMessage).Content != "first" {
		t.Errorf("items[0] = %q, want %q", s.Items[0].(internal.UserMessage).Content, "first")
	}
	if s.Items[2].(internal.UserMessage).Content != "third" {
		t.Errorf("items[2] = %q, want %q", s.Items[2].(internal.UserMessage).Content, "third")
	}
}

// 2.3: AppendMany() ≡ N Append() calls
func TestAppendManyEquivalent(t *testing.T) {
	s1 := New()
	s1.Append(NewUserMessage("a"))
	s1.Append(NewUserMessage("b"))
	s1.Append(NewUserMessage("c"))

	s2 := New()
	s2.AppendMany(NewUserMessage("a"), NewUserMessage("b"), NewUserMessage("c"))

	if len(s1.Items) != len(s2.Items) {
		t.Fatalf("AppendMany: got %d items, want %d", len(s2.Items), len(s1.Items))
	}
	for i := range s1.Items {
		c1 := s1.Items[i].(internal.UserMessage).Content
		c2 := s2.Items[i].(internal.UserMessage).Content
		if c1 != c2 {
			t.Errorf("items[%d]: got %q, want %q", i, c2, c1)
		}
	}
}

// 2.4: Mixed item types coexist
func TestMixedItemTypes(t *testing.T) {
	s := New()
	s.Append(internal.UserMessage{Content: "hello"})
	s.Append(internal.AssistantMessage{Content: "hi back"})
	s.Append(internal.ToolCallRequestMessage{ToolCallID: "c1", Name: "weather"})
	s.Append(internal.ToolCallResultMessage{ToolCallID: "c1", Content: "sunny"})
	s.Append(internal.SystemMessage{Content: "be nice"})

	if len(s.Items) != 5 {
		t.Fatalf("got %d items, want 5", len(s.Items))
	}

	_, ok := s.Items[0].(internal.UserMessage)
	if !ok {
		t.Errorf("items[0] wrong type: %T", s.Items[0])
	}
	_, ok = s.Items[1].(internal.AssistantMessage)
	if !ok {
		t.Errorf("items[1] wrong type: %T", s.Items[1])
	}
	_, ok = s.Items[2].(internal.ToolCallRequestMessage)
	if !ok {
		t.Errorf("items[2] wrong type: %T", s.Items[2])
	}
	_, ok = s.Items[3].(internal.ToolCallResultMessage)
	if !ok {
		t.Errorf("items[3] wrong type: %T", s.Items[3])
	}
	_, ok = s.Items[4].(internal.SystemMessage)
	if !ok {
		t.Errorf("items[4] wrong type: %T", s.Items[4])
	}
}

// 2.5: NewUserMessage() wraps correctly
func TestNewUserMessage(t *testing.T) {
	um := NewUserMessage("test input")
	if um.Content != "test input" {
		t.Errorf("NewUserMessage: got %q, want %q", um.Content, "test input")
	}
}
