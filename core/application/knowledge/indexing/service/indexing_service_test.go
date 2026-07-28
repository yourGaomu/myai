package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	indexingcommand "myai/core/application/knowledge/indexing/command"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
	documentprocessorport "myai/core/port/knowledge/documentprocessor"
)

func TestIndexingServiceRunsFullPipeline(t *testing.T) {
	fixtures := newIndexingFixtures()
	service, err := New(fixtures.configuration())
	if err != nil {
		t.Fatal(err)
	}

	submitted, err := service.Submit(context.Background(), indexingcommand.Submit{DocumentID: "document-1", IndexProfileID: "index-1"})
	if err != nil {
		t.Fatal(err)
	}
	if submitted.Job.Status != domainknowledge.IndexingJobStatusPending || submitted.Job.Stage != domainknowledge.IndexingStageParse {
		t.Fatalf("unexpected submitted job: %#v", submitted.Job)
	}

	completed, err := service.Run(context.Background(), indexingcommand.Run{JobID: submitted.Job.ID})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Job.Status != domainknowledge.IndexingJobStatusCompleted || completed.Job.Stage != domainknowledge.IndexingStageCompleted {
		t.Fatalf("unexpected completed job: %#v", completed.Job)
	}
	if completed.Document.Status != domainknowledge.DocumentStatusReady || completed.Processed == nil || completed.Processed.ChunkCount != 2 {
		t.Fatalf("unexpected completed document/result: %#v %#v", completed.Document, completed.Processed)
	}
	if len(fixtures.provider.requests) != 2 || len(fixtures.provider.requests[0].Inputs) != 1 || len(fixtures.provider.requests[1].Inputs) != 1 {
		t.Fatalf("expected two paged embedding requests: %#v", fixtures.provider.requests)
	}
	if len(fixtures.vectors.upserts) != 2 || len(fixtures.vectors.upserts[0]) != 1 || len(fixtures.vectors.upserts[1]) != 1 {
		t.Fatalf("expected two paged vector upserts: %#v", fixtures.vectors.upserts)
	}
	if len(fixtures.vectors.definitions) != 1 || fixtures.vectors.definitions[0].EmbeddingProfileID != "embedding-1" || fixtures.vectors.definitions[0].DistanceMetricID != "cosine" {
		t.Fatalf("unexpected vector index definition: %#v", fixtures.vectors.definitions)
	}
	if fixtures.chunks.markEmbeddedCalls != 2 {
		t.Fatalf("expected chunks to be marked after vector upsert, got %d calls", fixtures.chunks.markEmbeddedCalls)
	}
	if len(fixtures.keywords.upserts) != 2 || len(fixtures.keywords.upserts[0]) != 1 || len(fixtures.keywords.upserts[1]) != 1 {
		t.Fatalf("expected two paged keyword upserts: %#v", fixtures.keywords.upserts)
	}
	if fixtures.vectors.upsertedBeforeMark != fixtures.chunks.markEmbeddedCalls {
		t.Fatalf("vector upsert must happen before mark embedded: upsert=%d mark=%d", fixtures.vectors.upsertedBeforeMark, fixtures.chunks.markEmbeddedCalls)
	}
	if got, want := strings.Join(fixtures.events, ","), "vector,mark,vector,mark"; got != want {
		t.Fatalf("unexpected vector/chunk event order: got %q want %q", got, want)
	}
}

