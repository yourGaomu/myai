package generation

import (
	"context"
	"time"

	modelport "myai/core/port/model"
	"myai/core/session"
)

type AssistantMessageWriter interface {
	SaveAssistantMessage(ctx context.Context, current *session.Session, result modelport.ChatResult, createdAt time.Time) error
}
