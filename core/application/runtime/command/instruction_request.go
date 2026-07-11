package command

import "myai/core/session"

type InstructionRequest struct {
	AgentMode     session.AgentMode
	ForceChatMode bool
	Input         string
}
