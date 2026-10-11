package langchaingo

import (
	"context"

	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

type assistantReplayKey struct{}

type AssistantReplay struct {
	Content   string
	Reasoning string
}

func WithRequest(ctx context.Context, request modelport.GenerateRequest) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	// 1. SDK 的 MessageContent 没有推理字段，用请求级上下文送到协议适配器。
	// 1.1 数据只属于本次请求，不写入共享客户端，避免并发会话串流。
	replay := make([]AssistantReplay, 0)
	for _, message := range request.Messages {
		if message.Role == domainmessage.RoleAssistant {
			replay = append(replay, AssistantReplay{Content: message.Text(), Reasoning: message.Reasoning})
		}
	}
	return context.WithValue(ctx, assistantReplayKey{}, replay)
}

func AssistantReplayData(ctx context.Context) []AssistantReplay {
	values, _ := ctx.Value(assistantReplayKey{}).([]AssistantReplay)
	return append([]AssistantReplay(nil), values...)
}
