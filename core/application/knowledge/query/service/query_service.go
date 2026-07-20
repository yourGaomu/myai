package service

import (
	"context"
	"errors"
	"strings"

	queryapi "myai/core/application/knowledge/query/api"
	querycommand "myai/core/application/knowledge/query/command"
	queryresult "myai/core/application/knowledge/query/result"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

const defaultJobLimit = 200

type QueryService struct {
	KnowledgeBases     knowledgeport.KnowledgeBaseRepository
	DocumentRepository knowledgeport.DocumentRepository
	Jobs               knowledgeport.IndexingJobQueryRepository
	Profiles           knowledgeport.IndexProfileCatalog
}

var _ queryapi.Service = QueryService{}

func (service QueryService) Documents(ctx context.Context, command querycommand.Documents) (queryresult.Documents, error) {
	if service.KnowledgeBases == nil || service.DocumentRepository == nil {
		return queryresult.Documents{}, errors.New("knowledge document query repositories are not configured")
	}
	knowledgeBaseID := strings.TrimSpace(command.KnowledgeBaseID)
	if knowledgeBaseID == "" {
		return queryresult.Documents{}, errors.New("knowledge base id is required")
	}
	if _, err := service.KnowledgeBases.Get(ctx, knowledgeBaseID); err != nil {
		return queryresult.Documents{}, err
	}
	documents, err := service.DocumentRepository.ListByKnowledgeBase(ctx, knowledgeBaseID, command.IncludeDeleted)
	if err != nil {
		return queryresult.Documents{}, err
	}
	jobs := []domainknowledge.IndexingJob{}
	if service.Jobs != nil {
		limit := command.JobLimit
		if limit < 1 {
			limit = defaultJobLimit
		}
		jobs, err = service.Jobs.ListByKnowledgeBase(ctx, knowledgeBaseID, limit)
		if err != nil {
			return queryresult.Documents{}, err
		}
	}
	return queryresult.Documents{Documents: documents, Jobs: jobs}, nil
}

func (service QueryService) IndexProfiles(ctx context.Context, command querycommand.IndexProfiles) (queryresult.IndexProfiles, error) {
	if service.Profiles == nil {
		return queryresult.IndexProfiles{}, errors.New("knowledge index profile catalog is not configured")
	}
	profiles, err := service.Profiles.ListIndexProfiles(ctx, command.IncludeDeleted)
	if err != nil {
		return queryresult.IndexProfiles{}, err
	}
	return queryresult.IndexProfiles{Profiles: profiles}, nil
}
