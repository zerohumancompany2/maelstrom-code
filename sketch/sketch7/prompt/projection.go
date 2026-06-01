package prompt

import (
	"fmt"
	"strings"

	ctxpkg "github.com/comalice/inference_sketch/sketch/sketch7/context"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

type Input struct {
	Payload    ctxpkg.Payload
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

type ContextProjection struct{}

func (ContextProjection) Name() string { return "context" }

func (ContextProjection) Build(input Input) (ProjectionResult, error) {
	payload := input.Payload
	if payload.PayloadID == "" {
		builder := ctxpkg.Builder{MaxMessages: input.MaxHistory}
		payload = builder.Build("payload-context", nil, input.Session, input.History, input.Workflow)
	}
	result := ProjectionResult{}
	for _, section := range payload.Sections {
		step := ProvenanceStep{ProjectionName: "context", Operation: "project-section", OutputDescriptor: section.Name}
		result.Segments = append(result.Segments, PromptSegment{Role: section.Role, Content: section.Content, Step: step})
		result.Steps = append(result.Steps, step)
	}
	for _, message := range payload.Messages {
		step := ProvenanceStep{ProjectionName: "context", Operation: "project-message", OutputDescriptor: message.Kind}
		result.Segments = append(result.Segments, PromptSegment{Role: message.Role, Name: message.Name, CallID: message.CallID, Content: message.Content, Step: step})
		result.Steps = append(result.Steps, step)
	}
	return result, nil
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
	step := ProvenanceStep{ProjectionName: p.Name(), Operation: "project-static", OutputDescriptor: p.Role + " prompt"}
	return ProjectionResult{Segments: []Segment{PromptSegment{Role: p.Role, Content: p.Prompt, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}

type RecentHistoryProjection struct{}

func (RecentHistoryProjection) Name() string { return "recent-history" }

func (RecentHistoryProjection) Build(input Input) (ProjectionResult, error) {
	result := ProjectionResult{}
	messages := ctxpkg.Builder{MaxMessages: input.MaxHistory}.Build("payload-history", nil, input.Session, input.History, input.Workflow).Messages
	for _, message := range messages {
		step := ProvenanceStep{ProjectionName: "recent-history", Operation: "project-message", OutputDescriptor: message.Kind}
		result.Segments = append(result.Segments, PromptSegment{Role: message.Role, Name: message.Name, CallID: message.CallID, Content: message.Content, Step: step})
		result.Steps = append(result.Steps, step)
	}
	return result, nil
}

type CognitiveProjection struct{}

func (CognitiveProjection) Name() string { return "cognitive-state" }

func (CognitiveProjection) Build(input Input) (ProjectionResult, error) {
	state := input.Session.Cognitive
	content := fmt.Sprintf("Cognitive mode: %s. Prompt: %s. Visible tools: %s. Enabled tools: %s.", state.CurrentState, strings.TrimSpace(state.Prompt), joinListCompat(state.VisibleTools), joinListCompat(state.EnabledTools))
	step := ProvenanceStep{ProjectionName: "cognitive-state", Operation: "project-cognitive", OutputDescriptor: "cognitive prompt segment", RuntimeDescriptor: state.CurrentState}
	return ProjectionResult{Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}

type WorkflowProjection struct{}

func (WorkflowProjection) Name() string { return "workflow-state" }

func (WorkflowProjection) Build(input Input) (ProjectionResult, error) {
	if input.Session.Workflow == nil {
		return ProjectionResult{}, nil
	}
	workflow := input.Session.Workflow
	content := fmt.Sprintf("Workflow %s is in state %s. Description: %s. Context: %s. Visible tools: %s. Enabled tools: %s.", workflow.WorkflowID, workflow.CurrentState, workflow.Description, workflow.Context, joinListCompat(workflow.VisibleTools), joinListCompat(workflow.EnabledTools))
	step := ProvenanceStep{ProjectionName: "workflow-state", Operation: "project-workflow", OutputDescriptor: "workflow prompt segment", RuntimeDescriptor: workflow.CurrentState}
	return ProjectionResult{Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}

type BindingProjection struct{}

func (BindingProjection) Name() string { return "binding" }

func (BindingProjection) Build(input Input) (ProjectionResult, error) {
	if !input.Session.Binding.Bound {
		return ProjectionResult{}, nil
	}
	content := fmt.Sprintf("Active binding: workflow=%s.", input.Session.Binding.WorkflowID)
	step := ProvenanceStep{ProjectionName: "binding", Operation: "project-binding", OutputDescriptor: "binding prompt segment", RuntimeDescriptor: input.Session.Binding.WorkflowID}
	return ProjectionResult{Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}}, Steps: []ProvenanceStep{step}}, nil
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
	return ProjectionResult{Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}

func joinListCompat(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
