package runner

import (
	"reflect"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/tools"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
)

func TestBuildCognitiveStateMapCopiesAuthoredStateMetadata(t *testing.T) {
	definition := agent.Definition{Cognitive: agent.StatechartDefinition{States: []agent.StateDefinition{{
		Name:         "observe",
		VisibleTools: []string{"transition_state", "weather"},
		EnabledTools: []string{"transition_state"},
		Prompt:       "Observe before acting.",
	}}}}

	states := BuildCognitiveStateMap(definition)
	snapshot, ok := states["observe"]
	if !ok {
		t.Fatal("missing cognitive snapshot for observe")
	}
	if snapshot.CurrentState != "observe" {
		t.Fatalf("current state = %q, want observe", snapshot.CurrentState)
	}
	if !reflect.DeepEqual(snapshot.VisibleTools, []string{"transition_state", "weather"}) {
		t.Fatalf("visible tools = %#v", snapshot.VisibleTools)
	}
	if !reflect.DeepEqual(snapshot.EnabledTools, []string{"transition_state"}) {
		t.Fatalf("enabled tools = %#v", snapshot.EnabledTools)
	}
	if snapshot.Prompt != "Observe before acting." {
		t.Fatalf("prompt = %q, want authored prompt", snapshot.Prompt)
	}

	definition.Cognitive.States[0].VisibleTools[0] = "mutated"
	if states["observe"].VisibleTools[0] != "transition_state" {
		t.Fatal("expected BuildCognitiveStateMap to copy tool slices")
	}
}

func TestReduceWorkflowStateFromHistoryUsesLatestDurableTransition(t *testing.T) {
	base := &runtime.WorkflowSnapshot{
		WorkflowID:     "workflow-001",
		CurrentState:   "available",
		VisibleTools:   []string{"transition_state", "weather"},
		EnabledTools:   []string{"transition_state"},
		Description:    "Weather lookup",
		Context:        "Paris",
		LastBoundAgent: "weather-agent",
	}
	history := workflow.NewHistory("workflow-001")
	history.Append(workflow.StateTransitionRecord{BaseRecord: history.NextRecord("workflow_state_transition"), FromState: "available", ToState: "lookup_pending", Trigger: "begin_lookup"})
	history.Append(workflow.NoteRecord{BaseRecord: history.NextRecord("workflow_note"), AuthorAgent: "weather-agent", Content: "Looking up weather"})
	history.Append(workflow.StateTransitionRecord{BaseRecord: history.NextRecord("workflow_state_transition"), FromState: "lookup_pending", ToState: "data_ready", Trigger: "weather_received"})

	derived := runtime.ReduceWorkflowStateFromHistory(history, base)

	if derived.CurrentState != "data_ready" {
		t.Fatalf("current state = %q, want data_ready", derived.CurrentState)
	}
	if derived.Description != base.Description || derived.Context != base.Context {
		t.Fatalf("derived snapshot lost metadata: %+v", derived)
	}
	if base.CurrentState != "available" {
		t.Fatalf("base snapshot mutated to %q", base.CurrentState)
	}
}

func TestReduceWorkflowStateFromHistoryBackfillsLastBoundAgentFromBindingHistory(t *testing.T) {
	base := &runtime.WorkflowSnapshot{WorkflowID: "workflow-001", CurrentState: "available"}
	history := workflow.NewHistory("workflow-001")
	history.Append(workflow.BindingRefRecord{BaseRecord: history.NextRecord("binding_ref"), BindingID: "bind-1", AgentID: "weather-agent", Action: "bind"})

	derived := runtime.ReduceWorkflowStateFromHistory(history, base)

	if derived.LastBoundAgent != "weather-agent" {
		t.Fatalf("last bound agent = %q, want weather-agent", derived.LastBoundAgent)
	}
}

func TestReduceWorkflowStateFromHistoryReturnsZeroWhenBaseMissing(t *testing.T) {
	history := workflow.NewHistory("workflow-001")
	history.Append(workflow.StateTransitionRecord{BaseRecord: history.NextRecord("workflow_state_transition"), FromState: "available", ToState: "data_ready", Trigger: "weather_received"})

	derived := runtime.ReduceWorkflowStateFromHistory(history, nil)

	if !reflect.DeepEqual(derived, runtime.WorkflowSnapshot{}) {
		t.Fatalf("derived = %+v, want zero snapshot", derived)
	}
}

