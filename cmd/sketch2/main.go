package main

import (
	"fmt"
	"strings"
)

func main() {
	history := NewSessionHistory()
	history.Append(UserRecord{Content: "Plan a weather lookup for Paris and tell me the result."})

	agent := AgentRuntime{
		Spec: AgentSpec{
			Name:    "weather-agent",
			Model:   ModelSpec{Provider: "openrouter", Name: "z-ai/glm-4.5-air:free", Temperature: 0.7, ContextLimit: 32768},
			Tools:   []ToolSpec{{Name: "weather"}},
			Context: ContextSpec{MaxHistoryItems: 8, StateScopes: []string{"workflow"}},
			CognitiveStates: []CognitiveStateSpec{
				{Name: "research-plan", Purpose: "Track whether the workflow is gathering data or ready to answer."},
			},
		},
		Provider: StubProvider{},
		Tools:    StubToolExecutor{},
	}

	agent.Assembler = Assembler{Chunks: []Chunk{
		SystemPromptChunk{},
		RecentHistoryChunk{},
		WorkflowStateChunk{StateName: "research-plan"},
	}}

	if err := agent.RunTurn(history); err != nil {
		fmt.Printf("turn failed: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Final durable history:")
	for i, record := range history.Records {
		fmt.Printf("  %02d. %s\n", i+1, describeRecord(record))
	}

	fmt.Println()
	fmt.Println("Sketch2 takeaway: AgentRuntime is the composition root, while chunks only receive the focused runtime views they need.")
}

type AgentSpec struct {
	Name            string
	Model           ModelSpec
	Tools           []ToolSpec
	Context         ContextSpec
	CognitiveStates []CognitiveStateSpec
}

type ModelSpec struct {
	Provider     string
	Name         string
	Temperature  float64
	ContextLimit int
}

type ToolSpec struct {
	Name string
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
	Spec      AgentSpec
	Assembler Assembler
	Provider  Provider
	Tools     ToolExecutor
}

func (a AgentRuntime) RunTurn(history *SessionHistory) error {
	fmt.Printf("[agent] run turn agent=%s model=%s provider=%s\n", a.Spec.Name, a.Spec.Model.Name, a.Spec.Model.Provider)

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

		for _, write := range assembled.Writes {
			history.Append(write)
		}

		request := BuildProviderRequest(a.Spec, assembled)
		response, err := a.Provider.Send(request)
		if err != nil {
			return err
		}

		hasToolCalls := false
		for _, record := range response.Records {
			history.Append(record)

			toolCall, ok := record.(ToolCallRecord)
			if !ok {
				continue
			}

			hasToolCalls = true
			result, err := a.Tools.Execute(ToolExecutionInput{
				Agent: a.Spec,
				Call:  toolCall,
			})
			if err != nil {
				return err
			}
			history.Append(result)
		}

		if !hasToolCalls {
			fmt.Println("[turn] complete; provider returned no tool calls")
			return nil
		}

		fmt.Println("[turn] tool calls detected; rebuild context for same turn")
		if iteration == 2 {
			fmt.Println("[turn] sketch guard: stop after second iteration")
			return nil
		}
	}
}

type Record interface {
	RecordKind() string
}

type UserRecord struct {
	Content string
}

func (UserRecord) RecordKind() string { return "user" }

type AssistantRecord struct {
	Content string
}

func (AssistantRecord) RecordKind() string { return "assistant" }

type ToolCallRecord struct {
	CallID string
	Name   string
	Input  string
}

func (ToolCallRecord) RecordKind() string { return "tool_call" }

type ToolResultRecord struct {
	CallID string
	Output string
}

func (ToolResultRecord) RecordKind() string { return "tool_result" }

type StateRecord struct {
	Scope   string
	Name    string
	Content string
}

func (StateRecord) RecordKind() string { return "state" }

type SessionHistory struct {
	Records []Record
}

func NewSessionHistory() *SessionHistory {
	return &SessionHistory{}
}

func (h *SessionHistory) Append(record Record) {
	fmt.Printf("[history] append %s\n", describeRecord(record))
	h.Records = append(h.Records, record)
}

func (h *SessionHistory) StateIndex() StateIndex {
	latest := map[string]StateRecord{}
	for _, record := range h.Records {
		state, ok := record.(StateRecord)
		if !ok {
			continue
		}
		latest[stateKey(state.Scope, state.Name)] = state
	}
	return StateIndex{Latest: latest}
}

type StateIndex struct {
	Latest map[string]StateRecord
}

func (s StateIndex) Get(scope, name string) (StateRecord, bool) {
	record, ok := s.Latest[stateKey(scope, name)]
	return record, ok
}

