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

type DocumentService struct {
	KnowledgeBases knowledgeport.KnowledgeBaseRepository
	Documents      knowledgeport.DocumentRepository
	Chunks         knowledgeport.ChunkRepository
	Objects        knowledgeport.DocumentObjectStore
	Sources        knowledgeport.DocumentSourceReader
	Indexing       indexingapi.Service
	Jobs           knowledgeport.IndexingJobRepository
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
	documentID := strings.TrimSpace(service.IDs.NewID())
	if documentID == "" {
		return documentresult.Ingest{}, errors.New("document id generator returned an empty id")
	}
	objectKey := path.Join("knowledge", knowledgeBaseID, documentID, "v1", safeObjectName(content.FileName))
	object, err := service.Objects.Put(ctx, objectKey, bytes.NewReader(content.Data), content.Size, content.ContentType)
	if err != nil {
		return documentresult.Ingest{}, err
	}
	now := service.now()
	document := domainknowledge.Document{
		ID: documentID, KnowledgeBaseID: knowledgeBaseID, FileName: content.FileName,
		ContentType: content.ContentType, ObjectKey: object.ObjectKey, ContentHash: object.ContentHash,
		Version: 1, Status: domainknowledge.DocumentStatusUploaded, CreatedAt: now, UpdatedAt: now,
	}
	if err := document.Validate(); err != nil {
		return documentresult.Ingest{}, err
	}
	if err := service.Documents.Save(ctx, document); err != nil {
		return documentresult.Ingest{}, err
	}
	submitted, err := service.Indexing.Submit(ctx, indexingcommand.Submit{DocumentID: document.ID, IndexProfileID: base.ActiveIndexProfileID})
	if err != nil {
		return documentresult.Ingest{}, err
	}
	if err := service.startIndexing(submitted.Job.ID); err != nil {
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
	if service.KnowledgeBases == nil || service.Documents == nil || service.Objects == nil || service.Sources == nil {
		return errors.New("knowledge document ingest dependencies are not configured")
	}
	if service.Indexing == nil || service.IDs == nil {
		return errors.New("knowledge indexing dependencies are not configured")
	}
	return nil
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
