package assembly

import "github.com/comalice/inference_sketch/sketch/sketch6/session"

type StateProjectionChunk struct {
	ChartName string
}

func (c StateProjectionChunk) Name() string { return "state-projection-" + c.ChartName }

func (c StateProjectionChunk) Build(input Input) (ChunkResult, error) {
	recordIDs := latestTransitionRecordIDs(input.History, c.ChartName)
	state := input.Charts.State(c.ChartName)
	step := ProvenanceStep{ChunkName: c.Name(), Operation: "project-state", InputRecordIDs: recordIDs, OutputDescriptor: c.ChartName + " state"}
	return ChunkResult{Segments: []Segment{StateSegment{ChartName: c.ChartName, State: state, RecordIDs: recordIDs, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}

func latestTransitionRecordIDs(history *session.History, chartName string) []string {
	for i := len(history.Records) - 1; i >= 0; i-- {
		transition, ok := history.Records[i].(session.StateTransitionRecord)
		if ok && transition.ChartName == chartName {
			return []string{transition.RecordID()}
		}
	}
	return nil
}
