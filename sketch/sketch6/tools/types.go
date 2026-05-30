package tools

import (
	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
)

type Definition struct {
	Name       string
	Parameters map[string]string
}

type ExecutionRequest struct {
	Agent   agent.Spec
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
