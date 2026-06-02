package runner

import (
	"fmt"
	"strings"

	ctxpkg "github.com/comalice/inference_sketch/sketch/sketch7/context"
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
	StopToken   string
}

func (l Loop) Run(agent runtime.Agent, agentDef defs.AgentDefinition, workflowDef *defs.WorkflowDefinition, sessionHistory *logs.SessionHistory, workflowHistory *logs.WorkflowHistory) error {
	assembler := prompt.Assembler{Projections: l.Projections}
	contextBuilder := ctxpkg.Builder{MaxMessages: l.MaxHistory}
	for iteration := 1; ; iteration++ {
		view := BuildSessionView(agent, agentDef, workflowDef, sessionHistory, workflowHistory)
		payloadID := sessionHistory.NextBundleID()
		inferencePayload := contextBuilder.Build(payloadID, ctxpkg.BuildSections(agentDef, view, sessionHistory, ctxpkg.RepoContextOptions{RootDir: ".", RefreshEveryTurns: 12, MaxFilesToInspect: 2000, MaxTopLevelEntries: 8, MaxExtensionsToShow: 5}), view, sessionHistory, workflowHistory)
		persistContextSnapshots(sessionHistory, inferencePayload)
		assembled, err := assembler.Assemble(prompt.Input{Payload: inferencePayload})
		if err != nil {
			return err
		}

		payload := prompt.BuildPayload(agent, inferencePayload.PayloadID, sessionHistory.SessionID, assembled)
		request, err := l.Provider.BuildRequest(agent, payload, toolDefinitions(l.Tools))
		if err != nil {
			return err
		}
		sessionHistory.Append(logs.InferenceEnvelopeRecord{SessionBaseRecord: sessionHistory.NextRecord("inference_envelope"), PayloadID: inferencePayload.PayloadID, ModelRef: inferencePayload.ModelRef, ProviderRef: agent.ProviderName, IncludedContextRecordIDs: contextRecordIDs(inferencePayload.Sections), IncludedTranscriptKinds: messageKinds(inferencePayload.Messages), IncludedToolNames: inferencePayload.Tools})
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

		if l.shouldStop(sessionHistory) {
			return nil
		}
		if !hasToolCalls {
			return nil
		}
		if iteration == 8 {
			return fmt.Errorf("loop guard tripped")
		}
	}
}

func (l Loop) shouldStop(history *logs.SessionHistory) bool {
	if history == nil || strings.TrimSpace(l.StopToken) == "" || len(history.Records) == 0 {
		return false
	}
	last, ok := history.Records[len(history.Records)-1].(logs.AssistantMessageRecord)
	if !ok {
		return false
	}
	return strings.Contains(last.Content, l.StopToken)
}

func persistContextSnapshots(history *logs.SessionHistory, payload ctxpkg.Payload) {
	turn := ctxpkg.InteractionTurnCount(history)
	for i, section := range payload.Sections {
		if section.LogicalKey == "" || section.SourceKind == "static" {
			continue
		}
		latest := latestContextSnapshotRecord(history, section.LogicalKey)
		if latest != nil && latest.ContentHash == ctxpkg.HashContent(section.Content) {
			payload.Sections[i].RecordID = latest.RecordID()
			continue
		}
		record := logs.ContextSnapshotRecord{SessionBaseRecord: history.NextRecord("context_snapshot"), PayloadID: payload.PayloadID, LogicalKey: section.LogicalKey, SectionName: section.Name, SectionType: section.SectionType, SourceKind: section.SourceKind, Content: section.Content, ContentHash: ctxpkg.HashContent(section.Content), GeneratedAtTurn: turn, RefreshEveryNTurns: section.RefreshEveryTurns, RetentionMode: section.RetentionMode}
		if latest != nil {
			record.SupersedesRecordID = latest.RecordID()
		}
		history.Append(record)
		payload.Sections[i].RecordID = record.RecordID()
	}
}

func latestContextSnapshotRecord(history *logs.SessionHistory, logicalKey string) *logs.ContextSnapshotRecord {
	for i := len(history.Records) - 1; i >= 0; i-- {
		rec, ok := history.Records[i].(logs.ContextSnapshotRecord)
		if ok && rec.LogicalKey == logicalKey {
			copy := rec
			return &copy
		}
	}
	return nil
}

func contextRecordIDs(sections []ctxpkg.Section) []string {
	ids := []string{}
	for _, section := range sections {
		if section.RecordID != "" {
			ids = append(ids, section.RecordID)
		}
	}
	return ids
}

func messageKinds(messages []ctxpkg.Message) []string {
	kinds := make([]string, 0, len(messages))
	for _, message := range messages {
		kinds = append(kinds, message.Kind)
	}
	return kinds
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
			Required:    append([]string(nil), def.Required...),
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
