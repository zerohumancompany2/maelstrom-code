package assembly

import (
	"fmt"
	"strings"

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
	RuntimeView      runtime.RuntimeView
	History          *session.History
	Charts           charts.Snapshot
	MaxHistoryItems  int
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
		case "cognitive_state":
			chunks = append(chunks, CognitiveStateChunk{})
		case "workflow_state":
			chunks = append(chunks, WorkflowStateChunk{})
		case "binding":
			chunks = append(chunks, BindingChunk{})
		default:
			return Plan{}, ErrUnknownChunkType{Type: chunk.Type}
		}
	}
	return Plan{Assembler: Assembler{Chunks: chunks}, MaxHistoryItems: maxHistoryItems}, nil
}

type ErrUnknownChunkType struct{ Type string }

func (e ErrUnknownChunkType) Error() string { return "unknown context chunk type \"" + e.Type + "\"" }

type CognitiveStateChunk struct{}

func (CognitiveStateChunk) Name() string { return "cognitive-state" }

func (CognitiveStateChunk) Build(input Input) (ChunkResult, error) {
	state := input.RuntimeView.Cognitive
	content := fmt.Sprintf("Cognitive mode: %s. Prompt: %s. Visible tools: %s. Enabled tools: %s.", state.CurrentState, strings.TrimSpace(state.Prompt), joinList(state.VisibleTools), joinList(state.EnabledTools))
	step := ProvenanceStep{ChunkName: "cognitive-state", Operation: "project-cognitive", OutputDescriptor: "cognitive state chunk"}
	return ChunkResult{Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}

type WorkflowStateChunk struct{}

func (WorkflowStateChunk) Name() string { return "workflow-state" }

func (WorkflowStateChunk) Build(input Input) (ChunkResult, error) {
	if input.RuntimeView.Workflow == nil {
		return ChunkResult{}, nil
	}
	workflow := input.RuntimeView.Workflow
	content := fmt.Sprintf("Workflow %s is in state %s. Description: %s. Context: %s. Visible tools: %s. Enabled tools: %s.", workflow.WorkflowID, workflow.CurrentState, workflow.Description, workflow.Context, joinList(workflow.VisibleTools), joinList(workflow.EnabledTools))
	step := ProvenanceStep{ChunkName: "workflow-state", Operation: "project-workflow", OutputDescriptor: "workflow state chunk"}
	return ChunkResult{Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}

type BindingChunk struct{}

func (BindingChunk) Name() string { return "binding" }

func (BindingChunk) Build(input Input) (ChunkResult, error) {
	if !input.RuntimeView.Binding.Bound {
		return ChunkResult{}, nil
	}
	content := fmt.Sprintf("Active binding: workflow=%s.", input.RuntimeView.Binding.WorkflowID)
	step := ProvenanceStep{ChunkName: "binding", Operation: "project-binding", OutputDescriptor: "binding chunk"}
	return ChunkResult{Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}

func joinList(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
