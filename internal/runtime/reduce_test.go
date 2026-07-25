package runtime

import (
	"strings"
	"testing"

	"github.com/comalice/maelstrom/internal/defs"
	"github.com/comalice/maelstrom/internal/logs"
)

func TestReduceCognitiveStateUsesInitialStateWhenHistoryEmpty(t *testing.T) {
	history := logs.NewSessionHistory("session-001")
	chart := defs.StatechartDefinition{
		InitialState: "observe",
		States: []defs.StateDefinition{{
			Name:         "observe",
			VisibleTools: []string{"transition_state"},
			EnabledTools: []string{"transition_state"},
			Prompt:       "Observe first.",
		}},
	}

	view := ReduceCognitiveState(history, chart.InitialState, chart)
	if view.CurrentState != "observe" {
		t.Fatalf("CurrentState = %q, want observe", view.CurrentState)
	}
	if view.Prompt != "Observe first." {
		t.Fatalf("Prompt = %q, want %q", view.Prompt, "Observe first.")
	}
}

func TestReduceCognitiveStateCarriesBounds(t *testing.T) {
	history := logs.NewSessionHistory("session-bounds-cognitive")
	chart := defs.StatechartDefinition{
		InitialState: "observe",
		States: []defs.StateDefinition{{
			Name: "observe",
			Bounds: defs.StateBoundsContract{
				MaxInferenceTurns:      3,
				MaxToolCalls:           6,
				MaxWallTimeSeconds:     120,
				MaxFinalizationRetries: 1,
			},
		}},
	}

	view := ReduceCognitiveState(history, chart.InitialState, chart)
	if view.Bounds.MaxInferenceTurns != 3 || view.Bounds.MaxToolCalls != 6 || view.Bounds.MaxWallTimeSeconds != 120 || view.Bounds.MaxFinalizationRetries != 1 {
		t.Fatalf("Bounds = %+v, want configured bounds", view.Bounds)
	}
}

func TestReduceCognitiveStateUsesLatestTransition(t *testing.T) {
	history := logs.NewSessionHistory("session-002")
	history.Append(logs.CognitiveTransitionRecord{
		SessionBaseRecord: history.NextRecord("cognitive_transition"),
		FromState:         "observe",
		ToState:           "act",
		Trigger:           "begin_action",
	})
	chart := defs.StatechartDefinition{
		InitialState: "observe",
		States: []defs.StateDefinition{
			{Name: "observe", Prompt: "Observe first."},
			{Name: "act", Prompt: "Act now.", VisibleTools: []string{"edit_file"}, EnabledTools: []string{"edit_file"}},
		},
	}

	view := ReduceCognitiveState(history, chart.InitialState, chart)
	if view.CurrentState != "act" {
		t.Fatalf("CurrentState = %q, want act", view.CurrentState)
	}
	if view.Prompt != "Act now." {
		t.Fatalf("Prompt = %q, want %q", view.Prompt, "Act now.")
	}
	if len(view.EnabledTools) != 1 || view.EnabledTools[0] != "edit_file" {
		t.Fatalf("EnabledTools = %v, want [edit_file]", view.EnabledTools)
	}
}

func TestReduceWorkflowStateUsesInitialStateWhenHistoryEmpty(t *testing.T) {
	history := logs.NewWorkflowHistory("workflow-001")
	def := defs.WorkflowDefinition{
		Name:        "conversation_to_execution",
		Description: "Conversation to execution",
		Context:     "Implement the requested change.",
		Statechart: defs.StatechartDefinition{
			InitialState: "chatting",
			States: []defs.StateDefinition{{
				Name:         "chatting",
				VisibleTools: []string{"bind_workflow"},
				EnabledTools: []string{"bind_workflow"},
			}},
		},
	}

	view := ReduceWorkflowState(history, def, "weather-agent")
	if view.CurrentState != "chatting" {
		t.Fatalf("CurrentState = %q, want chatting", view.CurrentState)
	}
	if view.LastBoundAgent != "weather-agent" {
		t.Fatalf("LastBoundAgent = %q, want weather-agent", view.LastBoundAgent)
	}
	if len(view.EnabledTools) != 1 || view.EnabledTools[0] != "bind_workflow" {
		t.Fatalf("EnabledTools = %v, want [bind_workflow]", view.EnabledTools)
	}
}

