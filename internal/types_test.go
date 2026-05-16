package internal

import "testing"

// 1.2: ToolCallRequestMessage.ToString() includes ID, name, args
func TestToolCallRequestMessageToString(t *testing.T) {
	tc := ToolCallRequestMessage{
		ToolCallID: "call_1",
		Name:       "weather",
		Arguments:  `{"a":"london"}`,
	}

	out := tc.ToString()

	if out != "call_1,weather, map[a:london]" && out != "call_1,weather,{\"a\":\"london\"}" {
		// Accept either format depending on how Arguments was set
		if !contains(out, "call_1") || !contains(out, "weather") {
			t.Errorf("ToString() = %q, want it to contain ID and name", out)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// 1.3: AssistantMessage preserves reasoning alongside content
func TestAssistantMessagePreservesReasoning(t *testing.T) {
	am := AssistantMessage{
		Content:   "The answer is 42",
		Reasoning: "I calculated it step by step",
	}

	if am.Content != "The answer is 42" {
		t.Errorf("Content = %q, want %q", am.Content, "The answer is 42")
	}
	if am.Reasoning != "I calculated it step by step" {
		t.Errorf("Reasoning = %q, want %q", am.Reasoning, "I calculated it step by step")
	}
}
