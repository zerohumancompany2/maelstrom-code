package main

import (
	"context"
	"fmt"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/binding"
	"github.com/comalice/inference_sketch/sketch/sketch6/charts"
	"github.com/comalice/inference_sketch/sketch/sketch6/hydrate"
	"github.com/comalice/inference_sketch/sketch/sketch6/inference"
	"github.com/comalice/inference_sketch/sketch/sketch6/model"
	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/registry"
	agentregistry "github.com/comalice/inference_sketch/sketch/sketch6/registry/agent"
	modelregistry "github.com/comalice/inference_sketch/sketch/sketch6/registry/model"
	"github.com/comalice/inference_sketch/sketch/sketch6/runner"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/tools"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
)

func main() {
	history := session.NewHistory("session-006")
	history.Append(session.UserMessageRecord{BaseRecord: history.NextRecord("user"), Content: "Research the weather in Paris, then answer succinctly."})
	workflowInstance := workflow.Instance{ID: "workflow-001", SpecID: "ticket-triage", Input: "ticket=42"}
	workflowHistory := workflow.NewHistory(workflowInstance.ID)
	bindingLog := binding.NewLog()
	store := inference.NewStore()
	ctx := context.Background()

	modelStore := registry.NewMemoryStore[model.Definition]()
	modelReg := registry.NewMemoryRegistry[model.Definition]()
	modelIngestor := modelregistry.NewIngestor(modelStore, modelReg)
	modelYAML := []byte("apiVersion: maelstrom/v1\nkind: Model\nname: glm-4.5-air\nproviders:\n  - name: openrouter\n    modelRef: z-ai/glm-4.5-air:free\nlimits:\n  contextWindow: 32768\n  maxOutputTokens: 8192\ndefaults:\n  temperature: 0.7\n  topP: 1.0\ncapabilities:\n  tools: true\n  reasoning: true\n  multimodal: false\n  streaming: true\n")
	if _, err := modelIngestor.Ingest(ctx, "glm-4.5-air", "models/glm-4.5-air.yaml", modelYAML); err != nil {
		fmt.Printf("model ingest failed: %v\n", err)
		return
	}

	agentStore := registry.NewMemoryStore[agent.Definition]()
	agentReg := registry.NewMemoryRegistry[agent.Definition]()
	agentIngestor := agentregistry.NewIngestor(agentStore, agentReg)
	agentYAML := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: weather-agent\ndescription: Reliable weather lookup with concise answers\nmodel: glm-4.5-air\noverrides:\n  temperature: 0.4\ntools: [transition_state, weather]\ncontext:\n  inputBudget: 24000\n  chunks:\n    - type: system\n      prompt: |\n        You are a cheerful, accurate weather assistant.\n        Always respond in Celsius.\n        Be concise and friendly.\n      budgetPct: 0.05\n      policy: fail\n      priority: 10\n    - type: messages\n      flexible: true\n      policy: hard\n      priority: 5\n    - type: state\n      chart: agent\n      priority: 4\n    - type: state\n      chart: workflow\n      priority: 4\n")
	result, err := agentIngestor.Ingest(ctx, "weather-agent", "agents/weather-agent.yaml", agentYAML)
	if err != nil {
		fmt.Printf("agent ingest failed: %v\n", err)
		return
	}
	def := result.Revision.Value

	chartSet := charts.NewSet()
	toolRegistry := tools.NewToolRegistry(
		tools.WeatherTool{},
		tools.TransitionTool{Charts: chartSet},
	)
	hydrator := hydrate.Hydrator{Models: modelReg, Tools: toolRegistry}
	runtimeAgent, err := hydrator.Hydrate(def)
	if err != nil {
		fmt.Printf("hydrate failed: %v\n", err)
		return
	}
	loop := runner.Loop{
		Provider: provider.Stub{},
		Tools:    toolRegistry,
		Recorder: inference.Recorder{GitCommit: "cafebabe-sketch6", AssemblyPipeline: "context/v0.6"},
	}

	bindRecord := binding.Record{ID: bindingLog.NextID(), AgentID: runtimeAgent.Name, WorkflowID: workflowInstance.ID, Action: "bind", Reason: "ticket discovered in queue"}
	bindingLog.Append(bindRecord)
	history.Append(session.WorkflowBindingRefRecord{BaseRecord: history.NextRecord("workflow_binding_ref"), BindingID: bindRecord.ID, WorkflowID: workflowInstance.ID, Action: bindRecord.Action})
	workflowHistory.Append(workflow.BindingRefRecord{BaseRecord: workflowHistory.NextRecord("binding_ref"), BindingID: bindRecord.ID, AgentID: runtimeAgent.Name, Action: bindRecord.Action})
	workflowHistory.Append(workflow.StateTransitionRecord{BaseRecord: workflowHistory.NextRecord("workflow_state_transition"), FromState: "idle", ToState: "active", Trigger: "agent_bound", DerivedFromIDs: []string{bindRecord.ID}})

	if err := loop.Run(runtimeAgent, def, history, store); err != nil {
		fmt.Printf("run failed: %v\n", err)
	}

	unbindRecord := binding.Record{ID: bindingLog.NextID(), AgentID: runtimeAgent.Name, WorkflowID: workflowInstance.ID, Action: "unbind", Reason: "agent completed current workflow segment"}
	bindingLog.Append(unbindRecord)
	history.Append(session.WorkflowBindingRefRecord{BaseRecord: history.NextRecord("workflow_binding_ref"), BindingID: unbindRecord.ID, WorkflowID: workflowInstance.ID, Action: unbindRecord.Action})
	workflowHistory.Append(workflow.BindingRefRecord{BaseRecord: workflowHistory.NextRecord("binding_ref"), BindingID: unbindRecord.ID, AgentID: runtimeAgent.Name, Action: unbindRecord.Action})
	workflowHistory.Append(workflow.StateTransitionRecord{BaseRecord: workflowHistory.NextRecord("workflow_state_transition"), FromState: "active", ToState: "waiting", Trigger: "agent_unbound", DerivedFromIDs: []string{unbindRecord.ID}})

	fmt.Println("Final session history:")
	for i, record := range history.Records {
		fmt.Printf("  %02d. %s\n", i+1, session.DescribeRecord(record))
	}

	fmt.Println("Workflow history:")
	for i, record := range workflowHistory.Records {
		fmt.Printf("  %02d. %s\n", i+1, workflow.DescribeRecord(record))
	}

	fmt.Println("Binding log:")
	for i, record := range bindingLog.Records {
		fmt.Printf("  %02d. %s agent=%s workflow=%s reason=%q\n", i+1, record.Action, record.AgentID, record.WorkflowID, record.Reason)
	}

	fmt.Println("Inference records:")
	for i, record := range store.Records {
		fmt.Printf("  %02d. %s payload=%s sources=%d payload_lines=%d\n", i+1, record.InferenceID, record.PayloadID, len(record.SourceRecordIDs), len(record.Payload))
	}
}