func TestReduceWorkflowStateCarriesBounds(t *testing.T) {
	history := logs.NewWorkflowHistory("workflow-bounds")
	def := defs.WorkflowDefinition{Statechart: defs.StatechartDefinition{
		InitialState: "validating",
		States: []defs.StateDefinition{{
			Name: "validating",
			Bounds: defs.StateBoundsContract{
				MaxInferenceTurns:      2,
				MaxToolCalls:           4,
				MaxWallTimeSeconds:     90,
				MaxFinalizationRetries: 2,
			},
		}},
	}}

	view := ReduceWorkflowState(history, def, "builder")
	if view.Bounds.MaxInferenceTurns != 2 || view.Bounds.MaxToolCalls != 4 || view.Bounds.MaxWallTimeSeconds != 90 || view.Bounds.MaxFinalizationRetries != 2 {
		t.Fatalf("Bounds = %+v, want configured workflow bounds", view.Bounds)
	}
}

func TestReduceWorkflowStateUsesLatestTransitionAndBindingRef(t *testing.T) {
	history := logs.NewWorkflowHistory("workflow-002")
	history.Append(logs.WorkflowBindingRefRecord{
		WorkflowBaseRecord: history.NextRecord("workflow_binding_ref"),
		BindingID:          "bind-001",
		AgentID:            "builder-agent",
		Action:             "bind",
	})
	history.Append(logs.WorkflowTransitionRecord{
		WorkflowBaseRecord: history.NextRecord("workflow_transition"),
		FromState:          "planning",
		ToState:            "implementing",
		Trigger:            "start_implementation",
	})
	def := defs.WorkflowDefinition{
		Statechart: defs.StatechartDefinition{
			InitialState: "chatting",
			States: []defs.StateDefinition{
				{Name: "chatting"},
				{Name: "implementing", VisibleTools: []string{"edit_file", "run_tests"}, EnabledTools: []string{"edit_file", "run_tests"}},
			},
		},
	}

	view := ReduceWorkflowState(history, def, "")
	if view.CurrentState != "implementing" {
		t.Fatalf("CurrentState = %q, want implementing", view.CurrentState)
	}
	if view.LastBoundAgent != "builder-agent" {
		t.Fatalf("LastBoundAgent = %q, want builder-agent", view.LastBoundAgent)
	}
	if len(view.VisibleTools) != 2 {
		t.Fatalf("VisibleTools = %v, want 2 tools", view.VisibleTools)
	}
}

func TestReduceWorkflowStateArtifactsKeepsLatestPerStatePreservingFirstOrder(t *testing.T) {
	history := logs.NewWorkflowHistory("workflow-artifacts")
	history.Append(logs.WorkflowArtifactRecord{
		WorkflowBaseRecord: history.NextRecord("workflow_artifact"),
		StateName:          "intake",
		SchemaName:         "intake_v1",
		ByAgent:            "builder",
		Content:            `{"summary":"old"}`,
	})
	history.Append(logs.WorkflowArtifactRecord{
		WorkflowBaseRecord: history.NextRecord("workflow_artifact"),
		StateName:          "inspecting",
		SchemaName:         "inspect_v1",
		ByAgent:            "builder",
		Content:            `{"findings":"none"}`,
	})
	history.Append(logs.WorkflowArtifactRecord{
		WorkflowBaseRecord: history.NextRecord("workflow_artifact"),
		StateName:          "intake",
		SchemaName:         "intake_v1",
		ByAgent:            "reviewer",
		Content:            `{"summary":"new"}`,
	})
	def := defs.WorkflowDefinition{Statechart: defs.StatechartDefinition{InitialState: "intake"}}

	view := ReduceWorkflowState(history, def, "builder")
	if len(view.Artifacts) != 2 {
		t.Fatalf("Artifacts = %+v, want 2 entries", view.Artifacts)
	}
	if view.Artifacts[0].StateName != "intake" || view.Artifacts[0].ByAgent != "reviewer" || !strings.Contains(view.Artifacts[0].Content, `"summary":"new"`) {
		t.Fatalf("Artifacts[0] = %+v, want intake with newer content by reviewer", view.Artifacts[0])
	}
	if view.Artifacts[1].StateName != "inspecting" || !strings.Contains(view.Artifacts[1].Content, `"findings":"none"`) {
		t.Fatalf("Artifacts[1] = %+v, want inspecting", view.Artifacts[1])
	}
}

