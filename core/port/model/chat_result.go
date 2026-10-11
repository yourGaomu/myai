package model

import domainmessage "myai/core/domain/message"

type ChatResult struct {
	// 1. Message 保存最终采样原文；Reasoning 可供界面展示本次运行的汇总。
	Message   *domainmessage.Message
	Content   string
	Reasoning string
	Usage     TokenUsage
	ToolCalls []ToolCall
}
