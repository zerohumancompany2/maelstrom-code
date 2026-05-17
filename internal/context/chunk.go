package context

import (
	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/session"
)

type ContextChunk interface {
	Build(s session.Session) ([]internal.SessionItem, error)
}

type SystemChunk struct {
	Prompt string
}

func (sc SystemChunk) Build(s session.Session) ([]internal.SessionItem, error) {
	prompt := sc.Prompt
	if prompt == "" {
		prompt = "You are a helpful assistant." // TODO lift validation
	}

	return []internal.SessionItem{
		internal.SystemMessage{Content: prompt},
	}, nil
}

type MessagesChunk struct{}

func (mc MessagesChunk) Build(s session.Session) ([]internal.SessionItem, error) {
	return s.Items, nil
}
