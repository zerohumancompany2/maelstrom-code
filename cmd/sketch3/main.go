package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func main() {
	history := NewSessionHistory("session-001")
	history.Append(UserMessageRecord{BaseRecord: history.NextRecord("user"), Content: "Plan a weather lookup for Paris and tell me the result."})

	inferences := NewInferenceStore()
	agent := buildSketchAgent()

	if err := agent.RunTurn(history, inferences); err != nil {
		fmt.Printf("turn failed: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Final session history:")
	for i, record := range history.Records {
		fmt.Printf("  %02d. %s\n", i+1, describeSessionRecord(record))
	}

	fmt.Println()
	fmt.Println("Durable inference records:")
	for i, record := range inferences.Records {
		fmt.Printf("  %02d. %s\n", i+1, describeInferenceRecord(record))
	}

	fmt.Println()
	fmt.Println("Sketch3 takeaway: session records, inference bundles, and inference records are separate artifacts with explicit provenance between them.")
}

func buildSketchAgent() AgentRuntime {
	registry := NewToolRegistry(WeatherTool{})
	spec := AgentSpec{
		ID:      "weather-agent",
		Version: "v0.0.3-sketch",
		Model:   ModelSpec{Provider: "openrouter", Name: "z-ai/glm-4.5-air:free", Temperature: 0.7, ContextLimit: 32768},
		Context: ContextSpec{MaxHistoryItems: 8, StateScopes: []string{"workflow"}},
		Tools:   registry.Definitions(),
		CognitiveStates: []CognitiveStateSpec{
			{Name: "research-plan", Purpose: "Track workflow readiness as tools complete."},
		},
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
		Recorder:   InferenceRecorder{GitCommit: "deadbeef-sketch3"},
		Normalizer: ToolNormalizer{Definitions: registry.Definitions()},
	}
}

type AgentSpec struct {
	ID              string
	Version         string
	Model           ModelSpec
	Context         ContextSpec
	Tools           []ToolDefinition
	CognitiveStates []CognitiveStateSpec
}

type ModelSpec struct {
	Provider     string
	Name         string
	Temperature  float64
	ContextLimit int
}

type ContextSpec struct {
	MaxHistoryItems int
	StateScopes     []string
}

type CognitiveStateSpec struct {
	Name    string
	Purpose string
}

type AgentRuntime struct {
	Spec       AgentSpec
	Assembler  Assembler
	Provider   Provider
	Tools      ToolExecutor
	Recorder   InferenceRecorder
	Normalizer ToolNormalizer
}

func (a AgentRuntime) RunTurn(history *SessionHistory, store *InferenceStore) error {
	fmt.Printf("[agent] run turn agent=%s version=%s model=%s\n", a.Spec.ID, a.Spec.Version, a.Spec.Model.Name)

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

		bundle := BuildInferenceBundle(a.Spec, history, assembled)
		record := a.Recorder.RecordBundle(bundle)
		store.Append(record)

		response, err := a.Provider.Send(bundle)
		if err != nil {
			return err
		}

		hasToolCalls := false
		for _, output := range response.Outputs {
			sessionRecords, err := a.consumeProviderOutput(history, output)
			if err != nil {
				return err
			}
			for _, sessionRecord := range sessionRecords {
				history.Append(sessionRecord)
				if _, ok := sessionRecord.(ToolCallRequestRecord); ok {
					hasToolCalls = true
				}
			}
		}

		if !hasToolCalls {
			fmt.Println("[turn] complete; no provider tool calls remain")
			return nil
		}

		fmt.Println("[turn] tool calls recorded; rebuild inference bundle")
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

		result, err := a.Tools.Execute(ToolExecutionRequest{
			Agent: a.Spec,
			Call:  validated,
		})
		if err != nil {
			return nil, err
		}

		resultRecord := ToolCallResultRecord{
			BaseRecord: history.NextRecord("tool_call_result"),
			CallID:     result.CallID,
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
	Source() string
}

type BaseRecord struct {
	ID         string
	Kind       string
	SourceName string
}

func (b BaseRecord) RecordID() string   { return b.ID }
func (b BaseRecord) RecordKind() string { return b.Kind }
func (b BaseRecord) Source() string     { return b.SourceName }

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
	Scope   string
	Name    string
	Content string
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
	return BaseRecord{
		ID:         fmt.Sprintf("%s-r%03d", h.SessionID, h.sequence),
		Kind:       kind,
		SourceName: "session",
	}
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
	Provenance() []string
}

type PromptSegment struct {
	Role      string
	Content   string
	RecordIDs []string
}

func (p PromptSegment) SegmentKind() string  { return "prompt" }
func (p PromptSegment) TokenText() string    { return p.Content }
func (p PromptSegment) Provenance() []string { return append([]string(nil), p.RecordIDs...) }

type StateSegment struct {
	Scope     string
	Name      string
	Content   string
	RecordIDs []string
}

func (s StateSegment) SegmentKind() string  { return "state" }
func (s StateSegment) TokenText() string    { return s.Content }
func (s StateSegment) Provenance() []string { return append([]string(nil), s.RecordIDs...) }

type AssemblyInput struct {
	Agent   AgentSpec
	History *SessionHistory
	State   StateIndex
}

type AssemblyResult struct {
	Segments           []Segment
	PendingStateWrites []SessionRecord
	Trace              []string
}

type Chunk interface {
	Name() string
	Build(input AssemblyInput) (ChunkResult, error)
}

type ChunkResult struct {
	Segments           []Segment
	PendingStateWrites []SessionRecord
	Trace              []string
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
		result.Trace = append(result.Trace, chunkResult.Trace...)
	}
	fmt.Printf("[assemble] complete segments=%d state_writes=%d\n", len(result.Segments), len(result.PendingStateWrites))
	return result, nil
}

