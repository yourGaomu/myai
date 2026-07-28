package session

import (
	"strings"

	"myai/core/contextmgr"
	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	agentplan "myai/core/plan"
)

// systemPrompt 是所有会话共享的稳定前缀。Chat/Plan 切换不会修改它，动态规则由 runtime 层按轮注入。
const systemPrompt = `You are myai, a local AI coding assistant.

Work carefully inside the user's current workspace.

Core behavior:
- Be concise, practical, and honest about what you changed or could not verify.
- Prefer using tools to inspect real project files instead of guessing.
- Before editing an existing file, read or search the relevant file first.
- If a tool fails, use the error to decide the next step instead of repeating the same failing call.

Tool usage:
- Use list_files to inspect directories.
- Use read_file to inspect a known file.
- Use read_asset to download and parse files the user uploaded from mobile when their message contains an uploaded_file short_url or code.
- Use search_files to find text or files across the workspace.
- Use edit_file for small, targeted changes to existing files.
- Use write_file for new files or when replacing a whole file is clearly safer.
- Use install_skill when the user explicitly asks to install a SkillHub skill by name.
- Do not claim you have inspected an uploaded file until read_asset returns parsed content or metadata.
- Do not use shell to edit files through echo, cat, sed, powershell redirection, or similar text-writing commands when edit_file or write_file can do the job.
- Use shell only for running commands, such as tests, builds, dependency installation, project scripts, git status, gofmt, formatters, generators, or linters.
- It is acceptable to use shell for commands that intentionally rewrite files, such as gofmt -w, prettier --write, npm run format, lint --fix, or code generators, when that command is the right project workflow.
- After code changes, run a relevant verification command with shell when available, such as go test ./....

Safety:
- Do not run destructive commands.
- Do not edit files outside the workspace.
- Do not expose secrets, API keys, tokens, or credentials.
- Ask for clarification when the user's intent is risky or ambiguous.

Final response:
- Summarize the important changes.
- Mention verification results or say when verification was not run.
- Keep the response focused and easy to scan.`

const subagentSystemPrompt = `You are a myai background subagent responsible for completing one assigned task independently.

Execution behavior:
- Treat the assigned user message as a complete task and begin immediately.
- Analysis, research, explanation, and review are concrete tasks even when no code change is requested.
- Do not ask what the user wants, wait for another message, or return a generic greeting.
- When the task depends on workspace facts, use the allowed inspection tools before making claims.
- Use only the tools available to this subagent and respect its read-only or isolated-workspace limits.
- If blocked, return the specific blocker, supporting evidence, and the next actionable step.

Final response:
- Directly answer every requested point.
- Support workspace conclusions with exact file paths and relevant functions, types, or configuration keys.
- Distinguish verified facts from assumptions.
- Report changes and verification only when they actually occurred.`

type PermissionMode string
type AgentMode string
type Kind string

const (
	PermissionModeReadonly PermissionMode = "readonly"
	PermissionModeAsk      PermissionMode = "ask"
	PermissionModeFull     PermissionMode = "full"

	AgentModeChat AgentMode = "chat"
	AgentModePlan AgentMode = "plan"

	KindUser     Kind = "user"
	KindSubagent Kind = "subagent"
)

type Session struct {
	// Session 是聊天聚合根：消息、模式、上下文摘要、用量和当前 Plan 必须作为一致状态更新。
	ID                   string
	Kind                 Kind
	ParentSessionID      string
	ParentTaskID         string
	AgentDefinitionID    string
	AgentDefinitionVer   int64
	SystemInstruction    string
	AllowedTools         []string
	EnforceToolAllowlist bool
	WorkspaceRoot        string
	WorkspaceSandboxID   string
	MaxToolRounds        int
	Model                string
	AgentMode            AgentMode
	PermissionMode       PermissionMode
	ContextWindowK       int
	Summary              string
	CompactedMessages    int
	Usage                llm.TokenUsage
	LastUsage            llm.TokenUsage
	CurrentPlan          *agentplan.Plan
	RAGSettings          RAGSettings
	GenerationSettings   generation.Settings
	StyleInstruction     string
	Messages             []domainmessage.Message
}

