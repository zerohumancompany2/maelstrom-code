package providers

import (
	"fmt"

	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/context"
)

type ProviderResponse interface {
	ToSessionItems() []internal.SessionItem
}

type Provider interface {
	Send(*context.InferenceBundle) (ProviderResponse, error)
}

func GetProviderByName(n string) (Provider, error) {
	switch n {
	case "openrouter":
		return &OpenRouterAPI{}, nil
	default:
		return nil, fmt.Errorf("Could not find provider %v", n)
	}
}
