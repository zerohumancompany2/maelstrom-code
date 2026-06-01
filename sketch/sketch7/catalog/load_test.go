package catalog

import "testing"

func TestLoadModelParsesDefinition(t *testing.T) {
	raw := []byte("apiVersion: maelstrom/v1\nkind: Model\nname: glm-4.5-air\nproviders:\n  - name: openrouter\n    modelRef: z-ai/glm-4.5-air:free\nlimits:\n  contextWindow: 32768\n  maxOutputTokens: 8192\ndefaults:\n  temperature: 0.7\n  topP: 1.0\ncapabilities:\n  tools: true\n  reasoning: true\n  multimodal: false\n  streaming: true\n")
	def, err := LoadModel(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name != "glm-4.5-air" {
		t.Fatalf("Name = %q, want glm-4.5-air", def.Name)
	}
	if len(def.Providers) != 1 || def.Providers[0].Name != "openrouter" {
		t.Fatalf("Providers = %+v, want openrouter", def.Providers)
	}
}

func TestLoadAgentSupportsProjectionShape(t *testing.T) {
	raw := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: weather-agent\ndescription: Reliable weather lookup\nmodel: glm-4.5-air\ntools: [transition_state]\ncontext:\n  inputBudget: 24000\n  projections:\n    - type: system\n      prompt: You are helpful.\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      prompt: Observe first.\n  transitions: []\n")
	def, err := LoadAgent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name != "weather-agent" {
		t.Fatalf("Name = %q, want weather-agent", def.Name)
	}
	if len(def.Context.Projections) != 1 || def.Context.Projections[0].Type != "system" {
		t.Fatalf("Projections = %+v, want one system projection", def.Context.Projections)
	}
	if def.Cognitive.InitialState != "observe" {
		t.Fatalf("InitialState = %q, want observe", def.Cognitive.InitialState)
	}
}

func TestLoadAgentFallsBackToChunksShape(t *testing.T) {
	raw := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: legacy-agent\nmodel: glm-4.5-air\ncontext:\n  inputBudget: 1000\n  chunks:\n    - type: system\n      prompt: Legacy shape.\ncognitive:\n  initialState: observe\n  states: []\n  transitions: []\n")
	def, err := LoadAgent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(def.Context.Projections) != 1 || def.Context.Projections[0].Prompt != "Legacy shape." {
		t.Fatalf("Projections = %+v, want legacy chunk carried into projections", def.Context.Projections)
	}
}

func TestLoadAgentParsesProjectionPolicyFields(t *testing.T) {
	raw := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: policy-agent\nmodel: glm-4.5-air\ncontext:\n  inputBudget: 1000\n  projections:\n    - type: repo_context\n      refreshEveryNTurns: 12\n      retentionMode: latest_effective\n    - type: messages\n      retentionMode: coherent_tail\ncognitive:\n  initialState: observe\n  states: []\n  transitions: []\n")
	def, err := LoadAgent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(def.Context.Projections) != 2 {
		t.Fatalf("got %d projections, want 2", len(def.Context.Projections))
	}
	if def.Context.Projections[0].RefreshEveryNTurns == nil || *def.Context.Projections[0].RefreshEveryNTurns != 12 {
		t.Fatalf("RefreshEveryNTurns = %#v, want 12", def.Context.Projections[0].RefreshEveryNTurns)
	}
	if def.Context.Projections[0].RetentionMode != "latest_effective" {
		t.Fatalf("RetentionMode = %q, want latest_effective", def.Context.Projections[0].RetentionMode)
	}
	if def.Context.Projections[1].RetentionMode != "coherent_tail" {
		t.Fatalf("messages RetentionMode = %q, want coherent_tail", def.Context.Projections[1].RetentionMode)
	}
}

func TestLoadAgentRejectsInvalidProjectionPolicyFields(t *testing.T) {
	raw := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: invalid-agent\nmodel: glm-4.5-air\ncontext:\n  inputBudget: 1000\n  projections:\n    - type: system\n      prompt: hi\n      retentionMode: latest_effective\ncognitive:\n  initialState: observe\n  states: []\n  transitions: []\n")
	_, err := LoadAgent(raw)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadWorkflowParsesDefinition(t *testing.T) {
	raw := []byte("apiVersion: maelstrom/v1\nkind: Workflow\nname: conversation-to-execution\ndescription: Chat through implementation\ncontext: Build carefully.\nstatechart:\n  initialState: chatting\n  states:\n    - name: chatting\n    - name: planning\n  transitions:\n    - trigger: commit_plan\n      from: chatting\n      to: planning\n")
	def, err := LoadWorkflow(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name != "conversation-to-execution" {
		t.Fatalf("Name = %q, want conversation-to-execution", def.Name)
	}
	if len(def.Statechart.Transitions) != 1 || def.Statechart.Transitions[0].To != "planning" {
		t.Fatalf("Transitions = %+v, want chatting->planning", def.Statechart.Transitions)
	}
}

func TestLoadIntoMemoryRoutesByKind(t *testing.T) {
	memory := NewMemory()
	modelRaw := []byte("apiVersion: maelstrom/v1\nkind: Model\nname: test-model\nproviders:\n  - name: openrouter\n    modelRef: test/model\nlimits:\n  contextWindow: 1\n  maxOutputTokens: 1\ndefaults:\n  temperature: 0.1\n  topP: 1.0\ncapabilities:\n  tools: true\n  reasoning: false\n  multimodal: false\n  streaming: false\n")
	agentRaw := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: test-agent\nmodel: test-model\ncontext:\n  inputBudget: 10\n  projections: []\ncognitive:\n  initialState: observe\n  states: []\n  transitions: []\n")
	workflowRaw := []byte("apiVersion: maelstrom/v1\nkind: Workflow\nname: test-workflow\nstatechart:\n  initialState: chatting\n  states: []\n  transitions: []\n")

	if err := LoadIntoMemory(memory, modelRaw); err != nil {
		t.Fatalf("load model: %v", err)
	}
	if err := LoadIntoMemory(memory, agentRaw); err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if err := LoadIntoMemory(memory, workflowRaw); err != nil {
		t.Fatalf("load workflow: %v", err)
	}
	if _, ok := memory.GetModel("test-model"); !ok {
		t.Fatal("expected model in memory catalog")
	}
	if _, ok := memory.GetAgent("test-agent"); !ok {
		t.Fatal("expected agent in memory catalog")
	}
	if _, ok := memory.GetWorkflow("test-workflow"); !ok {
		t.Fatal("expected workflow in memory catalog")
	}
}

func TestReloadReplacesExistingDefinition(t *testing.T) {
	memory := NewMemory()
	first := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: test-agent\ndescription: First description\nmodel: test-model\ncontext:\n  inputBudget: 10\n  projections: []\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      prompt: Observe first.\n  transitions: []\n")
	second := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: test-agent\ndescription: Updated description\nmodel: test-model\ncontext:\n  inputBudget: 10\n  projections: []\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      prompt: Updated prompt.\n  transitions: []\n")

	if err := LoadIntoMemory(memory, first); err != nil {
		t.Fatalf("initial load: %v", err)
	}
	if err := memory.Reload(second); err != nil {
		t.Fatalf("reload: %v", err)
	}
	agent, ok := memory.GetAgent("test-agent")
	if !ok {
		t.Fatal("expected reloaded agent in memory")
	}
	if agent.Description != "Updated description" {
		t.Fatalf("Description = %q, want Updated description", agent.Description)
	}
	if len(agent.Cognitive.States) != 1 || agent.Cognitive.States[0].Prompt != "Updated prompt." {
		t.Fatalf("States = %+v, want updated prompt", agent.Cognitive.States)
	}
}
