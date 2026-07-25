package tools

import (
	"testing"

	"github.com/comalice/maelstrom/internal/logs"
	"github.com/comalice/maelstrom/internal/provider"
	"github.com/comalice/maelstrom/internal/runtime"
)

func TestInterruptToolEmitsInterruptRecord(t *testing.T) {
	tool := InterruptTool{}
	history := logs.NewSessionHistory("session-008")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-interrupt-001", ToolName: "interrupt_session", Arguments: map[string]string{"reason": "user pressed escape", "requested_by": "user"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if len(result.SessionRecords) != 1 {
		t.Fatalf("got %d session records, want 1", len(result.SessionRecords))
	}
	record, ok := result.SessionRecords[0].(logs.InterruptRecord)
	if !ok {
		t.Fatalf("record is %T, want InterruptRecord", result.SessionRecords[0])
	}
	if record.Reason != "user pressed escape" || record.RequestedBy != "user" {
		t.Fatalf("record = %+v, want escape/user", record)
	}
}

func TestResumeToolEmitsResumeRecord(t *testing.T) {
	tool := ResumeTool{}
	history := logs.NewSessionHistory("session-009")
	request := ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-resume-001", ToolName: "resume_session", Arguments: map[string]string{"reason": "user finished guidance"}}},
	}

	result, err := tool.Execute(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SessionRecords) != 1 {
		t.Fatalf("got %d session records, want 1", len(result.SessionRecords))
	}
	record, ok := result.SessionRecords[0].(logs.ResumeRecord)
	if !ok {
		t.Fatalf("record is %T, want ResumeRecord", result.SessionRecords[0])
	}
	if record.Reason != "user finished guidance" {
		t.Fatalf("record = %+v, want guidance reason", record)
	}
}

func TestInteractionReducerTracksInterruptAndResumeRecords(t *testing.T) {
	history := logs.NewSessionHistory("session-010")
	interruptTool := InterruptTool{}
	resumeTool := ResumeTool{}

	interruptResult, err := interruptTool.Execute(ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-interrupt-010", ToolName: "interrupt_session", Arguments: map[string]string{"reason": "clarification needed"}}},
	})
	if err != nil {
		t.Fatalf("interrupt execute: %v", err)
	}
	for _, record := range interruptResult.SessionRecords {
		history.Append(record)
	}
	interrupted := runtime.ReduceInteractionMode(history)
	if interrupted.Mode != runtime.InteractionInterrupted {
		t.Fatalf("Mode = %q, want %q", interrupted.Mode, runtime.InteractionInterrupted)
	}

	resumeResult, err := resumeTool.Execute(ExecutionRequest{
		History: history,
		Call:    provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-resume-010", ToolName: "resume_session", Arguments: map[string]string{"reason": "user clarified"}}},
	})
	if err != nil {
		t.Fatalf("resume execute: %v", err)
	}
	for _, record := range resumeResult.SessionRecords {
		history.Append(record)
	}
	resumable := runtime.ReduceInteractionMode(history)
	if resumable.Mode != runtime.InteractionResumable {
		t.Fatalf("Mode = %q, want %q", resumable.Mode, runtime.InteractionResumable)
	}
	if resumable.Reason != "user clarified" {
		t.Fatalf("Reason = %q, want user clarified", resumable.Reason)
	}
}
