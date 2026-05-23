package context

import (
	"github.com/comalice/inference_sketch/internal/session"
)

type ContextChunk interface {
	Build(s session.Session) ([]ContextItem, error)
}

type SystemChunk struct {
	Prompt string
}

func (sc SystemChunk) Build(s session.Session) ([]ContextItem, error) {
	prompt := sc.Prompt
	if prompt == "" {
		prompt = "You are a helpful assistant." // TODO lift validation
	}

	return []ContextItem{
		ContextSystemMessage{Content: prompt},
	}, nil
}

type MessagesChunk struct{}

func (mc MessagesChunk) Build(s session.Session) ([]ContextItem, error) {
	items := make([]ContextItem, 0, len(s.Items))
	for _, item := range s.Items {
		contextItem, err := SessionItemToContextItem(item)
		if err != nil {
			return nil, err
		}
		items = append(items, contextItem)
	}
	return items, nil
}
