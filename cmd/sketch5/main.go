package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/qmuntal/stateless"
)

func main() {
	history := NewSessionHistory("session-005")
	history.Append(UserMessageRecord{BaseRecord: history.NextRecord("user"), Content: "Research the weather in Paris, then answer succinctly."})

	store := NewInferenceStore()
	agent := buildSketchAgent()

	if err := agent.RunTurn(history, store); err != nil {
		fmt.Printf("turn failed: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Final session history:")
	for i, record := range history.Records {
		fmt.Printf("  %02d. %s\n", i+1, describeSessionRecord(record))
	}

	fmt.Println()
	fmt.Println("Inference records:")
	for i, record := range store.Records {
		fmt.Printf("  %02d. %s\n", i+1, describeInferenceRecord(record))
	}

	fmt.Println()
	fmt.Println("Sketch5 takeaway: agent/workflow charts are parallel, inferred from durable transition records, and advanced only through explicit transition tools.")
}

func buildSketchAgent() AgentRuntime {
	registry := NewToolRegistry(
		WeatherTool{},
		TransitionTool{Charts: NewChartSet()},
	)

	return AgentRuntime{
		Spec: AgentSpec{
			ID:      "weather-agent",
			Version: "v0.0.5-sketch",
			Model:   ModelSpec{Provider: "openrouter", Name: "z-ai/glm-4.5-air:free", Temperature: 0.7, ContextLimit: 32768},
			Context: ContextSpec{MaxHistoryItems: 10},
			Tools:   registry.Definitions(),
		},
		Assembler: Assembler{Chunks: []Chunk{
			SystemPromptChunk{},
			RecentHistoryChunk{},
			StateProjectionChunk{ChartName: "agent"},
			StateProjectionChunk{ChartName: "workflow"},
		}},
		Provider:   StubProvider{},
		Tools:      registry,
		Normalizer: ToolNormalizer{Definitions: registry.Definitions()},
		Recorder:   InferenceRecorder{GitCommit: "cafebabe-sketch5", AssemblyPipeline: "context/v0.5"},
	}
}

type AgentSpec struct {
	ID      string
	Version string
	Model   ModelSpec
	Context ContextSpec
	Tools   []ToolDefinition
}

type ModelSpec struct {
	Provider     string
	Name         string
	Temperature  float64
	ContextLimit int
}

type ContextSpec struct {
	MaxHistoryItems int
}

type AgentRuntime struct {
	Spec       AgentSpec
	Assembler  Assembler
	Provider   Provider
	Tools      ToolExecutor
	Normalizer ToolNormalizer
	Recorder   InferenceRecorder
}

func (a AgentRuntime) RunTurn(history *SessionHistory, store *InferenceStore) error {
	fmt.Printf("[agent] run turn agent=%s version=%s\n", a.Spec.ID, a.Spec.Version)

	for iteration := 1; ; iteration++ {
		fmt.Printf("[turn] iteration=%d\n", iteration)

		assembled, err := a.Assembler.Assemble(AssemblyInput{
			Agent:   a.Spec,
			History: history,
			Charts:  BuildChartSnapshot(history),
		})
		if err != nil {
			return err
		}

		bundle := BuildInferenceBundle(a.Spec, history.NextBundleID(), history.SessionID, assembled)
		store.Append(a.Recorder.RecordBundle(bundle))

		response, err := a.Provider.Send(bundle)
		if err != nil {
			return err
		}

		hasToolCalls := false
		for _, output := range response.Outputs {
			records, err := a.consumeProviderOutput(history, output)
			if err != nil {
				return err
			}
			for _, record := range records {
				history.Append(record)
				if _, ok := record.(ToolCallRequestRecord); ok {
					hasToolCalls = true
				}
			}
		}

		if !hasToolCalls {
			fmt.Println("[turn] complete; provider returned final answer")
			return nil
		}

		fmt.Println("[turn] tool call observed; continue same turn")
		if iteration == 5 {
			fmt.Println("[turn] sketch guard: stop after fifth iteration")
			return nil
		}
	}
}

func (a AgentRuntime) consumeProviderOutput(history *SessionHistory, output ProviderOutput) ([]SessionRecord, error) {
	switch v := output.(type) {
	case ProviderAssistantOutput:
		return []SessionRecord{AssistantMessageRecord{BaseRecord: history.NextRecord("assistant"), Content: v.Content}}, nil
	case ProviderToolCallOutput:
		validated, err := a.Normalizer.Normalize(v)
		if err != nil {
			return nil, err
		}

		requestRecord := ToolCallRequestRecord{
			BaseRecord: history.NextRecord("tool_call_request"),
			CallID:     validated.CallID,
			ToolName:   validated.Definition.Name,
			Arguments:  string(validated.ArgumentsJSON),
		}

		result, err := a.Tools.Execute(ToolExecutionRequest{
			Agent:   a.Spec,
			Call:    validated,
			History: history,
			Request: requestRecord,
		})
		if err != nil {
			return nil, err
		}

		records := []SessionRecord{requestRecord}
		records = append(records, result.Records...)
		records = append(records, ToolCallResultRecord{
			BaseRecord: history.NextRecord("tool_call_result"),
			CallID:     validated.CallID,
			ToolName:   result.ToolName,
			Content:    result.DisplayContent,
			IsError:    result.IsError,
		})
		return records, nil
	default:
		return nil, fmt.Errorf("unknown provider output %T", output)
	}
}

type SessionRecord interface {
	RecordID() string
	RecordKind() string
}

type BaseRecord struct {
	ID   string
	Kind string
}

func (b BaseRecord) RecordID() string   { return b.ID }
func (b BaseRecord) RecordKind() string { return b.Kind }

type UserMessageRecord struct {
	BaseRecord
	Content string
}

type AssistantMessageRecord struct {
	BaseRecord
	Content string
}

type ToolCallRequestRecord struct {
	BaseRecord
	CallID    string
	ToolName  string
	Arguments string
}

type ToolCallResultRecord struct {
	BaseRecord
	CallID   string
	ToolName string
	Content  string
	IsError  bool
}

type StateTransitionRecord struct {
	BaseRecord
	ChartName      string
	FromState      string
	ToState        string
	Trigger        string
	DerivedFromIDs []string
}

type SessionHistory struct {
	SessionID string
	Records   []SessionRecord
	sequence  int
	bundles   int
}

func NewSessionHistory(sessionID string) *SessionHistory {
	return &SessionHistory{SessionID: sessionID}
}

func (h *SessionHistory) NextRecord(kind string) BaseRecord {
	h.sequence++
	return BaseRecord{ID: fmt.Sprintf("%s-r%03d", h.SessionID, h.sequence), Kind: kind}
}

func (h *SessionHistory) NextBundleID() string {
	h.bundles++
	return fmt.Sprintf("%s-bundle-%03d", h.SessionID, h.bundles)
}

func (h *SessionHistory) Append(record SessionRecord) {
	fmt.Printf("[history] append %s\n", describeSessionRecord(record))
	h.Records = append(h.Records, record)
}

type ChartSnapshot struct {
	States map[string]string
}

func BuildChartSnapshot(history *SessionHistory) ChartSnapshot {
	states := map[string]string{
		"agent":    "idle",
		"workflow": "idle",
	}

	for _, record := range history.Records {
		transition, ok := record.(StateTransitionRecord)
		if !ok {
			continue
		}
		states[transition.ChartName] = transition.ToState
	}

	return ChartSnapshot{States: states}
}

func (c ChartSnapshot) State(chartName string) string {
	if state, ok := c.States[chartName]; ok {
		return state
	}
	return "unknown"
}

type Segment interface {
	SegmentKind() string
	TokenText() string
	SourceRecordIDs() []string
	ProvenanceStep() ProvenanceStep
}

type PromptSegment struct {
	Role      string
	Content   string
	RecordIDs []string
	Step      ProvenanceStep
}

func (p PromptSegment) SegmentKind() string            { return "prompt" }
func (p PromptSegment) TokenText() string              { return p.Content }
func (p PromptSegment) SourceRecordIDs() []string      { return append([]string(nil), p.RecordIDs...) }
func (p PromptSegment) ProvenanceStep() ProvenanceStep { return p.Step }

type StateSegment struct {
	ChartName string
	State     string
	RecordIDs []string
	Step      ProvenanceStep
}

func (s StateSegment) SegmentKind() string            { return "state" }
func (s StateSegment) TokenText() string              { return s.State }
func (s StateSegment) SourceRecordIDs() []string      { return append([]string(nil), s.RecordIDs...) }
func (s StateSegment) ProvenanceStep() ProvenanceStep { return s.Step }

type ProvenanceStep struct {
	ChunkName        string
	Operation        string
	InputRecordIDs   []string
	OutputDescriptor string
}

type AssemblyInput struct {
	Agent   AgentSpec
	History *SessionHistory
	Charts  ChartSnapshot
}

type AssemblyResult struct {
	Segments []Segment
	Steps    []ProvenanceStep
}

type Chunk interface {
	Name() string
	Build(input AssemblyInput) (ChunkResult, error)
}

type ChunkResult struct {
	Segments []Segment
	Steps    []ProvenanceStep
}

type Assembler struct {
	Chunks []Chunk
}

func (a Assembler) Assemble(input AssemblyInput) (AssemblyResult, error) {
	fmt.Printf("[assemble] start agent=%s chunks=%d\n", input.Agent.ID, len(a.Chunks))
	result := AssemblyResult{}
	for _, chunk := range a.Chunks {
		fmt.Printf("[assemble] build chunk=%s\n", chunk.Name())
		chunkResult, err := chunk.Build(input)
		if err != nil {
			return AssemblyResult{}, fmt.Errorf("build chunk %s: %w", chunk.Name(), err)
		}
		result.Segments = append(result.Segments, chunkResult.Segments...)
		result.Steps = append(result.Steps, chunkResult.Steps...)
	}
	return result, nil
}

type SystemPromptChunk struct{}

func (SystemPromptChunk) Name() string { return "system-prompt" }

func (SystemPromptChunk) Build(input AssemblyInput) (ChunkResult, error) {
	content := fmt.Sprintf("You are %s version %s on model %s. Tools: %s.", input.Agent.ID, input.Agent.Version, input.Agent.Model.Name, joinToolNames(input.Agent.Tools))
	step := ProvenanceStep{ChunkName: "system-prompt", Operation: "synthesize", OutputDescriptor: "system prompt"}
	return ChunkResult{Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}

type RecentHistoryChunk struct{}

func (RecentHistoryChunk) Name() string { return "recent-history" }

func (RecentHistoryChunk) Build(input AssemblyInput) (ChunkResult, error) {
	limit := input.Agent.Context.MaxHistoryItems
	start := len(input.History.Records) - limit
	if start < 0 {
		start = 0
	}

	result := ChunkResult{}
	for _, record := range input.History.Records[start:] {
		switch v := record.(type) {
		case UserMessageRecord:
			step := ProvenanceStep{ChunkName: "recent-history", Operation: "project-user", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "user prompt segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "user", Content: v.Content, RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case AssistantMessageRecord:
			step := ProvenanceStep{ChunkName: "recent-history", Operation: "project-assistant", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "assistant prompt segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "assistant", Content: v.Content, RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case ToolCallRequestRecord:
			step := ProvenanceStep{ChunkName: "recent-history", Operation: "project-tool-call", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "tool-call segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "assistant", Content: fmt.Sprintf("tool call %s(%s)", v.ToolName, v.Arguments), RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case ToolCallResultRecord:
			step := ProvenanceStep{ChunkName: "recent-history", Operation: "project-tool-result", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "tool result segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "tool", Content: v.Content, RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case StateTransitionRecord:
			fmt.Printf("[chunk:recent-history] skip state transition chart=%s\n", v.ChartName)
		}
	}
	return result, nil
}

type StateProjectionChunk struct {
	ChartName string
}

func (c StateProjectionChunk) Name() string { return "state-projection-" + c.ChartName }

func (c StateProjectionChunk) Build(input AssemblyInput) (ChunkResult, error) {
	recordIDs := latestTransitionRecordIDs(input.History, c.ChartName)
	state := input.Charts.State(c.ChartName)
	step := ProvenanceStep{ChunkName: c.Name(), Operation: "project-state", InputRecordIDs: recordIDs, OutputDescriptor: c.ChartName + " state"}
	return ChunkResult{
		Segments: []Segment{StateSegment{ChartName: c.ChartName, State: state, RecordIDs: recordIDs, Step: step}},
		Steps:    []ProvenanceStep{step},
	}, nil
}

func latestTransitionRecordIDs(history *SessionHistory, chartName string) []string {
	for i := len(history.Records) - 1; i >= 0; i-- {
		transition, ok := history.Records[i].(StateTransitionRecord)
		if ok && transition.ChartName == chartName {
			return []string{transition.RecordID()}
		}
	}
	return nil
}

type InferenceBundle struct {
	BundleID        string
	SessionID       string
	AgentID         string
	AgentVersion    string
	Provider        string
	ModelName       string
	RenderedPayload []string
	SourceRecordIDs []string
	Steps           []ProvenanceStep
}

func BuildInferenceBundle(agent AgentSpec, bundleID, sessionID string, assembled AssemblyResult) InferenceBundle {
	sourceSet := map[string]struct{}{}
	rendered := make([]string, 0, len(assembled.Segments))
	for _, segment := range assembled.Segments {
		for _, id := range segment.SourceRecordIDs() {
			sourceSet[id] = struct{}{}
		}
		switch v := segment.(type) {
		case PromptSegment:
			rendered = append(rendered, fmt.Sprintf("role=%s content=%q", v.Role, v.Content))
		case StateSegment:
			rendered = append(rendered, fmt.Sprintf("state=%s value=%q", v.ChartName, v.State))
		default:
			rendered = append(rendered, fmt.Sprintf("segment=%s content=%q", segment.SegmentKind(), segment.TokenText()))
		}
	}

	sourceIDs := make([]string, 0, len(sourceSet))
	for id := range sourceSet {
		sourceIDs = append(sourceIDs, id)
	}
	sort.Strings(sourceIDs)

	return InferenceBundle{
		BundleID:        bundleID,
		SessionID:       sessionID,
		AgentID:         agent.ID,
		AgentVersion:    agent.Version,
		Provider:        agent.Model.Provider,
		ModelName:       agent.Model.Name,
		RenderedPayload: rendered,
		SourceRecordIDs: sourceIDs,
		Steps:           append([]ProvenanceStep(nil), assembled.Steps...),
	}
}

type InferenceRecord struct {
	InferenceID      string
	BundleID         string
	SessionID        string
	AgentID          string
	AgentVersion     string
	Provider         string
	ModelName        string
	GitCommit        string
	AssemblyPipeline string
	SourceRecordIDs  []string
	Steps            []ProvenanceStep
	Payload          []string
}

type InferenceRecorder struct {
	GitCommit        string
	AssemblyPipeline string
}

func (r InferenceRecorder) RecordBundle(bundle InferenceBundle) InferenceRecord {
	inferenceID := bundle.BundleID + "-record"
	return InferenceRecord{
		InferenceID:      inferenceID,
		BundleID:         bundle.BundleID,
		SessionID:        bundle.SessionID,
		AgentID:          bundle.AgentID,
		AgentVersion:     bundle.AgentVersion,
		Provider:         bundle.Provider,
		ModelName:        bundle.ModelName,
		GitCommit:        r.GitCommit,
		AssemblyPipeline: r.AssemblyPipeline,
		SourceRecordIDs:  append([]string(nil), bundle.SourceRecordIDs...),
		Steps:            append([]ProvenanceStep(nil), bundle.Steps...),
		Payload:          append([]string(nil), bundle.RenderedPayload...),
	}
}

type InferenceStore struct {
	Records []InferenceRecord
}

func NewInferenceStore() *InferenceStore { return &InferenceStore{} }

func (s *InferenceStore) Append(record InferenceRecord) {
	fmt.Printf("[inference-store] append %s\n", describeInferenceRecord(record))
	s.Records = append(s.Records, record)
}

type ProviderOutput interface{ providerOutput() }

type ProviderAssistantOutput struct{ Content string }

func (ProviderAssistantOutput) providerOutput() {}

type ProviderToolCallOutput struct {
	CallID    string
	ToolName  string
	Arguments json.RawMessage
}

func (ProviderToolCallOutput) providerOutput() {}

type ProviderResponse struct {
	Outputs []ProviderOutput
}

type Provider interface {
	Send(bundle InferenceBundle) (ProviderResponse, error)
}

type StubProvider struct{}

func (StubProvider) Send(bundle InferenceBundle) (ProviderResponse, error) {
	fmt.Printf("[provider] send bundle=%s\n", bundle.BundleID)
	for _, line := range bundle.RenderedPayload {
		fmt.Printf("[provider]   %s\n", line)
	}

	agentState := extractState(bundle.RenderedPayload, "agent")
	workflowState := extractState(bundle.RenderedPayload, "workflow")

	switch {
	case agentState == "idle":
		return ProviderResponse{Outputs: []ProviderOutput{ProviderToolCallOutput{CallID: "call_agent_1", ToolName: "transition_state", Arguments: json.RawMessage(`{"chart":"agent","trigger":"start_research"}`)}}}, nil
	case workflowState == "idle":
		return ProviderResponse{Outputs: []ProviderOutput{ProviderToolCallOutput{CallID: "call_workflow_1", ToolName: "transition_state", Arguments: json.RawMessage(`{"chart":"workflow","trigger":"begin_lookup"}`)}}}, nil
	case workflowState == "lookup_pending":
		return ProviderResponse{Outputs: []ProviderOutput{ProviderToolCallOutput{CallID: "call_weather_1", ToolName: "weather", Arguments: json.RawMessage(`{"location":"Paris"}`)}}}, nil
	case workflowState == "data_ready" && agentState == "researching":
		return ProviderResponse{Outputs: []ProviderOutput{ProviderToolCallOutput{CallID: "call_agent_2", ToolName: "transition_state", Arguments: json.RawMessage(`{"chart":"agent","trigger":"draft_answer"}`)}}}, nil
	case workflowState == "data_ready" && agentState == "answering":
		return ProviderResponse{Outputs: []ProviderOutput{ProviderAssistantOutput{Content: "It is 70C, rainy, with winds out of the SSW in Paris."}}}, nil
	default:
		return ProviderResponse{Outputs: []ProviderOutput{ProviderAssistantOutput{Content: "State machine reached an unexpected configuration."}}}, nil
	}
}

func extractState(lines []string, chart string) string {
	prefix := fmt.Sprintf("state=%s value=", chart)
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(line[len(prefix):], `"`)
		}
	}
	return ""
}

type ToolDefinition struct {
	Name       string
	Parameters map[string]string
}

type Tool interface {
	Definition() ToolDefinition
	Execute(request ToolExecutionRequest) (ToolExecutionResult, error)
}

type ToolRegistry struct {
	definitions map[string]ToolDefinition
	tools       map[string]Tool
}

func NewToolRegistry(tools ...Tool) ToolRegistry {
	registry := ToolRegistry{definitions: map[string]ToolDefinition{}, tools: map[string]Tool{}}
	for _, tool := range tools {
		def := tool.Definition()
		registry.definitions[def.Name] = def
		registry.tools[def.Name] = tool
	}
	return registry
}

func (r ToolRegistry) Definitions() []ToolDefinition {
	defs := make([]ToolDefinition, 0, len(r.definitions))
	for _, def := range r.definitions {
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

type ValidatedToolCall struct {
	CallID        string
	Definition    ToolDefinition
	Arguments     map[string]string
	ArgumentsJSON json.RawMessage
}

type ToolNormalizer struct {
	Definitions []ToolDefinition
}

func (n ToolNormalizer) Normalize(output ProviderToolCallOutput) (ValidatedToolCall, error) {
	fmt.Printf("[tool-normalizer] normalize tool=%s call_id=%s\n", output.ToolName, output.CallID)
	var def ToolDefinition
	found := false
	for _, candidate := range n.Definitions {
		if candidate.Name == output.ToolName {
			def = candidate
			found = true
			break
		}
	}
	if !found {
		return ValidatedToolCall{}, fmt.Errorf("unknown tool %q", output.ToolName)
	}

	args := map[string]string{}
	if err := json.Unmarshal(output.Arguments, &args); err != nil {
		return ValidatedToolCall{}, fmt.Errorf("decode tool args: %w", err)
	}
	for required := range def.Parameters {
		if _, ok := args[required]; !ok {
			return ValidatedToolCall{}, fmt.Errorf("missing arg %q", required)
		}
	}

	return ValidatedToolCall{CallID: output.CallID, Definition: def, Arguments: args, ArgumentsJSON: append(json.RawMessage(nil), output.Arguments...)}, nil
}

type ToolExecutionRequest struct {
	Agent   AgentSpec
	Call    ValidatedToolCall
	History *SessionHistory
	Request ToolCallRequestRecord
}

type ToolExecutionResult struct {
	ToolName       string
	DisplayContent string
	IsError        bool
	Records        []SessionRecord
}

type ToolExecutor interface {
	Execute(request ToolExecutionRequest) (ToolExecutionResult, error)
}

func (r ToolRegistry) Execute(request ToolExecutionRequest) (ToolExecutionResult, error) {
	tool, ok := r.tools[request.Call.Definition.Name]
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("missing tool implementation %q", request.Call.Definition.Name)
	}
	return tool.Execute(request)
}

type WeatherTool struct{}

func (WeatherTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "weather", Parameters: map[string]string{"location": "string"}}
}

func (WeatherTool) Execute(request ToolExecutionRequest) (ToolExecutionResult, error) {
	location := request.Call.Arguments["location"]
	chartSnapshot := BuildChartSnapshot(request.History)
	workflowState := chartSnapshot.State("workflow")
	if workflowState != "lookup_pending" {
		return ToolExecutionResult{ToolName: "weather", DisplayContent: fmt.Sprintf("tool error: workflow must be lookup_pending, got %s", workflowState), IsError: true}, nil
	}

	transitionRecord := StateTransitionRecord{
		BaseRecord:     request.History.NextRecord("state_transition"),
		ChartName:      "workflow",
		FromState:      workflowState,
		ToState:        "data_ready",
		Trigger:        "weather_received",
		DerivedFromIDs: []string{request.Request.RecordID()},
	}

	return ToolExecutionResult{
		ToolName:       "weather",
		DisplayContent: fmt.Sprintf("70C, rainy, winds out of SSW in %s", location),
		Records:        []SessionRecord{transitionRecord},
	}, nil
}

type TransitionTool struct {
	Charts ChartSet
}

func (TransitionTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "transition_state", Parameters: map[string]string{"chart": "string", "trigger": "string"}}
}

func (t TransitionTool) Execute(request ToolExecutionRequest) (ToolExecutionResult, error) {
	chartName := request.Call.Arguments["chart"]
	trigger := request.Call.Arguments["trigger"]
	fromState := BuildChartSnapshot(request.History).State(chartName)
	toState, err := t.Charts.Fire(chartName, fromState, trigger)
	if err != nil {
		return ToolExecutionResult{ToolName: "transition_state", DisplayContent: fmt.Sprintf("transition error: %v", err), IsError: true}, nil
	}

	transitionRecord := StateTransitionRecord{
		BaseRecord:     request.History.NextRecord("state_transition"),
		ChartName:      chartName,
		FromState:      fromState,
		ToState:        toState,
		Trigger:        trigger,
		DerivedFromIDs: []string{request.Request.RecordID()},
	}

	return ToolExecutionResult{
		ToolName:       "transition_state",
		DisplayContent: fmt.Sprintf("transitioned %s: %s -> %s via %s", chartName, fromState, toState, trigger),
		Records:        []SessionRecord{transitionRecord},
	}, nil
}

type ChartSet struct {
	charts map[string]*ChartDefinition
}

func NewChartSet() ChartSet {
	return ChartSet{charts: map[string]*ChartDefinition{
		"agent":    buildAgentChart(),
		"workflow": buildWorkflowChart(),
	}}
}

func (c ChartSet) Fire(chartName, currentState, trigger string) (string, error) {
	chart, ok := c.charts[chartName]
	if !ok {
		return "", fmt.Errorf("unknown chart %q", chartName)
	}
	return chart.Fire(currentState, trigger)
}

type ChartDefinition struct {
	name        string
	transitions map[string]map[string]string
}

func (c *ChartDefinition) Fire(currentState, trigger string) (string, error) {
	machine := stateless.NewStateMachine(currentState)
	seenStates := map[string]struct{}{}
	for from, triggers := range c.transitions {
		seenStates[from] = struct{}{}
		config := machine.Configure(from)
		for trig, to := range triggers {
			seenStates[to] = struct{}{}
			config.Permit(stateless.Trigger(trig), to)
		}
		_ = seenStates
	}

	if err := machine.Fire(stateless.Trigger(trigger)); err != nil {
		return "", err
	}
	state, err := machine.State(context.Background())
	if err != nil {
		return "", err
	}
	result, ok := state.(string)
	if !ok {
		return "", fmt.Errorf("chart %s produced non-string state %T", c.name, state)
	}
	return result, nil
}

func buildAgentChart() *ChartDefinition {
	return &ChartDefinition{name: "agent", transitions: map[string]map[string]string{
		"idle":        {"start_research": "researching"},
		"researching": {"draft_answer": "answering"},
	}}
}

func buildWorkflowChart() *ChartDefinition {
	return &ChartDefinition{name: "workflow", transitions: map[string]map[string]string{
		"idle":           {"begin_lookup": "lookup_pending"},
		"lookup_pending": {"weather_received": "data_ready"},
	}}
}

func joinToolNames(defs []ToolDefinition) string {
	if len(defs) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(defs))
	for _, def := range defs {
		parts = append(parts, def.Name)
	}
	return strings.Join(parts, ", ")
}

func describeSessionRecord(record SessionRecord) string {
	switch v := record.(type) {
	case UserMessageRecord:
		return fmt.Sprintf("%s user(%q)", v.RecordID(), v.Content)
	case AssistantMessageRecord:
		return fmt.Sprintf("%s assistant(%q)", v.RecordID(), v.Content)
	case ToolCallRequestRecord:
		return fmt.Sprintf("%s tool_call_request(call_id=%s tool=%s args=%s)", v.RecordID(), v.CallID, v.ToolName, v.Arguments)
	case ToolCallResultRecord:
		return fmt.Sprintf("%s tool_call_result(call_id=%s tool=%s content=%q error=%t)", v.RecordID(), v.CallID, v.ToolName, v.Content, v.IsError)
	case StateTransitionRecord:
		return fmt.Sprintf("%s state_transition(chart=%s from=%s to=%s trigger=%s from_records=%v)", v.RecordID(), v.ChartName, v.FromState, v.ToState, v.Trigger, v.DerivedFromIDs)
	default:
		return fmt.Sprintf("unknown(%T)", record)
	}
}

func describeInferenceRecord(record InferenceRecord) string {
	return fmt.Sprintf("%s bundle=%s provider=%s model=%s git=%s pipeline=%s sources=%d steps=%d payload_lines=%d", record.InferenceID, record.BundleID, record.Provider, record.ModelName, record.GitCommit, record.AssemblyPipeline, len(record.SourceRecordIDs), len(record.Steps), len(record.Payload))
}
