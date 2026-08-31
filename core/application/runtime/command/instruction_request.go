package command

import "myai/core/session"

type InstructionRequest struct {
	AgentMode        session.AgentMode
	ForceChatMode    bool
	ForcePlanMode    bool
	Input            string
	StyleInstruction string
}
