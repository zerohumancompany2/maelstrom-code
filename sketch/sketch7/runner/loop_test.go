package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch7/statecharts"
	"github.com/comalice/inference_sketch/sketch/sketch7/tools"
)

type scriptedProvider struct {
	responses []provider.Response
	requests  []provider.Request
	builds    []provider.Request
	index     int
}

func (s *scriptedProvider) BuildRequest(agent runtime.Agent, payload prompt.Payload, tools []provider.ToolDefinition) (provider.Request, error) {
	request, err := provider.BuildRequest(agent, payload, tools)
	if err != nil {
		return provider.Request{}, err
	}
	s.builds = append(s.builds, request)
	return request, nil
}

func (s *scriptedProvider) Send(request provider.Request) (provider.Response, error) {
	s.requests = append(s.requests, request)
	if s.index >= len(s.responses) {
		return provider.Response{}, fmt.Errorf("no scripted response for request %d", s.index)
	}
	response := s.responses[s.index]
	s.index++
	return response, nil
}

func TestBuildSessionViewReducesBoundWorkflowState(t *testing.T) {
	agent := runtime.Agent{Name: "builder"}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", Prompt: "Observe first."}}}}
	workflowDef := defs.WorkflowDefinition{Description: "Conversation flow", Context: "Implement feature", Statechart: defs.StatechartDefinition{InitialState: "planning", States: []defs.StateDefinition{{Name: "planning"}, {Name: "implementing"}}}}
	sessionHistory := logs.NewSessionHistory("session-001")
	sessionHistory.Append(logs.SessionWorkflowBindingRecord{SessionBaseRecord: sessionHistory.NextRecord("workflow_binding_ref"), BindingID: "bind-001", WorkflowID: "workflow-001", Action: "bind"})
	workflowHistory := logs.NewWorkflowHistory("workflow-001")
	workflowHistory.Append(logs.WorkflowTransitionRecord{WorkflowBaseRecord: workflowHistory.NextRecord("workflow_transition"), FromState: "planning", ToState: "implementing", Trigger: "start_implementation"})

	view := BuildSessionView(agent, agentDef, &workflowDef, sessionHistory, workflowHistory)
	if !view.Binding.Bound {
		t.Fatal("expected binding to be active")
	}
	if view.Workflow == nil || view.Workflow.CurrentState != "implementing" {
		t.Fatalf("workflow state = %+v, want implementing", view.Workflow)
	}
}

func TestLoopRunProcessesToolCallAndAssistantResponse(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"chart": "agent", "trigger": "begin_action"})
	fakeProvider := &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{
		provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-001", ToolName: "transition_state", Arguments: map[string]string{"chart": "agent", "trigger": "begin_action"}, RawArgs: raw}},
		provider.AssistantOutput{Content: "Done."},
	}}}
	toolRegistry := tools.NewRegistry(tools.TransitionTool{AgentChart: statecharts.Compile("agent", defs.StatechartDefinition{InitialState: "observe", Transitions: []defs.TransitionDefinition{{Trigger: "begin_action", From: "observe", To: "act"}}})})
	loop := Loop{
		Provider: fakeProvider,
		Tools:    toolRegistry,
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 10,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model"}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", Prompt: "Observe first."}, {Name: "act", Prompt: "Act now."}}}}
	sessionHistory := logs.NewSessionHistory("session-002")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Do the thing."})

	err := loop.Run(agent, agentDef, nil, sessionHistory, nil)
	if err == nil {
		foundAssistant := false
		foundTransition := false
		for _, record := range sessionHistory.Records {
			switch v := record.(type) {
			case logs.AssistantMessageRecord:
				if v.Content == "Done." {
					foundAssistant = true
				}
			case logs.CognitiveTransitionRecord:
				if v.ToState == "act" {
					foundTransition = true
				}
			}
		}
		if !foundAssistant || !foundTransition {
			t.Fatalf("expected assistant and transition records, got %#v", sessionHistory.Records)
		}
		return
	}
	if err.Error() != "loop guard tripped" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoopRunStopsWhenProviderReturnsOnlyAssistantOutput(t *testing.T) {
	fakeProvider := &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{
		provider.AssistantOutput{Content: "All done."},
	}}}
	loop := Loop{
		Provider: fakeProvider,
		Tools:    tools.NewRegistry(),
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 10,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model"}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe"}}
	sessionHistory := logs.NewSessionHistory("session-003")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Summarize the current task."})

	err := loop.Run(agent, agentDef, nil, sessionHistory, nil)
	if err != nil && err.Error() != "loop guard tripped" {
		t.Fatalf("unexpected error: %v", err)
	}
	foundAssistant := false
	for _, record := range sessionHistory.Records {
		if msg, ok := record.(logs.AssistantMessageRecord); ok && msg.Content == "All done." {
			foundAssistant = true
		}
	}
	if !foundAssistant {
		t.Fatalf("expected assistant message in session history, got %#v", sessionHistory.Records)
	}
	foundEnvelope := false
	foundContextSnapshot := false
	for _, record := range sessionHistory.Records {
		switch record.(type) {
		case logs.InferenceEnvelopeRecord:
			foundEnvelope = true
		case logs.ContextSnapshotRecord:
			foundContextSnapshot = true
		}
	}
	if !foundEnvelope {
		t.Fatalf("expected inference envelope record, got %#v", sessionHistory.Records)
	}
	if foundContextSnapshot {
		return
	}
}

