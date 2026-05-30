package assembly

import (
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch6/session"
)

type RecentHistoryChunk struct{}

func (RecentHistoryChunk) Name() string { return "recent-history" }

func (RecentHistoryChunk) Build(input Input) (ChunkResult, error) {
	limit := input.Agent.Context.MaxHistoryItems
	start := len(input.History.Records) - limit
	if start < 0 {
		start = 0
	}
	result := ChunkResult{}
	for _, record := range input.History.Records[start:] {
		switch v := record.(type) {
		case session.UserMessageRecord:
			step := ProvenanceStep{ChunkName: "recent-history", Operation: "project-user", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "user prompt segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "user", Content: v.Content, RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case session.AssistantMessageRecord:
			step := ProvenanceStep{ChunkName: "recent-history", Operation: "project-assistant", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "assistant prompt segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "assistant", Content: v.Content, RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case session.ToolCallRequestRecord:
			step := ProvenanceStep{ChunkName: "recent-history", Operation: "project-tool-call", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "tool-call segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "assistant", Content: fmt.Sprintf("tool call %s(%s)", v.ToolName, v.Arguments), RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case session.ToolCallResultRecord:
			step := ProvenanceStep{ChunkName: "recent-history", Operation: "project-tool-result", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "tool result segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "tool", Content: v.Content, RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		}
	}
	return result, nil
}
