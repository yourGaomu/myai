package service

import (
	"context"
	"errors"
	"testing"
	"time"

	retrievalcommand "myai/core/application/knowledge/retrieval/command"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

func TestRetrievalServiceUsesLocalResultsWithoutRemoteFallback(t *testing.T) {
	local := &fakeVectorStore{hits: []domainknowledge.VectorHit{{EmbeddingID: "embedding-1", ChunkID: "chunk-1", Score: 0.9, Rank: 1}}}
	remote := &fakeVectorStore{hits: []domainknowledge.VectorHit{{EmbeddingID: "embedding-2", ChunkID: "chunk-2", Score: 0.95, Rank: 1}}}
	service := newTestRetrievalService(t, Configuration{
		LocalVectors:    local,
		RemoteVectors:   remote,
		MinLocalResults: 1,
		MinLocalScore:   0.5,
	}, []domainknowledge.Chunk{testChunk("chunk-1")})
	result, err := service.Retrieve(context.Background(), retrievalcommand.Retrieve{Query: testRetrievalQuery(1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].ChunkID != "chunk-1" || result.Hits[0].SourceName != "guide.md" {
		t.Fatalf("unexpected local result: %#v", result)
	}
	if result.Diagnostics.RemoteFallback || remote.searchCalls != 0 || !result.Diagnostics.LocalQuality.Passed {
		t.Fatalf("remote fallback must not run: %#v", result.Diagnostics)
	}
}

func TestRetrievalServiceFallsBackAndFillsLocalCache(t *testing.T) {
	local := &fakeVectorStore{}
	remote := &fakeVectorStore{hits: []domainknowledge.VectorHit{{EmbeddingID: "embedding-2", ChunkID: "chunk-2", Score: 0.9, Rank: 1}}}
	keywords := &fakeKeywordStore{}
	service := newTestRetrievalService(t, Configuration{
		LocalVectors:       local,
		LocalKeywords:      keywords,
		RemoteVectors:      remote,
		MinLocalResults:    1,
		MinLocalScore:      0.5,
		CacheRemoteResults: true,
	}, []domainknowledge.Chunk{testChunk("chunk-2")})
	result, err := service.Retrieve(context.Background(), retrievalcommand.Retrieve{Query: testRetrievalQuery(1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].ChunkID != "chunk-2" || result.Hits[0].Origin != domainknowledge.RetrievalOriginRemote {
		t.Fatalf("unexpected remote result: %#v", result)
	}
	if !result.Diagnostics.RemoteFallback || remote.searchCalls != 1 || result.Diagnostics.CacheFillCount != 1 {
		t.Fatalf("unexpected fallback diagnostics: %#v", result.Diagnostics)
	}
	if local.ensureCalls != 1 || len(local.upserted) != 1 || local.upserted[0].ID != "embedding-2" {
		t.Fatalf("local vector cache was not filled: %#v", local.upserted)
	}
	if len(keywords.upserted) != 1 || keywords.upserted[0].ChunkID != "chunk-2" {
		t.Fatalf("local keyword cache was not filled: %#v", keywords.upserted)
	}
}

func TestRetrievalServiceStrictModeReturnsAllChannelFailure(t *testing.T) {
	local := &fakeVectorStore{healthErr: errors.New("local unavailable")}
	remote := &fakeVectorStore{searchErr: errors.New("remote unavailable")}
	keywords := &fakeKeywordStore{searchErr: errors.New("keyword unavailable")}
	service := newTestRetrievalService(t, Configuration{
		LocalVectors:    local,
		LocalKeywords:   keywords,
		RemoteVectors:   remote,
		MinLocalResults: 1,
		MinLocalScore:   0.5,
	}, nil)
	result, err := service.Retrieve(context.Background(), retrievalcommand.Retrieve{Query: testRetrievalQuery(1), Strict: true})
	if err == nil {
		t.Fatalf("expected strict retrieval error, got %#v", result)
	}
	if len(result.Diagnostics.Warnings) < 3 {
		t.Fatalf("expected degradation diagnostics, got %#v", result.Diagnostics)
	}
}

func TestRetrievalServiceKeepsKeywordResultsWhenEmbeddingProviderFails(t *testing.T) {
	profiles := testProfileRepository()
	remote := &fakeVectorStore{}
	service, err := New(Configuration{
		KnowledgeBases:  fakeKnowledgeBaseRepository{base: testKnowledgeBase()},
		Documents:       fakeDocumentRepository{document: testDocument()},
		Profiles:        profiles,
		Chunks:          &fakeChunkRepository{chunks: []domainknowledge.Chunk{testChunk("chunk-1")}},
		Embeddings:      fakeEmbeddingResolver{err: errors.New("model unavailable")},
		LocalVectors:    &fakeVectorStore{},
		LocalKeywords:   &fakeKeywordStore{hits: []domainknowledge.KeywordHit{{ChunkID: "chunk-1", Score: 1, Rank: 1}}},
		RemoteVectors:   remote,
		MinLocalResults: 1,
		MinLocalScore:   0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Retrieve(context.Background(), retrievalcommand.Retrieve{Query: testRetrievalQuery(1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].Channel != domainknowledge.RetrievalChannelKeyword {
		t.Fatalf("expected degraded keyword result, got %#v", result)
	}
	if remote.searchCalls != 0 || len(result.Diagnostics.Warnings) == 0 {
		t.Fatalf("unexpected degraded diagnostics: %#v", result.Diagnostics)
	}
}

func TestRetrievalServiceRejectsProfileMismatch(t *testing.T) {
	profiles := testProfileRepository()
	profiles.index.EmbeddingProfileID = "another-profile"
	service, err := New(Configuration{
		KnowledgeBases:  fakeKnowledgeBaseRepository{base: testKnowledgeBase()},
		Documents:       fakeDocumentRepository{document: testDocument()},
		Profiles:        profiles,
		Chunks:          &fakeChunkRepository{},
		Embeddings:      fakeEmbeddingResolver{provider: fakeEmbeddingProvider{}},
		RemoteVectors:   &fakeVectorStore{},
		MinLocalResults: 1,
		MinLocalScore:   0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Retrieve(context.Background(), retrievalcommand.Retrieve{Query: testRetrievalQuery(1)}); err == nil {
		t.Fatal("expected profile mismatch error")
	}
}

func newTestRetrievalService(t *testing.T, overrides Configuration, chunks []domainknowledge.Chunk) *RetrievalService {
	t.Helper()
	configuration := Configuration{
		KnowledgeBases:      fakeKnowledgeBaseRepository{base: testKnowledgeBase()},
		Documents:           fakeDocumentRepository{document: testDocument()},
		Profiles:            testProfileRepository(),
		Chunks:              &fakeChunkRepository{chunks: chunks},
		Embeddings:          fakeEmbeddingResolver{provider: fakeEmbeddingProvider{}},
		LocalVectors:        overrides.LocalVectors,
		LocalKeywords:       overrides.LocalKeywords,
		RemoteVectors:       overrides.RemoteVectors,
		CandidateMultiplier: overrides.CandidateMultiplier,
		MaxCandidates:       overrides.MaxCandidates,
		MinLocalResults:     overrides.MinLocalResults,
		MinLocalScore:       overrides.MinLocalScore,
		RRFK:                overrides.RRFK,
		CacheRemoteResults:  overrides.CacheRemoteResults,
	}
	service, err := New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func testRetrievalQuery(topK int) domainknowledge.RetrievalQuery {
	return domainknowledge.RetrievalQuery{
		Text:               "plan mode",
		KnowledgeBaseIDs:   []string{"kb-1"},
		IndexProfileID:     "index-1",
		EmbeddingProfileID: "embedding-profile-1",
		TopK:               topK,
	}
}

func testKnowledgeBase() domainknowledge.KnowledgeBase {
	return domainknowledge.KnowledgeBase{ID: "kb-1", Name: "Project", RAGEnabled: true, ActiveIndexProfileID: "index-1"}
}

func testDocument() domainknowledge.Document {
	return domainknowledge.Document{ID: "document-1", KnowledgeBaseID: "kb-1", FileName: "guide.md", ObjectKey: "guide.md", ContentHash: "hash", Version: 1, Status: domainknowledge.DocumentStatusReady}
}

func testChunk(id string) domainknowledge.Chunk {
	return domainknowledge.Chunk{
		ID: id, KnowledgeBaseID: "kb-1", DocumentID: "document-1", DocumentVersion: 1,
		ParsingProfileID: "parsing-1", ChunkingProfileID: "chunking-1", Ordinal: 0,
		Text: "Plan mode implementation", ContentHash: "chunk-hash", StartOffset: 0, EndOffset: 24,
		EmbeddingProfileIDs: []string{"embedding-profile-1"}, SyncSequence: 1,
	}
}

func testProfileRepository() fakeProfileRepository {
	return fakeProfileRepository{
		index: domainknowledge.IndexProfile{
			ID: "index-1", Name: "Index", ParsingProfileID: "parsing-1", ChunkingProfileID: "chunking-1",
			EmbeddingProfileID: "embedding-profile-1", DistanceMetricID: "cosine", Status: domainknowledge.IndexProfileStatusActive,
		},
		embedding: domainknowledge.EmbeddingProfile{
			ID: "embedding-profile-1", Name: "Embedding", ModelID: "model-1", Provider: "test",
			Model: "embedding-model", ModelVersion: "v1", Dimensions: 2,
		},
	}
}

type fakeVectorStore struct {
	hits        []domainknowledge.VectorHit
	healthErr   error
	searchErr   error
	searchCalls int
	ensureCalls int
	upserted    []domainknowledge.EmbeddingVector
}

func (store *fakeVectorStore) EnsureIndex(context.Context, domainknowledge.VectorIndexDefinition) error {
	store.ensureCalls++
	return nil
}

func (store *fakeVectorStore) Upsert(_ context.Context, embeddings []domainknowledge.EmbeddingVector) error {
	store.upserted = append(store.upserted, embeddings...)
	return nil
}

func (store *fakeVectorStore) Search(context.Context, domainknowledge.VectorQuery) ([]domainknowledge.VectorHit, error) {
	store.searchCalls++
	return append([]domainknowledge.VectorHit(nil), store.hits...), store.searchErr
}

func (store *fakeVectorStore) MarkDeleted(context.Context, domainknowledge.VectorDeletion) error {
	return nil
}
func (store *fakeVectorStore) Health(context.Context) error { return store.healthErr }

type fakeKeywordStore struct {
	hits      []domainknowledge.KeywordHit
	searchErr error
	upserted  []domainknowledge.KeywordDocument
}

func (store *fakeKeywordStore) Upsert(_ context.Context, documents []domainknowledge.KeywordDocument) error {
	store.upserted = append(store.upserted, documents...)
	return nil
}

func (store *fakeKeywordStore) Search(context.Context, domainknowledge.KeywordQuery) ([]domainknowledge.KeywordHit, error) {
	return append([]domainknowledge.KeywordHit(nil), store.hits...), store.searchErr
}

func (store *fakeKeywordStore) MarkDeleted(context.Context, []string, int64) error { return nil }

type fakeEmbeddingProvider struct{}

func (fakeEmbeddingProvider) EmbedQuery(_ context.Context, request domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	return domainknowledge.EmbedResult{
		EmbeddingProfileID: request.EmbeddingProfileID,
		Dimensions:         2,
		Outputs:            []domainknowledge.EmbedOutput{{ID: request.Inputs[0].ID, Vector: []float32{1, 0}}},
	}, nil
}

func (fakeEmbeddingProvider) EmbedDocuments(_ context.Context, request domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	outputs := make([]domainknowledge.EmbedOutput, 0, len(request.Inputs))
	for _, input := range request.Inputs {
		outputs = append(outputs, domainknowledge.EmbedOutput{ID: input.ID, Vector: []float32{1, 0}})
	}
	return domainknowledge.EmbedResult{EmbeddingProfileID: request.EmbeddingProfileID, Dimensions: 2, Outputs: outputs}, nil
}

type fakeEmbeddingResolver struct {
	provider knowledgeport.EmbeddingProvider
	err      error
}

func (resolver fakeEmbeddingResolver) Resolve(domainknowledge.EmbeddingProfile) (knowledgeport.EmbeddingProvider, error) {
	return resolver.provider, resolver.err
}

type fakeKnowledgeBaseRepository struct {
	base domainknowledge.KnowledgeBase
}

func (repository fakeKnowledgeBaseRepository) Get(context.Context, string) (domainknowledge.KnowledgeBase, error) {
	return repository.base, nil
}

func (fakeKnowledgeBaseRepository) List(context.Context, bool) ([]domainknowledge.KnowledgeBase, error) {
	return nil, nil
}

func (fakeKnowledgeBaseRepository) Save(context.Context, domainknowledge.KnowledgeBase) error {
	return nil
}
func (fakeKnowledgeBaseRepository) MarkDeleted(context.Context, string, time.Time, string, int64) error {
	return nil
}

type fakeDocumentRepository struct {
	document domainknowledge.Document
}

func (repository fakeDocumentRepository) Get(context.Context, string) (domainknowledge.Document, error) {
	return repository.document, nil
}

func (fakeDocumentRepository) ListByKnowledgeBase(context.Context, string, bool) ([]domainknowledge.Document, error) {
	return nil, nil
}

func (fakeDocumentRepository) Save(context.Context, domainknowledge.Document) error { return nil }
func (fakeDocumentRepository) MarkDeleted(context.Context, string, time.Time, string, int64) error {
	return nil
}

type fakeChunkRepository struct {
	chunks []domainknowledge.Chunk
}

func (repository *fakeChunkRepository) SaveAll(context.Context, []domainknowledge.Chunk) error {
	return nil
}
func (repository *fakeChunkRepository) GetByIDs(_ context.Context, ids []string) ([]domainknowledge.Chunk, error) {
	byID := make(map[string]domainknowledge.Chunk, len(repository.chunks))
	for _, chunk := range repository.chunks {
		byID[chunk.ID] = chunk
	}
	result := make([]domainknowledge.Chunk, 0, len(ids))
	for _, id := range ids {
		if chunk, exists := byID[id]; exists {
			result = append(result, chunk)
		}
	}
	return result, nil
}

func (*fakeChunkRepository) ListByDocumentPage(context.Context, string, int64, string, string, int, int) ([]domainknowledge.Chunk, error) {
	return nil, nil
}

func (*fakeChunkRepository) ListMissingEmbeddings(context.Context, string, string, string, int) ([]domainknowledge.Chunk, error) {
	return nil, nil
}

func (*fakeChunkRepository) MarkEmbedded(context.Context, []string, string, time.Time) error {
	return nil
}
func (*fakeChunkRepository) MarkDeletedByDocument(context.Context, string, time.Time, int64) error {
	return nil
}

type fakeProfileRepository struct {
	index     domainknowledge.IndexProfile
	embedding domainknowledge.EmbeddingProfile
}

func (fakeProfileRepository) GetParsingProfile(context.Context, string) (domainknowledge.ParsingProfile, error) {
	return domainknowledge.ParsingProfile{}, nil
}

func (fakeProfileRepository) SaveParsingProfile(context.Context, domainknowledge.ParsingProfile) error {
	return nil
}

func (repository fakeProfileRepository) GetEmbeddingProfile(context.Context, string) (domainknowledge.EmbeddingProfile, error) {
	return repository.embedding, nil
}

func (fakeProfileRepository) SaveEmbeddingProfile(context.Context, domainknowledge.EmbeddingProfile) error {
	return nil
}

func (fakeProfileRepository) GetChunkingProfile(context.Context, string) (domainknowledge.ChunkingProfile, error) {
	return domainknowledge.ChunkingProfile{}, nil
}

func (fakeProfileRepository) SaveChunkingProfile(context.Context, domainknowledge.ChunkingProfile) error {
	return nil
}

func (repository fakeProfileRepository) GetIndexProfile(context.Context, string) (domainknowledge.IndexProfile, error) {
	return repository.index, nil
}

func (fakeProfileRepository) SaveIndexProfile(context.Context, domainknowledge.IndexProfile) error {
	return nil
}

func (fakeProfileRepository) MarkParsingProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}

func (fakeProfileRepository) MarkEmbeddingProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}

func (fakeProfileRepository) MarkChunkingProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}

func (fakeProfileRepository) MarkIndexProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}