func TestLoopRunPersistsAndIncludesStateTaskContextSnapshot(t *testing.T) {
	providerScript := &scriptedProvider{responses: []provider.Response{{Outputs: []provider.Output{provider.AssistantOutput{Content: "Done."}}}}}
	loop := Loop{
		Provider: providerScript,
		Tools:    tools.NewRegistry(),
		Projections: []prompt.Projection{
			prompt.ContextProjection{},
		},
		MaxHistory: 10,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: []string{"read_file", "run_command"}}
	agentDef := defs.AgentDefinition{
		Context: defs.ContextDefinition{Projections: []defs.ProjectionDefinition{{Type: "state_task"}}},
		Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{
			Name:         "observe",
			Prompt:       "Gather evidence before acting.",
			EnabledTools: []string{"read_file"},
			Outputs:      defs.StateOutputContract{SchemaName: "cognitive_step_v1", RequiredFields: []string{"summary"}},
		}}},
	}
	sessionHistory := logs.NewSessionHistory("session-state-task-snapshot")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Inspect first."})

	if err := loop.Run(agent, agentDef, nil, sessionHistory, nil); err != nil {
		t.Fatalf("loop run: %v", err)
	}

	foundSnapshotID := ""
	foundEnvelope := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.ContextSnapshotRecord:
			if v.LogicalKey == "state_task" && strings.Contains(v.Content, "Current task: Gather evidence before acting.") {
				foundSnapshotID = v.RecordID()
			}
		case logs.InferenceEnvelopeRecord:
			for _, id := range v.IncludedContextRecordIDs {
				if foundSnapshotID != "" && id == foundSnapshotID {
					foundEnvelope = true
				}
			}
		}
	}
	if foundSnapshotID == "" {
		t.Fatalf("expected state_task context snapshot, got %#v", sessionHistory.Records)
	}
	if !foundEnvelope {
		t.Fatalf("expected inference envelope to include state_task snapshot %q, got %#v", foundSnapshotID, sessionHistory.Records)
	}
	if len(providerScript.requests) != 1 {
		t.Fatalf("got %d provider requests, want 1", len(providerScript.requests))
	}
	foundPromptLine := false
	for _, line := range providerScript.requests[0].Lines {
		if strings.Contains(line.Content, "Current task: Gather evidence before acting.") && strings.Contains(line.Content, "Required output schema: cognitive_step_v1") {
			foundPromptLine = true
		}
		for _, forbidden := range []string{"Cognitive mode", "Workflow directive", "Suggested next transitions"} {
			if strings.Contains(line.Content, forbidden) {
				t.Fatalf("state_task prompt line contains forbidden old phrasing %q: %q", forbidden, line.Content)
			}
		}
	}
	if !foundPromptLine {
		t.Fatalf("expected provider request to include state_task prompt line, got %#v", providerScript.requests[0].Lines)
	}
}

func TestLoopRunAppendsInitialStateEnterRecords(t *testing.T) {
	providerScript := &scriptedProvider{responses: []provider.Response{{Outputs: []provider.Output{provider.AssistantOutput{Content: "Done."}}}}}
	loop := Loop{Provider: providerScript, Tools: tools.NewRegistry(), Projections: []prompt.Projection{prompt.ContextProjection{}}, MaxHistory: 10}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model"}
	agentDef := defs.AgentDefinition{
		Context:   defs.ContextDefinition{Projections: []defs.ProjectionDefinition{{Type: "state_task"}}},
		Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", Prompt: "Observe."}}},
	}
	workflowDef := defs.WorkflowDefinition{Statechart: defs.StatechartDefinition{InitialState: "intake", States: []defs.StateDefinition{{Name: "intake"}}}}
	sessionHistory := logs.NewSessionHistory("session-state-enter")
	sessionHistory.Append(logs.SessionWorkflowBindingRecord{SessionBaseRecord: sessionHistory.NextRecord("workflow_binding_ref"), BindingID: "bind-state-enter", WorkflowID: "workflow-state-enter", Action: "bind"})
	workflowHistory := logs.NewWorkflowHistory("workflow-state-enter")

	if err := loop.Run(agent, agentDef, &workflowDef, sessionHistory, workflowHistory); err != nil {
		t.Fatalf("loop run: %v", err)
	}

	foundCognitive := false
	foundWorkflow := false
	for _, record := range sessionHistory.Records {
		enter, ok := record.(logs.StateEnterRecord)
		if !ok {
			continue
		}
		if enter.Chart == "cognitive" && enter.StateName == "observe" {
			foundCognitive = true
		}
		if enter.Chart == "workflow" && enter.StateName == "intake" {
			foundWorkflow = true
		}
	}
	if !foundCognitive || !foundWorkflow {
		t.Fatalf("expected cognitive and workflow enter records, got %#v", sessionHistory.Records)
	}
}

