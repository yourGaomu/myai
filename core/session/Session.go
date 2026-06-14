package session

import (
	"github.com/tmc/langchaingo/llms"

	"myai/core/contextmgr"
)

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
- Use search_files to find text or files across the workspace.
- Use edit_file for small, targeted changes to existing files.
- Use write_file for new files or when replacing a whole file is clearly safer.
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

type PermissionMode string

const (
	PermissionModeReadonly PermissionMode = "readonly"
	PermissionModeAsk      PermissionMode = "ask"
	PermissionModeFull     PermissionMode = "full"
)

type Session struct {
	ID                string
	Model             string
	PermissionMode    PermissionMode
	ContextWindowK    int
	Summary           string
	CompactedMessages int
	Messages          []llms.MessageContent
}

func newSession(id, model string, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, messages []llms.MessageContent) *Session {
	if len(messages) == 0 {
		messages = defaultMessages()
	}
	permissionMode = NormalizePermissionMode(permissionMode)
	contextWindowK = contextmgr.NormalizeWindowK(contextWindowK)

	return &Session{
		ID:                id,
		Model:             model,
		PermissionMode:    permissionMode,
		ContextWindowK:    contextWindowK,
		Summary:           summary,
		CompactedMessages: contextmgr.NormalizeCompactedMessages(messages, compactedMessages),
		Messages:          messages,
	}
}

func (s *Session) AddUserMessage(content string) {
	s.Messages = append(s.Messages,
		llms.TextParts(llms.ChatMessageTypeHuman, content),
	)
}

func (s *Session) AddAssistantMessage(content string) {
	s.Messages = append(s.Messages,
		llms.TextParts(llms.ChatMessageTypeAI, content),
	)
}

func (s *Session) Clear() {
	s.Messages = defaultMessages()
	s.Summary = ""
	s.CompactedMessages = 0
}

func defaultMessages() []llms.MessageContent {
	return []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
	}
}

func SystemPrompt() string {
	return systemPrompt
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
