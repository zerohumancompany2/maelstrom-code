package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func main() {
	history := NewSessionHistory("session-004")
	history.Append(UserMessageRecord{BaseRecord: history.NextRecord("user"), Content: "Plan a weather lookup for Paris and tell me the result."})

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
	fmt.Println("Sketch4 takeaway: bundle provenance is single-sided from ingested inputs, while emitted state transitions remain durable session facts for later turns.")
}

func buildSketchAgent() AgentRuntime {
	registry := NewToolRegistry(WeatherTool{})
	spec := AgentSpec{
		ID:      "weather-agent",
		Version: "v0.0.4-sketch",
		Model:   ModelSpec{Provider: "openrouter", Name: "z-ai/glm-4.5-air:free", Temperature: 0.7, ContextLimit: 32768},
		Context: ContextSpec{MaxHistoryItems: 8},
		Tools:   registry.Definitions(),
	}

	return AgentRuntime{
		Spec: spec,
		Assembler: Assembler{Chunks: []Chunk{
			SystemPromptChunk{},
			RecentHistoryChunk{},
			WorkflowStateChunk{Scope: "workflow", StateName: "research-plan"},
		}},
		Provider:   StubProvider{},
		Tools:      registry,
		Normalizer: ToolNormalizer{Definitions: registry.Definitions()},
		Recorder: InferenceRecorder{
			GitCommit:        "feedface-sketch4",
			AssemblyPipeline: "context/v0.4",
		},
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
			State:   history.StateIndex(),
		})
		if err != nil {
			return err
		}

		for _, record := range assembled.PendingStateWrites {
			history.Append(record)
		}

		bundle := BuildInferenceBundle(a.Spec, history.SessionID, assembled)
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
		if iteration == 3 {
			fmt.Println("[turn] sketch guard: stop after third iteration")
			return nil
		}
	}
}

func (a AgentRuntime) consumeProviderOutput(history *SessionHistory, output ProviderOutput) ([]SessionRecord, error) {
	switch v := output.(type) {
	case ProviderAssistantOutput:
		return []SessionRecord{
			AssistantMessageRecord{BaseRecord: history.NextRecord("assistant"), Content: v.Content},
		}, nil
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

		result, err := a.Tools.Execute(ToolExecutionRequest{Agent: a.Spec, Call: validated})
		if err != nil {
			return nil, err
		}

		resultRecord := ToolCallResultRecord{
			BaseRecord: history.NextRecord("tool_call_result"),
			CallID:     validated.CallID,
			ToolName:   result.ToolName,
			Content:    result.DisplayContent,
			IsError:    result.IsError,
		}

		return []SessionRecord{requestRecord, resultRecord}, nil
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
	Scope          string
	Name           string
	Content        string
	DerivedFromIDs []string
}

type SessionHistory struct {
	SessionID string
	Records   []SessionRecord
	sequence  int
}

func NewSessionHistory(sessionID string) *SessionHistory {
	return &SessionHistory{SessionID: sessionID}
}

func (h *SessionHistory) NextRecord(kind string) BaseRecord {
	h.sequence++
	return BaseRecord{ID: fmt.Sprintf("%s-r%03d", h.SessionID, h.sequence), Kind: kind}
}

func (h *SessionHistory) Append(record SessionRecord) {
	fmt.Printf("[history] append %s\n", describeSessionRecord(record))
	h.Records = append(h.Records, record)
}

func (h *SessionHistory) StateIndex() StateIndex {
	latest := map[string]StateTransitionRecord{}
	for _, record := range h.Records {
		state, ok := record.(StateTransitionRecord)
		if !ok {
			continue
		}
		latest[state.Scope+"/"+state.Name] = state
	}
	return StateIndex{Latest: latest}
}

type StateIndex struct {
	Latest map[string]StateTransitionRecord
}

func (s StateIndex) Get(scope, name string) (StateTransitionRecord, bool) {
	record, ok := s.Latest[scope+"/"+name]
	return record, ok
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
	Scope     string
	Name      string
	Content   string
	RecordIDs []string
	Step      ProvenanceStep
}

func (s StateSegment) SegmentKind() string            { return "state" }
func (s StateSegment) TokenText() string              { return s.Content }
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
	State   StateIndex
}

type AssemblyResult struct {
	Segments           []Segment
	PendingStateWrites []SessionRecord
	Steps              []ProvenanceStep
}

type Chunk interface {
	Name() string
	Build(input AssemblyInput) (ChunkResult, error)
}

type ChunkResult struct {
	Segments           []Segment
	PendingStateWrites []SessionRecord
	Steps              []ProvenanceStep
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
		result.PendingStateWrites = append(result.PendingStateWrites, chunkResult.PendingStateWrites...)
		result.Steps = append(result.Steps, chunkResult.Steps...)
	}
	fmt.Printf("[assemble] complete segments=%d state_writes=%d steps=%d\n", len(result.Segments), len(result.PendingStateWrites), len(result.Steps))
	return result, nil
}

