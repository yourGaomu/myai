package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	documentapi "myai/core/application/knowledge/document/api"
	documentcommand "myai/core/application/knowledge/document/command"
	documentresult "myai/core/application/knowledge/document/result"
	indexingapi "myai/core/application/knowledge/indexing/api"
	indexingcommand "myai/core/application/knowledge/indexing/command"
	domainknowledge "myai/core/domain/knowledge"
	asyncport "myai/core/port/async"
	knowledgeport "myai/core/port/knowledge"
)

const defaultMaxDocumentBytes = 50 * 1024 * 1024
const failurePersistenceTimeout = 5 * time.Second

type DocumentService struct {
	KnowledgeBases knowledgeport.KnowledgeBaseRepository
	Documents      knowledgeport.DocumentRepository
	Chunks         knowledgeport.ChunkRepository
	Objects        knowledgeport.DocumentObjectStore
	ObjectCleanup  knowledgeport.UncommittedDocumentObjectCleaner
	Sources        knowledgeport.DocumentSourceReader
	Indexing       indexingapi.Service
	Jobs           knowledgeport.IndexingJobRepository
	States         knowledgeport.IndexingStateRepository
	IDs            knowledgeport.IDGenerator
	Async          asyncport.Executor
	Now            func() time.Time
	OnAsyncError   func(error)
}

var _ documentapi.Service = DocumentService{}

func (service DocumentService) Ingest(ctx context.Context, command documentcommand.Ingest) (documentresult.Ingest, error) {
	if err := service.validateDependencies(); err != nil {
		return documentresult.Ingest{}, err
	}
	knowledgeBaseID := strings.TrimSpace(command.KnowledgeBaseID)
	base, err := service.KnowledgeBases.Get(ctx, knowledgeBaseID)
	if err != nil {
		return documentresult.Ingest{}, err
	}
	if !base.RAGEnabled || base.Deletion.Deleted {
		return documentresult.Ingest{}, fmt.Errorf("knowledge base %q is not enabled for RAG", knowledgeBaseID)
	}
	content, err := service.Sources.Read(ctx, domainknowledge.DocumentSourceRequest{URL: command.URL, Code: command.Code, MaxBytes: defaultMaxDocumentBytes})
	if err != nil {
		return documentresult.Ingest{}, err
	}
	if err := content.Validate(); err != nil {
		return documentresult.Ingest{}, fmt.Errorf("validate document source: %w", err)
	}
	documentID := strings.TrimSpace(service.IDs.NewID())
	if documentID == "" {
		return documentresult.Ingest{}, errors.New("document id generator returned an empty id")
	}
	objectKey := path.Join("knowledge", knowledgeBaseID, documentID, "v1", safeObjectName(content.FileName))
	object, err := service.Objects.Put(ctx, objectKey, bytes.NewReader(content.Data), content.Size, content.ContentType)
	if err != nil {
		return documentresult.Ingest{}, service.cleanupUncommittedObject(ctx, objectKey, fmt.Errorf("upload document object: %w", err))
	}
	now := service.now()
	document := domainknowledge.Document{
		ID: documentID, KnowledgeBaseID: knowledgeBaseID, FileName: content.FileName,
		ContentType: content.ContentType, ObjectKey: object.ObjectKey, ContentHash: object.ContentHash,
		Version: 1, Status: domainknowledge.DocumentStatusUploaded, CreatedAt: now, UpdatedAt: now,
	}
	if err := document.Validate(); err != nil {
		return documentresult.Ingest{}, service.cleanupUncommittedObject(ctx, object.ObjectKey, err)
	}
	if err := service.Documents.Save(ctx, document); err != nil {
		return documentresult.Ingest{}, service.cleanupUncommittedObject(ctx, object.ObjectKey, fmt.Errorf("save uploaded document: %w", err))
	}
	submitted, err := service.Indexing.Submit(ctx, indexingcommand.Submit{DocumentID: document.ID, IndexProfileID: base.ActiveIndexProfileID})
	if err != nil {
		cause := fmt.Errorf("submit document indexing: %w", err)
		failureErr := service.failDocument(ctx, &document, cause)
		return documentresult.Ingest{Document: document}, failureErr
	}
	if err := service.startIndexing(submitted.Job.ID); err != nil {
		if service.Async != nil {
			cause := fmt.Errorf("schedule document indexing: %w", err)
			failureErr := service.failDocumentAndJob(ctx, &document, &submitted.Job, cause)
			return documentresult.Ingest{Document: document, Job: submitted.Job}, failureErr
		}
		return documentresult.Ingest{Document: document, Job: submitted.Job}, err
	}
	return documentresult.Ingest{Document: document, Job: submitted.Job}, nil
}

func (service DocumentService) Retry(ctx context.Context, command documentcommand.Retry) (documentresult.Retry, error) {
	if service.Jobs == nil || service.Indexing == nil {
		return documentresult.Retry{}, errors.New("knowledge indexing service is not configured")
	}
	job, err := service.Jobs.Get(ctx, strings.TrimSpace(command.JobID))
	if err != nil {
		return documentresult.Retry{}, err
	}
	if err := service.startIndexing(job.ID); err != nil {
		if service.Async != nil {
			document, loadErr := service.Documents.Get(ctx, job.DocumentID)
			if loadErr != nil {
				return documentresult.Retry{Job: job}, errors.Join(fmt.Errorf("schedule document indexing: %w", err), fmt.Errorf("load document for failed scheduling state: %w", loadErr))
			}
			cause := fmt.Errorf("schedule document indexing: %w", err)
			failureErr := service.failDocumentAndJob(ctx, &document, &job, cause)
			return documentresult.Retry{Job: job}, failureErr
		}
		return documentresult.Retry{Job: job}, err
	}
	return documentresult.Retry{Job: job}, nil
}

