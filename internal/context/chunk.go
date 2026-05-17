package context

import (
	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/session"
)

type ContextChunk interface {
	Build(s session.Session) []internal.SessionItem
}

type SystemChunk struct{}

func (sc SystemChunk) Build(s session.Session) []internal.SessionItem {
	return []internal.SessionItem{
		internal.SystemMessage{Content: "You are a helpful assistant."},
	}
}

type MessagesChunk struct{}

func (mc MessagesChunk) Build(s session.Session) []internal.SessionItem {
	return s.Items
}
