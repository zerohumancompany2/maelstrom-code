package runner

import (
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch7/tools"
)

type Loop struct {
	Provider    provider.Provider
	Tools       tools.Executor
	Projections []prompt.Projection
	MaxHistory  int
}

func (l Loop) Run(agent runtime.Agent, agentDef defs.AgentDefinition, workflowDef *defs.WorkflowDefinition, sessionHistory *logs.SessionHistory, workflowHistory *logs.WorkflowHistory) error {
	assembler := prompt.Assembler{Projections: l.Projections}
	for iteration := 1; ; iteration++ {
		view := BuildSessionView(agent, agentDef, workflowDef, sessionHistory, workflowHistory)
		assembled, err := assembler.Assemble(prompt.Input{
			Session:    view,
			History:    sessionHistory,
			Workflow:   workflowHistory,
			MaxHistory: l.MaxHistory,
		})
		if err != nil {
			return err
		}

		payload := prompt.BuildPayload(agent, sessionHistory.NextBundleID(), sessionHistory.SessionID, assembled)
		request, err := l.Provider.BuildRequest(agent, payload, toolDefinitions(l.Tools))
		if err != nil {
			return err
		}
		response, err := l.Provider.Send(request)
		if err != nil {
			return err
		}

		hasToolCalls := false
		for _, output := range response.Outputs {
			records, workflowRecords, err := l.consumeProviderOutput(view, sessionHistory, workflowHistory, output)
			if err != nil {
				return err
			}
			for _, record := range records {
				sessionHistory.Append(record)
				if _, ok := record.(logs.ToolCallRequestRecord); ok {
					hasToolCalls = true
				}
			}
			for _, record := range workflowRecords {
				workflowHistory.Append(record)
			}
		}

		if !hasToolCalls {
			return nil
		}
		if iteration == 8 {
			return fmt.Errorf("loop guard tripped")
		}
	}
}

func toolDefinitions(executor tools.Executor) []provider.ToolDefinition {
	registry, ok := executor.(tools.Registry)
	if !ok {
		return nil
	}
	defs := registry.Definitions()
	converted := make([]provider.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		converted = append(converted, provider.ToolDefinition{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
		})
	}
	return converted
}

func (l Loop) consumeProviderOutput(view runtime.SessionView, history *logs.SessionHistory, workflowHistory *logs.WorkflowHistory, output provider.Output) ([]logs.SessionRecord, []logs.WorkflowRecord, error) {
	switch v := output.(type) {
	case provider.AssistantOutput:
		return []logs.SessionRecord{logs.AssistantMessageRecord{SessionBaseRecord: history.NextRecord("assistant"), Content: v.Content, Reasoning: v.Reasoning}}, nil, nil
	case provider.ToolRequestOutput:
		requestRecord := logs.ToolCallRequestRecord{SessionBaseRecord: history.NextRecord("tool_call_request"), CallID: v.Call.CallID, ToolName: v.Call.ToolName, Arguments: string(v.Call.RawArgs)}
		result, err := l.Tools.Execute(tools.ExecutionRequest{Agent: view.Agent, Session: view, Call: v, History: history, Workflow: workflowHistory})
		if err != nil {
			return nil, nil, err
		}
		records := []logs.SessionRecord{requestRecord}
		records = append(records, result.SessionRecords...)
		records = append(records, logs.ToolCallResultRecord{SessionBaseRecord: history.NextRecord("tool_call_result"), CallID: v.Call.CallID, ToolName: result.ToolName, Content: result.DisplayContent, IsError: result.IsError})
		return records, result.WorkflowRecords, nil
	default:
		return nil, nil, fmt.Errorf("unknown provider output %T", output)
	}
}
