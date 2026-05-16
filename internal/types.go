package internal

import "fmt"

/// MESSAGE TYPES

type SessionItem interface {
	isSessionItem() bool
}

type UserMessage struct {
	Content string
}

func (um UserMessage) isSessionItem() bool { return true }

type AssistantMessage struct {
	Content   string
	Reasoning string
}

func (am AssistantMessage) isSessionItem() bool { return true }

type SystemMessage struct{ Content string }

func (sm SystemMessage) isSessionItem() bool { return true }

type ToolCallRequestMessage struct {
	ToolCallID string
	Name       string
	Arguments  any
	Content    string
}

func (tcrm ToolCallRequestMessage) isSessionItem() bool { return true }
func (tcrm ToolCallRequestMessage) ToString() string {
	return fmt.Sprintf("%v,%v,%v", tcrm.ToolCallID, tcrm.Name, tcrm.Arguments)
}

type ToolCallResultMessage struct {
	ToolCallID string
	Content    string
}

func (tcrm ToolCallResultMessage) isSessionItem() bool { return true }

/// TOOL DEFINITIONS

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Strict      bool
}

type ToolParameters struct {
	Name        string
	Type        string
	Description string
	Required    bool
}
