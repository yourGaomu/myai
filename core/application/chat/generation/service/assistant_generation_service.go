package service

import (
	"context"
	"errors"
	"fmt"

	compactionresult "myai/core/application/chat/compaction/result"
	chatcontextservice "myai/core/application/chat/context/service"
	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
	chatport "myai/core/application/chat/port"
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
	OnCompactError    func(error)
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

	command.Session = session.Clone(command.Session)
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
	modelWindowTokens := 0
	if s.ModelMetadata != nil {
		if info, ok := s.ModelMetadata.GetModelInfo(command.Session.Model); ok {
			modelWindowTokens = info.ContextWindowTokens
		}
	}
	result, err := s.AgentRunner.Run(ctx, generationcommand.Run{
		Model: model, Session: command.Session, Stream: command.Stream,
		RequestID: command.RequestID, ForceChatMode: command.ForceChatMode, Internal: command.Internal, ModelWindowTokens: modelWindowTokens,
		Settings: settings,
	})
	if err != nil {
		return generationresult.GenerationResponse{}, err
	}
	// Preserve the completed answer in memory before accepting its durable write.
	commitResult, err := s.ResponseCommitter.Commit(generationcommand.Commit{
		Session: command.Session, LatestInput: command.LatestInput, Result: result, CapturePlan: command.CapturePlan,
		Internal: command.Internal,
	})
	if err != nil && commitResult.Session == nil {
		return generationresult.GenerationResponse{SessionID: command.Session.ID, Result: result}, err
	}
	if commitResult.Session != nil {
		command.Session = commitResult.Session
	}
	var messageID string
	if committed := commitResult.AssistantMessage; committed != nil && !command.Internal {
		// Persist the exact message assigned by the transcript store. The
		// result's display reasoning can include several earlier samples.
		persistedResult := result
		persistedResult.Message = committed
		if durable, ok := s.Persistence.(generationport.DurablePersistence); ok {
			if persistErr := durable.AcceptAssistant(ctx, command.Session, persistedResult); persistErr != nil {
				err = errors.Join(err, fmt.Errorf("accept assistant persistence: %w", persistErr))
			}
		} else if s.Persistence != nil {
			s.Persistence.PersistAssistant(command.Session, persistedResult)
		}
		if err == nil {
			messageID = committed.AssistantReplyRecordID()
		}
	}
	return generationresult.GenerationResponse{
		SessionID: command.Session.ID, MessageID: messageID, Result: result, Context: s.contextInfo(command.Session),
		Compact: compactInfo, Plan: commitResult.Plan,
	}, err
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
