package context

import (
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

type Payload struct {
	PayloadID string
	SessionID string
	AgentName string
	ModelRef  string
	Sections  []Section
	Messages  []Message
	Tools     []string
}

type Section struct {
	Name    string
	Role    string
	Content string
	Sticky  bool
}

type Message struct {
	Kind    string
	Role    string
	Name    string
	CallID  string
	Content string
}

type Builder struct {
	MaxMessages int
}

func (b Builder) Build(payloadID string, sectionPlan []Section, session runtime.SessionView, history *logs.SessionHistory, workflow *logs.WorkflowHistory) Payload {
	_ = workflow
	sections := append([]Section(nil), sectionPlan...)
	messages := buildTranscript(history)
	maxMessages := b.MaxMessages
	if maxMessages <= 0 {
		maxMessages = 12
	}
	messages = trimMessages(messages, maxMessages)
	return Payload{
		PayloadID: payloadID,
		SessionID: sessionID(history),
		AgentName: session.Agent.Name,
		ModelRef:  session.Agent.ProviderRef,
		Sections:  sections,
		Messages:  messages,
		Tools:     append([]string(nil), session.Agent.ToolNames...),
	}
}

func sessionID(history *logs.SessionHistory) string {
	if history == nil {
		return ""
	}
	return history.SessionID
}

func buildTranscript(history *logs.SessionHistory) []Message {
	if history == nil {
		return nil
	}
	messages := []Message{}
	for _, record := range history.Records {
		switch v := record.(type) {
		case logs.UserMessageRecord:
			messages = append(messages, Message{Kind: "message", Role: "user", Content: v.Content})
		case logs.AssistantMessageRecord:
			messages = append(messages, Message{Kind: "message", Role: "assistant", Content: v.Content})
		case logs.ToolCallRequestRecord:
			messages = append(messages, Message{Kind: "tool_call", Role: "assistant_tool_call", Name: v.ToolName, CallID: v.CallID, Content: v.Arguments})
		case logs.ToolCallResultRecord:
			messages = append(messages, Message{Kind: "tool_result", Role: "tool", Name: v.ToolName, CallID: v.CallID, Content: v.Content})
		}
	}
	return messages
}

func trimMessages(messages []Message, maxMessages int) []Message {
	if len(messages) <= maxMessages || maxMessages <= 0 {
		return append([]Message(nil), messages...)
	}
	trimmed := append([]Message(nil), messages[len(messages)-maxMessages:]...)
	for i := len(messages) - maxMessages - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			trimmed = append([]Message{messages[i]}, trimmed...)
			break
		}
	}
	if len(trimmed) >= 2 && trimmed[0].Role == "assistant_tool_call" && trimmed[1].Role == "tool" {
		return trimmed
	}
	return trimmed
}

func joinList(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