func TestReduceCognitiveStateFromHistoryUsesLatestAgentTransition(t *testing.T) {
	history := session.NewHistory("session-1")
	history.Append(session.StateTransitionRecord{BaseRecord: history.NextRecord("state_transition"), ChartName: "workflow", FromState: "available", ToState: "lookup_pending", Trigger: "begin_lookup"})
	history.Append(session.StateTransitionRecord{BaseRecord: history.NextRecord("state_transition"), ChartName: "agent", FromState: "observe", ToState: "act", Trigger: "begin_action"})

	states := map[string]runtime.CognitiveSnapshot{
		"observe": {CurrentState: "observe", Prompt: "Observe."},
		"act":     {CurrentState: "act", Prompt: "Act."},
	}

	derived := runtime.ReduceCognitiveStateFromHistory(history, "observe", states)

	if derived.CurrentState != "act" || derived.Prompt != "Act." {
		t.Fatalf("derived cognitive = %+v, want act snapshot", derived)
	}
}

func TestReduceCognitiveStateFromHistoryFallsBackToInitialState(t *testing.T) {
	derived := runtime.ReduceCognitiveStateFromHistory(session.NewHistory("session-1"), "observe", map[string]runtime.CognitiveSnapshot{
		"observe": {CurrentState: "observe", Prompt: "Observe."},
	})

	if derived.CurrentState != "observe" {
		t.Fatalf("current state = %q, want observe", derived.CurrentState)
	}
}

func TestReduceBindingStateUsesLatestBindingRecord(t *testing.T) {
	history := workflow.NewHistory("workflow-1")
	history.Append(workflow.BindingRefRecord{BaseRecord: history.NextRecord("binding_ref"), BindingID: "bind-1", AgentID: "agent-1", Action: "bind"})
	history.Append(workflow.BindingRefRecord{BaseRecord: history.NextRecord("binding_ref"), BindingID: "bind-2", AgentID: "agent-1", Action: "unbind"})

	binding := runtime.ReduceBindingState(history.Records)

	if binding.WorkflowID != "bind-2" || binding.Bound {
		t.Fatalf("binding = %+v, want latest unbound binding snapshot", binding)
	}
}

func TestConsumeProviderOutputForToolRequestWrapsRequestRecordsAndToolResult(t *testing.T) {
	loop := Loop{Tools: consumeToolExecutor{result: tools.ExecutionResult{
		ToolName:       "weather",
		DisplayContent: "70C, rainy, winds out of SSW in Paris",
		Records: []session.Record{
			session.StateTransitionRecord{BaseRecord: session.BaseRecord{ID: "manual-transition", Kind: "state_transition"}, ChartName: "workflow", FromState: "lookup_pending", ToState: "data_ready", Trigger: "weather_received"},
		},
	}}}
	history := session.NewHistory("session-1")
	output := provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-1", ToolName: "weather", RawArgs: []byte(`{"location":"Paris"}`)}}

	records, err := loop.consumeProviderOutput(runtime.Agent{Name: "weather-agent"}, history, output)
	if err != nil {
		t.Fatalf("consumeProviderOutput returned error: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("record count = %d, want 3", len(records))
	}
	requestRecord, ok := records[0].(session.ToolCallRequestRecord)
	if !ok {
		t.Fatalf("records[0] type = %T, want ToolCallRequestRecord", records[0])
	}
	if requestRecord.ToolName != "weather" || requestRecord.Arguments != `{"location":"Paris"}` {
		t.Fatalf("request record = %+v", requestRecord)
	}
	resultRecord, ok := records[2].(session.ToolCallResultRecord)
	if !ok {
		t.Fatalf("records[2] type = %T, want ToolCallResultRecord", records[2])
	}
	if resultRecord.ToolName != "weather" || resultRecord.Content != "70C, rainy, winds out of SSW in Paris" || resultRecord.IsError {
		t.Fatalf("result record = %+v", resultRecord)
	}
}

func TestConsumeProviderOutputForAssistantMessage(t *testing.T) {
	loop := Loop{}
	history := session.NewHistory("session-1")

	records, err := loop.consumeProviderOutput(runtime.Agent{Name: "weather-agent"}, history, provider.AssistantOutput{Content: "done"})
	if err != nil {
		t.Fatalf("consumeProviderOutput returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	assistant, ok := records[0].(session.AssistantMessageRecord)
	if !ok {
		t.Fatalf("records[0] type = %T, want AssistantMessageRecord", records[0])
	}
	if assistant.Content != "done" {
		t.Fatalf("assistant content = %q, want done", assistant.Content)
	}
}

type consumeToolExecutor struct {
	result tools.ExecutionResult
	err    error
}

func (c consumeToolExecutor) Execute(request tools.ExecutionRequest) (tools.ExecutionResult, error) {
	return c.result, c.err
}