func TestReduceBindingStateUsesLatestSessionBinding(t *testing.T) {
	history := logs.NewSessionHistory("session-003")
	history.Append(logs.SessionWorkflowBindingRecord{
		SessionBaseRecord: history.NextRecord("workflow_binding_ref"),
		BindingID:         "bind-001",
		WorkflowID:        "workflow-003",
		Action:            "bind",
	})
	history.Append(logs.SessionWorkflowBindingRecord{
		SessionBaseRecord: history.NextRecord("workflow_binding_ref"),
		BindingID:         "bind-002",
		WorkflowID:        "workflow-003",
		Action:            "unbind",
	})

	view := ReduceBindingState(history)
	if view.Bound {
		t.Fatalf("Bound = true, want false")
	}
	if view.WorkflowID != "workflow-003" {
		t.Fatalf("WorkflowID = %q, want workflow-003", view.WorkflowID)
	}
}

func TestReduceInteractionModeDefaultsToInteractive(t *testing.T) {
	history := logs.NewSessionHistory("session-004")
	view := ReduceInteractionMode(history)
	if view.Mode != InteractionInteractive {
		t.Fatalf("Mode = %q, want %q", view.Mode, InteractionInteractive)
	}
}

func TestReduceInteractionModeRunningWhenLatestRecordIsToolActivity(t *testing.T) {
	history := logs.NewSessionHistory("session-005")
	history.Append(logs.ToolCallRequestRecord{
		SessionBaseRecord: history.NextRecord("tool_call_request"),
		CallID:            "call-001",
		ToolName:          "run_tests",
		Arguments:         `{}`,
	})

	view := ReduceInteractionMode(history)
	if view.Mode != InteractionRunning {
		t.Fatalf("Mode = %q, want %q", view.Mode, InteractionRunning)
	}
}

func TestReduceInteractionModeInterruptedWhenLatestRecordIsInterrupt(t *testing.T) {
	history := logs.NewSessionHistory("session-006")
	history.Append(logs.InterruptRecord{
		SessionBaseRecord: history.NextRecord("interrupt"),
		Reason:            "user pressed escape",
		RequestedBy:       "user",
	})

	view := ReduceInteractionMode(history)
	if view.Mode != InteractionInterrupted {
		t.Fatalf("Mode = %q, want %q", view.Mode, InteractionInterrupted)
	}
	if view.Reason != "user pressed escape" {
		t.Fatalf("Reason = %q, want %q", view.Reason, "user pressed escape")
	}
}

func TestReduceInteractionModeResumableWhenLatestRecordIsResume(t *testing.T) {
	history := logs.NewSessionHistory("session-007")
	history.Append(logs.InterruptRecord{
		SessionBaseRecord: history.NextRecord("interrupt"),
		Reason:            "user wants to redirect",
		RequestedBy:       "user",
	})
	history.Append(logs.ResumeRecord{
		SessionBaseRecord: history.NextRecord("resume"),
		Reason:            "user finished guidance",
	})

	view := ReduceInteractionMode(history)
	if view.Mode != InteractionResumable {
		t.Fatalf("Mode = %q, want %q", view.Mode, InteractionResumable)
	}
	if view.Reason != "user finished guidance" {
		t.Fatalf("Reason = %q, want %q", view.Reason, "user finished guidance")
	}
}

func finalizationTestCognitiveView(maxTurns, maxToolCalls int) CognitiveView {
	return CognitiveView{
		CurrentState: "working",
		Outputs:      defs.StateOutputContract{SchemaName: "cognitive_step_v1", RequiredFields: []string{"summary"}},
		Bounds:       defs.StateBoundsContract{MaxInferenceTurns: maxTurns, MaxToolCalls: maxToolCalls},
	}
}

