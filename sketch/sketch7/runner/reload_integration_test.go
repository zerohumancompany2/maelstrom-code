package runner

import (
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/catalog"
	"github.com/comalice/inference_sketch/sketch/sketch7/compile"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/tools"
)

func TestReloadedAgentDefinitionAffectsSubsequentRun(t *testing.T) {
	memory := catalog.NewMemory()
	modelRaw := []byte("apiVersion: maelstrom/v1\nkind: Model\nname: test-model\nproviders:\n  - name: fake\n    modelRef: fake/model\nlimits:\n  contextWindow: 32768\n  maxOutputTokens: 1024\ndefaults:\n  temperature: 0.7\n  topP: 1.0\ncapabilities:\n  tools: true\n  reasoning: true\n  multimodal: false\n  streaming: false\n")
	agentV1 := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: test-agent\ndescription: Test agent\nmodel: test-model\ncontext:\n  inputBudget: 2048\n  projections:\n    - type: system\n      name: system\n      prompt: System prompt v1\n    - type: cognitive_state\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      prompt: Observe v1.\n  transitions: []\n")
	agentV2 := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: test-agent\ndescription: Test agent\nmodel: test-model\ncontext:\n  inputBudget: 2048\n  projections:\n    - type: system\n      name: system\n      prompt: System prompt v2\n    - type: cognitive_state\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      prompt: Observe v2.\n  transitions: []\n")

	if err := catalog.LoadIntoMemory(memory, modelRaw); err != nil {
		t.Fatalf("load model: %v", err)
	}
	if err := catalog.LoadIntoMemory(memory, agentV1); err != nil {
		t.Fatalf("load agent v1: %v", err)
	}
	modelDef, _ := memory.GetModel("test-model")
	agentDefV1, _ := memory.GetAgent("test-agent")
	toolRegistry := tools.NewRegistry()
	hydratedV1, err := compile.HydrateAgent(agentDefV1, modelDef, toolRegistry)
	if err != nil {
		t.Fatalf("hydrate v1: %v", err)
	}
	projectionsV1, _, err := prompt.BuildProjectionPlan(agentDefV1)
	if err != nil {
		t.Fatalf("projection plan v1: %v", err)
	}
	assembledV1, err := prompt.Assembler{Projections: projectionsV1}.Assemble(prompt.Input{Session: BuildSessionView(hydratedV1, agentDefV1, nil, logs.NewSessionHistory("session-reload-1"), nil), History: logs.NewSessionHistory("session-reload-1")})
	if err != nil {
		t.Fatalf("assemble v1: %v", err)
	}
	if assembledV1.Segments[0].TokenText() != "System prompt v1" {
		t.Fatalf("first prompt = %q, want System prompt v1", assembledV1.Segments[0].TokenText())
	}

	if err := memory.Reload(agentV2); err != nil {
		t.Fatalf("reload agent v2: %v", err)
	}
	agentDefV2, _ := memory.GetAgent("test-agent")
	hydratedV2, err := compile.HydrateAgent(agentDefV2, modelDef, toolRegistry)
	if err != nil {
		t.Fatalf("hydrate v2: %v", err)
	}
	projectionsV2, _, err := prompt.BuildProjectionPlan(agentDefV2)
	if err != nil {
		t.Fatalf("projection plan v2: %v", err)
	}
	assembledV2, err := prompt.Assembler{Projections: projectionsV2}.Assemble(prompt.Input{Session: BuildSessionView(hydratedV2, agentDefV2, nil, logs.NewSessionHistory("session-reload-2"), nil), History: logs.NewSessionHistory("session-reload-2")})
	if err != nil {
		t.Fatalf("assemble v2: %v", err)
	}
	if assembledV2.Segments[0].TokenText() != "System prompt v2" {
		t.Fatalf("first prompt = %q, want System prompt v2", assembledV2.Segments[0].TokenText())
	}
	if assembledV2.Segments[1].TokenText() == assembledV1.Segments[1].TokenText() {
		t.Fatalf("cognitive projection text did not change across reload: v1=%q v2=%q", assembledV1.Segments[1].TokenText(), assembledV2.Segments[1].TokenText())
	}
}

func TestReloadedWorkflowDefinitionAffectsSubsequentRun(t *testing.T) {
	memory := catalog.NewMemory()
	modelRaw := []byte("apiVersion: maelstrom/v1\nkind: Model\nname: test-model\nproviders:\n  - name: fake\n    modelRef: fake/model\nlimits:\n  contextWindow: 32768\n  maxOutputTokens: 1024\ndefaults:\n  temperature: 0.7\n  topP: 1.0\ncapabilities:\n  tools: true\n  reasoning: true\n  multimodal: false\n  streaming: false\n")
	agentRaw := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: test-agent\nmodel: test-model\ncontext:\n  inputBudget: 2048\n  projections:\n    - type: workflow_state\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n  transitions: []\n")
	workflowV1 := []byte("apiVersion: maelstrom/v1\nkind: Workflow\nname: test-workflow\ndescription: Workflow v1\ncontext: Context v1\nstatechart:\n  initialState: chatting\n  states:\n    - name: chatting\n  transitions: []\n")
	workflowV2 := []byte("apiVersion: maelstrom/v1\nkind: Workflow\nname: test-workflow\ndescription: Workflow v2\ncontext: Context v2\nstatechart:\n  initialState: chatting\n  states:\n    - name: chatting\n  transitions: []\n")

	_ = catalog.LoadIntoMemory(memory, modelRaw)
	_ = catalog.LoadIntoMemory(memory, agentRaw)
	_ = catalog.LoadIntoMemory(memory, workflowV1)
	modelDef, _ := memory.GetModel("test-model")
	agentDef, _ := memory.GetAgent("test-agent")
	workflowDefV1, _ := memory.GetWorkflow("test-workflow")
	hydrated, err := compile.HydrateAgent(agentDef, modelDef, tools.NewRegistry())
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	projections, _, err := prompt.BuildProjectionPlan(agentDef)
	if err != nil {
		t.Fatalf("projection plan: %v", err)
	}
	sessionHistory := logs.NewSessionHistory("session-reload-wf")
	sessionHistory.Append(logs.SessionWorkflowBindingRecord{SessionBaseRecord: sessionHistory.NextRecord("workflow_binding_ref"), BindingID: "bind-wf", WorkflowID: "workflow-instance", Action: "bind"})
	workflowHistory := logs.NewWorkflowHistory("workflow-instance")
	assembledV1, err := prompt.Assembler{Projections: projections}.Assemble(prompt.Input{Session: BuildSessionView(hydrated, agentDef, &workflowDefV1, sessionHistory, workflowHistory), History: sessionHistory, Workflow: workflowHistory})
	if err != nil {
		t.Fatalf("assemble v1: %v", err)
	}

	if err := memory.Reload(workflowV2); err != nil {
		t.Fatalf("reload workflow v2: %v", err)
	}
	workflowDefV2, _ := memory.GetWorkflow("test-workflow")
	assembledV2, err := prompt.Assembler{Projections: projections}.Assemble(prompt.Input{Session: BuildSessionView(hydrated, agentDef, &workflowDefV2, sessionHistory, workflowHistory), History: sessionHistory, Workflow: workflowHistory})
	if err != nil {
		t.Fatalf("assemble v2: %v", err)
	}
	if assembledV1.Segments[0].TokenText() == assembledV2.Segments[0].TokenText() {
		t.Fatalf("workflow projection text did not change across reload: v1=%q v2=%q", assembledV1.Segments[0].TokenText(), assembledV2.Segments[0].TokenText())
	}
}
