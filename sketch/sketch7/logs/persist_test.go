package logs

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoadStateRoundTrips(t *testing.T) {
	session := NewSessionHistory("session-persist-001")
	session.AgentID = "agent-persist"
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
	if loadedSession.AgentID != session.AgentID {
		t.Fatalf("loaded agent id = %q, want %q", loadedSession.AgentID, session.AgentID)
	}
	if loadedSession.bundles != 1 {
		t.Fatalf("loaded bundles = %d, want 1", loadedSession.bundles)
	}
	if loadedWorkflow == nil || loadedWorkflow.WorkflowID != workflow.WorkflowID || len(loadedWorkflow.Records) != 1 {
		t.Fatalf("loaded workflow mismatch: %+v", loadedWorkflow)
	}
}

func TestFileSessionStoreRoundTripsBySessionID(t *testing.T) {
	store := FileSessionStore{SessionDir: t.TempDir()}
	session := NewSessionHistory("session-store-001")
	session.AgentID = "agent-store"
	session.Append(UserMessageRecord{SessionBaseRecord: session.NextRecord("user"), Content: "hello"})
	if err := store.SaveSession(session.SessionID, session, nil); err != nil {
		t.Fatalf("save session: %v", err)
	}
	loadedSession, _, err := store.LoadSession(session.SessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if loadedSession.SessionID != session.SessionID || loadedSession.AgentID != session.AgentID {
		t.Fatalf("loaded session mismatch: %+v", loadedSession)
	}
	ids, err := store.ListSessions()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(ids) != 1 || ids[0] != session.SessionID {
		t.Fatalf("session ids = %#v, want [%q]", ids, session.SessionID)
	}
}
