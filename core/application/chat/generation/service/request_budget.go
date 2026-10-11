package service

import (
	"context"
	"encoding/json"
	"fmt"

	chatcontextport "myai/core/application/chat/context/port"
	generationcommand "myai/core/application/chat/generation/command"
	"myai/core/contextmgr"
	modelport "myai/core/port/model"
)

func (s AgentLoopService) prepareSnapshot(ctx context.Context, command generationcommand.Run, tools []modelport.Tool) (contextmgr.Snapshot, error) {
	// 1. 使用本次实际发送的工具集合估算开销，再预留输出与协议框架空间。
	toolTokens, err := estimateToolTokens(tools)
	if err != nil {
		return contextmgr.Snapshot{}, err
	}
	reserved := toolTokens + max(0, command.Settings.MaxOutputTokens) + 64
	s.appendPlanContext(command.Session, command.Session.Messages)
	if err := s.compactIfNeeded(ctx, command, reserved); err != nil {
		return contextmgr.Snapshot{}, err
	}
	makeSnapshot := func() contextmgr.Snapshot {
		if provider, ok := s.Contexts.(chatcontextport.BudgetedProvider); ok {
			return provider.SnapshotWithBudget(command.Session, reserved)
		}
		return s.Contexts.Snapshot(command.Session)
	}
	snapshot := makeSnapshot()
	// 2. 压缩或预算裁剪可能移除旧计划快照，必要时重新追加最新状态。
	if s.appendPlanContext(command.Session, snapshot.Messages) {
		snapshot = makeSnapshot()
	}
	// 3. 最后按完整消息重新核算；当前回合不能再缩减时，阻止超限请求。
	tokens := max(snapshot.Info.SelectedTokens, contextmgr.EstimateMessagesTokens(snapshot.Messages))
	window := contextmgr.NormalizeWindowK(command.Session.ContextWindowK) * 1000
	if snapshot.Info.WindowK > 0 {
		window = snapshot.Info.WindowK * 1000
	}
	if tokens+reserved > window {
		return contextmgr.Snapshot{}, fmt.Errorf("current conversation turn exceeds the configured context window: messages=%d, tools=%d, output=%d, margin=64, window=%d; shorten the request or tool output, or increase the session context window", tokens, toolTokens, command.Settings.MaxOutputTokens, window)
	}
	return snapshot, nil
}

func estimateToolTokens(tools []modelport.Tool) (int, error) {
	if len(tools) == 0 {
		return 0, nil
	}
	// 1. 这里只估算工具内容，不引入具体模型 SDK 或传输结构。
	encoded, err := json.Marshal(tools)
	if err != nil {
		return 0, fmt.Errorf("estimate tool context: %w", err)
	}
	return contextmgr.EstimateTextTokens(string(encoded)), nil
}