type SystemPromptChunk struct{}

func (SystemPromptChunk) Name() string { return "system-prompt" }

func (SystemPromptChunk) Build(input AssemblyInput) (ChunkResult, error) {
	fmt.Printf("[chunk:system-prompt] synthesize from agent=%s\n", input.Agent.ID)
	content := fmt.Sprintf(
		"You are %s version %s on model %s. Tools: %s.",
		input.Agent.ID,
		input.Agent.Version,
		input.Agent.Model.Name,
		joinToolNames(input.Agent.Tools),
	)
	return ChunkResult{
		Segments: []Segment{PromptSegment{Role: "system", Content: content}},
		Trace:    []string{"system prompt derived from agent spec"},
	}, nil
}

type RecentHistoryChunk struct{}

func (RecentHistoryChunk) Name() string { return "recent-history" }

func (RecentHistoryChunk) Build(input AssemblyInput) (ChunkResult, error) {
	limit := input.Agent.Context.MaxHistoryItems
	fmt.Printf("[chunk:recent-history] project recent session records limit=%d\n", limit)
	start := len(input.History.Records) - limit
	if start < 0 {
		start = 0
	}

	segments := make([]Segment, 0, len(input.History.Records)-start)
	for _, record := range input.History.Records[start:] {
		switch v := record.(type) {
		case UserMessageRecord:
			segments = append(segments, PromptSegment{Role: "user", Content: v.Content, RecordIDs: []string{v.RecordID()}})
		case AssistantMessageRecord:
			segments = append(segments, PromptSegment{Role: "assistant", Content: v.Content, RecordIDs: []string{v.RecordID()}})
		case ToolCallRequestRecord:
			segments = append(segments, PromptSegment{Role: "assistant", Content: fmt.Sprintf("tool call %s(%s)", v.ToolName, v.Arguments), RecordIDs: []string{v.RecordID()}})
		case ToolCallResultRecord:
			segments = append(segments, PromptSegment{Role: "tool", Content: v.Content, RecordIDs: []string{v.RecordID()}})
		case StateTransitionRecord:
			fmt.Printf("[chunk:recent-history] skip state record scope=%s name=%s\n", v.Scope, v.Name)
		}
	}

	return ChunkResult{
		Segments: segments,
		Trace:    []string{fmt.Sprintf("projected %d recent session records", len(segments))},
	}, nil
}

type WorkflowStateChunk struct {
	Scope     string
	StateName string
}

func (c WorkflowStateChunk) Name() string { return "workflow-state" }

func (c WorkflowStateChunk) Build(input AssemblyInput) (ChunkResult, error) {
	candidateContent, sourceIDs := deriveWorkflowState(input.History)
	result := ChunkResult{
		Segments: []Segment{StateSegment{Scope: c.Scope, Name: c.StateName, Content: candidateContent, RecordIDs: sourceIDs}},
		Trace:    []string{"workflow state projected into bundle"},
	}

	current, ok := input.State.Get(c.Scope, c.StateName)
	if ok && current.Content == candidateContent {
		fmt.Printf("[chunk:workflow-state] unchanged scope=%s name=%s\n", c.Scope, c.StateName)
		return result, nil
	}

	fmt.Printf("[chunk:workflow-state] changed scope=%s name=%s -> %q\n", c.Scope, c.StateName, candidateContent)
	result.PendingStateWrites = append(result.PendingStateWrites, StateTransitionRecord{
		BaseRecord: input.History.NextRecord("state_transition"),
		Scope:      c.Scope,
		Name:       c.StateName,
		Content:    candidateContent,
	})
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
	ModelName       string
	Provider        string
	Segments        []Segment
	AssemblyTrace   []string
	SourceRecordIDs []string
	RenderedPayload []string
}

