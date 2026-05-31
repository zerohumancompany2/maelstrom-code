package prompt

import (
	"fmt"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

type Input struct {
	Session    runtime.SessionView
	History    *logs.SessionHistory
	Workflow   *logs.WorkflowHistory
	MaxHistory int
}

type Projection interface {
	Name() string
	Build(input Input) (ProjectionResult, error)
}

type ProjectionResult struct {
	Segments []Segment
	Steps    []ProvenanceStep
}

type StaticProjection struct {
	ProjectionName string
	Role           string
	Prompt         string
}

func (p StaticProjection) Name() string {
	if strings.TrimSpace(p.ProjectionName) == "" {
		return "static"
	}
	return p.ProjectionName
}

func (p StaticProjection) Build(_ Input) (ProjectionResult, error) {
	step := ProvenanceStep{
		ProjectionName:   p.Name(),
		Operation:        "project-static",
		OutputDescriptor: p.Role + " prompt",
	}
	return ProjectionResult{
		Segments: []Segment{PromptSegment{Role: p.Role, Content: p.Prompt, Step: step}},
		Steps:    []ProvenanceStep{step},
	}, nil
}

type RecentHistoryProjection struct{}

func (RecentHistoryProjection) Name() string { return "recent-history" }

func (RecentHistoryProjection) Build(input Input) (ProjectionResult, error) {
	limit := input.MaxHistory
	if limit <= 0 {
		return ProjectionResult{}, nil
	}
	start := len(input.History.Records) - limit
	if start < 0 {
		start = 0
	}
	result := ProjectionResult{}
	for _, record := range input.History.Records[start:] {
		switch v := record.(type) {
		case logs.UserMessageRecord:
			step := ProvenanceStep{ProjectionName: "recent-history", Operation: "project-user", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "user prompt segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "user", Content: v.Content, RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case logs.AssistantMessageRecord:
			step := ProvenanceStep{ProjectionName: "recent-history", Operation: "project-assistant", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "assistant prompt segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "assistant", Content: v.Content, RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case logs.ToolCallRequestRecord:
			step := ProvenanceStep{ProjectionName: "recent-history", Operation: "project-tool-call", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "tool call prompt segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "assistant", Content: fmt.Sprintf("tool call %s(%s)", v.ToolName, v.Arguments), RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case logs.ToolCallResultRecord:
			step := ProvenanceStep{ProjectionName: "recent-history", Operation: "project-tool-result", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "tool result prompt segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "tool", Content: v.Content, RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		}
	}
	return result, nil
}

type CognitiveProjection struct{}

func (CognitiveProjection) Name() string { return "cognitive-state" }

func (CognitiveProjection) Build(input Input) (ProjectionResult, error) {
	state := input.Session.Cognitive
	content := fmt.Sprintf("Cognitive mode: %s. Prompt: %s. Visible tools: %s. Enabled tools: %s.", state.CurrentState, strings.TrimSpace(state.Prompt), joinList(state.VisibleTools), joinList(state.EnabledTools))
	step := ProvenanceStep{ProjectionName: "cognitive-state", Operation: "project-cognitive", OutputDescriptor: "cognitive prompt segment", RuntimeDescriptor: state.CurrentState}
	return ProjectionResult{
		Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}},
		Steps:    []ProvenanceStep{step},
	}, nil
}

type WorkflowProjection struct{}

func (WorkflowProjection) Name() string { return "workflow-state" }

func (WorkflowProjection) Build(input Input) (ProjectionResult, error) {
	if input.Session.Workflow == nil {
		return ProjectionResult{}, nil
	}
	workflow := input.Session.Workflow
	content := fmt.Sprintf("Workflow %s is in state %s. Description: %s. Context: %s. Visible tools: %s. Enabled tools: %s.", workflow.WorkflowID, workflow.CurrentState, workflow.Description, workflow.Context, joinList(workflow.VisibleTools), joinList(workflow.EnabledTools))
	step := ProvenanceStep{ProjectionName: "workflow-state", Operation: "project-workflow", OutputDescriptor: "workflow prompt segment", RuntimeDescriptor: workflow.CurrentState}
	return ProjectionResult{
		Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}},
		Steps:    []ProvenanceStep{step},
	}, nil
}

type BindingProjection struct{}

func (BindingProjection) Name() string { return "binding" }

func (BindingProjection) Build(input Input) (ProjectionResult, error) {
	if !input.Session.Binding.Bound {
		return ProjectionResult{}, nil
	}
	content := fmt.Sprintf("Active binding: workflow=%s.", input.Session.Binding.WorkflowID)
	step := ProvenanceStep{ProjectionName: "binding", Operation: "project-binding", OutputDescriptor: "binding prompt segment", RuntimeDescriptor: input.Session.Binding.WorkflowID}
	return ProjectionResult{
		Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}},
		Steps:    []ProvenanceStep{step},
	}, nil
}

type InteractionProjection struct{}

func (InteractionProjection) Name() string { return "interaction" }

func (InteractionProjection) Build(input Input) (ProjectionResult, error) {
	interaction := input.Session.Interaction
	content := fmt.Sprintf("Session interaction mode: %s.", interaction.Mode)
	if strings.TrimSpace(interaction.Reason) != "" {
		content = fmt.Sprintf("Session interaction mode: %s. Reason: %s.", interaction.Mode, interaction.Reason)
	}
	step := ProvenanceStep{ProjectionName: "interaction", Operation: "project-interaction", OutputDescriptor: "interaction prompt segment", RuntimeDescriptor: string(interaction.Mode)}
	return ProjectionResult{
		Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}},
		Steps:    []ProvenanceStep{step},
	}, nil
}

func joinList(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
