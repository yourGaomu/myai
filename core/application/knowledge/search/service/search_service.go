package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	retrievalapi "myai/core/application/knowledge/retrieval/api"
	retrievalcommand "myai/core/application/knowledge/retrieval/command"
	retrievalport "myai/core/application/knowledge/retrieval/port"
	searchapi "myai/core/application/knowledge/search/api"
	searchcommand "myai/core/application/knowledge/search/command"
	searchresult "myai/core/application/knowledge/search/result"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type SearchService struct {
	Categories     knowledgeport.KnowledgeCategoryRepository
	KnowledgeBases knowledgeport.KnowledgeBaseRepository
	Profiles       knowledgeport.ProfileRepository
	Retrieval      retrievalapi.Service
	Fusion         retrievalport.RankFusion
}

var _ searchapi.Service = SearchService{}

func (service SearchService) Search(ctx context.Context, command searchcommand.Search) (searchresult.Search, error) {
	if err := service.validateDependencies(); err != nil {
		return searchresult.Search{}, err
	}
	command.Text = strings.TrimSpace(command.Text)
	if command.Text == "" {
		return searchresult.Search{}, errors.New("knowledge search text is required")
	}
	if command.TopK == 0 {
		command.TopK = 8
	}
	if command.TopK < 1 {
		return searchresult.Search{}, errors.New("knowledge search top_k must be positive")
	}

	bases, err := service.resolveKnowledgeBases(ctx, command.KnowledgeBaseIDs, command.CategoryIDs)
	if err != nil {
		return searchresult.Search{}, err
	}
	if len(bases) == 0 {
		return searchresult.Search{}, errors.New("no enabled knowledge bases matched the selected scope")
	}

	groups := groupKnowledgeBasesByProfile(bases)
	profileIDs := make([]string, 0, len(groups))
	for profileID := range groups {
		profileIDs = append(profileIDs, profileID)
	}
	sort.Strings(profileIDs)

	output := searchresult.Search{
		Hits: []domainknowledge.RetrievalHit{},
		Diagnostics: searchresult.Diagnostics{
			ResolvedKnowledgeBaseIDs: knowledgeBaseIDs(bases),
			ProfileSearches:          make([]searchresult.ProfileSearch, 0, len(profileIDs)),
			Warnings:                 []string{},
		},
	}
	hitLists := make([][]domainknowledge.RetrievalHit, 0, len(profileIDs))
	var groupErrors []error
	for _, profileID := range profileIDs {
		if err := ctx.Err(); err != nil {
			return searchresult.Search{}, err
		}
		profile, err := service.Profiles.GetIndexProfile(ctx, profileID)
		if err != nil {
			wrapped := fmt.Errorf("load index profile %q: %w", profileID, err)
			groupErrors = append(groupErrors, wrapped)
			output.Diagnostics.ProfileSearches = append(output.Diagnostics.ProfileSearches, searchresult.ProfileSearch{IndexProfileID: profileID, KnowledgeBaseIDs: knowledgeBaseIDs(groups[profileID]), Error: wrapped.Error()})
			continue
		}
		response, err := service.Retrieval.Retrieve(ctx, retrievalcommand.Retrieve{
			Query: domainknowledge.RetrievalQuery{
				Text: command.Text, KnowledgeBaseIDs: knowledgeBaseIDs(groups[profileID]),
				IndexProfileID: profile.ID, EmbeddingProfileID: profile.EmbeddingProfileID, TopK: command.TopK,
			},
			Strict: command.Strict,
		})
		profileResult := searchresult.ProfileSearch{
			IndexProfileID: profile.ID, EmbeddingProfileID: profile.EmbeddingProfileID,
			KnowledgeBaseIDs: knowledgeBaseIDs(groups[profileID]), Diagnostics: response.Diagnostics,
		}
		if err != nil {
			profileResult.Error = err.Error()
			groupErrors = append(groupErrors, fmt.Errorf("search index profile %q: %w", profile.ID, err))
		} else if len(response.Hits) > 0 {
			hitLists = append(hitLists, response.Hits)
		}
		output.Diagnostics.ProfileSearches = append(output.Diagnostics.ProfileSearches, profileResult)
	}

	output.Hits, err = service.fuseProfileHits(hitLists, command.TopK)
	if err != nil {
		return searchresult.Search{}, err
	}
	for _, groupErr := range groupErrors {
		output.Diagnostics.Warnings = append(output.Diagnostics.Warnings, groupErr.Error())
	}
	if command.Strict && len(output.Hits) == 0 && len(groupErrors) > 0 {
		return output, errors.Join(groupErrors...)
	}
	return output, nil
}