func newSession(id, model string, agentMode AgentMode, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, ragSettings RAGSettings, generationSettings generation.Settings, styleInstruction string, messages []domainmessage.Message) *Session {
	if len(messages) == 0 {
		messages = defaultMessages()
	}
	agentMode = NormalizeAgentMode(agentMode)
	permissionMode = NormalizePermissionMode(permissionMode)
	contextWindowK = contextmgr.NormalizeWindowK(contextWindowK)

	return &Session{
		ID:                 id,
		Model:              model,
		AgentMode:          agentMode,
		PermissionMode:     permissionMode,
		ContextWindowK:     contextWindowK,
		Summary:            summary,
		CompactedMessages:  contextmgr.NormalizeCompactedMessages(messages, compactedMessages),
		Usage:              usage,
		LastUsage:          lastUsage,
		RAGSettings:        CloneRAGSettings(ragSettings),
		GenerationSettings: generation.Clone(generationSettings),
		StyleInstruction:   styleInstruction,
		Messages:           messages,
	}
}

func (s *Session) AddUserMessage(content string) {
	s.AddUserTurn("", content)
}

func (s *Session) AddUserTurn(runtimeInstruction string, content string) {
	s.AddUserTurnWithContext("", runtimeInstruction, content)
}

func (s *Session) AddUserTurnWithContext(ragContext string, runtimeInstruction string, content string) {
	if ragMessage := domainmessage.RAGContext(ragContext); ragMessage.IsSynthetic() {
		s.Messages = append(s.Messages, ragMessage)
	}
	if runtimeMessage := domainmessage.RuntimeInstruction(runtimeInstruction); runtimeMessage.IsSynthetic() {
		s.Messages = append(s.Messages, runtimeMessage)
	}
	s.Messages = append(s.Messages,
		domainmessage.Text(domainmessage.RoleUser, content),
	)
}

func (s *Session) AddAssistantMessage(content string) {
	s.Messages = append(s.Messages,
		domainmessage.Text(domainmessage.RoleAssistant, content),
	)
}

func (s *Session) AddUsage(usage llm.TokenUsage) {
	s.Usage = s.Usage.Add(usage)
	s.LastUsage = usage
}

func (s *Session) Clear() {
	// 清空会话时恢复固定 system 消息，同时移除摘要、用量和未完成计划。
	s.Messages = defaultMessages(s.SystemInstruction)
	s.Summary = ""
	s.CompactedMessages = 0
	s.Usage = llm.TokenUsage{}
	s.LastUsage = llm.TokenUsage{}
	s.CurrentPlan = nil
}

func defaultMessages(systemInstruction ...string) []domainmessage.Message {
	prompt := systemPrompt
	if len(systemInstruction) > 0 {
		if instruction := strings.TrimSpace(systemInstruction[0]); instruction != "" {
			prompt = subagentSystemPrompt + "\n\nRole-specific instructions:\n" + instruction
		}
	}
	return []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, prompt),
	}
}

func SystemPrompt() string {
	return systemPrompt
}

func SystemPromptWithInstruction(instruction string) string {
	return defaultMessages(instruction)[0].Text()
}

func NormalizeKind(kind Kind) Kind {
	if kind == KindSubagent {
		return KindSubagent
	}
	return KindUser
}

func NormalizePermissionMode(mode PermissionMode) PermissionMode {
	switch mode {
	case PermissionModeReadonly, PermissionModeFull:
		return mode
	default:
		return PermissionModeAsk
	}
}

func IsPermissionMode(mode PermissionMode) bool {
	switch mode {
	case PermissionModeReadonly, PermissionModeAsk, PermissionModeFull:
		return true
	default:
		return false
	}
}

func NormalizeAgentMode(mode AgentMode) AgentMode {
	switch mode {
	case AgentModePlan:
		return mode
	default:
		return AgentModeChat
	}
}

func IsAgentMode(mode AgentMode) bool {
	switch mode {
	case AgentModeChat, AgentModePlan:
		return true
	default:
		return false
	}
}