func TestLoopRunFinalizesCognitiveStateWhenInferenceBoundHit(t *testing.T) {
	providerScript := &scriptedProvider{responses: []provider.Response{{Outputs: []provider.Output{provider.AssistantOutput{Content: `{"summary":"done"}`}}}}}
	loop := Loop{Provider: providerScript, Tools: tools.NewRegistry(tools.ReadFileTool{RootDir: t.TempDir()}), Projections: []prompt.Projection{prompt.ContextProjection{}}, MaxHistory: 10}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: []string{"read_file"}}
	agentDef := boundedFinalizationAgentDef(1, 0)
	sessionHistory := logs.NewSessionHistory("session-finalize-valid")
	seedCognitiveBoundHit(sessionHistory)

	if err := loop.Run(agent, agentDef, nil, sessionHistory, nil); err != nil {
		t.Fatalf("loop run: %v", err)
	}
	if len(providerScript.requests) != 1 {
		t.Fatalf("got %d provider requests, want 1", len(providerScript.requests))
	}
	if len(providerScript.requests[0].Tools) != 0 {
		t.Fatalf("finalization request exposed tools: %+v", providerScript.requests[0].Tools)
	}
	requestText := requestText(providerScript.requests[0])
	if !strings.Contains(requestText, "Finalization mode: tool use is closed") {
		t.Fatalf("request text = %q, want finalization instruction", requestText)
	}
	foundEval := false
	foundExit := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.OutputContractEvaluationRecord:
			if v.ValidationStatus == "valid" && v.SourceRecordID != "" && v.ParserVersion != "" && v.HostVersion != "" {
				foundEval = true
			}
		case logs.StateExitRecord:
			if v.Chart == "cognitive" && v.StateName == "observe" && v.Reason == "completed" && v.CompletionAccepted {
				foundExit = true
			}
		}
	}
	if !foundEval || !foundExit {
		t.Fatalf("expected valid eval and accepted state exit, got %#v", sessionHistory.Records)
	}
}

func TestLoopRunRetriesInvalidFinalizationOnceByDefault(t *testing.T) {
	providerScript := &scriptedProvider{responses: []provider.Response{
		{Outputs: []provider.Output{provider.AssistantOutput{Content: `not json`}}},
		{Outputs: []provider.Output{provider.AssistantOutput{Content: `{"summary":"recovered"}`}}},
	}}
	loop := Loop{Provider: providerScript, Tools: tools.NewRegistry(tools.ReadFileTool{RootDir: t.TempDir()}), Projections: []prompt.Projection{prompt.ContextProjection{}}, MaxHistory: 10}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: []string{"read_file"}}
	agentDef := boundedFinalizationAgentDef(1, 0)
	sessionHistory := logs.NewSessionHistory("session-finalize-retry")
	seedCognitiveBoundHit(sessionHistory)

	if err := loop.Run(agent, agentDef, nil, sessionHistory, nil); err != nil {
		t.Fatalf("loop run: %v", err)
	}
	if len(providerScript.requests) != 2 {
		t.Fatalf("got %d provider requests, want 2", len(providerScript.requests))
	}
	for _, request := range providerScript.requests {
		if len(request.Tools) != 0 {
			t.Fatalf("finalization request exposed tools: %+v", request.Tools)
		}
	}
	foundRetry := false
	foundCompletion := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.RetryRecord:
			if v.Reason == "invalid_finalization_output" && v.Attempt == 1 {
				foundRetry = true
			}
		case logs.CompletionRecord:
			if v.Completed && v.StopReason == "state_finalized" {
				foundCompletion = true
			}
		}
	}
	if !foundRetry || !foundCompletion {
		t.Fatalf("expected retry and state_finalized completion, got %#v", sessionHistory.Records)
	}
}

func TestLoopRunFailsAfterFinalizationRetriesExhausted(t *testing.T) {
	providerScript := &scriptedProvider{responses: []provider.Response{
		{Outputs: []provider.Output{provider.AssistantOutput{Content: `not json`}}},
		{Outputs: []provider.Output{provider.AssistantOutput{Content: `still not json`}}},
	}}
	loop := Loop{Provider: providerScript, Tools: tools.NewRegistry(), Projections: []prompt.Projection{prompt.ContextProjection{}}, MaxHistory: 10}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model"}
	agentDef := boundedFinalizationAgentDef(1, 1)
	sessionHistory := logs.NewSessionHistory("session-finalize-fail")
	seedCognitiveBoundHit(sessionHistory)

	err := loop.Run(agent, agentDef, nil, sessionHistory, nil)
	if err == nil || !strings.Contains(err.Error(), "cognitive finalization failed validation") {
		t.Fatalf("error = %v, want finalization validation failure", err)
	}
	foundExit := false
	foundCompletion := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.StateExitRecord:
			if v.Chart == "cognitive" && v.Reason == "validation_failed" {
				foundExit = true
			}
		case logs.CompletionRecord:
			if !v.Completed && v.StopReason == "finalization_validation_failed" {
				foundCompletion = true
			}
		}
	}
	if !foundExit || !foundCompletion {
		t.Fatalf("expected validation failure exit and completion, got %#v", sessionHistory.Records)
	}
}

