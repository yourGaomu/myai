package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	indexingapi "myai/core/application/knowledge/indexing/api"
	indexingcommand "myai/core/application/knowledge/indexing/command"
	indexingresult "myai/core/application/knowledge/indexing/result"
	domainknowledge "myai/core/domain/knowledge"
	documentprocessorport "myai/core/port/knowledge/documentprocessor"
)

const failurePersistenceTimeout = 5 * time.Second

type IndexingService struct {
	configuration Configuration
	runningMu     sync.Mutex
	running       map[string]struct{}
}

var _ indexingapi.Service = (*IndexingService)(nil)

func New(configuration Configuration) (*IndexingService, error) {
	configuration = configuration.normalize()
	if err := configuration.validate(); err != nil {
		return nil, err
	}
	return &IndexingService{
		configuration: configuration,
		running:       make(map[string]struct{}),
	}, nil
}

func (service *IndexingService) Submit(ctx context.Context, command indexingcommand.Submit) (indexingresult.Submit, error) {
	documentID := strings.TrimSpace(command.DocumentID)
	if documentID == "" {
		return indexingresult.Submit{}, fmt.Errorf("indexing document id is required")
	}
	indexProfileID := strings.TrimSpace(command.IndexProfileID)
	if indexProfileID == "" {
		return indexingresult.Submit{}, fmt.Errorf("indexing profile id is required")
	}

	document, err := service.configuration.Documents.Get(ctx, documentID)
	if err != nil {
		return indexingresult.Submit{}, fmt.Errorf("load indexing document %q: %w", documentID, err)
	}
	if err := validateIndexableDocument(document); err != nil {
		return indexingresult.Submit{}, err
	}
	profile, err := service.configuration.Profiles.GetIndexProfile(ctx, indexProfileID)
	if err != nil {
		return indexingresult.Submit{}, fmt.Errorf("load index profile %q: %w", indexProfileID, err)
	}
	if err := validateActiveIndexProfile(profile); err != nil {
		return indexingresult.Submit{}, err
	}

	now := service.now()
	job := domainknowledge.IndexingJob{
		ID:              strings.TrimSpace(service.configuration.JobIDs.NewID()),
		KnowledgeBaseID: document.KnowledgeBaseID,
		DocumentID:      document.ID,
		IndexProfileID:  profile.ID,
		Stage:           domainknowledge.IndexingStageParse,
		Status:          domainknowledge.IndexingJobStatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := job.Validate(); err != nil {
		return indexingresult.Submit{}, fmt.Errorf("build indexing job: %w", err)
	}
	if err := service.configuration.States.SaveDocumentAndJob(ctx, document, job); err != nil {
		return indexingresult.Submit{}, fmt.Errorf("save submitted indexing state: %w", err)
	}
	return indexingresult.Submit{Job: job}, nil
}

func (service *IndexingService) Run(ctx context.Context, command indexingcommand.Run) (indexingresult.Run, error) {
	jobID := strings.TrimSpace(command.JobID)
	if jobID == "" {
		return indexingresult.Run{}, fmt.Errorf("indexing job id is required")
	}
	if !service.start(jobID) {
		return indexingresult.Run{}, fmt.Errorf("indexing job %q is already running", jobID)
	}
	defer service.finish(jobID)

	job, err := service.configuration.Jobs.Get(ctx, jobID)
	if err != nil {
		return indexingresult.Run{}, fmt.Errorf("load indexing job %q: %w", jobID, err)
	}
	document, err := service.configuration.Documents.Get(ctx, job.DocumentID)
	if err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, nil, fmt.Errorf("load indexing document %q: %w", job.DocumentID, err))
	}
	if err := validateJobDocument(job, document); err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, err)
	}
	if job.Status == domainknowledge.IndexingJobStatusCompleted {
		return indexingresult.Run{Job: job, Document: document}, nil
	}

	profiles, err := service.loadProfiles(ctx, job.IndexProfileID)
	if err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, err)
	}

	if job.Status == domainknowledge.IndexingJobStatusRunning || job.Status == domainknowledge.IndexingJobStatusFailed {
		job.RetryCount++
	}
	job.Status = domainknowledge.IndexingJobStatusRunning
	job.Stage = domainknowledge.IndexingStageParse
	job.TotalChunks = 0
	job.CompletedChunks = 0
	job.FailedChunks = 0
	job.LastError = ""
	job.CompletedAt = nil
	document.Status = domainknowledge.DocumentStatusParsing
	document.FailureReason = ""
	if err := service.saveProgress(ctx, &job, &document); err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, err)
	}

	summary, err := service.processDocument(ctx, &job, &document, profiles)
	if err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, err)
	}
	job.TotalChunks = summary.ChunkCount
	job.Stage = domainknowledge.IndexingStageEmbed
	document.Status = domainknowledge.DocumentStatusEmbedding
	if err := service.saveProgress(ctx, &job, &document); err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, err)
	}

	provider, err := service.configuration.Embeddings.Resolve(profiles.embedding)
	if err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, fmt.Errorf("resolve embedding profile %q: %w", profiles.embedding.ID, err))
	}
	if provider == nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, fmt.Errorf("embedding profile %q resolved to nil provider", profiles.embedding.ID))
	}
	if err := service.configuration.Vectors.EnsureIndex(ctx, domainknowledge.VectorIndexDefinition{
		EmbeddingProfileID: profiles.embedding.ID,
		Dimensions:         profiles.embedding.Dimensions,
		DistanceMetricID:   profiles.index.DistanceMetricID,
		Options:            cloneOptions(profiles.index.VectorIndexOptions),
	}); err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, fmt.Errorf("ensure vector index: %w", err))
	}
	if err := service.embedChunks(ctx, &job, &document, profiles, provider); err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, err)
	}

	job.Stage = domainknowledge.IndexingStageIndex
	document.Status = domainknowledge.DocumentStatusIndexing
	if err := service.saveProgress(ctx, &job, &document); err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, err)
	}
	if err := service.indexKeywords(ctx, document, profiles, job.TotalChunks); err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, err)
	}

	completedAt := service.now()
	job.Stage = domainknowledge.IndexingStageCompleted
	job.Status = domainknowledge.IndexingJobStatusCompleted
	job.CompletedChunks = job.TotalChunks
	job.FailedChunks = 0
	job.LastError = ""
	job.CompletedAt = &completedAt
	document.Status = domainknowledge.DocumentStatusReady
	document.FailureReason = ""
	if err := service.saveProgress(ctx, &job, &document); err != nil {
		return indexingresult.Run{}, service.fail(ctx, &job, &document, err)
	}

	return indexingresult.Run{Job: job, Document: document, Processed: &summary}, nil
}

