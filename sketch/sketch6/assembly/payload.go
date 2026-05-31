package assembly

import (
	"sort"

	"github.com/comalice/inference_sketch/sketch/sketch6/runtime"
)

func BuildPayload(agentSpec runtime.Agent, payloadID, sessionID string, assembled Result) InferencePayload {
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
	return InferencePayload{PayloadID: payloadID, SessionID: sessionID, AgentName: agentSpec.Name, LogicalModel: agentSpec.LogicalModel, SourceRecordIDs: sourceIDs, Segments: append([]Segment(nil), assembled.Segments...), Steps: append([]ProvenanceStep(nil), assembled.Steps...)}
}
