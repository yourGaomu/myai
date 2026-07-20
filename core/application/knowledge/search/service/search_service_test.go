package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	retrievalcommand "myai/core/application/knowledge/retrieval/command"
	retrievalresult "myai/core/application/knowledge/retrieval/result"
	retrievalservice "myai/core/application/knowledge/retrieval/service"
	searchcommand "myai/core/application/knowledge/search/command"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

func TestSearchServiceResolvesCategoryDescendants(t *testing.T) {
	service, retrieval := newSearchTestService(
		[]domainknowledge.KnowledgeCategory{
			{ID: "category-root", Name: "Root"},
			{ID: "category-child", Name: "Child", ParentID: "category-root", AncestorIDs: []string{"category-root"}},
			{ID: "category-other", Name: "Other"},
		},
		[]domainknowledge.KnowledgeBase{
			testSearchBase("kb-root", "category-root", "profile-1", true),
			testSearchBase("kb-child", "category-child", "profile-1", true),
			testSearchBase("kb-other", "category-other", "profile-1", true),
		},
		map[string][]domainknowledge.RetrievalHit{
			"profile-1": {testSearchHit("chunk-1", "kb-root", 1)},
		},
	)

	result, err := service.Search(context.Background(), searchcommand.Search{
		Text: "project architecture", CategoryIDs: []string{"category-root"}, TopK: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(retrieval.commands) != 1 {
		t.Fatalf("expected one profile search, got %d", len(retrieval.commands))
	}
	want := []string{"kb-child", "kb-root"}
	if got := retrieval.commands[0].Query.KnowledgeBaseIDs; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected descendant scope: got %#v want %#v", got, want)
	}
	if !reflect.DeepEqual(result.Diagnostics.ResolvedKnowledgeBaseIDs, want) {
		t.Fatalf("unexpected resolved scope diagnostics: %#v", result.Diagnostics.ResolvedKnowledgeBaseIDs)
	}
}

func TestSearchServiceUsesAllEnabledKnowledgeBasesForEmptyScope(t *testing.T) {
	service, retrieval := newSearchTestService(
		nil,
		[]domainknowledge.KnowledgeBase{
			testSearchBase("kb-1", "", "profile-1", true),
			testSearchBase("kb-2", "", "profile-1", true),
			testSearchBase("kb-disabled", "", "profile-1", false),
		},
		map[string][]domainknowledge.RetrievalHit{"profile-1": {testSearchHit("chunk-1", "kb-1", 1)}},
	)

	if _, err := service.Search(context.Background(), searchcommand.Search{Text: "project", TopK: 8}); err != nil {
		t.Fatal(err)
	}
	want := []string{"kb-1", "kb-2"}
	if got := retrieval.commands[0].Query.KnowledgeBaseIDs; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected empty-scope resolution: got %#v want %#v", got, want)
	}
}