func TestLoopRunFinalizationDoesNotExecuteReturnedToolRequest(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"command": "echo should-not-run"})
	providerScript := &scriptedProvider{responses: repeatedToolResponses("call-finalize-tool", "run_command", map[string]string{"command": "echo should-not-run"}, raw, 2)}
	loop := Loop{Provider: providerScript, Tools: tools.NewRegistry(tools.RunCommandTool{RootDir: t.TempDir()}), Projections: []prompt.Projection{prompt.ContextProjection{}}, MaxHistory: 10}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: []string{"run_command"}}
	agentDef := boundedFinalizationAgentDef(1, 1)
	sessionHistory := logs.NewSessionHistory("session-finalize-tool")
	seedCognitiveBoundHit(sessionHistory)

	err := loop.Run(agent, agentDef, nil, sessionHistory, nil)
	if err == nil || !strings.Contains(err.Error(), "cognitive finalization failed validation") {
		t.Fatalf("error = %v, want finalization validation failure", err)
	}
	foundRequest := false
	foundResult := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.ToolCallRequestRecord:
			if v.ToolName == "run_command" {
				foundRequest = true
			}
		case logs.ToolCallResultRecord:
			if v.ToolName == "run_command" {
				foundResult = true
			}
		}
	}
	if !foundRequest || foundResult {
		t.Fatalf("expected finalization tool request recorded without execution result, got %#v", sessionHistory.Records)
	}
}

func boundedFinalizationAgentDef(maxInferenceTurns, maxFinalizationRetries int) defs.AgentDefinition {
	return defs.AgentDefinition{
		Context: defs.ContextDefinition{Projections: []defs.ProjectionDefinition{{Type: "state_task"}}},
		Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{
			Name:    "observe",
			Prompt:  "Return the task conclusion.",
			Outputs: defs.StateOutputContract{SchemaName: "cognitive_step_v1", RequiredFields: []string{"summary"}},
			Bounds:  defs.StateBoundsContract{MaxInferenceTurns: maxInferenceTurns, MaxFinalizationRetries: maxFinalizationRetries},
		}}},
	}
}

func seedCognitiveBoundHit(history *logs.SessionHistory) {
	history.Append(logs.StateEnterRecord{SessionBaseRecord: history.NextRecord("state_enter"), Chart: "cognitive", StateName: "observe"})
	history.Append(logs.InferenceEnvelopeRecord{SessionBaseRecord: history.NextRecord("inference_envelope"), PayloadID: "seed", ModelRef: "fake-model", ProviderRef: "fake"})
}

func requestText(request provider.Request) string {
	parts := []string{}
	for _, line := range request.Lines {
		parts = append(parts, line.Content)
	}
	return strings.Join(parts, "\n")
}

func repeatedToolResponses(callIDPrefix, toolName string, args map[string]string, raw []byte, count int) []provider.Response {
	responses := make([]provider.Response, 0, count)
	for i := 0; i < count; i++ {
		responses = append(responses, provider.Response{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: fmt.Sprintf("%s-%d", callIDPrefix, i+1), ToolName: toolName, Arguments: args, RawArgs: raw}},
		}})
	}
	return responses
}

func TestLoopRunRecordsOutputContractEvaluationForAssistantJSON(t *testing.T) {
	fakeProvider := &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{
		provider.AssistantOutput{Content: `{"state":"act","action_type":"final","summary":"done","completion_signal":true,"final_response":"All done."}`},
	}}}
	loop := Loop{
		Provider: fakeProvider,
		Tools:    tools.NewRegistry(),
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
		},
		MaxHistory: 10,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model"}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "act", States: []defs.StateDefinition{{
		Name:   "act",
		Prompt: "Act now.",
		Outputs: defs.StateOutputContract{
			SchemaName:     "coding_step_v1",
			RequiredFields: []string{"state", "action_type", "summary", "completion_signal"},
			Strict:         true,
		},
	}}}}
	sessionHistory := logs.NewSessionHistory("session-003b")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Finish the task."})

	err := loop.Run(agent, agentDef, nil, sessionHistory, nil)
	if err != nil && err.Error() != "loop guard tripped" {
		t.Fatalf("unexpected error: %v", err)
	}
	foundEval := false
	foundCompletion := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.OutputContractEvaluationRecord:
			if v.ValidationStatus == "valid" && v.ActionType == "final" && v.CompletionSignal {
				foundEval = true
			}
		case logs.CompletionRecord:
			if v.Completed && v.StopReason == "assistant_only" {
				foundCompletion = true
			}
		}
	}
	if !foundEval || !foundCompletion {
		t.Fatalf("expected output evaluation and completion records, got %#v", sessionHistory.Records)
	}
}

func TestLoopRunRecordsToolValidationFailure(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{})
	fakeProvider := &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{
		provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-invalid-001", ToolName: "read_file", Arguments: map[string]string{}, RawArgs: raw}},
	}}}
	loop := Loop{
		Provider: fakeProvider,
		Tools:    tools.NewRegistry(tools.ReadFileTool{RootDir: t.TempDir()}),
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
		},
		MaxHistory: 10,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: []string{"read_file"}}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "act", States: []defs.StateDefinition{{
		Name:         "act",
		Prompt:       "Act now.",
		EnabledTools: []string{"read_file"},
	}}}}
	sessionHistory := logs.NewSessionHistory("session-003c")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Read the file."})

	err := loop.Run(agent, agentDef, nil, sessionHistory, nil)
	if err != nil && err.Error() != "loop guard tripped" {
		t.Fatalf("unexpected error: %v", err)
	}
	foundValidation := false
	foundRetry := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.ToolValidationRecord:
			if v.ToolName == "read_file" && !v.Valid && len(v.MissingFields) == 1 && v.MissingFields[0] == "path" {
				foundValidation = true
			}
		case logs.RetryRecord:
			if !v.Recovered && v.Reason == "missing_required_arguments" {
				foundRetry = true
			}
		}
	}
	if !foundValidation || !foundRetry {
		t.Fatalf("expected tool validation and retry records, got %#v", sessionHistory.Records)
	}
}

