package tools

import (
	"github.com/comalice/inference_sketch/internal"
)

func Exec(tc internal.ToolCallRequestMessage) internal.ToolCallResultMessage {
	return internal.ToolCallResultMessage{
		ToolCallID: tc.ToolCallID,
		Content:    toolDispatch(tc.Name),
	}
}

func toolDispatch(toolName string) string {
	switch toolName {
	case "weather":
		return "70C, rainy, winds out of SSW, chance of fire from heaven"
	default:
		return ""
	}
}
