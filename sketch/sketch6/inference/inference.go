package inference

import (
	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/assembly"
	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
)

type Record struct {
	InferenceID      string
	PayloadID        string
	SessionID        string
	AgentID          string
	AgentVersion     string
	Provider         string
	ModelName        string
	GitCommit        string
	AssemblyPipeline string
	SourceRecordIDs  []string
	Steps            []assembly.ProvenanceStep
	Payload          []string
}

type Recorder struct {
	GitCommit        string
	AssemblyPipeline string
}

func (r Recorder) RecordPayload(spec agent.Spec, payload assembly.InferencePayload, request provider.Request) Record {
	return Record{
		InferenceID:      payload.PayloadID + "-record",
		PayloadID:        payload.PayloadID,
		SessionID:        payload.SessionID,
		AgentID:          payload.AgentID,
		AgentVersion:     payload.AgentVersion,
		Provider:         spec.Model.Provider,
		ModelName:        spec.Model.Name,
		GitCommit:        r.GitCommit,
		AssemblyPipeline: r.AssemblyPipeline,
		SourceRecordIDs:  append([]string(nil), payload.SourceRecordIDs...),
		Steps:            append([]assembly.ProvenanceStep(nil), payload.Steps...),
		Payload:          append([]string(nil), request.Lines...),
	}
}

type Store struct {
	Records []Record
}

func NewStore() *Store { return &Store{} }

func (s *Store) Append(record Record) {
	s.Records = append(s.Records, record)
}
