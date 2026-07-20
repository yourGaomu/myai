package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	retrievalapi "myai/core/application/knowledge/retrieval/api"
	retrievalcommand "myai/core/application/knowledge/retrieval/command"
	retrievalresult "myai/core/application/knowledge/retrieval/result"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type RetrievalService struct {
	configuration Configuration
}

var _ retrievalapi.Service = (*RetrievalService)(nil)

func New(configuration Configuration) (*RetrievalService, error) {
	configuration = configuration.normalize()
	if err := configuration.validate(); err != nil {
		return nil, err
	}
	if configuration.QualityGate == nil {
		gate, err := NewLocalQualityGate(configuration.MinLocalResults, configuration.MinLocalScore)
		if err != nil {
			return nil, fmt.Errorf("create local quality gate: %w", err)
		}
		configuration.QualityGate = gate
	}
	if configuration.Fusion == nil {
		fusion, err := NewReciprocalRankFusion(configuration.RRFK)
		if err != nil {
			return nil, fmt.Errorf("create RRF: %w", err)
		}
		configuration.Fusion = fusion
	}
	return &RetrievalService{configuration: configuration}, nil
}

func (service *RetrievalService) Retrieve(ctx context.Context, command retrievalcommand.Retrieve) (retrievalresult.Retrieve, error) {
	var output retrievalresult.Retrieve
	if service == nil {
		return output, fmt.Errorf("retrieval service is nil")
	}
	if err := command.Query.Validate(); err != nil {
		return output, fmt.Errorf("validate retrieval command: %w", err)
	}
	if command.Query.TopK > service.configuration.MaxCandidates {
		return output, fmt.Errorf("retrieval top_k %d exceeds maximum %d", command.Query.TopK, service.configuration.MaxCandidates)
	}
	indexProfile, embeddingProfile, err := service.loadProfiles(ctx, command.Query)
	if err != nil {
		return output, err
	}
	warnings := make([]string, 0)
	var provider knowledgeport.EmbeddingProvider
	if service.configuration.LocalVectors != nil || service.configuration.RemoteVectors != nil {
		provider, err = service.configuration.Embeddings.Resolve(embeddingProfile)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("resolve embedding provider for profile %q: %v", embeddingProfile.ID, err))
		}
	}
	queryVector, err := service.embedQuery(ctx, command.Query, provider)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("embed query: %v", err))
	}
	candidateLimit := service.candidateLimit(command.Query.TopK)
	localState := service.searchLocal(ctx, command.Query, indexProfile, queryVector, candidateLimit)
	warnings = append(warnings, localState.warnings...)
	localLists := localState.rankedLists()
	localFused, err := service.configuration.Fusion.Fuse(localLists, candidateLimit)
	if err != nil {
		return output, fmt.Errorf("fuse local retrieval results: %w", err)
	}
	quality, err := service.configuration.QualityGate.Evaluate(domainknowledge.LocalQualityInput{
		Healthy:        localState.vectorHealthy,
		ProfileMatched: localState.profileMatched,
		ResultCount:    len(localFused),
		TopVectorScore: localState.topVectorScore,
	})
	if err != nil {
		return output, fmt.Errorf("evaluate local retrieval quality: %w", err)
	}
	output.Diagnostics = retrievalresult.Diagnostics{
		LocalVectorHits:  len(localState.vectorHits),
		LocalKeywordHits: len(localState.keywordHits),
		LocalQuality:     quality,
		Warnings:         warnings,
	}

	lists := localLists
	remoteHits := []domainknowledge.VectorHit(nil)
	if !quality.Passed && service.configuration.RemoteVectors != nil {
		output.Diagnostics.RemoteFallback = true
		if len(queryVector) == 0 {
			output.Diagnostics.Warnings = append(output.Diagnostics.Warnings, "remote vector search skipped because query embedding failed")
		} else {
			remoteHits, err = service.searchRemote(ctx, command.Query, indexProfile, queryVector, candidateLimit)
			if err != nil {
				output.Diagnostics.Warnings = append(output.Diagnostics.Warnings, fmt.Sprintf("remote vector search: %v", err))
			} else {
				output.Diagnostics.RemoteVectorHits = len(remoteHits)
				lists = append(lists, vectorRankedList(remoteHits, domainknowledge.RetrievalOriginRemote))
			}
		}
	}
	fused, err := service.configuration.Fusion.Fuse(lists, candidateLimit)
	if err != nil {
		return output, fmt.Errorf("fuse retrieval results: %w", err)
	}
	hydrated, chunks, hydrationWarnings := service.hydrate(ctx, command.Query, indexProfile, fused)
	output.Diagnostics.Warnings = append(output.Diagnostics.Warnings, hydrationWarnings...)
	output.Hits = hydrated
	if len(remoteHits) > 0 && service.configuration.CacheRemoteResults {
		cacheCount, cacheWarnings := service.fillLocalCache(ctx, command.Query, indexProfile, embeddingProfile, provider, localState, remoteHits, hydrated, chunks)
		output.Diagnostics.CacheFillCount = cacheCount
		output.Diagnostics.Warnings = append(output.Diagnostics.Warnings, cacheWarnings...)
	}
	if command.Strict && len(output.Hits) == 0 && len(output.Diagnostics.Warnings) > 0 {
		return output, errors.New(strings.Join(output.Diagnostics.Warnings, "; "))
	}
	return output, nil
}