func TestLoopRunRejectsCognitivelyDisabledToolWithoutExecution(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"command": "echo should-not-run"})
	providerScript := &scriptedProvider{responses: repeatedToolResponses("call-disabled-cognitive", "run_command", map[string]string{"command": "echo should-not-run"}, raw, 8)}
	loop := Loop{Provider: providerScript, Tools: tools.NewRegistry(tools.RunCommandTool{RootDir: t.TempDir()}), Projections: []prompt.Projection{prompt.ContextProjection{}}, MaxHistory: 10}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: []string{"read_file", "run_command"}}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", EnabledTools: []string{"read_file"}}}}}
	sessionHistory := logs.NewSessionHistory("session-disabled-cognitive")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Try command."})

	err := loop.Run(agent, agentDef, nil, sessionHistory, nil)
	if err == nil || err.Error() != "loop guard tripped" {
		t.Fatalf("error = %v, want loop guard after repeated disabled tool", err)
	}
	foundValidation := false
	foundToolResult := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.ToolValidationRecord:
			if v.ToolName == "run_command" && !v.Valid && v.Reason == "tool_not_enabled" {
				foundValidation = true
			}
		case logs.ToolCallResultRecord:
			if v.ToolName == "run_command" {
				foundToolResult = true
			}
		}
	}
	if !foundValidation || foundToolResult {
		t.Fatalf("expected disabled validation and no tool result, got %#v", sessionHistory.Records)
	}
}

func TestLoopRunRejectsWorkflowDisabledToolWithoutExecution(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"command": "echo should-not-run"})
	providerScript := &scriptedProvider{responses: repeatedToolResponses("call-disabled-workflow", "run_command", map[string]string{"command": "echo should-not-run"}, raw, 8)}
	loop := Loop{Provider: providerScript, Tools: tools.NewRegistry(tools.RunCommandTool{RootDir: t.TempDir()}), Projections: []prompt.Projection{prompt.ContextProjection{}}, MaxHistory: 10}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: []string{"read_file", "run_command"}}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", EnabledTools: []string{"read_file", "run_command"}}}}}
	workflowDef := defs.WorkflowDefinition{Statechart: defs.StatechartDefinition{InitialState: "planning", States: []defs.StateDefinition{{Name: "planning", EnabledTools: []string{"read_file"}}}}}
	sessionHistory := logs.NewSessionHistory("session-disabled-workflow")
	sessionHistory.Append(logs.SessionWorkflowBindingRecord{SessionBaseRecord: sessionHistory.NextRecord("workflow_binding_ref"), BindingID: "bind-disabled", WorkflowID: "workflow-disabled", Action: "bind"})
	workflowHistory := logs.NewWorkflowHistory("workflow-disabled")

	err := loop.Run(agent, agentDef, &workflowDef, sessionHistory, workflowHistory)
	if err == nil || err.Error() != "loop guard tripped" {
		t.Fatalf("error = %v, want loop guard after repeated disabled workflow tool", err)
	}
	foundValidation := false
	foundToolResult := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.ToolValidationRecord:
			if v.ToolName == "run_command" && !v.Valid && v.Reason == "tool_not_enabled" {
				foundValidation = true
			}
		case logs.ToolCallResultRecord:
			if v.ToolName == "run_command" {
				foundToolResult = true
			}
		}
	}
	if !foundValidation || foundToolResult {
		t.Fatalf("expected workflow disabled validation and no tool result, got %#v", sessionHistory.Records)
	}
}

