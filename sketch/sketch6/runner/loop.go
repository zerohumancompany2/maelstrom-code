package runner

import (
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/assembly"
	"github.com/comalice/inference_sketch/sketch/sketch6/charts"
	"github.com/comalice/inference_sketch/sketch/sketch6/inference"
	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/tools"
)

type Loop struct {
	Assembler assembly.Assembler
	Provider  provider.Provider
	Tools     tools.Executor
	Recorder  inference.Recorder
}

func (l Loop) Run(spec agent.Spec, history *session.History, store *inference.Store) error {
	for iteration := 1; ; iteration++ {
		assembled, err := l.Assembler.Assemble(assembly.Input{Agent: spec, History: history, Charts: charts.BuildSnapshot(history)})
		if err != nil {
			return err
		}

		payload := assembly.BuildPayload(spec, history.NextBundleID(), history.SessionID, assembled)
		request, err := l.Provider.BuildRequest(spec, payload)
		if err != nil {
			return err
		}
		store.Append(l.Recorder.RecordPayload(spec, payload, request))

		response, err := l.Provider.ParseResponse(request)
		if err != nil {
			return err
		}

		hasToolCalls := false
		for _, output := range response.Outputs {
			records, err := l.consumeProviderOutput(spec, history, output)
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
		if iteration == 5 {
			return fmt.Errorf("loop guard tripped")
		}
	}
}

func (l Loop) consumeProviderOutput(spec agent.Spec, history *session.History, output provider.Output) ([]session.Record, error) {
	switch v := output.(type) {
	case provider.AssistantOutput:
		return []session.Record{session.AssistantMessageRecord{BaseRecord: history.NextRecord("assistant"), Content: v.Content}}, nil
	case provider.ToolRequestOutput:
		requestRecord := session.ToolCallRequestRecord{BaseRecord: history.NextRecord("tool_call_request"), CallID: v.Call.CallID, ToolName: v.Call.ToolName, Arguments: string(v.Call.RawArgs)}
		result, err := l.Tools.Execute(tools.ExecutionRequest{Agent: spec, Call: v, History: history, Request: requestRecord})
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