func (service DocumentService) Delete(ctx context.Context, command documentcommand.Delete) error {
	if service.Documents == nil || service.Chunks == nil {
		return errors.New("knowledge document repositories are not configured")
	}
	document, err := service.Documents.Get(ctx, strings.TrimSpace(command.DocumentID))
	if err != nil {
		return err
	}
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		reason = "knowledge document deleted"
	}
	now := service.now()
	if err := service.Documents.MarkDeleted(ctx, document.ID, now, reason, document.SyncSequence); err != nil {
		return err
	}
	return service.Chunks.MarkDeletedByDocument(ctx, document.ID, now, document.SyncSequence)
}

// RecoverPending resumes jobs left pending or running by an unclean shutdown.
// One background task performs the scan result sequentially so startup does
// not overflow the shared executor queue.
func (service DocumentService) RecoverPending(ctx context.Context, limit int) error {
	if service.Jobs == nil || service.Indexing == nil {
		return errors.New("knowledge indexing service is not configured")
	}
	if limit < 1 {
		return errors.New("knowledge indexing recovery limit must be positive")
	}
	jobs, err := service.Jobs.ListPending(ctx, limit)
	if err != nil {
		return fmt.Errorf("list recoverable indexing jobs: %w", err)
	}
	if len(jobs) == 0 {
		return nil
	}
	run := func() error {
		var runErrors []error
		for _, job := range jobs {
			if _, runErr := service.Indexing.Run(context.Background(), indexingcommand.Run{JobID: job.ID}); runErr != nil {
				recoveryErr := fmt.Errorf("recover indexing job %q: %w", job.ID, runErr)
				runErrors = append(runErrors, recoveryErr)
				if service.OnAsyncError != nil {
					service.OnAsyncError(recoveryErr)
				}
			}
		}
		return errors.Join(runErrors...)
	}
	if service.Async == nil {
		return run()
	}
	if err := service.Async.Submit(func() { _ = run() }); err == nil {
		return nil
	} else {
		cause := fmt.Errorf("schedule indexing recovery: %w", err)
		failures := []error{cause}
		for index := range jobs {
			document, loadErr := service.Documents.Get(ctx, jobs[index].DocumentID)
			if loadErr != nil {
				failures = append(failures, fmt.Errorf("load document %q for recovery failure: %w", jobs[index].DocumentID, loadErr))
				continue
			}
			if failureErr := service.failDocumentAndJob(ctx, &document, &jobs[index], cause); failureErr != nil {
				failures = append(failures, fmt.Errorf("mark indexing job %q after recovery scheduling failure: %w", jobs[index].ID, failureErr))
			}
		}
		return errors.Join(failures...)
	}
}

func (service DocumentService) startIndexing(jobID string) error {
	if service.Async == nil {
		_, err := service.Indexing.Run(context.Background(), indexingcommand.Run{JobID: jobID})
		return err
	}
	return service.Async.Submit(func() {
		_, err := service.Indexing.Run(context.Background(), indexingcommand.Run{JobID: jobID})
		if err != nil && service.OnAsyncError != nil {
			service.OnAsyncError(err)
		}
	})
}

func (service DocumentService) validateDependencies() error {
	if service.KnowledgeBases == nil || service.Documents == nil || service.Objects == nil || service.ObjectCleanup == nil || service.Sources == nil {
		return errors.New("knowledge document ingest dependencies are not configured")
	}
	if service.Indexing == nil || service.Jobs == nil || service.States == nil || service.IDs == nil {
		return errors.New("knowledge indexing dependencies are not configured")
	}
	return nil
}

func (service DocumentService) cleanupUncommittedObject(ctx context.Context, objectKey string, cause error) error {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), failurePersistenceTimeout)
	defer cancel()
	if err := service.ObjectCleanup.DeleteUncommitted(cleanupContext, objectKey); err != nil {
		return errors.Join(cause, fmt.Errorf("remove uncommitted document object: %w", err))
	}
	return cause
}

func (service DocumentService) failDocument(ctx context.Context, document *domainknowledge.Document, cause error) error {
	persistenceContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), failurePersistenceTimeout)
	defer cancel()
	document.Status = domainknowledge.DocumentStatusFailed
	document.FailureReason = cause.Error()
	document.UpdatedAt = service.now()
	if err := service.Documents.Save(persistenceContext, *document); err != nil {
		return errors.Join(cause, fmt.Errorf("save failed document state: %w", err))
	}
	return cause
}

func (service DocumentService) failDocumentAndJob(ctx context.Context, document *domainknowledge.Document, job *domainknowledge.IndexingJob, cause error) error {
	persistenceContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), failurePersistenceTimeout)
	defer cancel()
	now := service.now()
	document.Status = domainknowledge.DocumentStatusFailed
	document.FailureReason = cause.Error()
	document.UpdatedAt = now
	job.Status = domainknowledge.IndexingJobStatusFailed
	job.LastError = cause.Error()
	job.CompletedAt = nil
	job.UpdatedAt = now
	if job.TotalChunks > job.CompletedChunks {
		job.FailedChunks = job.TotalChunks - job.CompletedChunks
	} else {
		job.FailedChunks = 0
	}
	if err := service.States.SaveDocumentAndJob(persistenceContext, *document, *job); err != nil {
		return errors.Join(cause, fmt.Errorf("save failed document indexing state: %w", err))
	}
	return cause
}

func (service DocumentService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func safeObjectName(fileName string) string {
	name := path.Base(strings.ReplaceAll(strings.TrimSpace(fileName), "\\", "/"))
	if name == "." || name == "/" || name == "" {
		return "document.bin"
	}
	return name
}