func (service *RetrievalService) loadProfiles(ctx context.Context, query domainknowledge.RetrievalQuery) (domainknowledge.IndexProfile, domainknowledge.EmbeddingProfile, error) {
	indexProfile, err := service.configuration.Profiles.GetIndexProfile(ctx, query.IndexProfileID)
	if err != nil {
		return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("load index profile %q: %w", query.IndexProfileID, err)
	}
	if err := indexProfile.Validate(); err != nil {
		return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("validate index profile %q: %w", indexProfile.ID, err)
	}
	if indexProfile.Status != domainknowledge.IndexProfileStatusActive || indexProfile.Deletion.Deleted {
		return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("index profile %q is not active", indexProfile.ID)
	}
	if indexProfile.EmbeddingProfileID != query.EmbeddingProfileID {
		return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("index profile %q uses embedding profile %q, not %q", indexProfile.ID, indexProfile.EmbeddingProfileID, query.EmbeddingProfileID)
	}
	embeddingProfile, err := service.configuration.Profiles.GetEmbeddingProfile(ctx, query.EmbeddingProfileID)
	if err != nil {
		return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("load embedding profile %q: %w", query.EmbeddingProfileID, err)
	}
	if err := embeddingProfile.Validate(); err != nil {
		return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("validate embedding profile %q: %w", embeddingProfile.ID, err)
	}
	if embeddingProfile.Deletion.Deleted {
		return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("embedding profile %q is deleted", embeddingProfile.ID)
	}
	for _, knowledgeBaseID := range query.KnowledgeBaseIDs {
		knowledgeBase, err := service.configuration.KnowledgeBases.Get(ctx, knowledgeBaseID)
		if err != nil {
			return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("load knowledge base %q: %w", knowledgeBaseID, err)
		}
		if err := knowledgeBase.Validate(); err != nil {
			return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("validate knowledge base %q: %w", knowledgeBase.ID, err)
		}
		if knowledgeBase.Deletion.Deleted || !knowledgeBase.RAGEnabled {
			return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("knowledge base %q is not enabled for RAG", knowledgeBase.ID)
		}
		if knowledgeBase.ActiveIndexProfileID != query.IndexProfileID {
			return domainknowledge.IndexProfile{}, domainknowledge.EmbeddingProfile{}, fmt.Errorf("knowledge base %q uses index profile %q, not %q", knowledgeBase.ID, knowledgeBase.ActiveIndexProfileID, query.IndexProfileID)
		}
	}
	return indexProfile, embeddingProfile, nil
}

func (service *RetrievalService) embedQuery(ctx context.Context, query domainknowledge.RetrievalQuery, provider knowledgeport.EmbeddingProvider) ([]float32, error) {
	if provider == nil {
		return nil, nil
	}
	result, err := provider.EmbedQuery(ctx, domainknowledge.EmbedRequest{
		EmbeddingProfileID: query.EmbeddingProfileID,
		Inputs:             []domainknowledge.EmbedInput{{ID: "retrieval-query", Text: query.Text}},
	})
	if err != nil {
		return nil, err
	}
	if len(result.Outputs) != 1 {
		return nil, fmt.Errorf("embedding provider returned %d query vectors, expected 1", len(result.Outputs))
	}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("validate query embedding: %w", err)
	}
	if result.EmbeddingProfileID != query.EmbeddingProfileID {
		return nil, fmt.Errorf("query embedding profile %q does not match requested profile %q", result.EmbeddingProfileID, query.EmbeddingProfileID)
	}
	if result.Outputs[0].ID != "retrieval-query" {
		return nil, fmt.Errorf("query embedding output id %q does not match request", result.Outputs[0].ID)
	}
	return append([]float32(nil), result.Outputs[0].Vector...), nil
}

func (service *RetrievalService) candidateLimit(topK int) int {
	limit := service.configuration.MaxCandidates
	if topK <= math.MaxInt/service.configuration.CandidateMultiplier {
		limit = topK * service.configuration.CandidateMultiplier
	}
	if limit > service.configuration.MaxCandidates {
		limit = service.configuration.MaxCandidates
	}
	return limit
}

type localSearchState struct {
	vectorHits     []domainknowledge.VectorHit
	keywordHits    []domainknowledge.KeywordHit
	vectorHealthy  bool
	profileMatched bool
	topVectorScore float64
	warnings       []string
}

