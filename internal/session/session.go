package session

import "github.com/comalice/inference_sketch/internal"

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

func (s *Session) PrettyPrint() string {
	out := ""
	for _, item := range s.Items {
		switch v := item.(type) {
		case internal.UserMessage:
			out += "User: " + v.Content + "\n"
		case internal.AssistantMessage:
			out += "Assistant: " + v.Content + "\n"
		case internal.ToolCallRequestMessage:
			out += "Tool Call Request: " + v.Name + "\n"
		case internal.ToolCallResultMessage:
			out += "Tool Call Result: " + v.Content + "\n"

		}
	}
	return out
}

func NewUserMessage(s string) internal.UserMessage {
	return internal.UserMessage{Content: s}
}