func TestLoopRunProcessesWorkflowTransitionWhenBound(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"chart": "workflow", "trigger": "start_implementation"})
	fakeProvider := &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{
		provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-004", ToolName: "transition_state", Arguments: map[string]string{"chart": "workflow", "trigger": "start_implementation"}, RawArgs: raw}},
		provider.AssistantOutput{Content: "Workflow advanced."},
	}}}
	toolRegistry := tools.NewRegistry(tools.TransitionTool{
		WorkflowChart: statecharts.Compile("workflow", defs.StatechartDefinition{InitialState: "planning", Transitions: []defs.TransitionDefinition{{Trigger: "start_implementation", From: "planning", To: "implementing"}}}),
	})
	loop := Loop{
		Provider: fakeProvider,
		Tools:    toolRegistry,
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 10,
	}
	enabledTools := []string{"transition_state"}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: enabledTools}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", EnabledTools: enabledTools}}}}
	workflowDef := defs.WorkflowDefinition{Description: "Conversation flow", Context: "Implement feature", Statechart: defs.StatechartDefinition{InitialState: "planning", States: []defs.StateDefinition{{Name: "planning"}, {Name: "implementing"}}}}
	sessionHistory := logs.NewSessionHistory("session-004")
	sessionHistory.Append(logs.SessionWorkflowBindingRecord{SessionBaseRecord: sessionHistory.NextRecord("workflow_binding_ref"), BindingID: "bind-004", WorkflowID: "workflow-004", Action: "bind"})
	workflowHistory := logs.NewWorkflowHistory("workflow-004")

	err := loop.Run(agent, agentDef, &workflowDef, sessionHistory, workflowHistory)
	if err == nil {
		foundSessionRef := false
		foundWorkflowTransition := false
		for _, record := range sessionHistory.Records {
			if ref, ok := record.(logs.WorkflowTransitionRefRecord); ok && ref.ToState == "implementing" {
				foundSessionRef = true
			}
		}
		for _, record := range workflowHistory.Records {
			if tr, ok := record.(logs.WorkflowTransitionRecord); ok && tr.ToState == "implementing" {
				foundWorkflowTransition = true
			}
		}
		if !foundSessionRef || !foundWorkflowTransition {
			t.Fatalf("expected linked workflow transition records, got session=%#v workflow=%#v", sessionHistory.Records, workflowHistory.Records)
		}
		return
	}
	if err.Error() != "loop guard tripped" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoopRunScriptedBindInterruptResumeFlow(t *testing.T) {
	bindRaw, _ := json.Marshal(map[string]string{"workflow_id": "workflow-010", "binding_id": "bind-010"})
	interruptRaw, _ := json.Marshal(map[string]string{"reason": "need clarification", "requested_by": "user"})
	resumeRaw, _ := json.Marshal(map[string]string{"reason": "user clarified"})
	providerScript := &scriptedProvider{responses: []provider.Response{
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-bind-010", ToolName: "bind_workflow", Arguments: map[string]string{"workflow_id": "workflow-010", "binding_id": "bind-010"}, RawArgs: bindRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-interrupt-010", ToolName: "interrupt_session", Arguments: map[string]string{"reason": "need clarification", "requested_by": "user"}, RawArgs: interruptRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-resume-010", ToolName: "resume_session", Arguments: map[string]string{"reason": "user clarified"}, RawArgs: resumeRaw}},
		}},
		{Outputs: []provider.Output{
			provider.AssistantOutput{Content: "Resumed successfully."},
		}},
	}}
	toolRegistry := tools.NewRegistry(tools.BindingTool{}, tools.InterruptTool{}, tools.ResumeTool{})
	loop := Loop{
		Provider: providerScript,
		Tools:    toolRegistry,
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.BindingProjection{},
			prompt.InteractionProjection{},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 10,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: []string{"bind_workflow", "interrupt_session", "resume_session"}}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", EnabledTools: []string{"bind_workflow", "interrupt_session", "resume_session"}}}}}
	workflowDef := defs.WorkflowDefinition{Description: "Conversation flow", Context: "Implement feature", Statechart: defs.StatechartDefinition{InitialState: "chatting"}}
	sessionHistory := logs.NewSessionHistory("session-010")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Start working, but I may interrupt."})
	workflowHistory := logs.NewWorkflowHistory("workflow-010")

	if err := loop.Run(agent, agentDef, &workflowDef, sessionHistory, workflowHistory); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundBind := false
	foundInterrupt := false
	foundResume := false
	foundAssistant := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.SessionWorkflowBindingRecord:
			if v.Action == "bind" && v.WorkflowID == "workflow-010" {
				foundBind = true
			}
		case logs.InterruptRecord:
			if v.Reason == "need clarification" {
				foundInterrupt = true
			}
		case logs.ResumeRecord:
			if v.Reason == "user clarified" {
				foundResume = true
			}
		case logs.AssistantMessageRecord:
			if v.Content == "Resumed successfully." {
				foundAssistant = true
			}
		}
	}
	if !foundBind || !foundInterrupt || !foundResume || !foundAssistant {
		t.Fatalf("expected bind/interrupt/resume/assistant records, got %#v", sessionHistory.Records)
	}
	if len(providerScript.requests) != 4 {
		t.Fatalf("got %d provider requests, want 4", len(providerScript.requests))
	}
}

