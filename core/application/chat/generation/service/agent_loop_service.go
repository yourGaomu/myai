package service

import (
	"context"
	"errors"
	"strings"

	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

const DefaultMaxToolRounds = 6

type AgentLoopService struct {
	// AgentLoopService 实现“模型 -> 工具 -> 模型”的循环，直到模型不再请求工具。
	Contexts            generationport.ContextProvider
	Tools               generationport.ToolCatalog
	RuntimeInstructions generationport.RuntimeInstructionProvider
	ToolExecutor        generationport.ToolExecutor
	ToolRecords         generationport.ToolExecutionRecordSink
	MaxToolRounds       int
}

var _ generationapi.AgentRunner = AgentLoopService{}

func (s AgentLoopService) Run(ctx context.Context, command generationcommand.Run) (modelport.ChatResult, error) {
	if command.Model == nil {
		return modelport.ChatResult{}, errors.New("model is nil")
	}
	if command.Session == nil {
		return modelport.ChatResult{}, errors.New("session is nil")
	}
	if s.Contexts == nil {
		return modelport.ChatResult{}, errors.New("context provider is nil")
	}

	totalUsage := modelport.TokenUsage{}
	reasoningParts := make([]string, 0, s.maxToolRounds())
	runtimePrompt := command.RuntimePrompt

	for round := 0; round < s.maxToolRounds(); round++ {
		// 每轮都重新构建快照，因为上一轮可能追加了 tool call 和 tool result。
		result, err := command.Model.Generate(ctx, modelport.GenerateRequest{
			Messages: s.Contexts.Snapshot(command.Session, runtimePrompt).Messages,
			Tools:    s.toolsForSession(command.Session, command.ForceChatMode),
			Stream:   command.Stream,
		})
		if err != nil {
			return modelport.ChatResult{}, err
		}

		totalUsage = totalUsage.Add(result.Usage)
		reasoningParts = appendReasoningPart(reasoningParts, result.Reasoning)
		if len(result.ToolCalls) == 0 {
			// 没有工具调用表示模型已经给出最终回答，汇总所有轮次的 usage 和 reasoning 后结束。
			return finalizeResult(result, totalUsage, reasoningParts), nil
		}

		toolResult, err := s.executeTools(ctx, generationcommand.ToolExecution{
			Session: command.Session, Calls: result.ToolCalls, Stream: command.Stream, RequestID: command.RequestID,
		})
		if err != nil {
			return modelport.ChatResult{}, err
		}
		s.recordToolExecution(ctx, toolResult)
		// 工具调用与结果都进入会话，下一轮模型才能基于真实执行结果继续推理。
		command.Session.Messages = append(command.Session.Messages, domainmessage.ToolCallMessage(result.ToolCalls))
		command.Session.Messages = append(command.Session.Messages, toolResult.Messages...)
		runtimePrompt = s.runtimePrompt(ctx, command.Session, command.LatestInput, command.ForceChatMode, runtimePrompt)
	}

	// 达到工具轮数上限后进行一次无工具生成，避免模型无限调用工具。
	result, err := command.Model.Generate(ctx, modelport.GenerateRequest{
		Messages: s.Contexts.Snapshot(command.Session, runtimePrompt).Messages,
		Stream:   command.Stream,
	})
	if err != nil {
		return modelport.ChatResult{}, err
	}
	totalUsage = totalUsage.Add(result.Usage)
	reasoningParts = appendReasoningPart(reasoningParts, result.Reasoning)
	return finalizeResult(result, totalUsage, reasoningParts), nil
}

func (s AgentLoopService) maxToolRounds() int {
	if s.MaxToolRounds > 0 {
		return s.MaxToolRounds
	}
	return DefaultMaxToolRounds
}

func (s AgentLoopService) toolsForSession(current *session.Session, forceChatMode bool) []modelport.Tool {
	if s.Tools == nil {
		return nil
	}
	return s.Tools.ToolsForSession(current, forceChatMode)
}

func (s AgentLoopService) executeTools(ctx context.Context, command generationcommand.ToolExecution) (generationresult.ToolExecution, error) {
	if s.ToolExecutor == nil {
		return generationresult.ToolExecution{}, errors.New("tool executor is nil")
	}
	return s.ToolExecutor.Execute(ctx, command)
}

func (s AgentLoopService) recordToolExecution(ctx context.Context, result generationresult.ToolExecution) {
	if s.ToolRecords == nil || len(result.Entries) == 0 && len(result.Assets) == 0 {
		return
	}
	s.ToolRecords.RecordToolExecution(ctx, generationcommand.ToolExecutionRecord{Entries: result.Entries, Assets: result.Assets})
}

func (s AgentLoopService) RecordToolExecution(ctx context.Context, result generationresult.ToolExecution) {
	s.recordToolExecution(ctx, result)
}

func (s AgentLoopService) runtimePrompt(ctx context.Context, current *session.Session, input string, forceChatMode bool, fallback string) string {
	if s.RuntimeInstructions == nil {
		return fallback
	}
	return s.RuntimeInstructions.Prompt(ctx, current, input, forceChatMode)
}

func finalizeResult(result modelport.ChatResult, usage modelport.TokenUsage, reasoningParts []string) modelport.ChatResult {
	result.Usage = usage
	result.Reasoning = strings.Join(reasoningParts, "\n")
	return result
}

func appendReasoningPart(parts []string, reasoning string) []string {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return parts
	}
	return append(parts, reasoning)
}