func (service SearchService) resolveKnowledgeBases(ctx context.Context, requestedBaseIDs []string, requestedCategoryIDs []string) ([]domainknowledge.KnowledgeBase, error) {
	allBases, err := service.KnowledgeBases.List(ctx, false)
	if err != nil {
		return nil, err
	}
	baseByID := make(map[string]domainknowledge.KnowledgeBase, len(allBases))
	for _, base := range allBases {
		baseByID[base.ID] = base
	}
	selected := make(map[string]domainknowledge.KnowledgeBase)
	for _, baseID := range normalizeIDs(requestedBaseIDs) {
		base, ok := baseByID[baseID]
		if !ok {
			return nil, fmt.Errorf("knowledge base %q: %w", baseID, knowledgeport.ErrNotFound)
		}
		if !base.RAGEnabled || base.Deletion.Deleted {
			return nil, fmt.Errorf("knowledge base %q is not enabled for RAG", baseID)
		}
		selected[base.ID] = base
	}

	categoryIDs := normalizeIDs(requestedCategoryIDs)
	if len(categoryIDs) > 0 {
		categories, err := service.Categories.List(ctx, false)
		if err != nil {
			return nil, err
		}
		known := make(map[string]struct{}, len(categories))
		for _, category := range categories {
			known[category.ID] = struct{}{}
		}
		includedCategories := make(map[string]struct{}, len(categoryIDs))
		for _, categoryID := range categoryIDs {
			if _, ok := known[categoryID]; !ok {
				return nil, fmt.Errorf("knowledge category %q: %w", categoryID, knowledgeport.ErrNotFound)
			}
			includedCategories[categoryID] = struct{}{}
			for _, category := range categories {
				if categoryHasAncestor(category, categoryID) {
					includedCategories[category.ID] = struct{}{}
				}
			}
		}
		for _, base := range allBases {
			if _, ok := includedCategories[base.CategoryID]; ok && base.RAGEnabled && !base.Deletion.Deleted {
				selected[base.ID] = base
			}
		}
	}

	if len(requestedBaseIDs) == 0 && len(requestedCategoryIDs) == 0 {
		for _, base := range allBases {
			if base.RAGEnabled && !base.Deletion.Deleted {
				selected[base.ID] = base
			}
		}
	}
	result := make([]domainknowledge.KnowledgeBase, 0, len(selected))
	for _, base := range selected {
		result = append(result, base)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result, nil
}

func (service SearchService) fuseProfileHits(lists [][]domainknowledge.RetrievalHit, limit int) ([]domainknowledge.RetrievalHit, error) {
	if len(lists) == 0 {
		return []domainknowledge.RetrievalHit{}, nil
	}
	if len(lists) == 1 {
		result := append([]domainknowledge.RetrievalHit(nil), lists[0]...)
		if len(result) > limit {
			result = result[:limit]
		}
		for index := range result {
			result[index].Rank = index + 1
		}
		return result, nil
	}
	rankedLists := make([]domainknowledge.RankedList, 0, len(lists))
	hitsByID := make(map[string]domainknowledge.RetrievalHit)
	for _, hits := range lists {
		if len(hits) == 0 {
			continue
		}
		items := make([]domainknowledge.RankedItem, 0, len(hits))
		for index, hit := range hits {
			rank := hit.Rank
			if rank < 1 {
				rank = index + 1
			}
			items = append(items, domainknowledge.RankedItem{ChunkID: hit.ChunkID, Rank: rank, Score: hit.Score})
			if _, exists := hitsByID[hit.ChunkID]; !exists {
				hitsByID[hit.ChunkID] = hit
			}
		}
		rankedLists = append(rankedLists, domainknowledge.RankedList{Channel: hits[0].Channel, Origin: hits[0].Origin, Items: items})
	}
	fused, err := service.Fusion.Fuse(rankedLists, limit)
	if err != nil {
		return nil, fmt.Errorf("fuse cross-profile knowledge search results: %w", err)
	}
	result := make([]domainknowledge.RetrievalHit, 0, len(fused))
	for _, item := range fused {
		hit, ok := hitsByID[item.ChunkID]
		if !ok {
			continue
		}
		hit.Score = item.Score
		hit.Rank = item.Rank
		result = append(result, hit)
	}
	return result, nil
}

func (service SearchService) validateDependencies() error {
	if service.Categories == nil || service.KnowledgeBases == nil || service.Profiles == nil {
		return errors.New("knowledge search repositories are not configured")
	}
	if service.Retrieval == nil {
		return errors.New("knowledge retrieval service is nil")
	}
	if service.Fusion == nil {
		return errors.New("knowledge search fusion is nil")
	}
	return nil
}

func groupKnowledgeBasesByProfile(bases []domainknowledge.KnowledgeBase) map[string][]domainknowledge.KnowledgeBase {
	groups := make(map[string][]domainknowledge.KnowledgeBase)
	for _, base := range bases {
		groups[base.ActiveIndexProfileID] = append(groups[base.ActiveIndexProfileID], base)
	}
	return groups
}

func knowledgeBaseIDs(bases []domainknowledge.KnowledgeBase) []string {
	ids := make([]string, 0, len(bases))
	for _, base := range bases {
		ids = append(ids, base.ID)
	}
	sort.Strings(ids)
	return ids
}

func normalizeIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func categoryHasAncestor(category domainknowledge.KnowledgeCategory, ancestorID string) bool {
	for _, value := range category.AncestorIDs {
		if value == ancestorID {
			return true
		}
	}
	return false
}
