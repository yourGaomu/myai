package command

import (
	generation "myai/core/domain/generation"
	"myai/core/session"
)

type SwitchModel struct {
	SessionID string
	ModelID   string
}

type SetPermissionMode struct {
	SessionID string
	Mode      string
}

type SetAgentMode struct {
	SessionID string
	Mode      string
}

type SetContextWindow struct {
	SessionID string
	WindowK   int
}

type SetGenerationSettings struct {
	SessionID string
	Settings  generation.Settings
}

type SetStyleInstruction struct {
	SessionID   string
	Instruction string
}

type SetRAGSettings struct {
	SessionID string
	Settings  session.RAGSettings
}
