package context

import (
	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/session"
)

type ContextChunk interface {
	Build(s session.Session) ([]internal.SessionItem, error)
}

type SystemChunk struct{}

func (sc SystemChunk) Build(s session.Session) ([]internal.SessionItem, error) {
	return []internal.SessionItem{
		internal.SystemMessage{Content: "You are a helpful assistant."},
	}, nil
}

type MessagesChunk struct{}

func (mc MessagesChunk) Build(s session.Session) ([]internal.SessionItem, error) {
	return s.Items, nil
}
