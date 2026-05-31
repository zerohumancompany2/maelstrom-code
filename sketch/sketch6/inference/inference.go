package inference

import (
	"github.com/comalice/inference_sketch/sketch/sketch6/assembly"
	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/runtime"
)

type Record struct {
	InferenceID      string
	PayloadID        string
	SessionID        string
	AgentName        string
	LogicalModel     string
	Provider         string
	ModelRef         string
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

func (r Recorder) RecordPayload(agent runtime.Agent, payload assembly.InferencePayload, request provider.Request) Record {
	return Record{
		InferenceID:      payload.PayloadID + "-record",
		PayloadID:        payload.PayloadID,
		SessionID:        payload.SessionID,
		AgentName:        payload.AgentName,
		LogicalModel:     payload.LogicalModel,
		Provider:         agent.ProviderName,
		ModelRef:         agent.ProviderRef,
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
