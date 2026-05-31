package prompt

import (
	"sort"

	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

type Payload struct {
	PayloadID       string
	SessionID       string
	AgentName       string
	LogicalModel    string
	SourceRecordIDs []string
	Segments        []Segment
	Steps           []ProvenanceStep
}

func BuildPayload(agent runtime.Agent, payloadID, sessionID string, assembled Result) Payload {
	sourceSet := map[string]struct{}{}
	for _, segment := range assembled.Segments {
		for _, id := range segment.SourceRecordIDs() {
			sourceSet[id] = struct{}{}
		}
	}
	sourceIDs := make([]string, 0, len(sourceSet))
	for id := range sourceSet {
		sourceIDs = append(sourceIDs, id)
	}
	sort.Strings(sourceIDs)
	return Payload{
		PayloadID:       payloadID,
		SessionID:       sessionID,
		AgentName:       agent.Name,
		LogicalModel:    agent.LogicalModel,
		SourceRecordIDs: sourceIDs,
		Segments:        append([]Segment(nil), assembled.Segments...),
		Steps:           append([]ProvenanceStep(nil), assembled.Steps...),
	}
}
