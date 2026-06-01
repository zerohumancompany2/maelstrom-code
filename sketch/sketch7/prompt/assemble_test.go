package prompt

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

func TestAssemblerBuildsDeterministicProjectionSequence(t *testing.T) {
	history := logs.NewSessionHistory("session-001")
	history.Append(logs.UserMessageRecord{SessionBaseRecord: history.NextRecord("user"), Content: "Please fix the flaky test."})
	workflowHistory := logs.NewWorkflowHistory("workflow-001")
	view := runtime.SessionView{
		Agent: runtime.Agent{Name: "builder", LogicalModel: "gpt-test"},
		Cognitive: runtime.CognitiveView{
			CurrentState: "observe",
			VisibleTools: []string{"transition_state", "run_tests"},
			EnabledTools: []string{"transition_state"},
			Prompt:       "Observe the request before acting.",
		},
		Binding: runtime.BindingView{WorkflowID: "workflow-001", Bound: true},
		Workflow: &runtime.WorkflowView{
			WorkflowID:   "workflow-001",
			CurrentState: "planning",
			Description:  "Plan and implement the requested change.",
			Context:      "Work carefully in the repo.",
		},
		Interaction: runtime.InteractionView{Mode: runtime.InteractionInteractive},
	}

	assembler := Assembler{Projections: []Projection{
		StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a precise coding agent."},
		InteractionProjection{},
		BindingProjection{},
		WorkflowProjection{},
		CognitiveProjection{},
		RecentHistoryProjection{},
	}}

	assembled, err := assembler.Assemble(Input{Session: view, History: history, Workflow: workflowHistory, MaxHistory: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assembled.Segments) != 6 {
		t.Fatalf("got %d segments, want 6", len(assembled.Segments))
	}

	first, ok := assembled.Segments[0].(PromptSegment)
	if !ok {
		t.Fatalf("first segment is %T, want PromptSegment", assembled.Segments[0])
	}
	if first.Content != "You are a precise coding agent." {
		t.Fatalf("first content = %q, want %q", first.Content, "You are a precise coding agent.")
	}

	last, ok := assembled.Segments[len(assembled.Segments)-1].(PromptSegment)
	if !ok {
		t.Fatalf("last segment is %T, want PromptSegment", assembled.Segments[len(assembled.Segments)-1])
	}
	if last.Role != "user" || last.Content != "Please fix the flaky test." {
		t.Fatalf("last segment = role=%q content=%q, want user prompt", last.Role, last.Content)
	}
}

func TestRecentHistoryProjectionRespectsHistoryLimit(t *testing.T) {
	history := logs.NewSessionHistory("session-002")
	history.Append(logs.UserMessageRecord{SessionBaseRecord: history.NextRecord("user"), Content: "first"})
	history.Append(logs.AssistantMessageRecord{SessionBaseRecord: history.NextRecord("assistant"), Content: "second"})
	history.Append(logs.UserMessageRecord{SessionBaseRecord: history.NextRecord("user"), Content: "third"})

	result, err := (RecentHistoryProjection{}).Build(Input{History: history, MaxHistory: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Segments) != 2 {
		t.Fatalf("got %d segments, want 2", len(result.Segments))
	}

	first, _ := result.Segments[0].(PromptSegment)
	second, _ := result.Segments[1].(PromptSegment)
	if first.Content != "second" || second.Content != "third" {
		t.Fatalf("got contents %q, %q; want second, third", first.Content, second.Content)
	}
}

func TestBuildPayloadCollectsUniqueSortedSourceRecordIDs(t *testing.T) {
	agent := runtime.Agent{Name: "builder", LogicalModel: "gpt-test"}
	step := ProvenanceStep{ProjectionName: "test", Operation: "project"}
	assembled := Result{Segments: []Segment{
		PromptSegment{Role: "user", Content: "hello", RecordIDs: []string{"r-002", "r-001"}, Step: step},
		PromptSegment{Role: "assistant", Content: "world", RecordIDs: []string{"r-001"}, Step: step},
	}}

	payload := BuildPayload(agent, "payload-001", "session-123", assembled)
	if len(payload.SourceRecordIDs) != 2 {
		t.Fatalf("got %d source ids, want 2", len(payload.SourceRecordIDs))
	}
	if payload.SourceRecordIDs[0] != "r-001" || payload.SourceRecordIDs[1] != "r-002" {
		t.Fatalf("SourceRecordIDs = %v, want [r-001 r-002]", payload.SourceRecordIDs)
	}
	if payload.AgentName != "builder" {
		t.Fatalf("AgentName = %q, want builder", payload.AgentName)
	}
}

func TestInteractionProjectionIncludesReasonWhenPresent(t *testing.T) {
	result, err := (InteractionProjection{}).Build(Input{Session: runtime.SessionView{Interaction: runtime.InteractionView{Mode: runtime.InteractionInterrupted, Reason: "user pressed escape"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Segments) != 1 {
		t.Fatalf("got %d segments, want 1", len(result.Segments))
	}
	segment, _ := result.Segments[0].(PromptSegment)
	if segment.Content != "Session interaction mode: interrupted. Reason: user pressed escape." {
		t.Fatalf("Content = %q, want interrupted reason string", segment.Content)
	}
}
