package tools

import (
	"testing"

	"github.com/comalice/inference_sketch/internal"
)

// 6.1: WeatherTool implements Tool interface
var _ Tool = WeatherTool{}

// 6.2: WeatherTool returns correct definition
func TestWeatherToolDefinition(t *testing.T) {
	def := WeatherTool{}.Definition()
	if def.Name != "weather" {
		t.Errorf("Name = %q, want %q", def.Name, "weather")
	}
	if !def.Strict {
		t.Error("Strict = false, want true")
	}
}

// 6.3: WeatherTool executes successfully
func TestWeatherToolExecute(t *testing.T) {
	content, err := WeatherTool{}.Execute(map[string]any{"location": "london"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
}

// 6.4: WeatherTool returns error for missing argument
func TestWeatherToolMissingArg(t *testing.T) {
	_, err := WeatherTool{}.Execute(map[string]any{})
	if err == nil {
		t.Error("expected error for missing location")
	}
}

// 6.5: NewRegistry registers tools
func TestNewRegistry(t *testing.T) {
	reg := NewRegistry(WeatherTool{})
	if len(reg.Definitions()) != 1 {
		t.Fatalf("got %d definitions, want 1", len(reg.Definitions()))
	}
	if reg.Definitions()[0].Name != "weather" {
		t.Errorf("Name = %q, want %q", reg.Definitions()[0].Name, "weather")
	}
}

// 6.6: Registry.Execute dispatches correctly
func TestRegistryExecute(t *testing.T) {
	reg := NewRegistry(WeatherTool{})
	content, err := reg.Execute("weather", map[string]any{"location": "paris"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
}

// 6.7: Registry.Execute returns error for unknown tool
func TestRegistryUnknownTool(t *testing.T) {
	reg := NewRegistry(WeatherTool{})
	_, err := reg.Execute("nonexistent", nil)
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

// 6.8: Dispatch handles JSON string arguments
func TestDispatchJSONArgs(t *testing.T) {
	reg := NewRegistry(WeatherTool{})
	tc := internal.ToolCallRequestMessage{
		ToolCallID: "c1",
		Name:       "weather",
		Arguments:  `{"location":"tokyo"}`,
	}
	result := Dispatch(reg, tc)
	if result.ToolCallID != "c1" {
		t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, "c1")
	}
	if result.Content == "" {
		t.Error("expected non-empty content")
	}
}

// 6.9: Dispatch handles map arguments
func TestDispatchMapArgs(t *testing.T) {
	reg := NewRegistry(WeatherTool{})
	tc := internal.ToolCallRequestMessage{
		ToolCallID: "c1",
		Name:       "weather",
		Arguments:  map[string]any{"location": "tokyo"},
	}
	result := Dispatch(reg, tc)
	if result.ToolCallID != "c1" {
		t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, "c1")
	}
	if result.Content == "" {
		t.Error("expected non-empty content")
	}
}

// 6.10: Dispatch wraps tool errors in result content
func TestDispatchWrapsError(t *testing.T) {
	reg := NewRegistry(WeatherTool{})
	tc := internal.ToolCallRequestMessage{
		ToolCallID: "c1",
		Name:       "weather",
		Arguments:  map[string]any{},
	}
	result := Dispatch(reg, tc)
	if result.Content == "" {
		t.Error("expected error content, got empty")
	}
}

// 6.11: Dispatch preserves ToolCallID
func TestDispatchPreservesID(t *testing.T) {
	reg := NewRegistry(WeatherTool{})
	tc := internal.ToolCallRequestMessage{
		ToolCallID: "call_xyz",
		Name:       "weather",
		Arguments:  map[string]any{"location": "nyc"},
	}
	result := Dispatch(reg, tc)
	if result.ToolCallID != "call_xyz" {
		t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, "call_xyz")
	}
}
