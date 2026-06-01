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

type messageBlock struct {
	Messages []Message
	Anchor   bool
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
	blocks := blockMessages(messages)
	selected := make([]messageBlock, 0, len(blocks))
	count := 0
	for i := len(blocks) - 1; i >= 0; i-- {
		block := blocks[i]
		if count+len(block.Messages) > maxMessages && len(selected) > 0 {
			continue
		}
		selected = append(selected, block)
		count += len(block.Messages)
		if count >= maxMessages {
			break
		}
	}
	anchor := latestUserAnchor(blocks)
	if anchor != nil && !containsAnchor(selected, *anchor) {
		selected = append(selected, *anchor)
	}
	if len(selected) > 0 {
		selected = expandLeadingToolExchange(selected, blocks)
	}
	reverseBlocks(selected)
	trimmed := make([]Message, 0, maxMessages+2)
	for _, block := range selected {
		trimmed = append(trimmed, block.Messages...)
	}
	return trimmed
}

func blockMessages(messages []Message) []messageBlock {
	blocks := []messageBlock{}
	for i := 0; i < len(messages); {
		msg := messages[i]
		if msg.Role == "assistant_tool_call" {
			block := messageBlock{Messages: []Message{msg}}
			if i+1 < len(messages) && messages[i+1].Role == "tool" && messages[i+1].CallID == msg.CallID {
				block.Messages = append(block.Messages, messages[i+1])
				i += 2
			} else {
				i++
			}
			blocks = append(blocks, block)
			continue
		}
		block := messageBlock{Messages: []Message{msg}, Anchor: msg.Role == "user"}
		blocks = append(blocks, block)
		i++
	}
	return blocks
}

func hasTrailingToolResult(block messageBlock) bool {
	if len(block.Messages) == 0 {
		return false
	}
	return block.Messages[len(block.Messages)-1].Role == "tool"
}

func latestUserAnchor(blocks []messageBlock) *messageBlock {
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Anchor {
			anchor := blocks[i]
			return &anchor
		}
	}
	return nil
}

func containsAnchor(blocks []messageBlock, anchor messageBlock) bool {
	if len(anchor.Messages) == 0 {
		return false
	}
	anchorContent := anchor.Messages[0].Content
	for _, block := range blocks {
		if len(block.Messages) > 0 && block.Messages[0].Role == "user" && block.Messages[0].Content == anchorContent {
			return true
		}
	}
	return false
}

func reverseBlocks(blocks []messageBlock) {
	for i, j := 0, len(blocks)-1; i < j; i, j = i+1, j-1 {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	}
}

func expandLeadingToolExchange(selected, all []messageBlock) []messageBlock {
	first := selected[0]
	if !hasTrailingToolResult(first) || len(first.Messages) == 0 {
		return selected
	}
	callID := first.Messages[0].CallID
	if callID == "" {
		return selected
	}
	for i := len(all) - 1; i >= 0; i-- {
		block := all[i]
		if len(block.Messages) == 0 || block.Messages[0].Role != "assistant_tool_call" {
			continue
		}
		if block.Messages[0].CallID != callID {
			continue
		}
		if containsBlock(selected, block) {
			return selected
		}
		return append(selected, block)
	}
	return selected
}

func containsBlock(blocks []messageBlock, target messageBlock) bool {
	if len(target.Messages) == 0 {
		return false
	}
	for _, block := range blocks {
		if len(block.Messages) == 0 {
			continue
		}
		if block.Messages[0].Role == target.Messages[0].Role && block.Messages[0].CallID == target.Messages[0].CallID && block.Messages[0].Content == target.Messages[0].Content {
			return true
		}
	}
	return false
}

func joinList(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