type SystemPromptChunk struct{}

func (SystemPromptChunk) Name() string { return "system-prompt" }

func (SystemPromptChunk) Build(input AssemblyInput) (ChunkResult, error) {
	content := fmt.Sprintf("You are %s version %s on model %s. Tools: %s.", input.Agent.ID, input.Agent.Version, input.Agent.Model.Name, joinToolNames(input.Agent.Tools))
	step := ProvenanceStep{ChunkName: "system-prompt", Operation: "synthesize", OutputDescriptor: "system prompt"}
	return ChunkResult{
		Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}},
		Steps:    []ProvenanceStep{step},
	}, nil
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
			step := ProvenanceStep{ChunkName: "recent-history", Operation: "project-tool-call", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "assistant tool-call segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "assistant", Content: fmt.Sprintf("tool call %s(%s)", v.ToolName, v.Arguments), RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case ToolCallResultRecord:
			step := ProvenanceStep{ChunkName: "recent-history", Operation: "project-tool-result", InputRecordIDs: []string{v.RecordID()}, OutputDescriptor: "tool result segment"}
			result.Segments = append(result.Segments, PromptSegment{Role: "tool", Content: v.Content, RecordIDs: []string{v.RecordID()}, Step: step})
			result.Steps = append(result.Steps, step)
		case StateTransitionRecord:
			fmt.Printf("[chunk:recent-history] skip state record scope=%s name=%s\n", v.Scope, v.Name)
		}
	}
	return result, nil
}

type WorkflowStateChunk struct {
	Scope     string
	StateName string
}

func (c WorkflowStateChunk) Name() string { return "workflow-state" }

func (c WorkflowStateChunk) Build(input AssemblyInput) (ChunkResult, error) {
	candidateContent, sourceIDs := deriveWorkflowState(input.History)
	step := ProvenanceStep{ChunkName: "workflow-state", Operation: "derive-state", InputRecordIDs: sourceIDs, OutputDescriptor: c.Scope + "/" + c.StateName}
	result := ChunkResult{
		Segments: []Segment{StateSegment{Scope: c.Scope, Name: c.StateName, Content: candidateContent, RecordIDs: sourceIDs, Step: step}},
		Steps:    []ProvenanceStep{step},
	}

	current, ok := input.State.Get(c.Scope, c.StateName)
	if ok && current.Content == candidateContent {
		fmt.Printf("[chunk:workflow-state] unchanged scope=%s name=%s\n", c.Scope, c.StateName)
		return result, nil
	}

	stateRecord := StateTransitionRecord{
		BaseRecord:     input.History.NextRecord("state_transition"),
		Scope:          c.Scope,
		Name:           c.StateName,
		Content:        candidateContent,
		DerivedFromIDs: append([]string(nil), sourceIDs...),
	}
	result.PendingStateWrites = append(result.PendingStateWrites, stateRecord)
	return result, nil
}

