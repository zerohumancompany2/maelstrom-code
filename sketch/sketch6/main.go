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
	workflowregistry "github.com/comalice/inference_sketch/sketch/sketch6/registry/workflow"
	"github.com/comalice/inference_sketch/sketch/sketch6/runner"
	"github.com/comalice/inference_sketch/sketch/sketch6/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch6/session"
	"github.com/comalice/inference_sketch/sketch/sketch6/tools"
	"github.com/comalice/inference_sketch/sketch/sketch6/workflow"
)

func main() {
	history := session.NewHistory("session-006")
	history.Append(session.UserMessageRecord{BaseRecord: history.NextRecord("user"), Content: "Research the weather in Paris, then answer succinctly."})
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

	workflowStore := registry.NewMemoryStore[workflow.Definition]()
	workflowReg := registry.NewMemoryRegistry[workflow.Definition]()
	workflowIngestor := workflowregistry.NewIngestor(workflowStore, workflowReg)
	workflowYAML := []byte("apiVersion: maelstrom/v1\nkind: Workflow\nname: weather-ticket\ndescription: Weather lookup ticket\ncontext: Deliver a succinct, correct weather report.\nstatechart:\n  initialState: available\n  states:\n    - name: available\n      description: Ready for an agent to begin\n      visibleTools: [transition_state, weather]\n      enabledTools: [transition_state]\n    - name: lookup_pending\n      description: Waiting on weather lookup\n      visibleTools: [transition_state, weather]\n      enabledTools: [transition_state, weather]\n    - name: data_ready\n      description: Weather data available; ready to answer\n      visibleTools: [transition_state]\n      enabledTools: [transition_state]\n  transitions:\n    - trigger: begin_lookup\n      from: available\n      to: lookup_pending\n    - trigger: weather_received\n      from: lookup_pending\n      to: data_ready\n")
	workflowResult, err := workflowIngestor.Ingest(ctx, "weather-ticket", "workflows/weather-ticket.yaml", workflowYAML)
	if err != nil {
		fmt.Printf("workflow ingest failed: %v\n", err)
		return
	}
	workflowDef := workflowResult.Revision.Value
	workflowInstance := workflow.Instance{ID: "workflow-001", SpecID: workflowDef.Name, Input: "ticket=42"}
	workflowHistory := workflow.NewHistory(workflowInstance.ID)

	agentStore := registry.NewMemoryStore[agent.Definition]()
	agentReg := registry.NewMemoryRegistry[agent.Definition]()
	agentIngestor := agentregistry.NewIngestor(agentStore, agentReg)
	agentYAML := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: weather-agent\ndescription: Reliable weather lookup with concise answers\nmodel: glm-4.5-air\noverrides:\n  temperature: 0.4\ntools: [transition_state, weather]\ncontext:\n  inputBudget: 24000\n  chunks:\n    - type: system\n      prompt: |\n        You are a cheerful, accurate weather assistant.\n        Always respond in Celsius.\n        Be concise and friendly.\n    - type: binding\n    - type: workflow_state\n    - type: cognitive_state\n    - type: messages\ncognitive:\n  initialState: observe\n  states:\n    - name: observe\n      description: Gather facts and inspect context\n      visibleTools: [transition_state, weather]\n      enabledTools: [transition_state]\n      prompt: Observe the task and gather facts before acting.\n    - name: orient\n      description: Orient to current facts\n      visibleTools: [transition_state, weather]\n      enabledTools: [transition_state]\n      prompt: Orient to the gathered facts and determine the next action.\n    - name: act\n      description: Perform allowed actions\n      visibleTools: [transition_state, weather]\n      enabledTools: [transition_state, weather]\n      prompt: Act on the current plan using the available tools.\n  transitions:\n    - trigger: start_research\n      from: observe\n      to: orient\n    - trigger: begin_action\n      from: orient\n      to: act\n    - trigger: draft_answer\n      from: act\n      to: observe\n")
	result, err := agentIngestor.Ingest(ctx, "weather-agent", "agents/weather-agent.yaml", agentYAML)
	if err != nil {
		fmt.Printf("agent ingest failed: %v\n", err)
		return
	}
	def := result.Revision.Value

	chartSet := charts.NewSet(def, workflowDef)
	toolRegistry := tools.NewToolRegistry(
		tools.WeatherTool{WorkflowHistory: workflowHistory, InitialState: workflowDef.Statechart.InitialState},
		tools.TransitionTool{Charts: chartSet, WorkflowHistory: workflowHistory, InitialStates: map[string]string{"agent": def.Cognitive.InitialState, "workflow": workflowDef.Statechart.InitialState}},
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
		Recorder: inference.Recorder{GitCommit: "cafebabe-sketch6", AssemblyPipeline: "context/v0.7"},
	}

	bindRecord := binding.Record{ID: bindingLog.NextID(), AgentID: runtimeAgent.Name, WorkflowID: workflowInstance.ID, Action: "bind", Reason: "ticket discovered in queue"}
	bindingLog.Append(bindRecord)
	history.Append(session.WorkflowBindingRefRecord{BaseRecord: history.NextRecord("workflow_binding_ref"), BindingID: bindRecord.ID, WorkflowID: workflowInstance.ID, Action: bindRecord.Action})
	workflowHistory.Append(workflow.BindingRefRecord{BaseRecord: workflowHistory.NextRecord("binding_ref"), BindingID: bindRecord.ID, AgentID: runtimeAgent.Name, Action: bindRecord.Action})

	workflowSnapshot := &runtime.WorkflowSnapshot{WorkflowID: workflowInstance.ID, CurrentState: workflowDef.Statechart.InitialState, VisibleTools: []string{"transition_state", "weather"}, EnabledTools: []string{"transition_state"}, Description: workflowDef.Description, Context: workflowDef.Context, LastBoundAgent: runtimeAgent.Name}
	bindingSnapshot := runtime.BindingSnapshot{WorkflowID: workflowInstance.ID, Bound: true}

	if err := loop.Run(runtimeAgent, def, workflowSnapshot, bindingSnapshot, history, workflowHistory, store); err != nil {
		fmt.Printf("run failed: %v\n", err)
	}

	unbindRecord := binding.Record{ID: bindingLog.NextID(), AgentID: runtimeAgent.Name, WorkflowID: workflowInstance.ID, Action: "unbind", Reason: "agent completed current workflow segment"}
	bindingLog.Append(unbindRecord)
	history.Append(session.WorkflowBindingRefRecord{BaseRecord: history.NextRecord("workflow_binding_ref"), BindingID: unbindRecord.ID, WorkflowID: workflowInstance.ID, Action: unbindRecord.Action})
	workflowHistory.Append(workflow.BindingRefRecord{BaseRecord: workflowHistory.NextRecord("binding_ref"), BindingID: unbindRecord.ID, AgentID: runtimeAgent.Name, Action: unbindRecord.Action})

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
