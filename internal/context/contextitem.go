package context

import (
	"fmt"

	"github.com/comalice/inference_sketch/internal"
)

type ContextItem interface {
	isContextItem()
	TokenText() string
}

type ContextUserMessage struct {
	Content string
}

func (ContextUserMessage) isContextItem() {}

func (m ContextUserMessage) TokenText() string { return m.Content }

type ContextAssistantMessage struct {
	Content   string
	Reasoning string
}

func (ContextAssistantMessage) isContextItem() {}

func (m ContextAssistantMessage) TokenText() string { return m.Content + m.Reasoning }

type ContextSystemMessage struct {
	Content string
}

func (ContextSystemMessage) isContextItem() {}

func (m ContextSystemMessage) TokenText() string { return m.Content }

type ContextToolCallRequestMessage struct {
	ToolCallID string
	Name       string
	Arguments  any
	Content    string
}

func (ContextToolCallRequestMessage) isContextItem() {}

func (m ContextToolCallRequestMessage) TokenText() string { return m.Content }

type ContextToolCallResultMessage struct {
	ToolCallID string
	Content    string
}

func (ContextToolCallResultMessage) isContextItem() {}

func (m ContextToolCallResultMessage) TokenText() string { return m.Content }

func SessionItemToContextItem(item internal.SessionItem) (ContextItem, error) {
	switch msg := item.(type) {
	case internal.UserMessage:
		return ContextUserMessage{Content: msg.Content}, nil
	case internal.SystemMessage:
		return ContextSystemMessage{Content: msg.Content}, nil
	case internal.AssistantMessage:
		return ContextAssistantMessage{Content: msg.Content, Reasoning: msg.Reasoning}, nil
	case internal.ToolCallRequestMessage:
		return ContextToolCallRequestMessage{
			ToolCallID: msg.ToolCallID,
			Name:       msg.Name,
			Arguments:  msg.Arguments,
			Content:    msg.Content,
		}, nil
	case internal.ToolCallResultMessage:
		return ContextToolCallResultMessage{ToolCallID: msg.ToolCallID, Content: msg.Content}, nil
	default:
		return nil, fmt.Errorf("convert session item: unsupported type %T", item)
	}
}
