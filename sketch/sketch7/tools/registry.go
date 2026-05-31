package tools

import "fmt"

type Registry struct {
	definitions map[string]Definition
	tools       map[string]Tool
}

func NewRegistry(tools ...Tool) Registry {
	registry := Registry{definitions: map[string]Definition{}, tools: map[string]Tool{}}
	for _, tool := range tools {
		def := tool.Definition()
		registry.definitions[def.Name] = def
		registry.tools[def.Name] = tool
	}
	return registry
}

func (r Registry) Execute(request ExecutionRequest) (ExecutionResult, error) {
	tool, ok := r.tools[request.Call.Call.ToolName]
	if !ok {
		return ExecutionResult{}, fmt.Errorf("missing tool implementation %q", request.Call.Call.ToolName)
	}
	return tool.Execute(request)
}