func TestSearchServiceGroupsProfilesAndFusesTheirHits(t *testing.T) {
	service, retrieval := newSearchTestService(
		nil,
		[]domainknowledge.KnowledgeBase{
			testSearchBase("kb-1", "", "profile-1", true),
			testSearchBase("kb-2", "", "profile-2", true),
		},
		map[string][]domainknowledge.RetrievalHit{
			"profile-1": {
				testSearchHit("chunk-common", "kb-1", 1),
				testSearchHit("chunk-profile-1", "kb-1", 2),
			},
			"profile-2": {
				testSearchHit("chunk-common", "kb-2", 1),
				testSearchHit("chunk-profile-2", "kb-2", 2),
			},
		},
	)

	result, err := service.Search(context.Background(), searchcommand.Search{Text: "project", TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(retrieval.commands) != 2 {
		t.Fatalf("expected two profile searches, got %d", len(retrieval.commands))
	}
	if len(result.Hits) != 3 || result.Hits[0].ChunkID != "chunk-common" {
		t.Fatalf("unexpected cross-profile fusion: %#v", result.Hits)
	}
}

func newSearchTestService(categories []domainknowledge.KnowledgeCategory, bases []domainknowledge.KnowledgeBase, hits map[string][]domainknowledge.RetrievalHit) (SearchService, *searchRetrievalService) {
	profiles := make(map[string]domainknowledge.IndexProfile)
	for _, base := range bases {
		profiles[base.ActiveIndexProfileID] = domainknowledge.IndexProfile{
			ID: base.ActiveIndexProfileID, Name: base.ActiveIndexProfileID,
			ParsingProfileID: "parsing-1", ChunkingProfileID: "chunking-1",
			EmbeddingProfileID: "embedding-" + base.ActiveIndexProfileID,
			DistanceMetricID:   "cosine", Status: domainknowledge.IndexProfileStatusActive,
		}
	}
	fusion, err := retrievalservice.NewReciprocalRankFusion(60)
	if err != nil {
		panic(err)
	}
	retrieval := &searchRetrievalService{hits: hits}
	return SearchService{
		Categories:     searchCategoryRepository{items: categories},
		KnowledgeBases: searchKnowledgeBaseRepository{items: bases},
		Profiles:       searchProfileRepository{profiles: profiles},
		Retrieval:      retrieval,
		Fusion:         fusion,
	}, retrieval
}

func testSearchBase(id string, categoryID string, profileID string, enabled bool) domainknowledge.KnowledgeBase {
	return domainknowledge.KnowledgeBase{
		ID: id, Name: id, CategoryID: categoryID, RAGEnabled: enabled, ActiveIndexProfileID: profileID,
	}
}

func testSearchHit(chunkID string, knowledgeBaseID string, rank int) domainknowledge.RetrievalHit {
	return domainknowledge.RetrievalHit{
		ChunkID: chunkID, KnowledgeBaseID: knowledgeBaseID, DocumentID: "document-1",
		Text: chunkID, Score: 1 / float64(rank), Rank: rank,
		Channel: domainknowledge.RetrievalChannelVector, Origin: domainknowledge.RetrievalOriginLocal,
	}
}

type searchRetrievalService struct {
	hits     map[string][]domainknowledge.RetrievalHit
	commands []retrievalcommand.Retrieve
}

func (service *searchRetrievalService) Retrieve(_ context.Context, command retrievalcommand.Retrieve) (retrievalresult.Retrieve, error) {
	service.commands = append(service.commands, command)
	hits := append([]domainknowledge.RetrievalHit(nil), service.hits[command.Query.IndexProfileID]...)
	return retrievalresult.Retrieve{Hits: hits}, nil
}

type searchCategoryRepository struct {
	items []domainknowledge.KnowledgeCategory
}

func (repository searchCategoryRepository) Get(_ context.Context, id string) (domainknowledge.KnowledgeCategory, error) {
	for _, category := range repository.items {
		if category.ID == id && !category.Deletion.Deleted {
			return category, nil
		}
	}
	return domainknowledge.KnowledgeCategory{}, knowledgeport.ErrNotFound
}

func (repository searchCategoryRepository) List(_ context.Context, includeDeleted bool) ([]domainknowledge.KnowledgeCategory, error) {
	result := make([]domainknowledge.KnowledgeCategory, 0, len(repository.items))
	for _, category := range repository.items {
		if includeDeleted || !category.Deletion.Deleted {
			result = append(result, category)
		}
	}
	return result, nil
}

func (searchCategoryRepository) Save(context.Context, domainknowledge.KnowledgeCategory) error {
	return nil
}

func (searchCategoryRepository) MarkDeleted(context.Context, string, time.Time, string) error {
	return nil
}

type searchKnowledgeBaseRepository struct {
	items []domainknowledge.KnowledgeBase
}

func (repository searchKnowledgeBaseRepository) Get(_ context.Context, id string) (domainknowledge.KnowledgeBase, error) {
	for _, base := range repository.items {
		if base.ID == id && !base.Deletion.Deleted {
			return base, nil
		}
	}
	return domainknowledge.KnowledgeBase{}, knowledgeport.ErrNotFound
}

func (repository searchKnowledgeBaseRepository) List(_ context.Context, includeDeleted bool) ([]domainknowledge.KnowledgeBase, error) {
	result := make([]domainknowledge.KnowledgeBase, 0, len(repository.items))
	for _, base := range repository.items {
		if includeDeleted || !base.Deletion.Deleted {
			result = append(result, base)
		}
	}
	return result, nil
}

func (searchKnowledgeBaseRepository) Save(context.Context, domainknowledge.KnowledgeBase) error {
	return nil
}

func (searchKnowledgeBaseRepository) MarkDeleted(context.Context, string, time.Time, string, int64) error {
	return nil
}

type searchProfileRepository struct {
	profiles map[string]domainknowledge.IndexProfile
}

func (searchProfileRepository) GetParsingProfile(context.Context, string) (domainknowledge.ParsingProfile, error) {
	return domainknowledge.ParsingProfile{}, knowledgeport.ErrNotFound
}

func (searchProfileRepository) SaveParsingProfile(context.Context, domainknowledge.ParsingProfile) error {
	return nil
}

func (searchProfileRepository) GetEmbeddingProfile(context.Context, string) (domainknowledge.EmbeddingProfile, error) {
	return domainknowledge.EmbeddingProfile{}, knowledgeport.ErrNotFound
}

func (searchProfileRepository) SaveEmbeddingProfile(context.Context, domainknowledge.EmbeddingProfile) error {
	return nil
}

func (searchProfileRepository) GetChunkingProfile(context.Context, string) (domainknowledge.ChunkingProfile, error) {
	return domainknowledge.ChunkingProfile{}, knowledgeport.ErrNotFound
}

func (searchProfileRepository) SaveChunkingProfile(context.Context, domainknowledge.ChunkingProfile) error {
	return nil
}

func (repository searchProfileRepository) GetIndexProfile(_ context.Context, id string) (domainknowledge.IndexProfile, error) {
	profile, ok := repository.profiles[id]
	if !ok {
		return domainknowledge.IndexProfile{}, knowledgeport.ErrNotFound
	}
	return profile, nil
}

func (searchProfileRepository) SaveIndexProfile(context.Context, domainknowledge.IndexProfile) error {
	return nil
}

func (searchProfileRepository) MarkParsingProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}

func (searchProfileRepository) MarkEmbeddingProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}

func (searchProfileRepository) MarkChunkingProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}

func (searchProfileRepository) MarkIndexProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}
