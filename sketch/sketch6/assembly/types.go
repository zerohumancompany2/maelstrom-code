package assembly

import (
	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/charts"
	"github.com/comalice/inference_sketch/sketch/sketch6/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
)

type ProvenanceStep struct {
	ChunkName        string
	Operation        string
	InputRecordIDs   []string
	OutputDescriptor string
}

type Segment interface {
	SegmentKind() string
	TokenText() string
	SourceRecordIDs() []string
	ProvenanceStep() ProvenanceStep
}

type PromptSegment struct {
	Role      string
	Content   string
	RecordIDs []string
	Step      ProvenanceStep
}

func (p PromptSegment) SegmentKind() string            { return "prompt" }
func (p PromptSegment) TokenText() string              { return p.Content }
func (p PromptSegment) SourceRecordIDs() []string      { return append([]string(nil), p.RecordIDs...) }
func (p PromptSegment) ProvenanceStep() ProvenanceStep { return p.Step }

type StateSegment struct {
	ChartName string
	State     string
	RecordIDs []string
	Step      ProvenanceStep
}

func (s StateSegment) SegmentKind() string            { return "state" }
func (s StateSegment) TokenText() string              { return s.State }
func (s StateSegment) SourceRecordIDs() []string      { return append([]string(nil), s.RecordIDs...) }
func (s StateSegment) ProvenanceStep() ProvenanceStep { return s.Step }

type Input struct {
	Agent           runtime.Agent
	History         *session.History
	Charts          charts.Snapshot
	MaxHistoryItems int
}

type Result struct {
	Segments []Segment
	Steps    []ProvenanceStep
}

type Chunk interface {
	Name() string
	Build(input Input) (ChunkResult, error)
}

type ChunkResult struct {
	Segments []Segment
	Steps    []ProvenanceStep
}

type InferencePayload struct {
	PayloadID       string
	SessionID       string
	AgentName       string
	LogicalModel    string
	SourceRecordIDs []string
	Segments        []Segment
	Steps           []ProvenanceStep
}

type Plan struct {
	Assembler       Assembler
	MaxHistoryItems int
}

func BuildPlan(def agent.Definition) (Plan, error) {
	chunks := make([]Chunk, 0, len(def.Context.Chunks))
	maxHistoryItems := 0
	for _, chunk := range def.Context.Chunks {
		switch chunk.Type {
		case "system":
			chunks = append(chunks, StaticSystemChunk{Prompt: chunk.Prompt})
		case "messages":
			chunks = append(chunks, RecentHistoryChunk{})
			if maxHistoryItems == 0 {
				maxHistoryItems = 10
			}
		case "state":
			chunks = append(chunks, StateProjectionChunk{ChartName: chunk.Chart})
		default:
			return Plan{}, ErrUnknownChunkType{Type: chunk.Type}
		}
	}
	return Plan{Assembler: Assembler{Chunks: chunks}, MaxHistoryItems: maxHistoryItems}, nil
}

type ErrUnknownChunkType struct{ Type string }

func (e ErrUnknownChunkType) Error() string { return "unknown context chunk type \"" + e.Type + "\"" }
