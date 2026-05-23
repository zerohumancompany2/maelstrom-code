package context

import (
	"fmt"
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
		Model:        "test-model",
		ContextLimit: 100,
		Tokenizer:    &HeuristicTokenizer{},
		Definition: ContextDefinition{
			Chunks: []ChunkSpec{
				{Chunk: MessagesChunk{}, BudgetPct: 1.0, Policy: &HardTruncate{}, Priority: 1, Flexible: true},
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
	first, ok := messages[0].(ContextUserMessage)
	if !ok || first.Content != "first" {
		t.Fatalf("messages[0] = %#v, want ContextUserMessage(first)", messages[0])
	}
}

// 3.3: NewFromDefinition copies top-level slices and config values
func TestNewFromDefinitionNoAlias(t *testing.T) {
	cd := ContextDefinition{
		Model:        "original",
		Tools:        []internal.ToolDefinition{{Name: "weather"}},
		Chunks:       []ChunkSpec{{Chunk: MessagesChunk{}, BudgetPct: 1.0, Policy: &HardTruncate{}, Priority: 1, Flexible: true}},
		ContextLimit: 42,
	}
	cm := NewFromDefinition(cd)

	// Mutate original
	cd.Model = "mutated"
	cd.Tools = append(cd.Tools, internal.ToolDefinition{Name: "extra"})
	cd.Chunks = append(cd.Chunks, ChunkSpec{Chunk: SystemChunk{Prompt: "x"}})
	cd.ContextLimit = 100

	if cm.Model != "original" {
		t.Errorf("Model = %q, want %q (aliasing original)", cm.Model, "original")
	}
	if len(cm.Tools) != 1 {
		t.Errorf("got %d tools, want 1 (aliasing original)", len(cm.Tools))
	}
	if len(cm.Definition.Chunks) != 1 {
		t.Errorf("got %d chunks, want 1 (aliasing original)", len(cm.Definition.Chunks))
	}
	if cm.ContextLimit != 42 {
		t.Errorf("ContextLimit = %d, want 42", cm.ContextLimit)
	}
}

func TestBuildMessagesRejectsInvalidContextLimit(t *testing.T) {
	cm := ContextMap{
		Model:        "test-model",
		ContextLimit: 0,
		Tokenizer:    &HeuristicTokenizer{},
		Definition: ContextDefinition{
			Chunks: []ChunkSpec{{Chunk: MessagesChunk{}, BudgetPct: 1.0, Policy: &HardTruncate{}, Priority: 1, Flexible: true}},
		},
	}

	_, err := cm.BuildMessages(session.New())
	if err == nil {
		t.Fatal("expected error for invalid context limit")
	}
}

func TestBuildMessagesAppliesChunkBudgets(t *testing.T) {
	cm := ContextMap{
		Model:        "test-model",
		ContextLimit: 4,
		Tokenizer:    &HeuristicTokenizer{},
		Definition: ContextDefinition{
			Chunks: []ChunkSpec{
				{Chunk: MessagesChunk{}, BudgetPct: 0.5, Policy: &HardTruncate{}, Priority: 1},
			},
		},
	}
	s := session.New()
	s.Append(session.NewUserMessage("1234"))
	s.Append(session.NewUserMessage("5678"))
	s.Append(session.NewUserMessage("9abc"))

	messages, err := cm.BuildMessages(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(messages))
	}
	if messages[0].(ContextUserMessage).Content != "5678" || messages[1].(ContextUserMessage).Content != "9abc" {
		t.Fatalf("got [%q, %q], want [5678, 9abc]", messages[0].(ContextUserMessage).Content, messages[1].(ContextUserMessage).Content)
	}
}

func TestBuildMessagesRebalancesByPriority(t *testing.T) {
	cm := ContextMap{
		Model:        "test-model",
		ContextLimit: 2,
		Tokenizer:    &HeuristicTokenizer{},
		Definition: ContextDefinition{
			Chunks: []ChunkSpec{
				{Chunk: SystemChunk{Prompt: "1234"}, BudgetPct: 1.0, Policy: &FailTruncate{}, Priority: 10},
				{Chunk: MessagesChunk{}, BudgetPct: 1.0, Policy: &HardTruncate{}, Priority: 1},
			},
		},
	}
	s := session.New()
	s.Append(session.NewUserMessage("5678"))
	s.Append(session.NewUserMessage("9abc"))

	messages, err := cm.BuildMessages(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(messages))
	}
	if _, ok := messages[0].(ContextSystemMessage); !ok {
		t.Fatalf("expected system message to survive rebalance, got %T", messages[0])
	}
	user, ok := messages[1].(ContextUserMessage)
	if !ok {
		t.Fatalf("expected remaining non-system message to be chat history, got %T", messages[1])
	}
	if user.Content != "9abc" {
		t.Fatalf("expected newest chat history to survive, got %q", user.Content)
	}
}

