package runner

import (
	"testing"

	"github.com/comalice/maelstrom/internal/catalog"
	"github.com/comalice/maelstrom/internal/compile"
	"github.com/comalice/maelstrom/internal/logs"
	"github.com/comalice/maelstrom/internal/prompt"
	"github.com/comalice/maelstrom/internal/provider"
	"github.com/comalice/maelstrom/internal/tools"
)

func TestCatalogLoadedDefinitionsCanDriveHydratedRunner(t *testing.T) {
	memory := catalog.NewMemory()
	modelRaw := []byte("apiVersion: maelstrom/v1\nkind: Model\nname: test-model\nproviders:\n  - name: fake\n    modelRef: fake/model\nlimits:\n  contextWindow: 32768\n  maxOutputTokens: 1024\ndefaults:\n  temperature: 0.7\n  topP: 1.0\ncapabilities:\n  tools: true\n  reasoning: true\n  multimodal: false\n  streaming: false\n")
	agentRaw := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: test-agent\ndescription: Test agent\nmodel: test-model\ntools: [bind_workflow, interrupt_session]\ncontext:\n  inputBudget: 2048\n  projections:\n    - type: system\n      name: system\n      prompt: You are helpful.\n    - type: interaction\n    - type: messages\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      prompt: Observe first.\n  transitions: []\n")
	workflowRaw := []byte("apiVersion: maelstrom/v1\nkind: Workflow\nname: test-workflow\ndescription: Test workflow\ncontext: Work carefully.\nstatechart:\n  initialState: chatting\n  states:\n    - name: chatting\n  transitions: []\n")

	if err := catalog.LoadIntoMemory(memory, modelRaw); err != nil {
		t.Fatalf("load model: %v", err)
	}
	if err := catalog.LoadIntoMemory(memory, agentRaw); err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if err := catalog.LoadIntoMemory(memory, workflowRaw); err != nil {
		t.Fatalf("load workflow: %v", err)
	}
	modelDef, _ := memory.GetModel("test-model")
	agentDef, _ := memory.GetAgent("test-agent")
	workflowDef, _ := memory.GetWorkflow("test-workflow")
	toolRegistry := tools.NewRegistry(tools.BindingTool{}, tools.InterruptTool{})
	hydratedAgent, err := compile.HydrateAgent(agentDef, modelDef, toolRegistry)
	if err != nil {
		t.Fatalf("hydrate agent: %v", err)
	}
	projections, maxHistory, err := prompt.BuildProjectionPlan(agentDef)
	if err != nil {
		t.Fatalf("build projection plan: %v", err)
	}
	loop := Loop{
		Provider:    &provider.FakeProvider{Response: provider.Response{Outputs: []provider.Output{provider.AssistantOutput{Content: "Acknowledged."}}}},
		Tools:       toolRegistry,
		Projections: projections,
		MaxHistory:  maxHistory,
	}
	sessionHistory := logs.NewSessionHistory("session-011")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: "Hello there."})
	workflowHistory := logs.NewWorkflowHistory("test-workflow-instance")

	if err := loop.Run(hydratedAgent, agentDef, &workflowDef, sessionHistory, workflowHistory); err != nil {
		t.Fatalf("loop run: %v", err)
	}
	foundAssistant := false
	for _, record := range sessionHistory.Records {
		if msg, ok := record.(logs.AssistantMessageRecord); ok && msg.Content == "Acknowledged." {
			foundAssistant = true
		}
	}
	if !foundAssistant {
		t.Fatalf("expected assistant output in session history, got %#v", sessionHistory.Records)
	}
}
