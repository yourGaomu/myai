package message

import (
	"encoding/json"
	"strings"

	domaintool "myai/core/domain/tool"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type SyntheticReason string

const (
	SyntheticReasonRuntimeInstruction SyntheticReason = "runtime_instruction"
	SyntheticReasonProjectInstruction SyntheticReason = "project_instruction"
	SyntheticReasonSkillInstruction   SyntheticReason = "skill_instruction"
	SyntheticReasonRAGContext         SyntheticReason = "rag_context"
	SyntheticReasonSubagentResult     SyntheticReason = "subagent_result"
)

const RuntimeInstructionPrefix = "Runtime instructions for this turn:"
const RAGContextPrefix = "Retrieved knowledge context for this turn:"

type PartType string

const (
	PartText       PartType = "text"
	PartToolCall   PartType = "tool_call"
	PartToolResult PartType = "tool_result"
)

type Message struct {
	Role            Role
	Parts           []Part
	SyntheticReason SyntheticReason
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
	ToolCallID       string
	Name             string
	Content          string
	Status           domaintool.ResultStatus
	ErrorCode        string
	ErrorMessage     string
	Truncated        bool
	FullContent      string
	FullErrorMessage string
	PromptTruncated  bool
}

// Clone returns an independent copy of a message, including pointer-backed
// tool parts. Persistence and background work must not retain the live Session
// message graph.
func Clone(message Message) Message {
	cloned := message
	if len(message.Parts) == 0 {
		return cloned
	}
	cloned.Parts = make([]Part, len(message.Parts))
	for index, part := range message.Parts {
		cloned.Parts[index] = part
		if part.ToolCall != nil {
			call := *part.ToolCall
			cloned.Parts[index].ToolCall = &call
		}
		if part.ToolResult != nil {
			result := *part.ToolResult
			cloned.Parts[index].ToolResult = &result
		}
	}
	return cloned
}

func CloneAll(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}
	cloned := make([]Message, len(messages))
	for index, message := range messages {
		cloned[index] = Clone(message)
	}
	return cloned
}

func Text(role Role, text string) Message {
	return Message{
		Role: role,
		Parts: []Part{
			{Type: PartText, Text: text},
		},
	}
}

func SyntheticText(reason SyntheticReason, text string) Message {
	message := Text(RoleSystem, text)
	message.SyntheticReason = reason
	return message
}

func SyntheticUserText(reason SyntheticReason, text string) Message {
	message := Text(RoleUser, text)
	message.SyntheticReason = reason
	return message
}

func RuntimeInstruction(text string) Message {
	text = strings.TrimSpace(text)
	if text == "" {
		return Message{}
	}
	return SyntheticText(SyntheticReasonRuntimeInstruction, RuntimeInstructionPrefix+"\n"+text)
}

func RAGContext(text string) Message {
	text = strings.TrimSpace(text)
	if text == "" {
		return Message{}
	}
	return SyntheticText(SyntheticReasonRAGContext, RAGContextPrefix+"\n"+text)
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

func (m Message) Text() string {
	parts := make([]string, 0, len(m.Parts))
	for _, part := range m.Parts {
		if part.Type == PartText {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func (m Message) IsSynthetic() bool {
	return m.SyntheticReason != ""
}

func (m Message) IsSyntheticReason(reason SyntheticReason) bool {
	return reason != "" && m.SyntheticReason == reason
}

func (m Message) RuntimeInstructionText() (string, bool) {
	if !m.IsSyntheticReason(SyntheticReasonRuntimeInstruction) {
		return "", false
	}
	text := strings.TrimSpace(m.Text())
	text = strings.TrimSpace(strings.TrimPrefix(text, RuntimeInstructionPrefix))
	return text, true
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

func (r ToolResult) PromptContent() string {
	status := r.Status
	if status == "" {
		status = domaintool.ResultStatusSuccess
	}
	payload := map[string]any{"status": status}
	if r.Content != "" {
		payload["content"] = r.Content
	}
	if r.ErrorCode != "" {
		payload["error_code"] = r.ErrorCode
	}
	if r.ErrorMessage != "" {
		payload["error_message"] = r.ErrorMessage
	}
	if r.Truncated || r.PromptTruncated {
		payload["truncated"] = true
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return r.Content
	}
	return string(encoded)
}
