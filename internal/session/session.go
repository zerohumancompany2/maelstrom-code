package session

import (
	"fmt"

	"github.com/comalice/inference_sketch/internal"
)

type Session struct {
	Items []internal.SessionItem
}

func New() Session {
	return Session{}
}

func (s *Session) Append(i internal.SessionItem) error {
	s.Items = append(s.Items, i)
	return nil
}

func (s *Session) AppendMany(i ...internal.SessionItem) error {
	for _, item := range i {
		s.Items = append(s.Items, item)
	}
	return nil
}

func (s *Session) PrettyPrintAll() string {
	out := ""
	for _, item := range s.Items {
		switch v := item.(type) {
		case internal.UserMessage:
			out += "User: " + v.Content + "\n"
		case internal.AssistantMessage:
			out += "Reasoning: " + v.Reasoning + "\n"
			out += "Assistant: " + v.Content + "\n"
		case internal.ToolCallRequestMessage:
			out += "Tool Call Request: " + v.Name + "\n"
		case internal.ToolCallResultMessage:
			out += "Tool Call Result: " + v.Content + "\n"
		case internal.SystemMessage:
			out += "System Message: " + v.Content + "\n"

		}
	}
	return out
}

func PrettyPrintItem(si internal.SessionItem) string {
	switch v := si.(type) {
	case internal.UserMessage:
		return v.Content
	case internal.AssistantMessage:
		return fmt.Sprintf("Reasoning: %v\nAssistant: %v\n", v.Reasoning, v.Content)
	case internal.ToolCallRequestMessage:
		return fmt.Sprintf("Tool Call Request:\n\t%v\n\t%v\n\t%v\n", v.Name, v.ToolCallID, v.Arguments)
	case internal.SystemMessage:
		return v.Content
	case internal.ToolCallResultMessage:
		return fmt.Sprintf("Tool Call Result:\n\t%v\n\t%v", v.ToolCallID, v.Content)
	default:
		return ""
	}
}

func NewUserMessage(s string) internal.UserMessage {
	return internal.UserMessage{Content: s}
}
