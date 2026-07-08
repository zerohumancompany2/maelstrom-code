package logs

import "fmt"

type WorkflowRecord interface {
	RecordID() string
	RecordKind() string
}

type WorkflowBaseRecord struct {
	ID   string
	Kind string
}

func (b WorkflowBaseRecord) RecordID() string   { return b.ID }
func (b WorkflowBaseRecord) RecordKind() string { return b.Kind }

type WorkflowTransitionRecord struct {
	WorkflowBaseRecord
	FromState      string
	ToState        string
	Trigger        string
	DerivedFromIDs []string
}

type WorkflowBindingRefRecord struct {
	WorkflowBaseRecord
	BindingID string
	AgentID   string
	Action    string
}

type WorkflowNoteRecord struct {
	WorkflowBaseRecord
	AuthorAgent string
	Content     string
}

// WorkflowStateExitRecord mirrors a workflow state exit into the workflow's
// own durable history so workflow lifecycle (including finalization outcomes)
// is reconstructable across sessions and agents without any one session log.
type WorkflowStateExitRecord struct {
	WorkflowBaseRecord
	StateName      string
	Reason         string
	ByAgent        string
	DerivedFromIDs []string
}

type WorkflowHistory struct {
	WorkflowID string
	Records    []WorkflowRecord
	sequence   int
}

func NewWorkflowHistory(workflowID string) *WorkflowHistory {
	return &WorkflowHistory{WorkflowID: workflowID}
}

func (h *WorkflowHistory) NextRecord(kind string) WorkflowBaseRecord {
	h.sequence++
	return WorkflowBaseRecord{ID: fmt.Sprintf("%s-wr%03d", h.WorkflowID, h.sequence), Kind: kind}
}

func (h *WorkflowHistory) Append(record WorkflowRecord) {
	h.Records = append(h.Records, record)
}