func TestIndexingServiceSkipsExistingEmbeddingProfile(t *testing.T) {
	fixtures := newIndexingFixtures()
	existing := validFixtureChunk(0)
	existing.EmbeddingProfileIDs = []string{"embedding-1"}
	fixtures.chunks.items["chunk-0"] = existing
	service, err := New(fixtures.configuration())
	if err != nil {
		t.Fatal(err)
	}

	submitted, err := service.Submit(context.Background(), indexingcommand.Submit{DocumentID: "document-1", IndexProfileID: "index-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Run(context.Background(), indexingcommand.Run{JobID: submitted.Job.ID}); err != nil {
		t.Fatal(err)
	}
	if len(fixtures.provider.requests) != 1 || len(fixtures.provider.requests[0].Inputs) != 1 {
		t.Fatalf("expected only the missing chunk to be embedded: %#v", fixtures.provider.requests)
	}
	if fixtures.provider.requests[0].Inputs[0].ID != "chunk-1" {
		t.Fatalf("unexpected missing chunk: %#v", fixtures.provider.requests[0].Inputs)
	}
}

func TestIndexingServiceMarksEmbeddedOnlyAfterVectorUpsert(t *testing.T) {
	fixtures := newIndexingFixtures()
	fixtures.vectors.err = errors.New("milvus unavailable")
	service, err := New(fixtures.configuration())
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service.Submit(context.Background(), indexingcommand.Submit{DocumentID: "document-1", IndexProfileID: "index-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Run(context.Background(), indexingcommand.Run{JobID: submitted.Job.ID}); err == nil {
		t.Fatal("expected vector upsert error")
	}
	if fixtures.chunks.markEmbeddedCalls != 0 {
		t.Fatal("failed vector upsert must not mark chunks embedded")
	}
	if fixtures.documents.current.Status != domainknowledge.DocumentStatusFailed {
		t.Fatalf("expected failed document state, got %s", fixtures.documents.current.Status)
	}
	if fixtures.jobs.current.Status != domainknowledge.IndexingJobStatusFailed {
		t.Fatalf("expected failed job state, got %s", fixtures.jobs.current.Status)
	}
}

func TestIndexingServicePersistsParserFailure(t *testing.T) {
	fixtures := newIndexingFixtures()
	fixtures.processor.err = errors.New("unsupported document")
	service, err := New(fixtures.configuration())
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service.Submit(context.Background(), indexingcommand.Submit{DocumentID: "document-1", IndexProfileID: "index-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Run(context.Background(), indexingcommand.Run{JobID: submitted.Job.ID}); err == nil {
		t.Fatal("expected parser error")
	}
	if fixtures.documents.current.Status != domainknowledge.DocumentStatusFailed || fixtures.documents.current.FailureReason == "" {
		t.Fatalf("expected parser failure document state: %#v", fixtures.documents.current)
	}
	if fixtures.jobs.current.Status != domainknowledge.IndexingJobStatusFailed || fixtures.jobs.current.LastError == "" {
		t.Fatalf("expected parser failure job state: %#v", fixtures.jobs.current)
	}
}

func TestIndexingServiceRejectsConcurrentRunForSameJob(t *testing.T) {
	fixtures := newIndexingFixtures()
	fixtures.processor.block = make(chan struct{})
	service, err := New(fixtures.configuration())
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service.Submit(context.Background(), indexingcommand.Submit{DocumentID: "document-1", IndexProfileID: "index-1"})
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() {
		_, runErr := service.Run(context.Background(), indexingcommand.Run{JobID: submitted.Job.ID})
		firstDone <- runErr
	}()
	<-fixtures.processor.started
	if _, err := service.Run(context.Background(), indexingcommand.Run{JobID: submitted.Job.ID}); err == nil {
		t.Fatal("expected concurrent run to be rejected")
	}
	close(fixtures.processor.block)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

type indexingFixtures struct {
	documents  *fakeDocumentRepository
	profiles   *fakeProfileRepository
	jobs       *fakeJobRepository
	states     *fakeIndexingStateRepository
	chunks     *fakeChunkRepository
	objects    *fakeObjectStore
	processor  *fakeDocumentProcessor
	provider   *fakeEmbeddingProvider
	vectors    *fakeVectorStore
	keywords   *fakeKeywordStore
	events     []string
	clock      time.Time
	jobCounter int
}

func newIndexingFixtures() *indexingFixtures {
	fixtures := &indexingFixtures{
		documents: &fakeDocumentRepository{current: validFixtureDocument()},
		profiles:  validFixtureProfiles(),
		jobs:      &fakeJobRepository{items: make(map[string]domainknowledge.IndexingJob)},
		chunks:    &fakeChunkRepository{items: make(map[string]domainknowledge.Chunk)},
		objects:   &fakeObjectStore{content: "document content"},
		processor: &fakeDocumentProcessor{started: make(chan struct{})},
		provider:  &fakeEmbeddingProvider{},
		vectors:   &fakeVectorStore{},
		keywords:  &fakeKeywordStore{},
		clock:     time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC),
	}
	fixtures.chunks.events = &fixtures.events
	fixtures.vectors.events = &fixtures.events
	fixtures.states = &fakeIndexingStateRepository{documents: fixtures.documents, jobs: fixtures.jobs}
	return fixtures
}

func (fixtures *indexingFixtures) configuration() Configuration {
	return Configuration{
		Documents:  fixtures.documents,
		Profiles:   fixtures.profiles,
		Jobs:       fixtures.jobs,
		States:     fixtures.states,
		Chunks:     fixtures.chunks,
		Objects:    fixtures.objects,
		Processor:  fixtures.processor,
		Embeddings: fixtures.providerResolver(),
		Vectors:    fixtures.vectors,
		Keywords:   fixtures.keywords,
		JobIDs: idGeneratorFunc(func() string {
			fixtures.jobCounter++
			return fmt.Sprintf("job-%d", fixtures.jobCounter)
		}),
		StableIDs: stableIDDeriver{},
		PageSize:  1,
		Now: func() time.Time {
			return fixtures.clock
		},
	}
}

func (fixtures *indexingFixtures) providerResolver() knowledgeport.EmbeddingModelResolver {
	return embeddingResolver{provider: fixtures.provider}
}

type idGeneratorFunc func() string

func (generator idGeneratorFunc) NewID() string { return generator() }

type stableIDDeriver struct{}

func (stableIDDeriver) ChunkID(identity domainknowledge.ChunkIdentity) string {
	return fmt.Sprintf("chunk-%d", identity.Ordinal)
}

func (stableIDDeriver) EmbeddingID(chunkID string, embeddingProfileID string) string {
	return "embedding-" + chunkID + "-" + embeddingProfileID
}

type embeddingResolver struct {
	provider knowledgeport.EmbeddingProvider
}

func (resolver embeddingResolver) Resolve(domainknowledge.EmbeddingProfile) (knowledgeport.EmbeddingProvider, error) {
	return resolver.provider, nil
}

type fakeDocumentRepository struct {
	current domainknowledge.Document
}

func (repository *fakeDocumentRepository) Get(context.Context, string) (domainknowledge.Document, error) {
	return repository.current, nil
}

func (repository *fakeDocumentRepository) ListByKnowledgeBase(context.Context, string, bool) ([]domainknowledge.Document, error) {
	return []domainknowledge.Document{repository.current}, nil
}

func (repository *fakeDocumentRepository) Save(_ context.Context, document domainknowledge.Document) error {
	repository.current = document
	return nil
}

func (repository *fakeDocumentRepository) MarkDeleted(context.Context, string, time.Time, string, int64) error {
	return nil
}

type fakeProfileRepository struct {
	index     domainknowledge.IndexProfile
	parsing   domainknowledge.ParsingProfile
	chunking  domainknowledge.ChunkingProfile
	embedding domainknowledge.EmbeddingProfile
}

func validFixtureProfiles() *fakeProfileRepository {
	return &fakeProfileRepository{
		index: domainknowledge.IndexProfile{
			ID: "index-1", Name: "default", ParsingProfileID: "parsing-1", ChunkingProfileID: "chunking-1", EmbeddingProfileID: "embedding-1", DistanceMetricID: "cosine", Status: domainknowledge.IndexProfileStatusActive,
		},
		parsing:   domainknowledge.ParsingProfile{ID: "parsing-1", Name: "plain", ParserID: "plain", ParserVersion: "1"},
		chunking:  domainknowledge.ChunkingProfile{ID: "chunking-1", Name: "fixed", StrategyID: "fixed", StrategyVersion: "1", MaxChunkSize: 100},
		embedding: domainknowledge.EmbeddingProfile{ID: "embedding-1", Name: "embedding", ModelID: "model-1", Provider: "fake", Model: "fake", Dimensions: 2},
	}
}

func (repository *fakeProfileRepository) GetParsingProfile(context.Context, string) (domainknowledge.ParsingProfile, error) {
	return repository.parsing, nil
}
func (repository *fakeProfileRepository) SaveParsingProfile(context.Context, domainknowledge.ParsingProfile) error {
	return nil
}
func (repository *fakeProfileRepository) GetEmbeddingProfile(context.Context, string) (domainknowledge.EmbeddingProfile, error) {
	return repository.embedding, nil
}
func (repository *fakeProfileRepository) SaveEmbeddingProfile(context.Context, domainknowledge.EmbeddingProfile) error {
	return nil
}
func (repository *fakeProfileRepository) GetChunkingProfile(context.Context, string) (domainknowledge.ChunkingProfile, error) {
	return repository.chunking, nil
}
func (repository *fakeProfileRepository) SaveChunkingProfile(context.Context, domainknowledge.ChunkingProfile) error {
	return nil
}
func (repository *fakeProfileRepository) GetIndexProfile(context.Context, string) (domainknowledge.IndexProfile, error) {
	return repository.index, nil
}
func (repository *fakeProfileRepository) SaveIndexProfile(context.Context, domainknowledge.IndexProfile) error {
	return nil
}
func (repository *fakeProfileRepository) MarkParsingProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}
func (repository *fakeProfileRepository) MarkEmbeddingProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}
func (repository *fakeProfileRepository) MarkChunkingProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}
func (repository *fakeProfileRepository) MarkIndexProfileDeleted(context.Context, string, time.Time, string) error {
	return nil
}

