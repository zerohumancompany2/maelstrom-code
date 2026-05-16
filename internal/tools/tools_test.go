package tools

import (
	"testing"

	"github.com/comalice/inference_sketch/internal"
)

// 6.1: Known tool returns non-empty result
func TestExecKnownTool(t *testing.T) {
	tc := internal.ToolCallRequestMessage{
		ToolCallID: "c1",
		Name:       "weather",
		Arguments:  "{}",
	}

	result := Exec(tc)
	if result.Content == "" {
		t.Error("weather tool returned empty content")
	}
}

// 6.2: Unknown tool returns empty string
func TestExecUnknownTool(t *testing.T) {
	tc := internal.ToolCallRequestMessage{
		ToolCallID: "c1",
		Name:       "nonexistent",
		Arguments:  "{}",
	}

	result := Exec(tc)
	if result.Content != "" {
		t.Errorf("unknown tool returned %q, want empty string", result.Content)
	}
}

// 6.3: ToolCallID preserved in result
func TestExecPreservesToolCallID(t *testing.T) {
	tc := internal.ToolCallRequestMessage{
		ToolCallID: "call_xyz",
		Name:       "weather",
	}

	result := Exec(tc)
	if result.ToolCallID != "call_xyz" {
		t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, "call_xyz")
	}
}
