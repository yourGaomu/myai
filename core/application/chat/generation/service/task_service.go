package service

import (
	"context"
	"errors"

	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
)

type TaskService struct {
	// TaskService 为每次生成建立 request_id 和工作区历史记录，再委托真正的生成服务。
	RequestIDs   generationport.RequestIDGenerator
	Recorders    generationport.TaskRecorderFactory
	Generator    generationapi.Generator
	OnSaveError  func(error)
	OnCloseError func(error)
}

var _ generationapi.TaskService = TaskService{}

func (s TaskService) Generate(ctx context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	if command.Session == nil {
		return generationresult.GenerationResponse{}, errors.New("session is nil")
	}
	if s.RequestIDs == nil {
		return generationresult.GenerationResponse{}, errors.New("request id generator is nil")
	}
	if s.Generator == nil {
		return generationresult.GenerationResponse{}, errors.New("generation handler is nil")
	}
	requestID := s.RequestIDs.NewRequestID()
	// Recorder 绑定到 context 后，文件工具可以把本次修改归入同一个可恢复检查点。
	recorder := s.newRecorder(generationcommand.TaskRecord{Title: command.Title, Reason: command.Reason, SessionID: command.Session.ID, RequestID: requestID})
	if recorder != nil {
		defer s.closeRecorder(recorder)
		defer s.saveRecorder(recorder)
		ctx = recorder.Attach(ctx)
	}
	return s.Generator.Generate(ctx, generationcommand.AssistantGeneration{
		Session: command.Session, LatestInput: command.LatestInput, RequestID: requestID,
		Stream: command.Stream, CapturePlan: command.CapturePlan, ForceChatMode: command.ForceChatMode,
	})
}

func (s TaskService) newRecorder(record generationcommand.TaskRecord) generationport.TaskRecorder {
	if s.Recorders == nil {
		return nil
	}
	return s.Recorders.NewTaskRecorder(record)
}

func (s TaskService) saveRecorder(recorder generationport.TaskRecorder) {
	if err := recorder.Save(context.Background()); err != nil && s.OnSaveError != nil {
		s.OnSaveError(err)
	}
}

func (s TaskService) closeRecorder(recorder generationport.TaskRecorder) {
	if err := recorder.Close(); err != nil && s.OnCloseError != nil {
		s.OnCloseError(err)
	}
}
