package providers

import (
	"fmt"

	"github.com/comalice/inference_sketch/internal"
)

// ProviderOptions carries per-request configuration (model, temperature, tools).
type ProviderOptions struct {
	Model       string
	Temperature float64
	Tools       []internal.ToolDefinition
}

type ProviderResponse interface {
	ToSessionItems() []internal.SessionItem
}

type Provider interface {
	Send(messages []internal.SessionItem, opts ProviderOptions) (ProviderResponse, error)
}

func GetProviderByName(n string) (Provider, error) {
	switch n {
	case "openrouter":
		return &OpenRouterAPI{}, nil
	default:
		return nil, fmt.Errorf("Could not find provider %v", n)
	}
}
