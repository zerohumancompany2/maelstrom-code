package tools

import (
	"fmt"
	"sort"

	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
)

type Definition struct {
	Name       string
	Parameters map[string]string
}

type ExecutionRequest struct {
	Agent   runtime.Agent
	Call    provider.ToolRequestOutput
	History *session.History
	Request session.ToolCallRequestRecord
}

type ExecutionResult struct {
	ToolName       string
	DisplayContent string
	IsError        bool
	Records        []session.Record
}

type Executor interface {
	Execute(request ExecutionRequest) (ExecutionResult, error)
}

type Tool interface {
	Definition() Definition
	Execute(request ExecutionRequest) (ExecutionResult, error)
}

func (r Registry) Lookup(name string) (Definition, bool) {
	def, ok := r.definitions[name]
	return def, ok
}

func (r Registry) Names() []string {
	names := make([]string, 0, len(r.definitions))
	for name := range r.definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r Registry) MustHave(name string) error {
	if _, ok := r.Lookup(name); !ok {
		return fmt.Errorf("missing tool definition %q", name)
	}
	return nil
}