type fakeJobRepository struct {
	items   map[string]domainknowledge.IndexingJob
	current domainknowledge.IndexingJob
	mu      sync.Mutex
}

type fakeIndexingStateRepository struct {
	documents *fakeDocumentRepository
	jobs      *fakeJobRepository
	err       error
	saves     int
}

func (repository *fakeIndexingStateRepository) SaveDocumentAndJob(ctx context.Context, document domainknowledge.Document, job domainknowledge.IndexingJob) error {
	if repository.err != nil {
		return repository.err
	}
	repository.saves++
	if err := repository.documents.Save(ctx, document); err != nil {
		return err
	}
	return repository.jobs.Save(ctx, job)
}

func (repository *fakeJobRepository) Get(_ context.Context, jobID string) (domainknowledge.IndexingJob, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	job, ok := repository.items[jobID]
	if !ok {
		return domainknowledge.IndexingJob{}, knowledgeport.ErrNotFound
	}
	return job, nil
}

func (repository *fakeJobRepository) Save(_ context.Context, job domainknowledge.IndexingJob) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.items[job.ID] = job
	repository.current = job
	return nil
}

func (repository *fakeJobRepository) ListPending(context.Context, int) ([]domainknowledge.IndexingJob, error) {
	return nil, nil
}

type fakeChunkRepository struct {
	items             map[string]domainknowledge.Chunk
	markEmbeddedCalls int
	events            *[]string
	mu                sync.Mutex
}