func cloneOptions(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (service *IndexingService) processDocument(ctx context.Context, job *domainknowledge.IndexingJob, document *domainknowledge.Document, profiles loadedProfiles) (domainknowledge.ProcessingSummary, error) {
	content, err := service.configuration.Objects.Open(ctx, document.ObjectKey)
	if err != nil {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("open document object %q: %w", document.ObjectKey, err)
	}

	sink := &chunkPersistenceSink{
		repository:      service.configuration.Chunks,
		stableIDs:       service.configuration.StableIDs,
		document:        *document,
		parsingProfile:  profiles.parsing,
		chunkingProfile: profiles.chunking,
		now:             service.configuration.Now,
		onFirstBatch: func(batchContext context.Context) error {
			job.Stage = domainknowledge.IndexingStageChunk
			document.Status = domainknowledge.DocumentStatusChunking
			return service.saveProgress(batchContext, job, document)
		},
	}
	summary, processErr := service.configuration.Processor.Process(ctx, documentprocessorport.Request{
		Document:        *document,
		ParsingProfile:  profiles.parsing,
		ChunkingProfile: profiles.chunking,
		Content:         content,
	}, sink)
	closeErr := content.Close()
	if processErr != nil || closeErr != nil {
		return domainknowledge.ProcessingSummary{}, errors.Join(processErr, closeErr)
	}
	if err := validateProcessingSummary(summary, *document, profiles); err != nil {
		return domainknowledge.ProcessingSummary{}, err
	}
	if sink.count != summary.ChunkCount {
		return domainknowledge.ProcessingSummary{}, fmt.Errorf("persisted %d chunks, processor reported %d", sink.count, summary.ChunkCount)
	}
	return summary, nil
}

func validateProcessingSummary(summary domainknowledge.ProcessingSummary, document domainknowledge.Document, profiles loadedProfiles) error {
	if err := summary.Validate(); err != nil {
		return fmt.Errorf("validate document processing summary: %w", err)
	}
	if summary.DocumentID != document.ID || summary.DocumentVersion != document.Version {
		return fmt.Errorf("document processing summary does not match document version")
	}
	if strings.TrimSpace(summary.ContentType) != strings.TrimSpace(document.ContentType) {
		return fmt.Errorf("document processing summary content type %q does not match %q", summary.ContentType, document.ContentType)
	}
	if summary.ParserID != profiles.parsing.ParserID || summary.ParserVersion != profiles.parsing.ParserVersion {
		return fmt.Errorf("document processing summary parser does not match parsing profile %q", profiles.parsing.ID)
	}
	if summary.ChunkingStrategyID != profiles.chunking.StrategyID || summary.ChunkingStrategyVersion != profiles.chunking.StrategyVersion {
		return fmt.Errorf("document processing summary strategy does not match chunking profile %q", profiles.chunking.ID)
	}
	return nil
}

func (service *IndexingService) start(jobID string) bool {
	service.runningMu.Lock()
	defer service.runningMu.Unlock()
	if _, exists := service.running[jobID]; exists {
		return false
	}
	service.running[jobID] = struct{}{}
	return true
}

func (service *IndexingService) finish(jobID string) {
	service.runningMu.Lock()
	delete(service.running, jobID)
	service.runningMu.Unlock()
}

func (service *IndexingService) now() time.Time {
	return service.configuration.Now().UTC()
}
