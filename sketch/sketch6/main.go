package main

import (
	"context"
	"fmt"
	"os"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/bind"
	"github.com/comalice/inference_sketch/sketch/sketch6/binding"
	"github.com/comalice/inference_sketch/sketch/sketch6/charts"
	"github.com/comalice/inference_sketch/sketch/sketch6/inference"
	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/registry"
	agentregistry "github.com/comalice/inference_sketch/sketch/sketch6/registry/agent"
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
	_ = os.Setenv("PROVIDER_NAME", "openrouter")
	_ = os.Setenv("MODEL_NAME", "z-ai/glm-4.5-air:free")

	agentStore := registry.NewMemoryStore[agent.Spec]()
	agentReg := registry.NewMemoryRegistry[agent.Spec]()
	agentIngestor := agentregistry.NewIngestor(agentStore, agentReg)
	agentYAML := []byte("apiVersion: maelstrom/v1\nkind: Agent\nname: weather-agent\nversion: v0.0.6-sketch\nprovider: ${PROVIDER_NAME}\nmodel: ${MODEL_NAME}\nmax_history: 10\ntools: [transition_state, weather]\ntemperature: 0.7\ncontext_limit: 32768\n")
	result, err := agentIngestor.Ingest(ctx, "weather-agent", "agents/weather-agent.yaml", agentYAML)
	if err != nil {
		fmt.Printf("agent ingest failed: %v\n", err)
		return
	}
	spec := result.Revision.Value

	chartSet := charts.NewSet()
	toolRegistry := tools.NewToolRegistry(
		tools.WeatherTool{},
		tools.TransitionTool{Charts: chartSet},
	)
	binder := bind.AgentBinder{
		Resolver: bind.OSEnvResolver{},
		Provider: provider.Stub{},
		Tools:    toolRegistry,
		Recorder: inference.Recorder{GitCommit: "cafebabe-sketch6", AssemblyPipeline: "context/v0.6"},
	}
	bound, err := binder.Bind(ctx, spec)
	if err != nil {
		fmt.Printf("bind failed: %v\n", err)
		return
	}

	bindRecord := binding.Record{ID: bindingLog.NextID(), AgentID: bound.Spec.ID, WorkflowID: workflowInstance.ID, Action: "bind", Reason: "ticket discovered in queue"}
	bindingLog.Append(bindRecord)
	history.Append(session.WorkflowBindingRefRecord{BaseRecord: history.NextRecord("workflow_binding_ref"), BindingID: bindRecord.ID, WorkflowID: workflowInstance.ID, Action: bindRecord.Action})
	workflowHistory.Append(workflow.BindingRefRecord{BaseRecord: workflowHistory.NextRecord("binding_ref"), BindingID: bindRecord.ID, AgentID: bound.Spec.ID, Action: bindRecord.Action})
	workflowHistory.Append(workflow.StateTransitionRecord{BaseRecord: workflowHistory.NextRecord("workflow_state_transition"), FromState: "idle", ToState: "active", Trigger: "agent_bound", DerivedFromIDs: []string{bindRecord.ID}})

	if err := bound.Loop.Run(bound.Spec, history, store); err != nil {
		fmt.Printf("run failed: %v\n", err)
	}

	unbindRecord := binding.Record{ID: bindingLog.NextID(), AgentID: bound.Spec.ID, WorkflowID: workflowInstance.ID, Action: "unbind", Reason: "agent completed current workflow segment"}
	bindingLog.Append(unbindRecord)
	history.Append(session.WorkflowBindingRefRecord{BaseRecord: history.NextRecord("workflow_binding_ref"), BindingID: unbindRecord.ID, WorkflowID: workflowInstance.ID, Action: unbindRecord.Action})
	workflowHistory.Append(workflow.BindingRefRecord{BaseRecord: workflowHistory.NextRecord("binding_ref"), BindingID: unbindRecord.ID, AgentID: bound.Spec.ID, Action: unbindRecord.Action})
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
