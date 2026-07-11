package service

import (
	"context"
	"fmt"

	compactionresult "myai/core/application/chat/compaction/result"
	chatcontextservice "myai/core/application/chat/context/service"
	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
	chatport "myai/core/application/chat/port"
	"myai/core/contextmgr"
	"myai/core/session"
)

type AssistantGenerationService struct {
	// 该服务编排一次完整回答：解析模型、构建运行时指令、压缩上下文、运行 Agent Loop、提交结果。
	Models              chatport.ModelProvider
	RuntimeInstructions generationport.RuntimeInstructionProvider
	Contexts            generationport.ContextProvider
	Compactor           generationport.AutoCompactor
	AgentRunner         generationapi.AgentRunner
	ResponseCommitter   generationapi.ResponseCommitter
	Persistence         generationport.Persistence
	OnCompactError      func(error)
}

var _ generationapi.Generator = AssistantGenerationService{}

func (s AssistantGenerationService) Generate(ctx context.Context, command generationcommand.AssistantGeneration) (generationresult.GenerationResponse, error) {
	if command.Session == nil {
		return generationresult.GenerationResponse{}, fmt.Errorf("session is nil")
	}
	if s.Models == nil {
		return generationresult.GenerationResponse{}, fmt.Errorf("model provider is nil")
	}
	if s.RuntimeInstructions == nil {
		return generationresult.GenerationResponse{}, fmt.Errorf("runtime instruction provider is nil")
	}
	if s.AgentRunner == nil {
		return generationresult.GenerationResponse{}, fmt.Errorf("agent runner is nil")
	}
	if s.ResponseCommitter == nil {
		return generationresult.GenerationResponse{}, fmt.Errorf("response committer is nil")
	}

	model := s.Models.GetModel(command.Session.Model)
	if model == nil {
		return generationresult.GenerationResponse{}, fmt.Errorf("model not found: %s", command.Session.Model)
	}
	// Plan/Skill 指令按本轮输入动态计算，不写回 Session.Messages，从而保持历史前缀稳定。
	runtimePrompt := s.RuntimeInstructions.Prompt(ctx, command.Session, command.LatestInput, command.ForceChatMode)
	compactInfo := compactionresult.CompactInfo{}
	if s.Compactor != nil {
		// 压缩发生在模型调用前，确保本轮上下文不会因超过窗口而丢失最新消息。
		info, err := s.Compactor.CompactIfNeeded(ctx, command.Session, model, runtimePrompt)
		if err != nil {
			if s.OnCompactError != nil {
				s.OnCompactError(err)
			}
		} else {
			compactInfo = info
		}
	}

	result, err := s.AgentRunner.Run(ctx, generationcommand.Run{
		Model: model, Session: command.Session, Stream: command.Stream, RuntimePrompt: runtimePrompt,
		LatestInput: command.LatestInput, RequestID: command.RequestID, ForceChatMode: command.ForceChatMode,
	})
	if err != nil {
		return generationresult.GenerationResponse{}, err
	}
	// 模型成功返回后先提交内存状态，再异步保存 assistant 消息和当前会话指针。
	commitResult, err := s.ResponseCommitter.Commit(generationcommand.Commit{
		Session: command.Session, LatestInput: command.LatestInput, Result: result, CapturePlan: command.CapturePlan,
	})
	if err != nil {
		return generationresult.GenerationResponse{}, err
	}
	if s.Persistence != nil {
		s.Persistence.PersistAssistant(command.Session, result)
		s.Persistence.PersistCurrentSession(command.Session.ID)
	}
	return generationresult.GenerationResponse{
		SessionID: command.Session.ID, Result: result, Context: s.contextInfo(command.Session, runtimePrompt),
		Compact: compactInfo, Plan: commitResult.Plan,
	}, nil
}

func (s AssistantGenerationService) contextInfo(current *session.Session, runtimePrompt string) contextmgr.Info {
	return chatcontextservice.QueryService{Contexts: s.Contexts}.InfoWithRuntimePrompt(current, runtimePrompt)
}
