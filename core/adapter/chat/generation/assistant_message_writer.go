package generation

import (
	"context"

	modelport "myai/core/port/model"
	"myai/core/session"
)

type AssistantMessageWriter interface {
	SaveAssistantMessage(ctx context.Context, current *session.Session, result modelport.ChatResult) error
}
