package context

import (
	"strings"
	"testing"

	"github.com/comalice/maelstrom/internal/logs"
)

func TestTrimMessagesKeepsLatestUserAnchor(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "ack"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "reply"},
	}
	trimmed := trimMessages(messages, 2)
	if len(trimmed) != 2 {
		t.Fatalf("got %d messages, want 2", len(trimmed))
	}
	if trimmed[0].Role != "user" || trimmed[0].Content != "second" {
		t.Fatalf("first trimmed message = %+v, want latest user message", trimmed[0])
	}
	if trimmed[1].Role != "assistant" || trimmed[1].Content != "reply" {
		t.Fatalf("second trimmed message = %+v, want latest assistant tail", trimmed[1])
	}
}

func TestTrimMessagesPrefersLatestCoherentTail(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "find replace_text"},
		{Role: "assistant_tool_call", Name: "search_files", CallID: "call-1", Content: `{"pattern":"replace_text"}`},
		{Role: "tool", Name: "search_files", CallID: "call-1", Content: "Backend: rg\nmatch"},
		{Role: "assistant", Content: "Here is the answer."},
	}
	trimmed := trimMessages(messages, 2)
	if len(trimmed) != 2 {
		t.Fatalf("got %d messages, want 2", len(trimmed))
	}
	if trimmed[0].Role != "user" || trimmed[1].Role != "assistant" {
		t.Fatalf("trimmed roles = %q, %q, want latest user/assistant tail", trimmed[0].Role, trimmed[1].Role)
	}
	if trimmed[0].Content != "find replace_text" || trimmed[1].Content != "Here is the answer." {
		t.Fatalf("trimmed contents = %+v, want coherent user/assistant tail", trimmed)
	}
}

func TestBuildTranscriptAddsUserCorrectionForInvalidStateRetry(t *testing.T) {
	history := logs.NewSessionHistory("session-retry-transcript")
	history.Append(logs.UserMessageRecord{SessionBaseRecord: history.NextRecord("user"), Content: "Summarize."})
	history.Append(logs.AssistantMessageRecord{SessionBaseRecord: history.NextRecord("assistant"), Content: "I will inspect."})
	history.Append(logs.RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "invalid_state_output", Attempt: 1})

	messages := buildTranscript(history)
	if len(messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(messages))
	}
	last := messages[len(messages)-1]
	if last.Role != "user" || last.Kind != "retry" {
		t.Fatalf("last message = %+v, want user retry correction", last)
	}
	if !strings.Contains(last.Content, "Return JSON only") {
		t.Fatalf("retry correction = %q, want JSON-only guidance", last.Content)
	}
}