func deriveWorkflowState(history *SessionHistory) (string, []string) {
	for i := len(history.Records) - 1; i >= 0; i-- {
		switch v := history.Records[i].(type) {
		case ToolCallResultRecord:
			return "ready to answer", []string{v.RecordID()}
		case ToolCallRequestRecord:
			return "awaiting tool result", []string{v.RecordID()}
		case UserMessageRecord:
			return "collecting external data", []string{v.RecordID()}
		}
	}
	return "idle", nil
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

func BuildInferenceBundle(agent AgentSpec, sessionID string, assembled AssemblyResult) InferenceBundle {
	fmt.Printf("[bundle] build from ingested segments=%d\n", len(assembled.Segments))
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
			rendered = append(rendered, fmt.Sprintf("state=%s/%s content=%q", v.Scope, v.Name, v.Content))
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
		BundleID:        fmt.Sprintf("%s-bundle-%03d", sessionID, len(sourceIDs)+len(assembled.Steps)),
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

type InferenceRecordRow struct {
	InferenceID      string
	BundleID         string
	SessionID        string
	AgentID          string
	AgentVersion     string
	Provider         string
	ModelName        string
	GitCommit        string
	AssemblyPipeline string
}

type InferenceSourceRow struct {
	InferenceID string
	RecordID    string
	Ordinal     int
}

type InferenceStepRow struct {
	InferenceID      string
	Ordinal          int
	ChunkName        string
	Operation        string
	InputRecordIDs   []string
	OutputDescriptor string
}

type InferencePayloadRow struct {
	InferenceID string
	Ordinal     int
	Line        string
}

type InferenceRecord struct {
	Inference InferenceRecordRow
	Sources   []InferenceSourceRow
	Steps     []InferenceStepRow
	Payload   []InferencePayloadRow
}

type InferenceRecorder struct {
	GitCommit        string
	AssemblyPipeline string
}

func (r InferenceRecorder) RecordBundle(bundle InferenceBundle) InferenceRecord {
	inferenceID := bundle.BundleID + "-record"
	fmt.Printf("[inference-record] persist inference=%s git=%s pipeline=%s\n", inferenceID, r.GitCommit, r.AssemblyPipeline)

	sources := make([]InferenceSourceRow, 0, len(bundle.SourceRecordIDs))
	for i, recordID := range bundle.SourceRecordIDs {
		sources = append(sources, InferenceSourceRow{InferenceID: inferenceID, RecordID: recordID, Ordinal: i})
	}

	steps := make([]InferenceStepRow, 0, len(bundle.Steps))
	for i, step := range bundle.Steps {
		steps = append(steps, InferenceStepRow{
			InferenceID:      inferenceID,
			Ordinal:          i,
			ChunkName:        step.ChunkName,
			Operation:        step.Operation,
			InputRecordIDs:   append([]string(nil), step.InputRecordIDs...),
			OutputDescriptor: step.OutputDescriptor,
		})
	}

	payload := make([]InferencePayloadRow, 0, len(bundle.RenderedPayload))
	for i, line := range bundle.RenderedPayload {
		payload = append(payload, InferencePayloadRow{InferenceID: inferenceID, Ordinal: i, Line: line})
	}

	return InferenceRecord{
		Inference: InferenceRecordRow{
			InferenceID:      inferenceID,
			BundleID:         bundle.BundleID,
			SessionID:        bundle.SessionID,
			AgentID:          bundle.AgentID,
			AgentVersion:     bundle.AgentVersion,
			Provider:         bundle.Provider,
			ModelName:        bundle.ModelName,
			GitCommit:        r.GitCommit,
			AssemblyPipeline: r.AssemblyPipeline,
		},
		Sources: sources,
		Steps:   steps,
		Payload: payload,
	}
}

type InferenceStore struct {
	Records []InferenceRecord
}

func NewInferenceStore() *InferenceStore {
	return &InferenceStore{}
}

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

	ready := false
	for _, line := range bundle.RenderedPayload {
		if strings.Contains(line, `state=workflow/research-plan content="ready to answer"`) {
			ready = true
		}
	}

	if !ready {
		return ProviderResponse{Outputs: []ProviderOutput{ProviderToolCallOutput{CallID: "call_1", ToolName: "weather", Arguments: json.RawMessage(`{"location":"Paris"}`)}}}, nil
	}

	return ProviderResponse{Outputs: []ProviderOutput{ProviderAssistantOutput{Content: "It is 70C, rainy, with winds out of the SSW in Paris."}}}, nil
}

type ToolDefinition struct {
	Name       string
	Parameters map[string]string
}

type Tool interface {
	Definition() ToolDefinition
	Execute(args map[string]string) (ToolExecutionResult, error)
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
	Agent AgentSpec
	Call  ValidatedToolCall
}

type ToolExecutionResult struct {
	ToolName       string
	DisplayContent string
	IsError        bool
}

type ToolExecutor interface {
	Execute(request ToolExecutionRequest) (ToolExecutionResult, error)
}

func (r ToolRegistry) Execute(request ToolExecutionRequest) (ToolExecutionResult, error) {
	tool, ok := r.tools[request.Call.Definition.Name]
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("missing tool implementation %q", request.Call.Definition.Name)
	}
	return tool.Execute(request.Call.Arguments)
}

type WeatherTool struct{}

func (WeatherTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "weather", Parameters: map[string]string{"location": "string"}}
}

func (WeatherTool) Execute(args map[string]string) (ToolExecutionResult, error) {
	return ToolExecutionResult{ToolName: "weather", DisplayContent: fmt.Sprintf("70C, rainy, winds out of SSW in %s", args["location"]), IsError: false}, nil
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
		return fmt.Sprintf("%s state_transition(scope=%s name=%s content=%q from=%v)", v.RecordID(), v.Scope, v.Name, v.Content, v.DerivedFromIDs)
	default:
		return fmt.Sprintf("unknown(%T)", record)
	}
}

func describeInferenceRecord(record InferenceRecord) string {
	return fmt.Sprintf("%s bundle=%s provider=%s model=%s git=%s pipeline=%s sources=%d steps=%d payload_lines=%d", record.Inference.InferenceID, record.Inference.BundleID, record.Inference.Provider, record.Inference.ModelName, record.Inference.GitCommit, record.Inference.AssemblyPipeline, len(record.Sources), len(record.Steps), len(record.Payload))
}