type stickyTruncate struct{}

func (st *stickyTruncate) Trim(items []ContextItem, budget int, tokenizer Tokenizer) ([]ContextItem, error) {
	if tokenizer.CountTokens(items...) > budget {
		return nil, fmt.Errorf("sticky chunk refuses truncation")
	}
	return items, nil
}

func TestBuildMessagesRebalanceUsesPolicyAndSkipsRefusals(t *testing.T) {
	cm := ContextMap{
		Model:        "test-model",
		ContextLimit: 3,
		Tokenizer:    &HeuristicTokenizer{},
		Definition: ContextDefinition{
			Chunks: []ChunkSpec{
				{Chunk: SystemChunk{Prompt: "1234"}, BudgetPct: 1.0, Policy: &FailTruncate{}, Priority: 10},
				{Chunk: MessagesChunk{}, BudgetPct: 1.0, Policy: &stickyTruncate{}, Priority: 1},
				{Chunk: staticChunk{items: []ContextItem{ContextUserMessage{Content: "def0"}}}, BudgetPct: 1.0, Policy: &HardTruncate{}, Priority: 2},
			},
		},
	}
	s := session.New()
	s.Append(session.NewUserMessage("5678"))
	s.Append(session.NewUserMessage("9abc"))

	messages, err := cm.BuildMessages(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(messages))
	}
	if messages[1].(ContextUserMessage).Content != "5678" || messages[2].(ContextUserMessage).Content != "9abc" {
		t.Fatalf("expected sticky chunk items to survive unchanged, got %#v", messages)
	}
}

type staticChunk struct {
	items []ContextItem
}

func (sc staticChunk) Build(session.Session) ([]ContextItem, error) {
	return sc.items, nil
}

func TestBuildMessagesErrorsWhenRebalanceCannotFitLimit(t *testing.T) {
	cm := ContextMap{
		Model:        "test-model",
		ContextLimit: 1,
		Tokenizer:    &HeuristicTokenizer{},
		Definition: ContextDefinition{
			Chunks: []ChunkSpec{
				{Chunk: SystemChunk{Prompt: "1234"}, BudgetPct: 1.0, Policy: &FailTruncate{}, Priority: 10},
				{Chunk: staticChunk{items: []ContextItem{ContextUserMessage{Content: "5678"}}}, BudgetPct: 1.0, Policy: &stickyTruncate{}, Priority: 1},
			},
		},
	}

	_, err := cm.BuildMessages(session.New())
	if err == nil {
		t.Fatal("expected rebalance failure when no chunk can be trimmed")
	}
}

func TestBuildMessagesFlexibleChunkUsesRemainingBudget(t *testing.T) {
	cm := ContextMap{
		Model:        "test-model",
		ContextLimit: 3,
		Tokenizer:    &HeuristicTokenizer{},
		Definition: ContextDefinition{
			Chunks: []ChunkSpec{
				{Chunk: SystemChunk{Prompt: "1234"}, BudgetPct: 1.0 / 3.0, Policy: &FailTruncate{}, Priority: 10},
				{Chunk: MessagesChunk{}, BudgetPct: 0, Policy: &HardTruncate{}, Priority: 1, Flexible: true},
			},
		},
	}
	s := session.New()
	s.Append(session.NewUserMessage("5678"))
	s.Append(session.NewUserMessage("9abc"))
	s.Append(session.NewUserMessage("def0"))

	messages, err := cm.BuildMessages(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(messages))
	}
	if messages[1].(ContextUserMessage).Content != "9abc" || messages[2].(ContextUserMessage).Content != "def0" {
		t.Fatalf("flexible chunk did not absorb remainder budget correctly")
	}
}