func finalizationTestWorkflowView(maxTurns, maxToolCalls int) WorkflowView {
	return WorkflowView{
		WorkflowID:   "workflow-fin",
		CurrentState: "triaging",
		Outputs:      defs.StateOutputContract{SchemaName: "triage_v1", RequiredFields: []string{"decision"}},
		Bounds:       defs.StateBoundsContract{MaxInferenceTurns: maxTurns, MaxToolCalls: maxToolCalls},
	}
}

func appendStateEnter(history *logs.SessionHistory, chart, state string) {
	history.Append(logs.StateEnterRecord{SessionBaseRecord: history.NextRecord("state_enter"), Chart: chart, StateName: state})
}

func appendInferenceEnvelopes(history *logs.SessionHistory, count int) {
	for i := 0; i < count; i++ {
		history.Append(logs.InferenceEnvelopeRecord{SessionBaseRecord: history.NextRecord("inference_envelope"), PayloadID: "p", ModelRef: "m", ProviderRef: "prov"})
	}
}

func appendToolCallRequests(history *logs.SessionHistory, count int) {
	for i := 0; i < count; i++ {
		history.Append(logs.ToolCallRequestRecord{SessionBaseRecord: history.NextRecord("tool_call_request"), CallID: "c", ToolName: "read_file"})
	}
}

func TestResolveFinalizationModeCognitiveOnlyWithReason(t *testing.T) {
	history := logs.NewSessionHistory("session-fin-cog")
	appendStateEnter(history, "cognitive", "working")
	appendToolCallRequests(history, 3)

	cognitive := finalizationTestCognitiveView(0, 3)
	mode := ResolveFinalizationMode(cognitive, nil, history)
	if !mode.IsFinalizing || !mode.RequireCognitive || mode.RequireWorkflow {
		t.Fatalf("mode = %+v, want cognitive-only finalization", mode)
	}
	if mode.Reason != "cognitive_max_tool_calls" {
		t.Fatalf("Reason = %q, want cognitive_max_tool_calls", mode.Reason)
	}
	if mode.RetryAttempt != 1 {
		t.Fatalf("RetryAttempt = %d, want 1", mode.RetryAttempt)
	}
}

func TestResolveFinalizationModeWorkflowOnlyBoundDriven(t *testing.T) {
	history := logs.NewSessionHistory("session-fin-wf")
	appendStateEnter(history, "workflow", "triaging")
	appendStateEnter(history, "cognitive", "working")
	appendInferenceEnvelopes(history, 2)

	cognitive := finalizationTestCognitiveView(4, 0)
	workflow := finalizationTestWorkflowView(2, 0)
	mode := ResolveFinalizationMode(cognitive, &workflow, history)
	if !mode.IsFinalizing || mode.RequireCognitive || !mode.RequireWorkflow {
		t.Fatalf("mode = %+v, want workflow-only finalization", mode)
	}
	if mode.Reason != "workflow_max_inference_turns" {
		t.Fatalf("Reason = %q, want workflow_max_inference_turns", mode.Reason)
	}
}

func TestResolveFinalizationModeCombinedJoinsReasons(t *testing.T) {
	history := logs.NewSessionHistory("session-fin-combined")
	appendStateEnter(history, "workflow", "triaging")
	appendStateEnter(history, "cognitive", "working")
	appendInferenceEnvelopes(history, 2)

	cognitive := finalizationTestCognitiveView(2, 0)
	workflow := finalizationTestWorkflowView(2, 0)
	mode := ResolveFinalizationMode(cognitive, &workflow, history)
	if !mode.IsFinalizing || !mode.RequireCognitive || !mode.RequireWorkflow {
		t.Fatalf("mode = %+v, want combined finalization", mode)
	}
	if mode.Reason != "cognitive_max_inference_turns+workflow_max_inference_turns" {
		t.Fatalf("Reason = %q, want joined reasons", mode.Reason)
	}
}

func TestResolveFinalizationModeWorkflowContractAloneDoesNotFinalize(t *testing.T) {
	history := logs.NewSessionHistory("session-fin-nobound")
	appendStateEnter(history, "workflow", "triaging")
	appendStateEnter(history, "cognitive", "working")
	appendInferenceEnvelopes(history, 1)

	cognitive := finalizationTestCognitiveView(0, 0)
	// Outputs declared but no bounds: presence of a contract must not force
	// finalization on its own.
	workflow := finalizationTestWorkflowView(0, 0)
	mode := ResolveFinalizationMode(cognitive, &workflow, history)
	if mode.IsFinalizing || mode.RequireWorkflow {
		t.Fatalf("mode = %+v, want no finalization from contract presence alone", mode)
	}
}

