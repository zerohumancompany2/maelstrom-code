package main

import (
	"fmt"
	"strings"
)

func main() {
	history := NewSessionHistory()
	orchestrator := TurnOrchestrator{
		Assembler: StubAssembler{
			Chunks: []Chunk{
				SystemPromptChunk{Prompt: "You are a helpful assistant."},
				RecentHistoryChunk{Limit: 6},
				WorkflowStateChunk{Key: "research-plan"},
			},
		},
		Provider: StubProvider{},
		Tools:    StubToolExecutor{},
	}

	history.Append(UserRecord{Content: "What is the weather in Paris?"})

	if err := orchestrator.RunTurn(history); err != nil {
		fmt.Printf("turn failed: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Final durable history:")
	for i, record := range history.Records {
		fmt.Printf("  %02d. %s\n", i+1, describeRecord(record))
	}

	fmt.Println()
	fmt.Println("Sketch takeaway: durable records, assembled segments, and provider payload are separate layers.")
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
	Name    string
	Content string
}

func (s StateSegment) SegmentKind() string { return "state" }
func (s StateSegment) TokenText() string   { return s.Content }

type AssemblyResult struct {
	Segments []Segment
	Writes   []Record
	Trace    []string
}

type Chunk interface {
	Name() string
	Build(history *SessionHistory) (ChunkResult, error)
}

type ChunkResult struct {
	Segments []Segment
	Writes   []Record
	Trace    []string
}

type StubAssembler struct {
	Chunks []Chunk
}

func (a StubAssembler) Assemble(history *SessionHistory) (AssemblyResult, error) {
	fmt.Println("[assemble] start")

	result := AssemblyResult{}
	for _, chunk := range a.Chunks {
		fmt.Printf("[assemble] build chunk=%s\n", chunk.Name())
		chunkResult, err := chunk.Build(history)
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

type SystemPromptChunk struct {
	Prompt string
}

func (c SystemPromptChunk) Name() string { return "system-prompt" }

func (c SystemPromptChunk) Build(history *SessionHistory) (ChunkResult, error) {
	fmt.Printf("[chunk:%s] synthesizing system prompt from config; history_records=%d\n", c.Name(), len(history.Records))
	return ChunkResult{
		Segments: []Segment{PromptSegment{Role: "system", Content: c.Prompt}},
		Trace:    []string{"system prompt included"},
	}, nil
}

type RecentHistoryChunk struct {
	Limit int
}

func (c RecentHistoryChunk) Name() string { return "recent-history" }

func (c RecentHistoryChunk) Build(history *SessionHistory) (ChunkResult, error) {
	fmt.Printf("[chunk:%s] selecting recent durable records limit=%d\n", c.Name(), c.Limit)

	start := len(history.Records) - c.Limit
	if start < 0 {
		start = 0
	}

	segments := make([]Segment, 0, len(history.Records)-start)
	for _, record := range history.Records[start:] {
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
			fmt.Printf("[chunk:%s] skipping durable state from prompt projection scope=%s\n", c.Name(), v.Scope)
		}
	}

	return ChunkResult{
		Segments: segments,
		Trace:    []string{fmt.Sprintf("projected %d history records into prompt segments", len(segments))},
	}, nil
}

type WorkflowStateChunk struct {
	Key string
}

func (c WorkflowStateChunk) Name() string { return "workflow-state" }

func (c WorkflowStateChunk) Build(history *SessionHistory) (ChunkResult, error) {
	fmt.Printf("[chunk:%s] deriving workflow state key=%s from history\n", c.Name(), c.Key)

	state := StateRecord{
		Scope:   c.Key,
		Content: fmt.Sprintf("derived from %d durable records", len(history.Records)),
	}

	return ChunkResult{
		Segments: []Segment{StateSegment{Name: c.Key, Content: state.Content}},
		Writes:   []Record{state},
		Trace:    []string{"workflow state refreshed"},
	}, nil
}

type ProviderRequest struct {
	Lines []string
}

type Provider interface {
	Send(request ProviderRequest) (ProviderResponse, error)
}

type ProviderResponse struct {
	Records []Record
}

type StubProvider struct{}

func (StubProvider) Send(request ProviderRequest) (ProviderResponse, error) {
	fmt.Println("[provider] send request")
	for _, line := range request.Lines {
		fmt.Printf("[provider]   %s\n", line)
	}

	return ProviderResponse{
		Records: []Record{
			ToolCallRecord{CallID: "call_1", Name: "weather", Input: `{"location":"Paris"}`},
		},
	}, nil
}

type ToolExecutor interface {
	Execute(call ToolCallRecord) (ToolResultRecord, error)
}

type StubToolExecutor struct{}

func (StubToolExecutor) Execute(call ToolCallRecord) (ToolResultRecord, error) {
	fmt.Printf("[tools] execute name=%s input=%s\n", call.Name, call.Input)
	return ToolResultRecord{
		CallID: call.CallID,
		Output: "70C, rainy, winds out of SSW in Paris",
	}, nil
}

type TurnOrchestrator struct {
	Assembler StubAssembler
	Provider  Provider
	Tools     ToolExecutor
}

func (o TurnOrchestrator) RunTurn(history *SessionHistory) error {
	fmt.Println("[turn] start")

	for iteration := 1; ; iteration++ {
		fmt.Printf("[turn] iteration=%d\n", iteration)

		assembled, err := o.Assembler.Assemble(history)
		if err != nil {
			return err
		}

		for _, write := range assembled.Writes {
			history.Append(write)
		}

		request := BuildProviderRequest(assembled)
		response, err := o.Provider.Send(request)
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
			result, err := o.Tools.Execute(toolCall)
			if err != nil {
				return err
			}
			history.Append(result)
		}

		if !hasToolCalls {
			fmt.Println("[turn] complete; no tool calls returned")
			return nil
		}

		fmt.Println("[turn] tool calls detected; re-assembling context")
		if iteration == 2 {
			fmt.Println("[turn] sketch guard stopping after second iteration")
			return nil
		}
	}
}

func BuildProviderRequest(assembled AssemblyResult) ProviderRequest {
	fmt.Printf("[payload] build request from segments=%d\n", len(assembled.Segments))

	lines := make([]string, 0, len(assembled.Segments))
	for _, segment := range assembled.Segments {
		switch v := segment.(type) {
		case PromptSegment:
			lines = append(lines, fmt.Sprintf("role=%s content=%q", v.Role, v.Content))
		case StateSegment:
			lines = append(lines, fmt.Sprintf("state=%s content=%q", v.Name, v.Content))
		default:
			lines = append(lines, fmt.Sprintf("segment=%s content=%q", segment.SegmentKind(), segment.TokenText()))
		}
	}

	return ProviderRequest{Lines: lines}
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
		return fmt.Sprintf("state(scope=%s content=%q)", v.Scope, v.Content)
	default:
		return fmt.Sprintf("unknown(%s)", strings.TrimSpace(fmt.Sprintf("%T", record)))
	}
}
