package logs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type persistedState struct {
	SessionID       string            `json:"session_id"`
	AgentID         string            `json:"agent_id,omitempty"`
	SessionSequence int               `json:"session_sequence"`
	SessionBundles  int               `json:"session_bundles"`
	SessionRecords  []json.RawMessage `json:"session_records"`
	WorkflowID      string            `json:"workflow_id,omitempty"`
	WorkflowSeq     int               `json:"workflow_sequence,omitempty"`
	WorkflowRecords []json.RawMessage `json:"workflow_records,omitempty"`
}

type recordEnvelope struct {
	Kind string          `json:"kind"`
	Body json.RawMessage `json:"body"`
}

func SaveState(path string, session *SessionHistory, workflow *WorkflowHistory) error {
	state := persistedState{}
	if session != nil {
		state.SessionID = session.SessionID
		state.AgentID = session.AgentID
		state.SessionSequence = session.sequence
		state.SessionBundles = session.bundles
		for _, record := range session.Records {
			raw, err := marshalSessionRecord(record)
			if err != nil {
				return err
			}
			state.SessionRecords = append(state.SessionRecords, raw)
		}
	}
	if workflow != nil {
		state.WorkflowID = workflow.WorkflowID
		state.WorkflowSeq = workflow.sequence
		for _, record := range workflow.Records {
			raw, err := marshalWorkflowRecord(record)
			if err != nil {
				return err
			}
			state.WorkflowRecords = append(state.WorkflowRecords, raw)
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func LoadState(path string) (*SessionHistory, *WorkflowHistory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var state persistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, nil, err
	}
	session := NewSessionHistory(state.SessionID)
	session.AgentID = state.AgentID
	session.sequence = state.SessionSequence
	session.bundles = state.SessionBundles
	for _, item := range state.SessionRecords {
		record, err := unmarshalSessionRecord(item)
		if err != nil {
			return nil, nil, err
		}
		session.Records = append(session.Records, record)
	}
	var workflow *WorkflowHistory
	if state.WorkflowID != "" {
		workflow = NewWorkflowHistory(state.WorkflowID)
		workflow.sequence = state.WorkflowSeq
		for _, item := range state.WorkflowRecords {
			record, err := unmarshalWorkflowRecord(item)
			if err != nil {
				return nil, nil, err
			}
			workflow.Records = append(workflow.Records, record)
		}
	}
	return session, workflow, nil
}

func marshalSessionRecord(record SessionRecord) (json.RawMessage, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	return json.Marshal(recordEnvelope{Kind: record.RecordKind(), Body: body})
}

func unmarshalSessionRecord(raw json.RawMessage) (SessionRecord, error) {
	var env recordEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	record, err := newSessionRecordByKind(env.Kind)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(env.Body, record); err != nil {
		return nil, err
	}
	return record, nil
}

func marshalWorkflowRecord(record WorkflowRecord) (json.RawMessage, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	return json.Marshal(recordEnvelope{Kind: record.RecordKind(), Body: body})
}

func unmarshalWorkflowRecord(raw json.RawMessage) (WorkflowRecord, error) {
	var env recordEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	record, err := newWorkflowRecordByKind(env.Kind)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(env.Body, record); err != nil {
		return nil, err
	}
	return record, nil
}

func newSessionRecordByKind(kind string) (SessionRecord, error) {
	switch kind {
	case "user":
		return &UserMessageRecord{}, nil
	case "assistant":
		return &AssistantMessageRecord{}, nil
	case "tool_call_request":
		return &ToolCallRequestRecord{}, nil
	case "tool_call_result":
		return &ToolCallResultRecord{}, nil
	case "cognitive_transition":
		return &CognitiveTransitionRecord{}, nil
	case "workflow_binding_ref":
		return &SessionWorkflowBindingRecord{}, nil
	case "workflow_transition_ref":
		return &WorkflowTransitionRefRecord{}, nil
	case "interrupt":
		return &InterruptRecord{}, nil
	case "resume":
		return &ResumeRecord{}, nil
	case "context_snapshot":
		return &ContextSnapshotRecord{}, nil
	case "inference_envelope":
		return &InferenceEnvelopeRecord{}, nil
	case "output_contract_evaluation":
		return &OutputContractEvaluationRecord{}, nil
	case "tool_validation":
		return &ToolValidationRecord{}, nil
	case "retry":
		return &RetryRecord{}, nil
	case "completion":
		return &CompletionRecord{}, nil
	default:
		return nil, fmt.Errorf("unknown session record kind %q", kind)
	}
}

func newWorkflowRecordByKind(kind string) (WorkflowRecord, error) {
	switch kind {
	case "workflow_transition":
		return &WorkflowTransitionRecord{}, nil
	case "workflow_binding_ref":
		return &WorkflowBindingRefRecord{}, nil
	case "workflow_note":
		return &WorkflowNoteRecord{}, nil
	default:
		return nil, fmt.Errorf("unknown workflow record kind %q", kind)
	}
}