func TestWorkflowBoundHitReasonRequiresOutputContract(t *testing.T) {
	history := logs.NewSessionHistory("session-fin-nocontract")
	appendStateEnter(history, "workflow", "triaging")
	appendInferenceEnvelopes(history, 5)

	workflow := WorkflowView{CurrentState: "triaging", Bounds: defs.StateBoundsContract{MaxInferenceTurns: 2}}
	if reason := WorkflowBoundHitReason(workflow, history); reason != "" {
		t.Fatalf("reason = %q, want empty without output contract", reason)
	}
}

func TestMaxFinalizationRetriesForModeTakesLargestParticipatingBudget(t *testing.T) {
	cognitive := finalizationTestCognitiveView(2, 0)
	cognitive.Bounds.MaxFinalizationRetries = 1
	workflow := finalizationTestWorkflowView(2, 0)
	workflow.Bounds.MaxFinalizationRetries = 3

	combined := FinalizationMode{IsFinalizing: true, RequireCognitive: true, RequireWorkflow: true}
	if got := MaxFinalizationRetriesForMode(combined, cognitive, &workflow); got != 3 {
		t.Fatalf("combined budget = %d, want 3", got)
	}
	cognitiveOnly := FinalizationMode{IsFinalizing: true, RequireCognitive: true}
	if got := MaxFinalizationRetriesForMode(cognitiveOnly, cognitive, &workflow); got != 1 {
		t.Fatalf("cognitive-only budget = %d, want 1", got)
	}
	workflowOnly := FinalizationMode{IsFinalizing: true, RequireWorkflow: true}
	if got := MaxFinalizationRetriesForMode(workflowOnly, cognitive, &workflow); got != 3 {
		t.Fatalf("workflow-only budget = %d, want 3", got)
	}
}

func TestReduceWorkflowStateCarriesAllowedTriggersFromStateAndTransitions(t *testing.T) {
	history := logs.NewWorkflowHistory("workflow-triggers")
	def := defs.WorkflowDefinition{Statechart: defs.StatechartDefinition{
		InitialState: "triaging",
		States:       []defs.StateDefinition{{Name: "triaging", AllowedTriggers: []string{"manual_escalate"}}, {Name: "done"}},
		Transitions:  []defs.TransitionDefinition{{From: "triaging", To: "done", Trigger: "finish"}, {From: "other", To: "done", Trigger: "ignore"}},
	}}

	view := ReduceWorkflowState(history, def, "builder")
	if len(view.AllowedTriggers) != 2 || view.AllowedTriggers[0] != "manual_escalate" || view.AllowedTriggers[1] != "finish" {
		t.Fatalf("AllowedTriggers = %v, want [manual_escalate finish]", view.AllowedTriggers)
	}
}

func TestFinalizationRetryCountForModeUsesMaxParticipatingChartWindow(t *testing.T) {
	history := logs.NewSessionHistory("session-retry-count")
	appendStateEnter(history, "workflow", "triaging")
	history.Append(logs.RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "invalid_finalization_output", Attempt: 1})
	history.Append(logs.RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "invalid_finalization_output", Attempt: 2})
	appendStateEnter(history, "cognitive", "observe")
	history.Append(logs.RetryRecord{SessionBaseRecord: history.NextRecord("retry"), Reason: "invalid_finalization_output", Attempt: 1})

	combined := FinalizationMode{IsFinalizing: true, RequireCognitive: true, RequireWorkflow: true}
	if got := FinalizationRetryCountForMode(combined, history); got != 3 {
		t.Fatalf("combined retry count = %d, want max participating count 3", got)
	}
	cognitiveOnly := FinalizationMode{IsFinalizing: true, RequireCognitive: true}
	if got := FinalizationRetryCountForMode(cognitiveOnly, history); got != 1 {
		t.Fatalf("cognitive retry count = %d, want 1", got)
	}
}