func BuildInferenceBundle(agent AgentSpec, history *SessionHistory, assembled AssemblyResult) InferenceBundle {
	fmt.Printf("[bundle] build from assembled segments=%d\n", len(assembled.Segments))
	sourceSet := map[string]struct{}{}
	rendered := make([]string, 0, len(assembled.Segments))
	for _, segment := range assembled.Segments {
		for _, id := range segment.Provenance() {
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
		BundleID:        fmt.Sprintf("%s-bundle-%03d", history.SessionID, len(history.Records)+1),
		SessionID:       history.SessionID,
		AgentID:         agent.ID,
		ModelName:       agent.Model.Name,
		Provider:        agent.Model.Provider,
		Segments:        assembled.Segments,
		AssemblyTrace:   append([]string(nil), assembled.Trace...),
		SourceRecordIDs: sourceIDs,
		RenderedPayload: rendered,
	}
}

type InferenceRecord struct {
	InferenceID     string
	SessionID       string
	AgentID         string
	AgentVersion    string
	GitCommit       string
	Provider        string
	ModelName       string
	PayloadLines    []string
	SourceRecordIDs []string
	AssemblyTrace   []string
	BundleID        string
}

type InferenceRecorder struct {
	GitCommit string
}

func (r InferenceRecorder) RecordBundle(bundle InferenceBundle) InferenceRecord {
	fmt.Printf("[inference-record] persist bundle=%s git=%s\n", bundle.BundleID, r.GitCommit)
	return InferenceRecord{
		InferenceID:     bundle.BundleID + "-record",
		SessionID:       bundle.SessionID,
		AgentID:         bundle.AgentID,
		AgentVersion:    "derived-at-runtime",
		GitCommit:       r.GitCommit,
		Provider:        bundle.Provider,
		ModelName:       bundle.ModelName,
		PayloadLines:    append([]string(nil), bundle.RenderedPayload...),
		SourceRecordIDs: append([]string(nil), bundle.SourceRecordIDs...),
		AssemblyTrace:   append([]string(nil), bundle.AssemblyTrace...),
		BundleID:        bundle.BundleID,
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

type ProviderOutput interface {
	providerOutput()
}

type ProviderAssistantOutput struct {
	Content string
}

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
	fmt.Printf("[provider] send bundle=%s agent=%s model=%s\n", bundle.BundleID, bundle.AgentID, bundle.ModelName)
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
		fmt.Println("[provider] return tool call output")
		return ProviderResponse{
			Outputs: []ProviderOutput{
				ProviderToolCallOutput{CallID: "call_1", ToolName: "weather", Arguments: json.RawMessage(`{"location":"Paris"}`)},
			},
		}, nil
	}

	fmt.Println("[provider] return assistant output")
	return ProviderResponse{
		Outputs: []ProviderOutput{ProviderAssistantOutput{Content: "It is 70C, rainy, with winds out of the SSW in Paris."}},
	}, nil
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
	registry := ToolRegistry{
		definitions: map[string]ToolDefinition{},
		tools:       map[string]Tool{},
	}
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
	matched := false
	for _, candidate := range n.Definitions {
		if candidate.Name == output.ToolName {
			def = candidate
			matched = true
			break
		}
	}
	if !matched {
		return ValidatedToolCall{}, fmt.Errorf("unknown tool %q", output.ToolName)
	}

	args := map[string]string{}
	if err := json.Unmarshal(output.Arguments, &args); err != nil {
		return ValidatedToolCall{}, fmt.Errorf("decode tool args: %w", err)
	}
	for required := range def.Parameters {
		if _, ok := args[required]; !ok {
			return ValidatedToolCall{}, fmt.Errorf("missing required arg %q for tool %q", required, def.Name)
		}
	}

	return ValidatedToolCall{
		CallID:        output.CallID,
		Definition:    def,
		Arguments:     args,
		ArgumentsJSON: append(json.RawMessage(nil), output.Arguments...),
	}, nil
}

type ToolExecutionRequest struct {
	Agent AgentSpec
	Call  ValidatedToolCall
}

type ToolExecutionResult struct {
	CallID         string
	ToolName       string
	DisplayContent string
	IsError        bool
}

type ToolExecutor interface {
	Execute(request ToolExecutionRequest) (ToolExecutionResult, error)
}

func (r ToolRegistry) Execute(request ToolExecutionRequest) (ToolExecutionResult, error) {
	fmt.Printf("[tools] execute tool=%s agent=%s\n", request.Call.Definition.Name, request.Agent.ID)
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
	location := args["location"]
	return ToolExecutionResult{
		CallID:         "call_1",
		ToolName:       "weather",
		DisplayContent: fmt.Sprintf("70C, rainy, winds out of SSW in %s", location),
		IsError:        false,
	}, nil
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
		return fmt.Sprintf("%s state_transition(scope=%s name=%s content=%q)", v.RecordID(), v.Scope, v.Name, v.Content)
	default:
		return fmt.Sprintf("unknown(%T)", record)
	}
}

func describeInferenceRecord(record InferenceRecord) string {
	return fmt.Sprintf(
		"%s bundle=%s provider=%s model=%s git=%s sources=%v",
		record.InferenceID,
		record.BundleID,
		record.Provider,
		record.ModelName,
		record.GitCommit,
		record.SourceRecordIDs,
	)
}