func (repository *fakeChunkRepository) SaveAll(_ context.Context, chunks []domainknowledge.Chunk) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for _, chunk := range chunks {
		if existing, ok := repository.items[chunk.ID]; ok {
			chunk.EmbeddingProfileIDs = append([]string(nil), existing.EmbeddingProfileIDs...)
		}
		repository.items[chunk.ID] = chunk
	}
	return nil
}

func (repository *fakeChunkRepository) GetByIDs(context.Context, []string) ([]domainknowledge.Chunk, error) {
	return nil, nil
}

func (repository *fakeChunkRepository) ListByDocumentPage(_ context.Context, documentID string, version int64, parsingID string, chunkingID string, afterOrdinal int, limit int) ([]domainknowledge.Chunk, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	items := make([]domainknowledge.Chunk, 0)
	for _, chunk := range repository.items {
		if chunk.DocumentID == documentID && chunk.DocumentVersion == version && chunk.ParsingProfileID == parsingID && chunk.ChunkingProfileID == chunkingID && chunk.Ordinal > afterOrdinal {
			items = append(items, chunk)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Ordinal < items[j].Ordinal })
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (repository *fakeChunkRepository) ListMissingEmbeddings(context.Context, string, string, string, int) ([]domainknowledge.Chunk, error) {
	return nil, nil
}

func (repository *fakeChunkRepository) MarkEmbedded(_ context.Context, chunkIDs []string, profileID string, updatedAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.markEmbeddedCalls++
	if repository.events != nil {
		*repository.events = append(*repository.events, "mark")
	}
	for _, chunkID := range chunkIDs {
		chunk := repository.items[chunkID]
		found := false
		for _, existing := range chunk.EmbeddingProfileIDs {
			if existing == profileID {
				found = true
				break
			}
		}
		if !found {
			chunk.EmbeddingProfileIDs = append(chunk.EmbeddingProfileIDs, profileID)
		}
		chunk.UpdatedAt = updatedAt
		repository.items[chunkID] = chunk
	}
	return nil
}

func (repository *fakeChunkRepository) MarkDeletedByDocument(context.Context, string, time.Time, int64) error {
	return nil
}

type fakeObjectStore struct{ content string }

func (store *fakeObjectStore) Put(context.Context, string, io.Reader, int64, string) (domainknowledge.ObjectRef, error) {
	return domainknowledge.ObjectRef{}, nil
}
func (store *fakeObjectStore) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(store.content)), nil
}
func (store *fakeObjectStore) Stat(context.Context, string) (domainknowledge.ObjectInfo, error) {
	return domainknowledge.ObjectInfo{}, nil
}

type fakeDocumentProcessor struct {
	err     error
	block   chan struct{}
	started chan struct{}
	once    sync.Once
}

