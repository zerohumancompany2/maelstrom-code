package tools

import (
	"fmt"

	"github.com/comalice/inference_sketch/internal"
)

// WeatherTool provides weather information for a location.
type WeatherTool struct{}

var _ Tool = WeatherTool{}

func (w WeatherTool) Definition() internal.ToolDefinition {
	return internal.ToolDefinition{
		Name:        "weather",
		Description: "Get the latest weather for a location.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{
					"type":        "string",
					"description": "location to query for",
				},
			},
			"required":             []string{"location"},
			"additionalProperties": false,
		},
		Strict: true,
	}
}

func (w WeatherTool) Execute(args map[string]any) (string, error) {
	loc, ok := args["location"].(string)
	if !ok {
		return "", fmt.Errorf("weather: missing or invalid location argument")
	}
	return fmt.Sprintf("70C, rainy, winds out of SSW in %s", loc), nil
}
