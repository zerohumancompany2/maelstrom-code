package logs

import "fmt"

type SessionRecord interface {
	RecordID() string
	RecordKind() string
}

type SessionBaseRecord struct {
	ID   string
	Kind string
}

func (b SessionBaseRecord) RecordID() string   { return b.ID }
func (b SessionBaseRecord) RecordKind() string { return b.Kind }

type UserMessageRecord struct {
	SessionBaseRecord
	Content string
}

type AssistantMessageRecord struct {
	SessionBaseRecord
	Content   string
	Reasoning string
}

type ToolCallRequestRecord struct {
	SessionBaseRecord
	CallID    string
	ToolName  string
	Arguments string
}

type ToolCallResultRecord struct {
	SessionBaseRecord
	CallID   string
	ToolName string
	Content  string
	IsError  bool
}

type CognitiveTransitionRecord struct {
	SessionBaseRecord
	FromState      string
	ToState        string
	Trigger        string
	DerivedFromIDs []string
}

type SessionWorkflowBindingRecord struct {
	SessionBaseRecord
	BindingID  string
	WorkflowID string
	Action     string
}

type WorkflowTransitionRefRecord struct {
	SessionBaseRecord
	WorkflowID       string
	FromState        string
	ToState          string
	Trigger          string
	DerivedFromIDs   []string
	WorkflowRecordID string
}

type InterruptRecord struct {
	SessionBaseRecord
	Reason         string
	RequestedBy    string
	DerivedFromIDs []string
}

type ResumeRecord struct {
	SessionBaseRecord
	Reason         string
	DerivedFromIDs []string
}

type SessionHistory struct {
	SessionID string
	Records   []SessionRecord
	sequence  int
	bundles   int
}

func NewSessionHistory(sessionID string) *SessionHistory {
	return &SessionHistory{SessionID: sessionID}
}

func (h *SessionHistory) NextRecord(kind string) SessionBaseRecord {
	h.sequence++
	return SessionBaseRecord{ID: fmt.Sprintf("%s-r%03d", h.SessionID, h.sequence), Kind: kind}
}

func (h *SessionHistory) NextBundleID() string {
	h.bundles++
	return fmt.Sprintf("%s-bundle-%03d", h.SessionID, h.bundles)
}

func (h *SessionHistory) Append(record SessionRecord) {
	h.Records = append(h.Records, record)
}
