package session

import "fmt"

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

type UserMessageRecord struct {
	BaseRecord
	Content string
}

type AssistantMessageRecord struct {
	BaseRecord
	Content string
}

type ToolCallRequestRecord struct {
	BaseRecord
	CallID    string
	ToolName  string
	Arguments string
}

type ToolCallResultRecord struct {
	BaseRecord
	CallID   string
	ToolName string
	Content  string
	IsError  bool
}

type StateTransitionRecord struct {
	BaseRecord
	ChartName      string
	FromState      string
	ToState        string
	Trigger        string
	DerivedFromIDs []string
}

type WorkflowBindingRefRecord struct {
	BaseRecord
	BindingID  string
	WorkflowID string
	Action     string
}

type History struct {
	SessionID string
	Records   []Record
	sequence  int
	bundles   int
}

func NewHistory(sessionID string) *History {
	return &History{SessionID: sessionID}
}

func (h *History) NextRecord(kind string) BaseRecord {
	h.sequence++
	return BaseRecord{ID: fmt.Sprintf("%s-r%03d", h.SessionID, h.sequence), Kind: kind}
}

func (h *History) NextBundleID() string {
	h.bundles++
	return fmt.Sprintf("%s-bundle-%03d", h.SessionID, h.bundles)
}

func (h *History) Append(record Record) {
	h.Records = append(h.Records, record)
}

func DescribeRecord(record Record) string {
	switch v := record.(type) {
	case UserMessageRecord:
		return fmt.Sprintf("%s user(%q)", v.RecordID(), v.Content)
	case AssistantMessageRecord:
		return fmt.Sprintf("%s assistant(%q)", v.RecordID(), v.Content)
	case ToolCallRequestRecord:
		return fmt.Sprintf("%s tool_call_request(call_id=%s tool=%s args=%s)", v.RecordID(), v.CallID, v.ToolName, v.Arguments)
	case ToolCallResultRecord:
		return fmt.Sprintf("%s tool_call_result(call_id=%s tool=%s content=%q error=%t)", v.RecordID(), v.CallID, v.ToolName, v.Content, v.IsError)
	case StateTransitionRecord:
		return fmt.Sprintf("%s state_transition(chart=%s from=%s to=%s trigger=%s from_records=%v)", v.RecordID(), v.ChartName, v.FromState, v.ToState, v.Trigger, v.DerivedFromIDs)
	case WorkflowBindingRefRecord:
		return fmt.Sprintf("%s workflow_binding_ref(binding=%s workflow=%s action=%s)", v.RecordID(), v.BindingID, v.WorkflowID, v.Action)
	default:
		return fmt.Sprintf("unknown(%T)", record)
	}
}
