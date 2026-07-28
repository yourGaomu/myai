package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	documentcommand "myai/core/application/knowledge/document/command"
	indexingcommand "myai/core/application/knowledge/indexing/command"
	indexingresult "myai/core/application/knowledge/indexing/result"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

func TestIngestRemovesUploadWhenDocumentWasNeverCommitted(t *testing.T) {
	fixtures := newDocumentFixtures()
	fixtures.documents.saveErr = errors.New("mongo unavailable")

	_, err := fixtures.service.Ingest(context.Background(), documentcommand.Ingest{KnowledgeBaseID: "knowledge-1", URL: "asset://guide"})
	if err == nil || !strings.Contains(err.Error(), "mongo unavailable") {
		t.Fatalf("expected document save error, got %v", err)
	}
	if fixtures.objects.deleteCalls != 1 {
		t.Fatalf("expected uncommitted upload cleanup, got %d calls", fixtures.objects.deleteCalls)
	}
	if fixtures.indexing.submitCalls != 0 {
		t.Fatal("indexing must not start before the document is committed")
	}
}

func TestIngestKeepsOriginalAndMarksDocumentFailedWhenSubmitFails(t *testing.T) {
	fixtures := newDocumentFixtures()
	fixtures.indexing.submitErr = errors.New("profile unavailable")

	result, err := fixtures.service.Ingest(context.Background(), documentcommand.Ingest{KnowledgeBaseID: "knowledge-1", URL: "asset://guide"})
	if err == nil || !strings.Contains(err.Error(), "profile unavailable") {
		t.Fatalf("expected submit error, got %v", err)
	}
	if result.Document.ID != "document-1" || fixtures.documents.current.Status != domainknowledge.DocumentStatusFailed {
		t.Fatalf("expected visible failed document, result=%#v saved=%#v", result.Document, fixtures.documents.current)
	}
	if fixtures.documents.current.FailureReason == "" {
		t.Fatal("failed document must retain the cause")
	}
	if fixtures.objects.deleteCalls != 0 {
		t.Fatal("committed document originals must remain in object storage")
	}
}

func TestIngestMarksDocumentAndJobFailedWhenAsyncSchedulingFails(t *testing.T) {
	fixtures := newDocumentFixtures()
	fixtures.async.err = errors.New("task queue is full")

	result, err := fixtures.service.Ingest(context.Background(), documentcommand.Ingest{KnowledgeBaseID: "knowledge-1", URL: "asset://guide"})
	if err == nil || !strings.Contains(err.Error(), "task queue is full") {
		t.Fatalf("expected scheduling error, got %v", err)
	}
	if result.Document.Status != domainknowledge.DocumentStatusFailed || result.Job.Status != domainknowledge.IndexingJobStatusFailed {
		t.Fatalf("expected failed result state, got document=%s job=%s", result.Document.Status, result.Job.Status)
	}
	if fixtures.states.saves != 1 || fixtures.states.document.Status != domainknowledge.DocumentStatusFailed || fixtures.states.job.Status != domainknowledge.IndexingJobStatusFailed {
		t.Fatalf("expected one atomic failed transition, got %#v", fixtures.states)
	}
	if fixtures.objects.deleteCalls != 0 {
		t.Fatal("scheduling failure must not remove the committed original")
	}
}

func TestIngestCompensatesWhenUploadReportsFailure(t *testing.T) {
	fixtures := newDocumentFixtures()
	fixtures.objects.putErr = errors.New("upload response lost")

	_, err := fixtures.service.Ingest(context.Background(), documentcommand.Ingest{KnowledgeBaseID: "knowledge-1", URL: "asset://guide"})
	if err == nil || !strings.Contains(err.Error(), "upload response lost") {
		t.Fatalf("expected upload error, got %v", err)
	}
	if fixtures.objects.deleteCalls != 1 || fixtures.documents.saves != 0 {
		t.Fatalf("expected upload compensation before persistence, deletes=%d saves=%d", fixtures.objects.deleteCalls, fixtures.documents.saves)
	}
}