type Segment interface {
	SegmentKind() string
	TokenText() string
}

type PromptSegment struct {
	Role    string
	Content string
}

func (p PromptSegment) SegmentKind() string { return "prompt" }
func (p PromptSegment) TokenText() string   { return p.Content }

type StateSegment struct {
	Scope   string
	Name    string
	Content string
}

func (s StateSegment) SegmentKind() string { return "state" }
func (s StateSegment) TokenText() string   { return s.Content }

type AssemblyInput struct {
	Agent   AgentSpec
	History *SessionHistory
	State   StateIndex
}

type AssemblyResult struct {
	Segments []Segment
	Writes   []Record
	Trace    []string
}

type Chunk interface {
	Name() string
	Build(input AssemblyInput) (ChunkResult, error)
}

type ChunkResult struct {
	Segments []Segment
	Writes   []Record
	Trace    []string
}

type Assembler struct {
	Chunks []Chunk
}

func (a Assembler) Assemble(input AssemblyInput) (AssemblyResult, error) {
	fmt.Printf("[assemble] start agent=%s chunks=%d\n", input.Agent.Name, len(a.Chunks))

	result := AssemblyResult{}
	for _, chunk := range a.Chunks {
		fmt.Printf("[assemble] build chunk=%s\n", chunk.Name())
		chunkResult, err := chunk.Build(input)
		if err != nil {
			return AssemblyResult{}, fmt.Errorf("build chunk %s: %w", chunk.Name(), err)
		}
		result.Segments = append(result.Segments, chunkResult.Segments...)
		result.Writes = append(result.Writes, chunkResult.Writes...)
		result.Trace = append(result.Trace, chunkResult.Trace...)
	}

	fmt.Printf("[assemble] complete segments=%d writes=%d\n", len(result.Segments), len(result.Writes))
	return result, nil
}

type SystemPromptChunk struct{}

func (SystemPromptChunk) Name() string { return "system-prompt" }