func (state localSearchState) rankedLists() []domainknowledge.RankedList {
	lists := make([]domainknowledge.RankedList, 0, 2)
	if len(state.vectorHits) > 0 {
		lists = append(lists, vectorRankedList(state.vectorHits, domainknowledge.RetrievalOriginLocal))
	}
	if len(state.keywordHits) > 0 {
		items := make([]domainknowledge.RankedItem, 0, len(state.keywordHits))
		for _, hit := range state.keywordHits {
			items = append(items, domainknowledge.RankedItem{ChunkID: hit.ChunkID, Rank: hit.Rank, Score: hit.Score})
		}
		lists = append(lists, domainknowledge.RankedList{
			Channel: domainknowledge.RetrievalChannelKeyword,
			Origin:  domainknowledge.RetrievalOriginLocal,
			Items:   items,
		})
	}
	return lists
}

func vectorRankedList(hits []domainknowledge.VectorHit, origin domainknowledge.RetrievalOrigin) domainknowledge.RankedList {
	items := make([]domainknowledge.RankedItem, 0, len(hits))
	for _, hit := range hits {
		items = append(items, domainknowledge.RankedItem{ChunkID: hit.ChunkID, Rank: hit.Rank, Score: hit.Score})
	}
	return domainknowledge.RankedList{Channel: domainknowledge.RetrievalChannelVector, Origin: origin, Items: items}
}

func (service *RetrievalService) searchLocal(ctx context.Context, query domainknowledge.RetrievalQuery, indexProfile domainknowledge.IndexProfile, queryVector []float32, limit int) localSearchState {
	state := localSearchState{}
	type vectorResult struct {
		hits           []domainknowledge.VectorHit
		healthy        bool
		profileMatched bool
		topScore       float64
		warnings       []string
	}
	type keywordResult struct {
		hits     []domainknowledge.KeywordHit
		warnings []string
	}
	var vectorResults chan vectorResult
	if service.configuration.LocalVectors != nil {
		vectorResults = make(chan vectorResult, 1)
		go func() {
			result := vectorResult{}
			if err := service.configuration.LocalVectors.Health(ctx); err != nil {
				result.warnings = append(result.warnings, fmt.Sprintf("local vector health: %v", err))
				vectorResults <- result
				return
			}
			result.healthy = true
			if len(queryVector) == 0 {
				result.warnings = append(result.warnings, "local vector search skipped because query embedding failed")
				vectorResults <- result
				return
			}
			hits, err := service.configuration.LocalVectors.Search(ctx, domainknowledge.VectorQuery{
				EmbeddingProfileID: query.EmbeddingProfileID,
				KnowledgeBaseIDs:   query.KnowledgeBaseIDs,
				DistanceMetricID:   indexProfile.DistanceMetricID,
				Vector:             queryVector,
				TopK:               limit,
			})
			if err != nil {
				result.warnings = append(result.warnings, fmt.Sprintf("local vector search: %v", err))
				vectorResults <- result
				return
			}
			result.profileMatched = true
			result.hits = hits
			if len(hits) > 0 {
				result.topScore = hits[0].Score
				for _, hit := range hits[1:] {
					if hit.Score > result.topScore {
						result.topScore = hit.Score
					}
				}
			}
			vectorResults <- result
		}()
	}
	var keywordResults chan keywordResult
	if service.configuration.LocalKeywords != nil {
		keywordResults = make(chan keywordResult, 1)
		go func() {
			result := keywordResult{}
			hits, err := service.configuration.LocalKeywords.Search(ctx, domainknowledge.KeywordQuery{
				Text:             query.Text,
				KnowledgeBaseIDs: query.KnowledgeBaseIDs,
				TopK:             limit,
			})
			if err != nil {
				result.warnings = append(result.warnings, fmt.Sprintf("local keyword search: %v", err))
				keywordResults <- result
				return
			}
			result.hits = hits
			keywordResults <- result
		}()
	}
	if vectorResults != nil {
		result := <-vectorResults
		state.vectorHits = result.hits
		state.vectorHealthy = result.healthy
		state.profileMatched = result.profileMatched
		state.topVectorScore = result.topScore
		state.warnings = append(state.warnings, result.warnings...)
	}
	if keywordResults != nil {
		result := <-keywordResults
		state.keywordHits = result.hits
		state.warnings = append(state.warnings, result.warnings...)
	}
	return state
}

func (service *RetrievalService) searchRemote(ctx context.Context, query domainknowledge.RetrievalQuery, indexProfile domainknowledge.IndexProfile, queryVector []float32, limit int) ([]domainknowledge.VectorHit, error) {
	return service.configuration.RemoteVectors.Search(ctx, domainknowledge.VectorQuery{
		EmbeddingProfileID: query.EmbeddingProfileID,
		KnowledgeBaseIDs:   query.KnowledgeBaseIDs,
		DistanceMetricID:   indexProfile.DistanceMetricID,
		Options:            indexProfile.VectorIndexOptions,
		Vector:             queryVector,
		TopK:               limit,
	})
}
