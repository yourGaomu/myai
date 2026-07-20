package repository

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	knowledgemapper "myai/core/adapter/persistence/mongo/knowledge/mapper"
	"myai/core/adapter/persistence/mongo/knowledge/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type ProfileRepository struct {
	operations mongotemplate.Operations
}

var _ knowledgeport.ProfileRepository = (*ProfileRepository)(nil)

func NewProfileRepository(client *gomongo.Client, database string) *ProfileRepository {
	if client == nil || database == "" {
		return NewProfileRepositoryWithOperations(mongotemplate.New(nil))
	}
	return NewProfileRepositoryWithOperations(mongotemplate.New(client.Database(database)))
}

func NewProfileRepositoryWithOperations(operations mongotemplate.Operations) *ProfileRepository {
	if operations == nil {
		operations = mongotemplate.New(nil)
	}
	return &ProfileRepository{operations: operations}
}

func (repository *ProfileRepository) GetParsingProfile(ctx context.Context, profileID string) (domainknowledge.ParsingProfile, error) {
	var document po.ParsingProfileDocument
	if err := repository.operations.FindOne(ctx, parsingProfilesCollection, activeIDFilter(profileID), &document); err != nil {
		return domainknowledge.ParsingProfile{}, repositoryError(err)
	}
	return knowledgemapper.ParsingProfileDomainFromDocument(document), nil
}

func (repository *ProfileRepository) SaveParsingProfile(ctx context.Context, profile domainknowledge.ParsingProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("validate parsing profile: %w", err)
	}
	if profile.Deletion.Deleted {
		return fmt.Errorf("save cannot persist a deleted parsing profile; use MarkParsingProfileDeleted")
	}
	document := knowledgemapper.ParsingProfileDocumentFromDomain(profile)
	return repository.save(ctx, parsingProfilesCollection, document.ID, document.CreatedAt, bson.M{
		"name":           document.Name,
		"parser_id":      document.ParserID,
		"parser_version": document.ParserVersion,
		"ocr":            document.OCR,
		"options":        document.Options,
		"updated_at":     document.UpdatedAt,
	})
}

func (repository *ProfileRepository) GetEmbeddingProfile(ctx context.Context, profileID string) (domainknowledge.EmbeddingProfile, error) {
	var document po.EmbeddingProfileDocument
	if err := repository.operations.FindOne(ctx, embeddingProfilesCollection, activeIDFilter(profileID), &document); err != nil {
		return domainknowledge.EmbeddingProfile{}, repositoryError(err)
	}
	return knowledgemapper.EmbeddingProfileDomainFromDocument(document), nil
}

func (repository *ProfileRepository) SaveEmbeddingProfile(ctx context.Context, profile domainknowledge.EmbeddingProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("validate embedding profile: %w", err)
	}
	if profile.Deletion.Deleted {
		return fmt.Errorf("save cannot persist a deleted embedding profile; use MarkEmbeddingProfileDeleted")
	}
	document := knowledgemapper.EmbeddingProfileDocumentFromDomain(profile)
	return repository.save(ctx, embeddingProfilesCollection, document.ID, document.CreatedAt, bson.M{
		"name":          document.Name,
		"model_id":      document.ModelID,
		"provider":      document.Provider,
		"model":         document.Model,
		"model_version": document.ModelVersion,
		"dimensions":    document.Dimensions,
		"normalize":     document.Normalize,
		"input_mode":    document.InputMode,
		"updated_at":    document.UpdatedAt,
	})
}

func (repository *ProfileRepository) GetChunkingProfile(ctx context.Context, profileID string) (domainknowledge.ChunkingProfile, error) {
	var document po.ChunkingProfileDocument
	if err := repository.operations.FindOne(ctx, chunkingProfilesCollection, activeIDFilter(profileID), &document); err != nil {
		return domainknowledge.ChunkingProfile{}, repositoryError(err)
	}
	return knowledgemapper.ChunkingProfileDomainFromDocument(document), nil
}

func (repository *ProfileRepository) SaveChunkingProfile(ctx context.Context, profile domainknowledge.ChunkingProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("validate chunking profile: %w", err)
	}
	if profile.Deletion.Deleted {
		return fmt.Errorf("save cannot persist a deleted chunking profile; use MarkChunkingProfileDeleted")
	}
	document := knowledgemapper.ChunkingProfileDocumentFromDomain(profile)
	return repository.save(ctx, chunkingProfilesCollection, document.ID, document.CreatedAt, bson.M{
		"name":             document.Name,
		"strategy_id":      document.StrategyID,
		"strategy_version": document.StrategyVersion,
		"max_chunk_size":   document.MaxChunkSize,
		"overlap":          document.Overlap,
		"tokenizer":        document.Tokenizer,
		"options":          document.Options,
		"updated_at":       document.UpdatedAt,
	})
}

