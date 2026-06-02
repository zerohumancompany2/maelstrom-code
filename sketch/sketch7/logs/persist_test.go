package logs

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoadStateRoundTrips(t *testing.T) {
	session := NewSessionHistory("session-persist-001")
	session.Append(UserMessageRecord{SessionBaseRecord: session.NextRecord("user"), Content: "hello"})
	session.Append(AssistantMessageRecord{SessionBaseRecord: session.NextRecord("assistant"), Content: "hi"})
	session.NextBundleID()
	workflow := NewWorkflowHistory("workflow-persist-001")
	workflow.Append(WorkflowTransitionRecord{WorkflowBaseRecord: workflow.NextRecord("workflow_transition"), FromState: "planning", ToState: "implementing", Trigger: "start"})

	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveState(path, session, workflow); err != nil {
		t.Fatalf("save state: %v", err)
	}
	loadedSession, loadedWorkflow, err := LoadState(path)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if loadedSession.SessionID != session.SessionID || len(loadedSession.Records) != len(session.Records) {
		t.Fatalf("loaded session mismatch: %+v", loadedSession)
	}
	if loadedSession.bundles != 1 {
		t.Fatalf("loaded bundles = %d, want 1", loadedSession.bundles)
	}
	if loadedWorkflow == nil || loadedWorkflow.WorkflowID != workflow.WorkflowID || len(loadedWorkflow.Records) != 1 {
		t.Fatalf("loaded workflow mismatch: %+v", loadedWorkflow)
	}
}
