package context_test

import (
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/context"
)

func TestSessionItemToContextItemPreservesType(t *testing.T) {
	item, err := context.SessionItemToContextItem(internal.AssistantMessage{Content: "answer", Reasoning: "because"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, ok := item.(context.ContextAssistantMessage)
	if !ok {
		t.Fatalf("got %T, want ContextAssistantMessage", item)
	}
	if msg.Content != "answer" {
		t.Fatalf("Content = %q, want %q", msg.Content, "answer")
	}
	if msg.Reasoning != "because" {
		t.Fatalf("Reasoning = %q, want %q", msg.Reasoning, "because")
	}
}

type unsupportedSessionItem struct{ internal.SessionItem }

func TestSessionItemToContextItemUnsupportedType(t *testing.T) {
	_, err := context.SessionItemToContextItem(unsupportedSessionItem{})
	if err == nil {
		t.Fatal("expected error for unsupported session item type")
	}
	if !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHeuristicTokenizerCountsContentTokens(t *testing.T) {
	tokenizer := &context.HeuristicTokenizer{}
	count := tokenizer.CountTokens(context.ContextUserMessage{Content: "1234"}, context.ContextSystemMessage{Content: "12345"})

	if count != 3 {
		t.Fatalf("got %d tokens, want 3", count)
	}
}

func TestHardTruncateKeepsNewestItemsInOrder(t *testing.T) {
	items := []context.ContextItem{
		context.ContextUserMessage{Content: "1234"},
		context.ContextUserMessage{Content: "5678"},
		context.ContextUserMessage{Content: "9abc"},
	}

	trimmed, err := (&context.HardTruncate{}).Trim(items, 2, &context.HeuristicTokenizer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(trimmed) != 2 {
		t.Fatalf("got %d items, want 2", len(trimmed))
	}
	first := trimmed[0].(context.ContextUserMessage)
	second := trimmed[1].(context.ContextUserMessage)
	if first.Content != "5678" || second.Content != "9abc" {
		t.Fatalf("got [%q, %q], want [5678, 9abc]", first.Content, second.Content)
	}
}

func TestFailTruncateErrorsWhenOverBudget(t *testing.T) {
	_, err := (&context.FailTruncate{}).Trim([]context.ContextItem{context.ContextSystemMessage{Content: "12345"}}, 1, &context.HeuristicTokenizer{})
	if err == nil {
		t.Fatal("expected error when context exceeds budget")
	}
}