func (repository *ProfileRepository) GetIndexProfile(ctx context.Context, profileID string) (domainknowledge.IndexProfile, error) {
	var document po.IndexProfileDocument
	if err := repository.operations.FindOne(ctx, indexProfilesCollection, activeIDFilter(profileID), &document); err != nil {
		return domainknowledge.IndexProfile{}, repositoryError(err)
	}
	return knowledgemapper.IndexProfileDomainFromDocument(document), nil
}

func (repository *ProfileRepository) SaveIndexProfile(ctx context.Context, profile domainknowledge.IndexProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("validate index profile: %w", err)
	}
	if profile.Deletion.Deleted {
		return fmt.Errorf("save cannot persist a deleted index profile; use MarkIndexProfileDeleted")
	}
	document := knowledgemapper.IndexProfileDocumentFromDomain(profile)
	return repository.save(ctx, indexProfilesCollection, document.ID, document.CreatedAt, bson.M{
		"name":                 document.Name,
		"parsing_profile_id":   document.ParsingProfileID,
		"chunking_profile_id":  document.ChunkingProfileID,
		"embedding_profile_id": document.EmbeddingProfileID,
		"distance_metric_id":   document.DistanceMetricID,
		"vector_index_options": document.VectorIndexOptions,
		"status":               document.Status,
		"failure_reason":       document.FailureReason,
		"updated_at":           document.UpdatedAt,
	})
}

func (repository *ProfileRepository) ListIndexProfiles(ctx context.Context, includeDeleted bool) ([]domainknowledge.IndexProfile, error) {
	filter := activeFilter()
	if includeDeleted {
		filter = bson.M{}
	}
	var documents []po.IndexProfileDocument
	if err := repository.operations.FindAll(ctx, indexProfilesCollection, filter, &documents, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}})); err != nil {
		return nil, repositoryError(err)
	}
	profiles := make([]domainknowledge.IndexProfile, 0, len(documents))
	for _, document := range documents {
		profiles = append(profiles, knowledgemapper.IndexProfileDomainFromDocument(document))
	}
	return profiles, nil
}

func (repository *ProfileRepository) MarkParsingProfileDeleted(ctx context.Context, profileID string, deletedAt time.Time, reason string) error {
	return repository.markDeleted(ctx, parsingProfilesCollection, profileID, deletedAt, reason)
}

func (repository *ProfileRepository) MarkEmbeddingProfileDeleted(ctx context.Context, profileID string, deletedAt time.Time, reason string) error {
	return repository.markDeleted(ctx, embeddingProfilesCollection, profileID, deletedAt, reason)
}

func (repository *ProfileRepository) MarkChunkingProfileDeleted(ctx context.Context, profileID string, deletedAt time.Time, reason string) error {
	return repository.markDeleted(ctx, chunkingProfilesCollection, profileID, deletedAt, reason)
}

func (repository *ProfileRepository) MarkIndexProfileDeleted(ctx context.Context, profileID string, deletedAt time.Time, reason string) error {
	return repository.markDeleted(ctx, indexProfilesCollection, profileID, deletedAt, reason)
}

func (repository *ProfileRepository) save(ctx context.Context, collection string, id string, createdAt time.Time, values bson.M) error {
	if id == "" {
		return fmt.Errorf("profile id is required")
	}
	_, err := repository.operations.UpdateOne(
		ctx,
		collection,
		activeIDFilter(id),
		bson.M{
			"$set": values,
			"$setOnInsert": bson.M{
				"_id":        id,
				"deleted":    false,
				"created_at": createdAt,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (repository *ProfileRepository) markDeleted(ctx context.Context, collection string, profileID string, deletedAt time.Time, reason string) error {
	if profileID == "" {
		return fmt.Errorf("profile id is required")
	}
	result, err := repository.operations.UpdateOne(
		ctx,
		collection,
		bson.M{"_id": profileID},
		bson.M{"$set": bson.M{
			"deleted":       true,
			"deleted_at":    deletedAt,
			"delete_reason": reason,
			"updated_at":    deletedAt,
		}},
	)
	return matchedOrNotFound(result, err)
}
