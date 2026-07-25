package tools

import (
	"fmt"
	"sort"

	"github.com/comalice/maelstrom/internal/logs"
	"github.com/comalice/maelstrom/internal/provider"
	"github.com/comalice/maelstrom/internal/runtime"
)

type Definition struct {
	Name        string
	Description string
	Parameters  map[string]string
	Required    []string
}

type ExecutionRequest struct {
	Agent    runtime.Agent
	Session  runtime.SessionView
	Call     provider.ToolRequestOutput
	History  *logs.SessionHistory
	Workflow *logs.WorkflowHistory
}

type ExecutionResult struct {
	ToolName        string
	DisplayContent  string
	IsError         bool
	SessionRecords  []logs.SessionRecord
	WorkflowRecords []logs.WorkflowRecord
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

func (r Registry) Definitions() []Definition {
	defs := make([]Definition, 0, len(r.definitions))
	for _, def := range r.definitions {
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

func (r Registry) MustHave(name string) error {
	if _, ok := r.Lookup(name); !ok {
		return fmt.Errorf("missing tool definition %q", name)
	}
	return nil
}
