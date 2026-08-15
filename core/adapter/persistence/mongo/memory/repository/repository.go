package repository

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"myai/core/adapter/persistence/mongo/memory/mapper"
	"myai/core/adapter/persistence/mongo/memory/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
)

const (
	memoriesCollection   = "ai_memories"
	candidatesCollection = "ai_memory_candidates"
	jobsCollection       = "ai_memory_extraction_jobs"
	dreamRunsCollection  = "ai_memory_dream_runs"
)

type Repository struct {
	template mongotemplate.Operations
	database *gomongo.Database
}

var _ memoryport.Store = (*Repository)(nil)

func New(client *gomongo.Client, database string) *Repository {
	if client == nil || strings.TrimSpace(database) == "" {
		return &Repository{template: mongotemplate.New(nil)}
	}
	target := client.Database(database)
	return &Repository{template: mongotemplate.New(target), database: target}
}

func (repository *Repository) EnsureIndexes(ctx context.Context) error {
	if repository == nil || repository.database == nil {
		return errors.New("mongo memory database is nil")
	}
	indexes := map[string][]gomongo.IndexModel{
		memoriesCollection: {
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "updated_at", Value: -1}}},
			{Keys: bson.D{{Key: "tags", Value: 1}}},
			{Keys: bson.D{{Key: "scope.type", Value: 1}, {Key: "scope.key", Value: 1}}},
		},
		candidatesCollection: {
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "updated_at", Value: -1}}},
			{Keys: bson.D{{Key: "origin_key", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
		},
		jobsCollection: {
			{Keys: bson.D{{Key: "agent_run_id", Value: 1}, {Key: "extractor_version", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "updated_at", Value: 1}}},
		},
		dreamRunsCollection: {
			{Keys: bson.D{{Key: "started_at", Value: -1}}},
		},
	}
	for collection, models := range indexes {
		if _, err := repository.database.Collection(collection).Indexes().CreateMany(ctx, models); err != nil {
			return err
		}
	}
	return nil
}

func (repository *Repository) Get(ctx context.Context, memoryID string) (domainmemory.Memory, error) {
	var document po.MemoryDocument
	if err := repository.template.FindOne(ctx, memoriesCollection, bson.M{"_id": strings.TrimSpace(memoryID)}, &document); err != nil {
		return domainmemory.Memory{}, translateError(err)
	}
	return mapper.MemoryDomainFromDocument(document), nil
}

