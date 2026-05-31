package runner

import (
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/assembly"
	"github.com/comalice/inference_sketch/sketch/sketch6/charts"
	"github.com/comalice/inference_sketch/sketch/sketch6/inference"
	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/tools"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
)

type Loop struct {
	Provider provider.Provider
	Tools    tools.Executor
	Recorder inference.Recorder
}

func (l Loop) Run(agentRuntime runtime.Agent, definition agent.Definition, workflowSnapshot *runtime.WorkflowSnapshot, binding runtime.BindingSnapshot, history *session.History, workflowHistory *workflow.History, store *inference.Store) error {
	plan, err := assembly.BuildPlan(definition)
	if err != nil {
		return err
	}
	for iteration := 1; ; iteration++ {
		cognitive := runtime.ReduceCognitiveStateFromHistory(history, definition.Cognitive.InitialState, BuildCognitiveStateMap(definition))
		view := runtime.RuntimeView{Agent: agentRuntime, Cognitive: cognitive, Binding: binding, Workflow: runtime.ReduceWorkflowState(runtime.ReduceWorkflowStateFromHistory(workflowHistory, workflowSnapshot), binding)}
		assembled, err := plan.Assembler.Assemble(assembly.Input{RuntimeView: view, History: history, Charts: charts.BuildSnapshot(history), MaxHistoryItems: plan.MaxHistoryItems})
		if err != nil {
			return err
		}

		payload := assembly.BuildPayload(agentRuntime, history.NextBundleID(), history.SessionID, assembled)
		request, err := l.Provider.BuildRequest(agentRuntime, payload)
		if err != nil {
			return err
		}
		store.Append(l.Recorder.RecordPayload(agentRuntime, payload, request))

		response, err := l.Provider.ParseResponse(request)
		if err != nil {
			return err
		}

		hasToolCalls := false
		for _, output := range response.Outputs {
			records, err := l.consumeProviderOutput(agentRuntime, history, output)
			if err != nil {
				return err
			}
			for _, record := range records {
				history.Append(record)
				if _, ok := record.(session.ToolCallRequestRecord); ok {
					hasToolCalls = true
				}
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

func BuildCognitiveStateMap(definition agent.Definition) map[string]runtime.CognitiveSnapshot {
	states := map[string]runtime.CognitiveSnapshot{}
	for _, state := range definition.Cognitive.States {
		states[state.Name] = runtime.CognitiveSnapshot{CurrentState: state.Name, VisibleTools: append([]string(nil), state.VisibleTools...), EnabledTools: append([]string(nil), state.EnabledTools...), Prompt: state.Prompt}
	}
	return states
}

func (l Loop) consumeProviderOutput(agentRuntime runtime.Agent, history *session.History, output provider.Output) ([]session.Record, error) {
	switch v := output.(type) {
	case provider.AssistantOutput:
		return []session.Record{session.AssistantMessageRecord{BaseRecord: history.NextRecord("assistant"), Content: v.Content}}, nil
	case provider.ToolRequestOutput:
		requestRecord := session.ToolCallRequestRecord{BaseRecord: history.NextRecord("tool_call_request"), CallID: v.Call.CallID, ToolName: v.Call.ToolName, Arguments: string(v.Call.RawArgs)}
		result, err := l.Tools.Execute(tools.ExecutionRequest{Agent: agentRuntime, Call: v, History: history, Request: requestRecord})
		if err != nil {
			return nil, err
		}
		records := []session.Record{requestRecord}
		records = append(records, result.Records...)
		records = append(records, session.ToolCallResultRecord{BaseRecord: history.NextRecord("tool_call_result"), CallID: v.Call.CallID, ToolName: result.ToolName, Content: result.DisplayContent, IsError: result.IsError})
		return records, nil
	default:
		return nil, fmt.Errorf("unknown provider output %T", output)
	}
}
