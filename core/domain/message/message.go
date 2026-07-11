package message

import "strings"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type PartType string

const (
	PartText       PartType = "text"
	PartToolCall   PartType = "tool_call"
	PartToolResult PartType = "tool_result"
)

type Message struct {
	Role  Role
	Parts []Part
}

type Part struct {
	Type       PartType
	Text       string
	ToolCall   *ToolCall
	ToolResult *ToolResult
}

type ToolCall struct {
	ID        string
	Type      string
	Name      string
	Arguments string
}

type ToolResult struct {
	ToolCallID string
	Name       string
	Content    string
}

func Text(role Role, text string) Message {
	return Message{
		Role: role,
		Parts: []Part{
			{Type: PartText, Text: text},
		},
	}
}

func ToolCallMessage(calls []ToolCall) Message {
	parts := make([]Part, 0, len(calls))
	for _, call := range calls {
		item := call
		parts = append(parts, Part{Type: PartToolCall, ToolCall: &item})
	}
	return Message{Role: RoleAssistant, Parts: parts}
}

func ToolResultMessage(result ToolResult) Message {
	item := result
	return Message{
		Role: RoleTool,
		Parts: []Part{
			{Type: PartToolResult, ToolResult: &item},
		},
	}
}

func CloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}
	cloned := make([]Message, len(messages))
	for index := range messages {
		cloned[index] = messages[index].Clone()
	}
	return cloned
}

func (m Message) Clone() Message {
	cloned := Message{
		Role:  m.Role,
		Parts: make([]Part, len(m.Parts)),
	}
	for index, part := range m.Parts {
		cloned.Parts[index] = part.Clone()
	}
	return cloned
}

func (p Part) Clone() Part {
	cloned := p
	if p.ToolCall != nil {
		call := *p.ToolCall
		cloned.ToolCall = &call
	}
	if p.ToolResult != nil {
		result := *p.ToolResult
		cloned.ToolResult = &result
	}
	return cloned
}

func (m Message) Text() string {
	parts := make([]string, 0, len(m.Parts))
	for _, part := range m.Parts {
		if part.Type == PartText {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func (m Message) HasToolCall() bool {
	_, ok := m.FirstToolCall()
	return ok
}

func (m Message) FirstToolCall() (ToolCall, bool) {
	for _, part := range m.Parts {
		if part.Type == PartToolCall && part.ToolCall != nil {
			return *part.ToolCall, true
		}
	}
	return ToolCall{}, false
}

func (m Message) FirstToolResult() (ToolResult, bool) {
	for _, part := range m.Parts {
		if part.Type == PartToolResult && part.ToolResult != nil {
			return *part.ToolResult, true
		}
	}
	return ToolResult{}, false
}
