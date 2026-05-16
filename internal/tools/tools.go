package tools

import (
	"encoding/json"
	"fmt"

	"github.com/comalice/inference_sketch/internal"
)

// Dispatch executes a tool call through the registry and returns a result message.
// It handles argument parsing and error wrapping.
func Dispatch(reg *Registry, tc internal.ToolCallRequestMessage) internal.ToolCallResultMessage {
	args, err := parseArguments(tc.Arguments)
	if err != nil {
		return internal.ToolCallResultMessage{
			ToolCallID: tc.ToolCallID,
			Content:    fmt.Sprintf("error parsing arguments: %v", err),
		}
	}

	content, err := reg.Execute(tc.Name, args)
	if err != nil {
		return internal.ToolCallResultMessage{
			ToolCallID: tc.ToolCallID,
			Content:    fmt.Sprintf("tool error: %v", err),
		}
	}

	return internal.ToolCallResultMessage{
		ToolCallID: tc.ToolCallID,
		Content:    content,
	}
}

// parseArguments converts raw arguments (any) into a map[string]any.
func parseArguments(raw any) (map[string]any, error) {
	switch v := raw.(type) {
	case map[string]any:
		return v, nil
	case string:
		var out map[string]any
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil, fmt.Errorf("invalid JSON arguments: %w", err)
		}
		return out, nil
	case []byte:
		var out map[string]any
		if err := json.Unmarshal(v, &out); err != nil {
			return nil, fmt.Errorf("invalid JSON arguments: %w", err)
		}
		return out, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("cannot marshal arguments: %w", err)
		}
		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, fmt.Errorf("invalid arguments structure: %w", err)
		}
		return out, nil
	}
}
