package service

import (
	"context"
	"fmt"
	"time"

	compactionresult "myai/core/application/chat/compaction/result"
	chatcontextservice "myai/core/application/chat/context/service"
	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
	chatport "myai/core/application/chat/port"
	memoryretrievalapi "myai/core/application/memory/retrieval/api"
	memoryretrievalcommand "myai/core/application/memory/retrieval/command"
	"myai/core/contextmgr"
	generation "myai/core/domain/generation"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type AssistantGenerationService struct {
	// 该服务编排一次完整回答：解析模型、构建运行时指令、压缩上下文、运行 Agent Loop、提交结果。
	Models            chatport.ModelProvider
	ModelMetadata     modelport.MetadataProvider
	Contexts          generationport.ContextProvider
	Compactor         generationport.AutoCompactor
	AgentRunner       generationapi.AgentRunner
	ResponseCommitter generationapi.ResponseCommitter
	Persistence       generationport.Persistence
	MemoryContext     memoryretrievalapi.ContextPreparer
	Now               func() time.Time
	OnCompactError    func(error)
	OnMemoryError     func(error)
}

var _ generationapi.Generator = AssistantGenerationService{}

func (s AssistantGenerationService) Generate(ctx context.Context, command generationcommand.AssistantGeneration) (generationresult.GenerationResponse, error) {
	if command.Session == nil {
		return generationresult.GenerationResponse{}, fmt.Errorf("session is nil")
	}
	if s.Models == nil {
		return generationresult.GenerationResponse{}, fmt.Errorf("model provider is nil")
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
	settings, err := s.resolveGenerationSettings(command.Session)
	if err != nil {
		return generationresult.GenerationResponse{}, err
	}
	compactInfo := compactionresult.CompactInfo{}
	if s.Compactor != nil {
		// Runtime instructions already live in Session.Messages before the user message.
		info, err := s.Compactor.CompactIfNeeded(ctx, command.Session, model)
		if err != nil {
			if s.OnCompactError != nil {
				s.OnCompactError(err)
			}
		} else {
			compactInfo = info
		}
	}
	memoryContext := ""
	if s.MemoryContext != nil {
		prepared, memoryErr := s.MemoryContext.Prepare(ctx, memoryretrievalcommand.Prepare{
			Input: command.LatestInput, SessionID: command.Session.ID,
		})
		if memoryErr != nil {
			if s.OnMemoryError != nil {
				s.OnMemoryError(memoryErr)
			}
		} else {
			memoryContext = prepared.Prompt
		}
	}

	result, err := s.AgentRunner.Run(ctx, generationcommand.Run{
		Model: model, Session: command.Session, Stream: command.Stream,
		RequestID: command.RequestID, ForceChatMode: command.ForceChatMode,
		Settings: settings, MemoryContext: memoryContext,
		EnvironmentContext: environmentContextPrompt(s.now(), command.Session.WorkspaceRoot),
	})
	if err != nil {
		return generationresult.GenerationResponse{}, err
	}
	// 模型成功返回后先提交内存状态，再异步保存 assistant 消息和当前会话指针。
	commitResult, err := s.ResponseCommitter.Commit(generationcommand.Commit{
		Session: command.Session, LatestInput: command.LatestInput, Result: result, CapturePlan: command.CapturePlan,
		Internal: command.Internal,
	})
	if err != nil {
		return generationresult.GenerationResponse{}, err
	}
	if s.Persistence != nil {
		if !command.Internal {
			s.Persistence.PersistAssistant(command.Session, result)
		}
	}
	return generationresult.GenerationResponse{
		SessionID: command.Session.ID, Result: result, Context: s.contextInfo(command.Session),
		Compact: compactInfo, Plan: commitResult.Plan,
	}, nil
}

func (s AssistantGenerationService) resolveGenerationSettings(current *session.Session) (generation.ResolvedSettings, error) {
	modelDefaults := generation.Settings{}
	if s.ModelMetadata != nil {
		info, ok := s.ModelMetadata.GetModelInfo(current.Model)
		if !ok {
			return generation.ResolvedSettings{}, fmt.Errorf("model metadata not found: %s", current.Model)
		}
		modelDefaults = info.DefaultGenerationSettings
	}
	return generation.Resolve(modelDefaults, current.GenerationSettings)
}

func (s AssistantGenerationService) contextInfo(current *session.Session) contextmgr.Info {
	return chatcontextservice.QueryService{Contexts: s.Contexts}.Info(context.Background(), current)
}

func (s AssistantGenerationService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
