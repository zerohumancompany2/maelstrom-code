package workflow

import "fmt"

type Instance struct {
	ID     string
	SpecID string
	Input  string
}

type Record interface {
	RecordID() string
	RecordKind() string
}

type BaseRecord struct {
	ID   string
	Kind string
}

func (b BaseRecord) RecordID() string   { return b.ID }
func (b BaseRecord) RecordKind() string { return b.Kind }

type StateTransitionRecord struct {
	BaseRecord
	FromState      string
	ToState        string
	Trigger        string
	DerivedFromIDs []string
}

type BindingRefRecord struct {
	BaseRecord
	BindingID string
	AgentID   string
	Action    string
}

type History struct {
	WorkflowID string
	Records    []Record
	sequence   int
}

func NewHistory(workflowID string) *History {
	return &History{WorkflowID: workflowID}
}

func (h *History) NextRecord(kind string) BaseRecord {
	h.sequence++
	return BaseRecord{ID: fmt.Sprintf("%s-wr%03d", h.WorkflowID, h.sequence), Kind: kind}
}

func (h *History) Append(record Record) {
	h.Records = append(h.Records, record)
}

func DescribeRecord(record Record) string {
	switch v := record.(type) {
	case StateTransitionRecord:
		return fmt.Sprintf("%s workflow_state_transition(from=%s to=%s trigger=%s from_records=%v)", v.RecordID(), v.FromState, v.ToState, v.Trigger, v.DerivedFromIDs)
	case BindingRefRecord:
		return fmt.Sprintf("%s workflow_binding_ref(binding=%s agent=%s action=%s)", v.RecordID(), v.BindingID, v.AgentID, v.Action)
	default:
		return fmt.Sprintf("unknown(%T)", record)
	}
}
