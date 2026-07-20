package mapper

import (
	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func IndexingJobDocumentFromDomain(job domainknowledge.IndexingJob) po.IndexingJobDocument {
	return po.IndexingJobDocument{
		ID:              job.ID,
		KnowledgeBaseID: job.KnowledgeBaseID,
		DocumentID:      job.DocumentID,
		IndexProfileID:  job.IndexProfileID,
		Stage:           string(job.Stage),
		Status:          string(job.Status),
		TotalChunks:     job.TotalChunks,
		CompletedChunks: job.CompletedChunks,
		FailedChunks:    job.FailedChunks,
		LastError:       job.LastError,
		RetryCount:      job.RetryCount,
		CreatedAt:       job.CreatedAt,
		UpdatedAt:       job.UpdatedAt,
		CompletedAt:     cloneTime(job.CompletedAt),
	}
}

func IndexingJobDomainFromDocument(document po.IndexingJobDocument) domainknowledge.IndexingJob {
	return domainknowledge.IndexingJob{
		ID:              document.ID,
		KnowledgeBaseID: document.KnowledgeBaseID,
		DocumentID:      document.DocumentID,
		IndexProfileID:  document.IndexProfileID,
		Stage:           domainknowledge.IndexingStage(document.Stage),
		Status:          domainknowledge.IndexingJobStatus(document.Status),
		TotalChunks:     document.TotalChunks,
		CompletedChunks: document.CompletedChunks,
		FailedChunks:    document.FailedChunks,
		LastError:       document.LastError,
		RetryCount:      document.RetryCount,
		CreatedAt:       document.CreatedAt,
		UpdatedAt:       document.UpdatedAt,
		CompletedAt:     cloneTime(document.CompletedAt),
	}
}