func (processor *fakeDocumentProcessor) Process(ctx context.Context, _ documentprocessorport.Request, sink documentprocessorport.ChunkSink) (domainknowledge.ProcessingSummary, error) {
	if processor.started == nil {
		processor.started = make(chan struct{})
	}
	processor.once.Do(func() { close(processor.started) })
	if processor.block != nil {
		select {
		case <-processor.block:
		case <-ctx.Done():
			return domainknowledge.ProcessingSummary{}, ctx.Err()
		}
	}
	if processor.err != nil {
		return domainknowledge.ProcessingSummary{}, processor.err
	}
	drafts := []domainknowledge.ChunkDraft{
		{Ordinal: 0, Text: "first", ContentHash: "hash-0", EndOffset: 5},
		{Ordinal: 1, Text: "second", ContentHash: "hash-1", StartOffset: 5, EndOffset: 11},
	}
	if err := sink.Accept(ctx, drafts); err != nil {
		return domainknowledge.ProcessingSummary{}, err
	}
	return domainknowledge.ProcessingSummary{DocumentID: "document-1", DocumentVersion: 1, ContentType: "text/plain", ParserID: "plain", ParserVersion: "1", ChunkingStrategyID: "fixed", ChunkingStrategyVersion: "1", ChunkCount: 2}, nil
}

type fakeEmbeddingProvider struct {
	requests []domainknowledge.EmbedRequest
}

func (provider *fakeEmbeddingProvider) EmbedDocuments(_ context.Context, request domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	provider.requests = append(provider.requests, request)
	outputs := make([]domainknowledge.EmbedOutput, 0, len(request.Inputs))
	for index, input := range request.Inputs {
		outputs = append(outputs, domainknowledge.EmbedOutput{ID: input.ID, Vector: []float32{float32(index + 1), 0.5}})
	}
	return domainknowledge.EmbedResult{EmbeddingProfileID: request.EmbeddingProfileID, Dimensions: 2, Outputs: outputs}, nil
}
func (provider *fakeEmbeddingProvider) EmbedQuery(context.Context, domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	return domainknowledge.EmbedResult{}, nil
}

type fakeVectorStore struct {
	definitions        []domainknowledge.VectorIndexDefinition
	upserts            [][]domainknowledge.EmbeddingVector
	err                error
	upsertedBeforeMark int
	events             *[]string
}

func (store *fakeVectorStore) EnsureIndex(_ context.Context, definition domainknowledge.VectorIndexDefinition) error {
	store.definitions = append(store.definitions, definition)
	return nil
}

func (store *fakeVectorStore) Upsert(_ context.Context, embeddings []domainknowledge.EmbeddingVector) error {
	if store.err != nil {
		return store.err
	}
	store.upsertedBeforeMark++
	if store.events != nil {
		*store.events = append(*store.events, "vector")
	}
	store.upserts = append(store.upserts, embeddings)
	return nil
}
func (store *fakeVectorStore) Search(context.Context, domainknowledge.VectorQuery) ([]domainknowledge.VectorHit, error) {
	return nil, nil
}
func (store *fakeVectorStore) MarkDeleted(context.Context, domainknowledge.VectorDeletion) error {
	return nil
}
func (store *fakeVectorStore) Health(context.Context) error { return nil }

type fakeKeywordStore struct {
	upserts [][]domainknowledge.KeywordDocument
}

func (store *fakeKeywordStore) Upsert(_ context.Context, documents []domainknowledge.KeywordDocument) error {
	store.upserts = append(store.upserts, documents)
	return nil
}
func (store *fakeKeywordStore) Search(context.Context, domainknowledge.KeywordQuery) ([]domainknowledge.KeywordHit, error) {
	return nil, nil
}
func (store *fakeKeywordStore) MarkDeleted(context.Context, []string, int64) error { return nil }

func validFixtureDocument() domainknowledge.Document {
	return domainknowledge.Document{
		ID: "document-1", KnowledgeBaseID: "knowledge-1", FileName: "document.txt", ContentType: "text/plain", ObjectKey: "documents/document-1", ContentHash: "document-hash", Version: 1, Status: domainknowledge.DocumentStatusUploaded,
	}
}

func validFixtureChunk(ordinal int) domainknowledge.Chunk {
	return domainknowledge.Chunk{
		ID: fmt.Sprintf("chunk-%d", ordinal), KnowledgeBaseID: "knowledge-1", DocumentID: "document-1", DocumentVersion: 1, ParsingProfileID: "parsing-1", ChunkingProfileID: "chunking-1", Ordinal: ordinal, Text: fmt.Sprintf("text-%d", ordinal), ContentHash: fmt.Sprintf("hash-%d", ordinal), EndOffset: ordinal + 1,
	}
}