func TestRecoverPendingMarksStateFailedWhenRecoveryCannotBeScheduled(t *testing.T) {
	fixtures := newDocumentFixtures()
	fixtures.documents.current = domainknowledge.Document{
		ID: "document-1", KnowledgeBaseID: "knowledge-1", FileName: "guide.md", ContentType: "text/markdown",
		ObjectKey: "knowledge/knowledge-1/document-1/v1/guide.md", ContentHash: "content-hash", Version: 1,
		Status: domainknowledge.DocumentStatusUploaded, CreatedAt: fixtures.indexing.job.CreatedAt, UpdatedAt: fixtures.indexing.job.UpdatedAt,
	}
	fixtures.jobs.pending = []domainknowledge.IndexingJob{fixtures.indexing.job}
	fixtures.async.err = errors.New("thread pool is closed")

	err := fixtures.service.RecoverPending(context.Background(), 10)
	if err == nil || !strings.Contains(err.Error(), "thread pool is closed") {
		t.Fatalf("expected recovery scheduling error, got %v", err)
	}
	if fixtures.states.saves != 1 || fixtures.states.document.Status != domainknowledge.DocumentStatusFailed || fixtures.states.job.Status != domainknowledge.IndexingJobStatusFailed {
		t.Fatalf("expected atomic recovery failure state, got %#v", fixtures.states)
	}
}

type documentFixtures struct {
	service   DocumentService
	documents *documentRepository
	objects   *documentObjectStore
	indexing  *indexingService
	states    *indexingStateRepository
	async     *asyncExecutor
	jobs      *jobRepository
}

func newDocumentFixtures() documentFixtures {
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	documents := &documentRepository{}
	objects := &documentObjectStore{}
	indexing := &indexingService{job: domainknowledge.IndexingJob{
		ID: "job-1", KnowledgeBaseID: "knowledge-1", DocumentID: "document-1", IndexProfileID: "index-1",
		Stage: domainknowledge.IndexingStageParse, Status: domainknowledge.IndexingJobStatusPending, CreatedAt: now, UpdatedAt: now,
	}}
	states := &indexingStateRepository{}
	async := &asyncExecutor{}
	jobs := &jobRepository{job: indexing.job}
	return documentFixtures{
		service: DocumentService{
			KnowledgeBases: knowledgeBaseRepository{base: domainknowledge.KnowledgeBase{
				ID: "knowledge-1", Name: "Project", RAGEnabled: true, ActiveIndexProfileID: "index-1",
			}},
			Documents:     documents,
			Chunks:        chunkRepository{},
			Objects:       objects,
			ObjectCleanup: objects,
			Sources: sourceReader{content: domainknowledge.DocumentSourceContent{
				FileName: "guide.md", ContentType: "text/markdown", Size: 5, Data: []byte("guide"),
			}},
			Indexing: indexing,
			Jobs:     jobs,
			States:   states,
			IDs:      staticID("document-1"),
			Async:    async,
			Now:      func() time.Time { return now },
		},
		documents: documents,
		objects:   objects,
		indexing:  indexing,
		states:    states,
		async:     async,
		jobs:      jobs,
	}
}

type knowledgeBaseRepository struct{ base domainknowledge.KnowledgeBase }

func (repository knowledgeBaseRepository) Get(context.Context, string) (domainknowledge.KnowledgeBase, error) {
	return repository.base, nil
}
func (knowledgeBaseRepository) List(context.Context, bool) ([]domainknowledge.KnowledgeBase, error) {
	return nil, nil
}
func (knowledgeBaseRepository) Save(context.Context, domainknowledge.KnowledgeBase) error { return nil }
func (knowledgeBaseRepository) MarkDeleted(context.Context, string, time.Time, string, int64) error {
	return nil
}

type documentRepository struct {
	current domainknowledge.Document
	saveErr error
	saves   int
}