func TestLoopRunReadEditValidateSummaryFlow(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "sample.txt")
	if err := os.WriteFile(filePath, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write sample file: %v", err)
	}
	readRaw, _ := json.Marshal(map[string]string{"path": "sample.txt"})
	replaceRaw, _ := json.Marshal(map[string]string{"path": "sample.txt", "old_text": "hello world", "new_text": "hello maelstrom"})
	commandRaw, _ := json.Marshal(map[string]string{"command": "cat sample.txt"})
	providerScript := &scriptedProvider{responses: []provider.Response{
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-read-020", ToolName: "read_file", Arguments: map[string]string{"path": "sample.txt"}, RawArgs: readRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-replace-020", ToolName: "replace_text", Arguments: map[string]string{"path": "sample.txt", "old_text": "hello world", "new_text": "hello maelstrom"}, RawArgs: replaceRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-command-020", ToolName: "run_command", Arguments: map[string]string{"command": "cat sample.txt"}, RawArgs: commandRaw}},
		}},
		{Outputs: []provider.Output{
			provider.AssistantOutput{Content: "The file was updated and validated."},
		}},
	}}
	toolRegistry := tools.NewRegistry(
		tools.ReadFileTool{RootDir: tempDir},
		tools.ReplaceTextTool{RootDir: tempDir},
		tools.RunCommandTool{RootDir: tempDir},
	)
	loop := Loop{
		Provider: providerScript,
		Tools:    toolRegistry,
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 10,
	}
	enabledTools := []string{"read_file", "replace_text", "run_command"}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: enabledTools}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", EnabledTools: enabledTools}}}}
	sessionHistory := logs.NewSessionHistory("session-020")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Update sample.txt and confirm the contents."})

	if err := loop.Run(agent, agentDef, nil, sessionHistory, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read updated sample file: %v", err)
	}
	if string(updated) != "hello maelstrom\n" {
		t.Fatalf("updated file = %q, want hello maelstrom\\n", string(updated))
	}
	foundSummary := false
	foundCommandResult := false
	for _, record := range sessionHistory.Records {
		switch v := record.(type) {
		case logs.AssistantMessageRecord:
			if v.Content == "The file was updated and validated." {
				foundSummary = true
			}
		case logs.ToolCallResultRecord:
			if v.ToolName == "run_command" && v.Content != "" {
				foundCommandResult = true
			}
		}
	}
	if !foundSummary || !foundCommandResult {
		t.Fatalf("expected summary and command result records, got %#v", sessionHistory.Records)
	}
	if len(providerScript.requests) != 4 {
		t.Fatalf("got %d provider requests, want 4", len(providerScript.requests))
	}
}

func TestLoopRunSearchSkeletonReadEditValidateFlow(t *testing.T) {
	tempDir := t.TempDir()
	goFile := filepath.Join(tempDir, "worker.go")
	goContent := `package sample

type Worker struct{}

func (w *Worker) Execute() string {
	return "old value"
}
`
	if err := os.WriteFile(goFile, []byte(goContent), 0o644); err != nil {
		t.Fatalf("write worker.go: %v", err)
	}
	searchRaw, _ := json.Marshal(map[string]string{"pattern": "Execute", "include_glob": "*.go"})
	skeletonRaw, _ := json.Marshal(map[string]string{"path": "worker.go"})
	readRaw, _ := json.Marshal(map[string]string{"path": "worker.go", "start_line": "3", "end_line": "6"})
	replaceRaw, _ := json.Marshal(map[string]string{"path": "worker.go", "old_text": "old value", "new_text": "new value"})
	commandRaw, _ := json.Marshal(map[string]string{"command": "cat worker.go"})
	providerScript := &scriptedProvider{responses: []provider.Response{
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-search-030", ToolName: "search_files", Arguments: map[string]string{"pattern": "Execute", "include_glob": "*.go"}, RawArgs: searchRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-skeleton-030", ToolName: "get_file_skeleton", Arguments: map[string]string{"path": "worker.go"}, RawArgs: skeletonRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-read-030", ToolName: "read_file", Arguments: map[string]string{"path": "worker.go", "start_line": "3", "end_line": "6"}, RawArgs: readRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-replace-030", ToolName: "replace_text", Arguments: map[string]string{"path": "worker.go", "old_text": "old value", "new_text": "new value"}, RawArgs: replaceRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-command-030", ToolName: "run_command", Arguments: map[string]string{"command": "cat worker.go"}, RawArgs: commandRaw}},
		}},
		{Outputs: []provider.Output{
			provider.AssistantOutput{Content: "Updated Execute() to return the new value."},
		}},
	}}
	toolRegistry := tools.NewRegistry(
		tools.SearchFilesTool{RootDir: tempDir, LookupPath: func(name string) (string, error) { return "", os.ErrNotExist }},
		tools.GetFileSkeletonTool{RootDir: tempDir},
		tools.ReadFileTool{RootDir: tempDir},
		tools.ReplaceTextTool{RootDir: tempDir},
		tools.RunCommandTool{RootDir: tempDir},
	)
	loop := Loop{
		Provider: providerScript,
		Tools:    toolRegistry,
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 20,
	}
	enabledTools := []string{"search_files", "get_file_skeleton", "read_file", "replace_text", "run_command"}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model", ToolNames: enabledTools}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe", States: []defs.StateDefinition{{Name: "observe", EnabledTools: enabledTools}}}}
	sessionHistory := logs.NewSessionHistory("session-030")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Find Execute and update its return value."})

	if err := loop.Run(agent, agentDef, nil, sessionHistory, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, err := os.ReadFile(goFile)
	if err != nil {
		t.Fatalf("read updated go file: %v", err)
	}
	if !strings.Contains(string(updated), `return "new value"`) {
		t.Fatalf("updated file = %q, want new value", string(updated))
	}
	foundSearch := false
	foundSkeleton := false
	foundSummary := false
	for _, record := range sessionHistory.Records {
		if result, ok := record.(logs.ToolCallResultRecord); ok {
			switch result.ToolName {
			case "search_files":
				if strings.Contains(result.Content, "worker.go") {
					foundSearch = true
				}
			case "get_file_skeleton":
				if strings.Contains(result.Content, "func (*Worker) Execute(...)") {
					foundSkeleton = true
				}
			}
		}
		if msg, ok := record.(logs.AssistantMessageRecord); ok && msg.Content == "Updated Execute() to return the new value." {
			foundSummary = true
		}
	}
	if !foundSearch || !foundSkeleton || !foundSummary {
		t.Fatalf("expected search, skeleton, and summary records, got %#v", sessionHistory.Records)
	}
	if len(providerScript.requests) != 6 {
		t.Fatalf("got %d provider requests, want 6", len(providerScript.requests))
	}
}

