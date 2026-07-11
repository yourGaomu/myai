package api

import (
	"context"

	messagecommand "myai/core/application/session/message/command"
	messageresult "myai/core/application/session/message/result"
)

type CommandService interface {
	AppendUserMessage(ctx context.Context, command messagecommand.AppendUserMessage) (messageresult.Command, error)
	PrepareRegeneration(ctx context.Context, command messagecommand.PrepareRegeneration) (messageresult.Command, error)
}
