package service

import (
	"context"
	"testing"

	generationresult "myai/core/application/chat/generation/result"
	retrievalcommand "myai/core/application/chat/retrieval/command"
	retrievalresult "myai/core/application/chat/retrieval/result"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestChatServiceDefersAutoRetrievalToKnowledgeSearchTool(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Kind: session.KindUser, Model: "model-1",
		RAGSettings: session.RAGSettings{Mode: session.RetrievalModeAuto},
	}
	preparer := &recordingChatRetrievalPreparer{}

	service := NewChatService(ChatDependencies{
		Models:           autoPlanModelRegistry{},
		SessionLoader:    autoPlanSessionLoader{current: current},
		MessageCommands:  &recordingAutoPlanMessages{current: current},
		GenerationTasks:  &recordingAutoPlanGeneration{response: generationresult.GenerationResponse{Result: modelport.ChatResult{Content: "answer"}}},
		RetrievalContext: preparer,
	})
	if _, err := service.SendMessageStreamForSession(context.Background(), current.ID, "项目架构如何实现？", modelport.ChatStreamHandler{}); err != nil {
		t.Fatalf("SendMessageStreamForSession() error = %v", err)
	}
	if preparer.calls != 0 {
		t.Fatalf("auto retrieval must be deferred to knowledge_search, Prepare calls = %d", preparer.calls)
	}
}

func TestChatServicePreparesAlwaysRetrievalBeforeGeneration(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Kind: session.KindUser, Model: "model-1",
		RAGSettings: session.RAGSettings{Mode: session.RetrievalModeAlways},
	}
	preparer := &recordingChatRetrievalPreparer{result: retrievalresult.Context{Prompt: "retrieved context"}}

	service := NewChatService(ChatDependencies{
		Models:           autoPlanModelRegistry{},
		SessionLoader:    autoPlanSessionLoader{current: current},
		MessageCommands:  &recordingAutoPlanMessages{current: current},
		GenerationTasks:  &recordingAutoPlanGeneration{response: generationresult.GenerationResponse{Result: modelport.ChatResult{Content: "answer"}}},
		RetrievalContext: preparer,
	})
	if _, err := service.SendMessageStreamForSession(context.Background(), current.ID, "项目架构如何实现？", modelport.ChatStreamHandler{}); err != nil {
		t.Fatalf("SendMessageStreamForSession() error = %v", err)
	}
	if preparer.calls != 1 {
		t.Fatalf("always retrieval must prepare exactly once, Prepare calls = %d", preparer.calls)
	}
}

type recordingChatRetrievalPreparer struct {
	calls  int
	result retrievalresult.Context
}

func (p *recordingChatRetrievalPreparer) Prepare(context.Context, retrievalcommand.Prepare) (retrievalresult.Context, error) {
	p.calls++
	return p.result, nil
}
