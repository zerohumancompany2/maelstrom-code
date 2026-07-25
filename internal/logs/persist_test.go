package logs

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoadStateRoundTrips(t *testing.T) {
	session := NewSessionHistory("session-persist-001")
	session.AgentID = "agent-persist"
	session.Append(UserMessageRecord{SessionBaseRecord: session.NextRecord("user"), Content: "hello"})
	session.Append(StateEnterRecord{SessionBaseRecord: session.NextRecord("state_enter"), Chart: "cognitive", StateName: "observe"})
	session.Append(AssistantMessageRecord{SessionBaseRecord: session.NextRecord("assistant"), Content: "hi"})
	session.Append(StateExitRecord{SessionBaseRecord: session.NextRecord("state_exit"), Chart: "cognitive", StateName: "observe", Reason: "completed", CompletionAccepted: true})
	session.NextBundleID()
	workflow := NewWorkflowHistory("workflow-persist-001")
	workflow.Append(WorkflowTransitionRecord{WorkflowBaseRecord: workflow.NextRecord("workflow_transition"), FromState: "planning", ToState: "implementing", Trigger: "start"})
	workflow.Append(WorkflowArtifactRecord{WorkflowBaseRecord: workflow.NextRecord("workflow_artifact"), StateName: "planning", SchemaName: "plan_v1", ByAgent: "builder", Content: `{"plan":"ship"}`, DerivedFromIDs: []string{"src-001"}})

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
	if _, ok := loadedSession.Records[1].(*StateEnterRecord); !ok {
		t.Fatalf("record[1] = %T, want *StateEnterRecord", loadedSession.Records[1])
	}
	if exit, ok := loadedSession.Records[3].(*StateExitRecord); !ok || !exit.CompletionAccepted {
		t.Fatalf("record[3] = %#v, want accepted *StateExitRecord", loadedSession.Records[3])
	}
	if loadedWorkflow == nil || loadedWorkflow.WorkflowID != workflow.WorkflowID || len(loadedWorkflow.Records) != 2 {
		t.Fatalf("loaded workflow mismatch: %+v", loadedWorkflow)
	}
	artifact, ok := loadedWorkflow.Records[1].(*WorkflowArtifactRecord)
	if !ok {
		t.Fatalf("record[1] = %T, want *WorkflowArtifactRecord", loadedWorkflow.Records[1])
	}
	if artifact.StateName != "planning" || artifact.SchemaName != "plan_v1" || artifact.ByAgent != "builder" || artifact.Content != `{"plan":"ship"}` || len(artifact.DerivedFromIDs) != 1 || artifact.DerivedFromIDs[0] != "src-001" {
		t.Fatalf("loaded artifact = %#v, want round-tripped WorkflowArtifactRecord", artifact)
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
