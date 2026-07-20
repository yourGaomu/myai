package service

import (
	"context"
	"errors"
	"strings"

	retrievalapi "myai/core/application/chat/retrieval/api"
	retrievalcommand "myai/core/application/chat/retrieval/command"
	retrievalport "myai/core/application/chat/retrieval/port"
	retrievalresult "myai/core/application/chat/retrieval/result"
	searchapi "myai/core/application/knowledge/search/api"
	searchcommand "myai/core/application/knowledge/search/command"
	"myai/core/session"
)

type ContextService struct {
	Search    searchapi.Service
	Policy    retrievalport.TriggerPolicy
	Formatter retrievalport.ContextFormatter
}

var _ retrievalapi.ContextPreparer = ContextService{}

func (service ContextService) Prepare(ctx context.Context, command retrievalcommand.Prepare) (retrievalresult.Context, error) {
	if command.Session == nil {
		return retrievalresult.Context{}, errors.New("session is nil")
	}
	settings := session.NormalizeRAGSettings(command.Session.RAGSettings)
	input := strings.TrimSpace(command.Input)
	if input == "" || settings.Mode == session.RetrievalModeOff || settings.Mode == session.RetrievalModeManual {
		return retrievalresult.Context{Query: input}, nil
	}
	if settings.Mode == session.RetrievalModeAuto {
		if service.Policy == nil || !service.Policy.ShouldRetrieve(input) {
			return retrievalresult.Context{Query: input}, nil
		}
	}
	if service.Search == nil {
		return retrievalresult.Context{Triggered: true, Query: input}, errors.New("knowledge search service is not configured")
	}
	response, err := service.Search.Search(ctx, searchcommand.Search{
		Text: input, KnowledgeBaseIDs: settings.KnowledgeBaseIDs,
		CategoryIDs: settings.CategoryIDs, TopK: settings.TopK, Strict: false,
	})
	if err != nil {
		return retrievalresult.Context{Triggered: true, Query: input}, err
	}
	hits := make([]retrievalresult.ContextHit, 0, len(response.Hits))
	for _, hit := range response.Hits {
		hits = append(hits, retrievalresult.ContextHit{
			KnowledgeBaseID: hit.KnowledgeBaseID, DocumentID: hit.DocumentID, ChunkID: hit.ChunkID,
			Text: hit.Text, SourceName: hit.SourceName, SourceLocation: hit.SourceLocation, Rank: hit.Rank,
		})
	}
	formatter := service.Formatter
	if formatter == nil {
		formatter = ContextFormatter{}
	}
	return retrievalresult.Context{Triggered: true, Query: input, Prompt: formatter.Format(input, hits), Search: response}, nil
}