func (repository *documentRepository) Get(context.Context, string) (domainknowledge.Document, error) {
	return repository.current, nil
}
func (repository *documentRepository) ListByKnowledgeBase(context.Context, string, bool) ([]domainknowledge.Document, error) {
	return nil, nil
}
func (repository *documentRepository) Save(_ context.Context, document domainknowledge.Document) error {
	repository.saves++
	if repository.saveErr != nil {
		return repository.saveErr
	}
	repository.current = document
	return nil
}
func (repository *documentRepository) MarkDeleted(context.Context, string, time.Time, string, int64) error {
	return nil
}

type documentObjectStore struct {
	deleteCalls int
	putErr      error
}

func (store *documentObjectStore) Put(_ context.Context, objectKey string, _ io.Reader, size int64, contentType string) (domainknowledge.ObjectRef, error) {
	if store.putErr != nil {
		return domainknowledge.ObjectRef{}, store.putErr
	}
	return domainknowledge.ObjectRef{ObjectKey: objectKey, ContentHash: "content-hash", Size: size, ContentType: contentType}, nil
}
func (*documentObjectStore) Open(context.Context, string) (io.ReadCloser, error) { return nil, nil }
func (*documentObjectStore) Stat(context.Context, string) (domainknowledge.ObjectInfo, error) {
	return domainknowledge.ObjectInfo{}, nil
}
func (store *documentObjectStore) DeleteUncommitted(context.Context, string) error {
	store.deleteCalls++
	return nil
}

type sourceReader struct {
	content domainknowledge.DocumentSourceContent
}

func (reader sourceReader) Read(context.Context, domainknowledge.DocumentSourceRequest) (domainknowledge.DocumentSourceContent, error) {
	return reader.content, nil
}

type indexingService struct {
	job         domainknowledge.IndexingJob
	submitErr   error
	submitCalls int
}

func (service *indexingService) Submit(context.Context, indexingcommand.Submit) (indexingresult.Submit, error) {
	service.submitCalls++
	return indexingresult.Submit{Job: service.job}, service.submitErr
}
func (*indexingService) Run(context.Context, indexingcommand.Run) (indexingresult.Run, error) {
	return indexingresult.Run{}, nil
}

type indexingStateRepository struct {
	document domainknowledge.Document
	job      domainknowledge.IndexingJob
	saves    int
}

func (repository *indexingStateRepository) SaveDocumentAndJob(_ context.Context, document domainknowledge.Document, job domainknowledge.IndexingJob) error {
	repository.saves++
	repository.document = document
	repository.job = job
	return nil
}

type jobRepository struct {
	job     domainknowledge.IndexingJob
	pending []domainknowledge.IndexingJob
}

func (repository *jobRepository) Get(context.Context, string) (domainknowledge.IndexingJob, error) {
	return repository.job, nil
}
func (*jobRepository) Save(context.Context, domainknowledge.IndexingJob) error { return nil }
func (repository *jobRepository) ListPending(context.Context, int) ([]domainknowledge.IndexingJob, error) {
	return append([]domainknowledge.IndexingJob(nil), repository.pending...), nil
}

type asyncExecutor struct{ err error }

func (executor *asyncExecutor) Submit(func()) error { return executor.err }

type staticID string

func (id staticID) NewID() string { return string(id) }

type chunkRepository struct{}

func (chunkRepository) SaveAll(context.Context, []domainknowledge.Chunk) error { return nil }
func (chunkRepository) GetByIDs(context.Context, []string) ([]domainknowledge.Chunk, error) {
	return nil, nil
}
func (chunkRepository) ListByDocumentPage(context.Context, string, int64, string, string, int, int) ([]domainknowledge.Chunk, error) {
	return nil, nil
}
func (chunkRepository) ListMissingEmbeddings(context.Context, string, string, string, int) ([]domainknowledge.Chunk, error) {
	return nil, nil
}
func (chunkRepository) MarkEmbedded(context.Context, []string, string, time.Time) error { return nil }
func (chunkRepository) MarkDeletedByDocument(context.Context, string, time.Time, int64) error {
	return nil
}

var _ knowledgeport.DocumentObjectStore = (*documentObjectStore)(nil)
