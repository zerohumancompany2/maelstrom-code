package tools

import (
	"fmt"

	"github.com/comalice/inference_sketch/internal"
)

// Tool is a callable capability exposed to the model.
type Tool interface {
	// Definition returns the tool's schema for API consumption.
	Definition() internal.ToolDefinition
	// Execute runs the tool with parsed arguments.
	Execute(args map[string]any) (string, error)
}

// Registry holds discoverable tools and dispatches calls.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a registry from concrete tool implementations.
func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		r.tools[t.Definition().Name] = t
	}
	return r
}

// Definitions returns all tool definitions for inclusion in inference bundles.
func (r *Registry) Definitions() []internal.ToolDefinition {
	defs := make([]internal.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// Execute looks up a tool by name and runs it with the given arguments.
func (r *Registry) Execute(name string, args map[string]any) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(args)
}
