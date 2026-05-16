package providers

import (
	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/context"
)

type ProviderResponse interface {
	ToSessionItems() []internal.SessionItem
}

type Provider interface {
	Send(*context.InferenceBundle) (ProviderResponse, error)
}