func TestLoopRunSelfRepoStyleMaintenanceFlow(t *testing.T) {
	tempDir := t.TempDir()
	goFile := filepath.Join(tempDir, "tools.go")
	goContent := `package sample

type ReplaceTextTool struct{}

func (ReplaceTextTool) Definition() string {
	return "Replace exact text in a file"
}

func helper() string {
	return "unchanged"
}
`
	if err := os.WriteFile(goFile, []byte(goContent), 0o644); err != nil {
		t.Fatalf("write tools.go: %v", err)
	}
	searchRaw, _ := json.Marshal(map[string]string{"pattern": "ReplaceTextTool", "include_glob": "*.go"})
	refsRaw, _ := json.Marshal(map[string]string{"symbol": "ReplaceTextTool", "include_glob": "*.go"})
	symbolRaw, _ := json.Marshal(map[string]string{"path": "tools.go", "symbol": "Definition"})
	replaceRaw, _ := json.Marshal(map[string]string{"path": "tools.go", "old_text": "Replace exact text in a file", "new_text": "Replace exact text in a file when matched once"})
	commandRaw, _ := json.Marshal(map[string]string{"command": "cat tools.go"})
	providerScript := &scriptedProvider{responses: []provider.Response{
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-search-040", ToolName: "search_files", Arguments: map[string]string{"pattern": "ReplaceTextTool", "include_glob": "*.go"}, RawArgs: searchRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-refs-040", ToolName: "find_references", Arguments: map[string]string{"symbol": "ReplaceTextTool", "include_glob": "*.go"}, RawArgs: refsRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-symbol-040", ToolName: "read_symbol", Arguments: map[string]string{"path": "tools.go", "symbol": "Definition"}, RawArgs: symbolRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-replace-040", ToolName: "replace_text", Arguments: map[string]string{"path": "tools.go", "old_text": "Replace exact text in a file", "new_text": "Replace exact text in a file when matched once"}, RawArgs: replaceRaw}},
		}},
		{Outputs: []provider.Output{
			provider.ToolRequestOutput{Call: provider.ToolCall{CallID: "call-command-040", ToolName: "run_command", Arguments: map[string]string{"command": "cat tools.go"}, RawArgs: commandRaw}},
		}},
		{Outputs: []provider.Output{
			provider.AssistantOutput{Content: "Updated ReplaceTextTool definition text and verified the file contents."},
		}},
	}}
	toolRegistry := tools.NewRegistry(
		tools.SearchFilesTool{RootDir: tempDir, LookupPath: func(name string) (string, error) { return "", os.ErrNotExist }},
		tools.FindReferencesTool{RootDir: tempDir, LookupPath: func(name string) (string, error) { return "", os.ErrNotExist }},
		tools.ReadSymbolTool{RootDir: tempDir},
		tools.ReplaceTextTool{RootDir: tempDir},
		tools.RunCommandTool{RootDir: tempDir},
	)
	loop := Loop{
		Provider: providerScript,
		Tools:    toolRegistry,
		Projections: []prompt.Projection{
			prompt.StaticProjection{ProjectionName: "system", Role: "system", Prompt: "You are a coding agent."},
			prompt.RecentHistoryProjection{},
		},
		MaxHistory: 20,
	}
	agent := runtime.Agent{Name: "builder", ProviderName: "fake", ProviderRef: "fake-model"}
	agentDef := defs.AgentDefinition{Cognitive: defs.StatechartDefinition{InitialState: "observe"}}
	sessionHistory := logs.NewSessionHistory("session-040")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Tighten the ReplaceTextTool definition text and verify it."})

	if err := loop.Run(agent, agentDef, nil, sessionHistory, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, err := os.ReadFile(goFile)
	if err != nil {
		t.Fatalf("read updated tools.go: %v", err)
	}
	if !strings.Contains(string(updated), "Replace exact text in a file when matched once") {
		t.Fatalf("updated file = %q, want tightened definition text", string(updated))
	}
	foundRefs := false
	foundSymbol := false
	foundSummary := false
	for _, record := range sessionHistory.Records {
		if result, ok := record.(logs.ToolCallResultRecord); ok {
			switch result.ToolName {
			case "find_references":
				if strings.Contains(result.Content, "ReplaceTextTool") {
					foundRefs = true
				}
			case "read_symbol":
				if strings.Contains(result.Content, "Definition") {
					foundSymbol = true
				}
			}
		}
		if msg, ok := record.(logs.AssistantMessageRecord); ok && msg.Content == "Updated ReplaceTextTool definition text and verified the file contents." {
			foundSummary = true
		}
	}
	if !foundRefs || !foundSymbol || !foundSummary {
		t.Fatalf("expected references, symbol, and summary records, got %#v", sessionHistory.Records)
	}
	if len(providerScript.requests) != 6 {
		t.Fatalf("got %d provider requests, want 6", len(providerScript.requests))
	}
}