func (repository *Repository) List(ctx context.Context, filter memoryport.ListFilter) ([]domainmemory.Memory, error) {
	query := bson.M{}
	if len(filter.Statuses) > 0 {
		query["status"] = bson.M{"$in": statusStrings(filter.Statuses)}
	} else if !filter.IncludeDeleted {
		query["status"] = bson.M{"$ne": string(domainmemory.StatusDeleted)}
	}
	if len(filter.Kinds) > 0 {
		query["kind"] = bson.M{"$in": kindStrings(filter.Kinds)}
	}
	if len(filter.ScopeTypes) > 0 {
		query["scope.type"] = bson.M{"$in": scopeTypeStrings(filter.ScopeTypes)}
	}
	if strings.TrimSpace(filter.ScopeKey) != "" {
		query["scope.key"] = strings.TrimSpace(filter.ScopeKey)
	}
	if len(filter.Tags) > 0 {
		query["tags"] = bson.M{"$all": filter.Tags}
	}
	if text := strings.TrimSpace(filter.Text); text != "" {
		regex := bson.M{"$regex": regexp.QuoteMeta(text), "$options": "i"}
		query["$or"] = bson.A{
			bson.M{"title": regex}, bson.M{"revisions.content.goal": regex},
			bson.M{"revisions.content.applicable_context": regex}, bson.M{"revisions.content.approach": regex},
			bson.M{"revisions.content.result": regex}, bson.M{"revisions.content.pain_points": regex},
			bson.M{"revisions.content.root_cause": regex}, bson.M{"revisions.content.lessons": regex},
			bson.M{"revisions.content.verification": regex},
		}
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var documents []po.MemoryDocument
	if err := repository.template.FindAll(ctx, memoriesCollection, query, &documents,
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetLimit(int64(limit))); err != nil {
		return nil, err
	}
	items := make([]domainmemory.Memory, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.MemoryDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) Save(ctx context.Context, memory domainmemory.Memory) error {
	if err := memory.Validate(); err != nil {
		return err
	}
	return repository.upsert(ctx, memoriesCollection, memory.ID, mapper.MemoryDocumentFromDomain(memory), []string{
		"human_locked", "supersedes_id", "use_count", "last_used_at", "deleted_at", "deletion_reason",
	})
}

func (repository *Repository) GetCandidate(ctx context.Context, candidateID string) (domainmemory.Candidate, error) {
	var document po.CandidateDocument
	if err := repository.template.FindOne(ctx, candidatesCollection, bson.M{"_id": strings.TrimSpace(candidateID)}, &document); err != nil {
		return domainmemory.Candidate{}, translateError(err)
	}
	return mapper.CandidateDomainFromDocument(document), nil
}

func (repository *Repository) ListCandidates(ctx context.Context, filter memoryport.CandidateFilter) ([]domainmemory.Candidate, error) {
	query := bson.M{}
	if len(filter.Statuses) > 0 {
		query["status"] = bson.M{"$in": candidateStatusStrings(filter.Statuses)}
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var documents []po.CandidateDocument
	if err := repository.template.FindAll(ctx, candidatesCollection, query, &documents,
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetLimit(int64(limit))); err != nil {
		return nil, err
	}
	items := make([]domainmemory.Candidate, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.CandidateDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) SaveCandidate(ctx context.Context, candidate domainmemory.Candidate) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	return repository.upsert(ctx, candidatesCollection, candidate.ID, mapper.CandidateDocumentFromDomain(candidate), []string{
		"origin_key", "target_memory_id", "review_note",
	})
}

func (repository *Repository) GetExtractionJob(ctx context.Context, jobID string) (domainmemory.ExtractionJob, error) {
	var document po.ExtractionJobDocument
	if err := repository.template.FindOne(ctx, jobsCollection, bson.M{"_id": strings.TrimSpace(jobID)}, &document); err != nil {
		return domainmemory.ExtractionJob{}, translateError(err)
	}
	return mapper.ExtractionJobDomainFromDocument(document), nil
}

func (repository *Repository) GetExtractionJobByRun(ctx context.Context, agentRunID string, extractorVersion string) (domainmemory.ExtractionJob, error) {
	var document po.ExtractionJobDocument
	if err := repository.template.FindOne(ctx, jobsCollection, bson.M{
		"agent_run_id": strings.TrimSpace(agentRunID), "extractor_version": strings.TrimSpace(extractorVersion),
	}, &document); err != nil {
		return domainmemory.ExtractionJob{}, translateError(err)
	}
	return mapper.ExtractionJobDomainFromDocument(document), nil
}

func (repository *Repository) ListExtractionJobs(ctx context.Context, statuses []domainmemory.JobStatus, limit int) ([]domainmemory.ExtractionJob, error) {
	query := bson.M{}
	if len(statuses) > 0 {
		query["status"] = bson.M{"$in": jobStatusStrings(statuses)}
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var documents []po.ExtractionJobDocument
	if err := repository.template.FindAll(ctx, jobsCollection, query, &documents,
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: 1}}).SetLimit(int64(limit))); err != nil {
		return nil, err
	}
	items := make([]domainmemory.ExtractionJob, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.ExtractionJobDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) SaveExtractionJob(ctx context.Context, job domainmemory.ExtractionJob) error {
	if err := job.Validate(); err != nil {
		return err
	}
	return repository.upsert(ctx, jobsCollection, job.ID, mapper.ExtractionJobDocumentFromDomain(job), []string{
		"attempts", "last_error", "completed_at",
	})
}

func (repository *Repository) GetDreamRun(ctx context.Context, runID string) (domainmemory.DreamRun, error) {
	var document po.DreamRunDocument
	if err := repository.template.FindOne(ctx, dreamRunsCollection, bson.M{"_id": strings.TrimSpace(runID)}, &document); err != nil {
		return domainmemory.DreamRun{}, translateError(err)
	}
	return mapper.DreamRunDomainFromDocument(document), nil
}

func (repository *Repository) ListDreamRuns(ctx context.Context, limit int) ([]domainmemory.DreamRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var documents []po.DreamRunDocument
	if err := repository.template.FindAll(ctx, dreamRunsCollection, bson.M{}, &documents,
		options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(int64(limit))); err != nil {
		return nil, err
	}
	items := make([]domainmemory.DreamRun, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.DreamRunDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) SaveDreamRun(ctx context.Context, run domainmemory.DreamRun) error {
	if err := run.Validate(); err != nil {
		return err
	}
	return repository.upsert(ctx, dreamRunsCollection, run.ID, mapper.DreamRunDocumentFromDomain(run), []string{
		"trigger", "candidate_count", "created_count", "merged_count", "superseded_count", "rejected_count",
		"actions", "last_error", "finished_at",
	})
}

func (repository *Repository) upsert(ctx context.Context, collection string, id string, document any, optionalFields []string) error {
	set, err := documentMap(document)
	if err != nil {
		return err
	}
	delete(set, "_id")
	unset := bson.M{}
	for _, field := range optionalFields {
		if _, ok := set[field]; !ok {
			unset[field] = ""
		}
	}
	update := bson.M{"$set": set, "$setOnInsert": bson.M{"_id": id}}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	_, err = repository.template.UpdateOne(ctx, collection, bson.M{"_id": id}, update, options.UpdateOne().SetUpsert(true))
	return translateError(err)
}

func documentMap(document any) (bson.M, error) {
	encoded, err := bson.Marshal(document)
	if err != nil {
		return nil, err
	}
	var result bson.M
	if err := bson.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func translateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, mongotemplate.ErrNotFound) || errors.Is(err, gomongo.ErrNoDocuments) {
		return memoryport.ErrNotFound
	}
	if gomongo.IsDuplicateKeyError(err) {
		return memoryport.ErrConflict
	}
	return err
}

func kindStrings(items []domainmemory.Kind) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, string(item))
	}
	return values
}

func statusStrings(items []domainmemory.Status) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, string(item))
	}
	return values
}

func scopeTypeStrings(items []domainmemory.ScopeType) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, string(item))
	}
	return values
}

func candidateStatusStrings(items []domainmemory.CandidateStatus) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, string(item))
	}
	return values
}

func jobStatusStrings(items []domainmemory.JobStatus) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, string(item))
	}
	return values
}