func (SystemPromptChunk) Build(input AssemblyInput) (ChunkResult, error) {
	fmt.Printf("[chunk:system-prompt] synthesize prompt from agent=%s\n", input.Agent.Name)
	content := fmt.Sprintf(
		"You are %s using model %s. Available tools: %s.",
		input.Agent.Name,
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
	fmt.Printf("[chunk:recent-history] project recent records limit=%d\n", limit)

	start := len(input.History.Records) - limit
	if start < 0 {
		start = 0
	}

	segments := make([]Segment, 0, len(input.History.Records)-start)
	for _, record := range input.History.Records[start:] {
		switch v := record.(type) {
		case UserRecord:
			segments = append(segments, PromptSegment{Role: "user", Content: v.Content})
		case AssistantRecord:
			segments = append(segments, PromptSegment{Role: "assistant", Content: v.Content})
		case ToolCallRecord:
			segments = append(segments, PromptSegment{Role: "assistant", Content: fmt.Sprintf("tool call %s(%s)", v.Name, v.Input)})
		case ToolResultRecord:
			segments = append(segments, PromptSegment{Role: "tool", Content: v.Output})
		case StateRecord:
			fmt.Printf("[chunk:recent-history] skip state scope=%s name=%s; not plain chat history\n", v.Scope, v.Name)
		}
	}

	return ChunkResult{
		Segments: segments,
		Trace:    []string{fmt.Sprintf("projected %d recent records", len(segments))},
	}, nil
}

type WorkflowStateChunk struct {
	StateName string
}

func (c WorkflowStateChunk) Name() string { return "workflow-state" }

func (c WorkflowStateChunk) Build(input AssemblyInput) (ChunkResult, error) {
	scope := "workflow"
	candidate := deriveWorkflowState(input.History)
	fmt.Printf("[chunk:workflow-state] derive scope=%s name=%s candidate=%q\n", scope, c.StateName, candidate.Content)

	result := ChunkResult{
		Segments: []Segment{StateSegment{Scope: scope, Name: c.StateName, Content: candidate.Content}},
		Trace:    []string{"workflow state projected into assembly"},
	}

	current, ok := input.State.Get(scope, c.StateName)
	if ok && current.Content == candidate.Content {
		fmt.Printf("[chunk:workflow-state] state unchanged; skip durable write scope=%s name=%s\n", scope, c.StateName)
		return result, nil
	}

	fmt.Printf("[chunk:workflow-state] state changed; emit durable write scope=%s name=%s\n", scope, c.StateName)
	result.Writes = append(result.Writes, StateRecord{
		Scope:   scope,
		Name:    c.StateName,
		Content: candidate.Content,
	})
	return result, nil
}

func deriveWorkflowState(history *SessionHistory) StateRecord {
	lastKind := "none"
	for i := len(history.Records) - 1; i >= 0; i-- {
		switch history.Records[i].(type) {
		case ToolResultRecord:
			lastKind = "tool_result"
			goto done
		case ToolCallRecord:
			lastKind = "tool_call"
			goto done
		case UserRecord:
			lastKind = "user"
			goto done
		}
	}

done:
	content := "idle"
	switch lastKind {
	case "user":
		content = "collecting external data"
	case "tool_call":
		content = "awaiting tool results"
	case "tool_result":
		content = "ready to answer"
	}

	return StateRecord{Scope: "workflow", Name: "research-plan", Content: content}
}

type ProviderRequest struct {
	AgentName string
	ModelName string
	Lines     []string
}

type ProviderResponse struct {
	Records []Record
}

type Provider interface {
	Send(request ProviderRequest) (ProviderResponse, error)
}

type StubProvider struct{}

func (StubProvider) Send(request ProviderRequest) (ProviderResponse, error) {
	fmt.Printf("[provider] send agent=%s model=%s\n", request.AgentName, request.ModelName)
	for _, line := range request.Lines {
		fmt.Printf("[provider]   %s\n", line)
	}

	shouldCallTool := true
	for _, line := range request.Lines {
		if strings.Contains(line, `state=workflow/research-plan content="ready to answer"`) {
			shouldCallTool = false
		}
	}

	if shouldCallTool {
		fmt.Println("[provider] returning tool call")
		return ProviderResponse{
			Records: []Record{ToolCallRecord{CallID: "call_1", Name: "weather", Input: `{"location":"Paris"}`}},
		}, nil
	}

	fmt.Println("[provider] returning assistant answer")
	return ProviderResponse{
		Records: []Record{AssistantRecord{Content: "It is 70C, rainy, with winds out of the SSW in Paris."}},
	}, nil
}

type ToolExecutionInput struct {
	Agent AgentSpec
	Call  ToolCallRecord
}

type ToolExecutor interface {
	Execute(input ToolExecutionInput) (ToolResultRecord, error)
}

type StubToolExecutor struct{}

func (StubToolExecutor) Execute(input ToolExecutionInput) (ToolResultRecord, error) {
	fmt.Printf("[tools] execute agent=%s tool=%s input=%s\n", input.Agent.Name, input.Call.Name, input.Call.Input)
	return ToolResultRecord{
		CallID: input.Call.CallID,
		Output: "70C, rainy, winds out of SSW in Paris",
	}, nil
}

func BuildProviderRequest(agent AgentSpec, assembled AssemblyResult) ProviderRequest {
	fmt.Printf("[payload] build request agent=%s segments=%d\n", agent.Name, len(assembled.Segments))

	lines := make([]string, 0, len(assembled.Segments))
	for _, segment := range assembled.Segments {
		switch v := segment.(type) {
		case PromptSegment:
			lines = append(lines, fmt.Sprintf("role=%s content=%q", v.Role, v.Content))
		case StateSegment:
			lines = append(lines, fmt.Sprintf("state=%s/%s content=%q", v.Scope, v.Name, v.Content))
		default:
			lines = append(lines, fmt.Sprintf("segment=%s content=%q", segment.SegmentKind(), segment.TokenText()))
		}
	}

	return ProviderRequest{
		AgentName: agent.Name,
		ModelName: agent.Model.Name,
		Lines:     lines,
	}
}

func stateKey(scope, name string) string {
	return scope + "/" + name
}

func joinToolNames(tools []ToolSpec) string {
	parts := make([]string, 0, len(tools))
	for _, tool := range tools {
		parts = append(parts, tool.Name)
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func describeRecord(record Record) string {
	switch v := record.(type) {
	case UserRecord:
		return fmt.Sprintf("user(%q)", v.Content)
	case AssistantRecord:
		return fmt.Sprintf("assistant(%q)", v.Content)
	case ToolCallRecord:
		return fmt.Sprintf("tool_call(id=%s name=%s input=%s)", v.CallID, v.Name, v.Input)
	case ToolResultRecord:
		return fmt.Sprintf("tool_result(id=%s output=%q)", v.CallID, v.Output)
	case StateRecord:
		return fmt.Sprintf("state(scope=%s name=%s content=%q)", v.Scope, v.Name, v.Content)
	default:
		return fmt.Sprintf("unknown(%s)", strings.TrimSpace(fmt.Sprintf("%T", record)))
	}
}
