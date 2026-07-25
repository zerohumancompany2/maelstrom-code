package tools

import (
	"fmt"

	"github.com/comalice/maelstrom/internal/logs"
)

type InterruptTool struct{}

func (InterruptTool) Definition() Definition {
	return Definition{
		Name:        "interrupt_session",
		Description: "Interrupt the current session and return to interactive input",
		Parameters:  map[string]string{"reason": "string", "requested_by": "string"},
	}
}

func (InterruptTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	reason := request.Call.Call.Arguments["reason"]
	requestedBy := request.Call.Call.Arguments["requested_by"]
	if requestedBy == "" {
		requestedBy = "user"
	}
	record := logs.InterruptRecord{
		SessionBaseRecord: request.History.NextRecord("interrupt"),
		Reason:            reason,
		RequestedBy:       requestedBy,
		DerivedFromIDs:    []string{request.Call.Call.CallID},
	}
	content := "session interrupted"
	if reason != "" {
		content = fmt.Sprintf("session interrupted: %s", reason)
	}
	return ExecutionResult{
		ToolName:       "interrupt_session",
		DisplayContent: content,
		SessionRecords: []logs.SessionRecord{record},
	}, nil
}

type ResumeTool struct{}

func (ResumeTool) Definition() Definition {
	return Definition{
		Name:        "resume_session",
		Description: "Resume the current session after user interaction",
		Parameters:  map[string]string{"reason": "string"},
	}
}

func (ResumeTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	reason := request.Call.Call.Arguments["reason"]
	record := logs.ResumeRecord{
		SessionBaseRecord: request.History.NextRecord("resume"),
		Reason:            reason,
		DerivedFromIDs:    []string{request.Call.Call.CallID},
	}
	content := "session resumable"
	if reason != "" {
		content = fmt.Sprintf("session resumable: %s", reason)
	}
	return ExecutionResult{
		ToolName:       "resume_session",
		DisplayContent: content,
		SessionRecords: []logs.SessionRecord{record},
	}, nil
}
