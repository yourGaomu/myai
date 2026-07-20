package repository

import (
	"context"
	"fmt"
	"strings"
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

type ChunkRepository struct {
	operations mongotemplate.Operations
}

var _ knowledgeport.ChunkRepository = (*ChunkRepository)(nil)

func NewChunkRepository(client *gomongo.Client, database string) *ChunkRepository {
	if client == nil || database == "" {
		return NewChunkRepositoryWithOperations(mongotemplate.New(nil))
	}
	return NewChunkRepositoryWithOperations(mongotemplate.New(client.Database(database)))
}

func NewChunkRepositoryWithOperations(operations mongotemplate.Operations) *ChunkRepository {
	if operations == nil {
		operations = mongotemplate.New(nil)
	}
	return &ChunkRepository{operations: operations}
}

func (repository *ChunkRepository) SaveAll(ctx context.Context, chunks []domainknowledge.Chunk) error {
	seen := make(map[string]struct{}, len(chunks))
	for _, chunk := range chunks {
		if err := chunk.Validate(); err != nil {
			return fmt.Errorf("validate knowledge chunk %q: %w", chunk.ID, err)
		}
		if chunk.Deletion.Deleted {
			return fmt.Errorf("save cannot persist deleted knowledge chunk %q; use MarkDeletedByDocument", chunk.ID)
		}
		if _, exists := seen[chunk.ID]; exists {
			return fmt.Errorf("knowledge chunk %q is duplicated", chunk.ID)
		}
		seen[chunk.ID] = struct{}{}
	}

	for _, chunk := range chunks {
		persistent := knowledgemapper.ChunkDocumentFromDomain(chunk)
		setOnInsert := bson.M{
			"_id":        persistent.ID,
			"deleted":    false,
			"created_at": persistent.CreatedAt,
		}
		if persistent.EmbeddingProfileIDs != nil {
			setOnInsert["embedding_profile_ids"] = persistent.EmbeddingProfileIDs
		}
		_, err := repository.operations.UpdateOne(
			ctx,
			chunksCollection,
			activeIDFilter(persistent.ID),
			bson.M{
				"$set": bson.M{
					"knowledge_base_id":   persistent.KnowledgeBaseID,
					"document_id":         persistent.DocumentID,
					"document_version":    persistent.DocumentVersion,
					"parsing_profile_id":  persistent.ParsingProfileID,
					"chunking_profile_id": persistent.ChunkingProfileID,
					"ordinal":             persistent.Ordinal,
					"text":                persistent.Text,
					"content_hash":        persistent.ContentHash,
					"start_offset":        persistent.StartOffset,
					"end_offset":          persistent.EndOffset,
					"source_page":         persistent.SourcePage,
					"source_heading":      persistent.SourceHeading,
					"sync_sequence":       persistent.SyncSequence,
					"updated_at":          persistent.UpdatedAt,
				},
				"$setOnInsert": setOnInsert,
			},
			options.UpdateOne().SetUpsert(true),
		)
		if err != nil {
			return fmt.Errorf("save knowledge chunk %q: %w", persistent.ID, err)
		}
	}
	return nil
}

func (repository *ChunkRepository) GetByIDs(ctx context.Context, chunkIDs []string) ([]domainknowledge.Chunk, error) {
	ids, err := normalizeIDs(chunkIDs, "chunk")
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []domainknowledge.Chunk{}, nil
	}
	filter := activeFilter()
	filter["_id"] = bson.M{"$in": ids}

	var documents []po.ChunkDocument
	if err := repository.operations.FindAll(ctx, chunksCollection, filter, &documents); err != nil {
		return nil, repositoryError(err)
	}
	byID := make(map[string]po.ChunkDocument, len(documents))
	for _, document := range documents {
		byID[document.ID] = document
	}
	result := make([]domainknowledge.Chunk, 0, len(documents))
	for _, id := range ids {
		if document, ok := byID[id]; ok {
			result = append(result, knowledgemapper.ChunkDomainFromDocument(document))
		}
	}
	return result, nil
}

func (repository *ChunkRepository) ListByDocumentPage(ctx context.Context, documentID string, documentVersion int64, parsingProfileID string, chunkingProfileID string, afterOrdinal int, limit int) ([]domainknowledge.Chunk, error) {
	if strings.TrimSpace(documentID) == "" {
		return nil, fmt.Errorf("document id is required")
	}
	if documentVersion < 1 {
		return nil, fmt.Errorf("document version must be at least 1")
	}
	if strings.TrimSpace(parsingProfileID) == "" {
		return nil, fmt.Errorf("parsing profile id is required")
	}
	if strings.TrimSpace(chunkingProfileID) == "" {
		return nil, fmt.Errorf("chunking profile id is required")
	}
	if afterOrdinal < -1 {
		return nil, fmt.Errorf("after ordinal must be at least -1")
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be positive")
	}
	filter := activeFilter()
	filter["document_id"] = documentID
	filter["document_version"] = documentVersion
	filter["parsing_profile_id"] = parsingProfileID
	filter["chunking_profile_id"] = chunkingProfileID
	filter["ordinal"] = bson.M{"$gt": afterOrdinal}

	var documents []po.ChunkDocument
	if err := repository.operations.FindAll(
		ctx,
		chunksCollection,
		filter,
		&documents,
		options.Find().SetSort(bson.D{{Key: "ordinal", Value: 1}}).SetLimit(int64(limit)),
	); err != nil {
		return nil, repositoryError(err)
	}
	result := make([]domainknowledge.Chunk, 0, len(documents))
	for _, document := range documents {
		result = append(result, knowledgemapper.ChunkDomainFromDocument(document))
	}
	return result, nil
}

func (repository *ChunkRepository) ListMissingEmbeddings(ctx context.Context, parsingProfileID string, chunkingProfileID string, embeddingProfileID string, limit int) ([]domainknowledge.Chunk, error) {
	if strings.TrimSpace(parsingProfileID) == "" {
		return nil, fmt.Errorf("parsing profile id is required")
	}
	if strings.TrimSpace(chunkingProfileID) == "" {
		return nil, fmt.Errorf("chunking profile id is required")
	}
	if strings.TrimSpace(embeddingProfileID) == "" {
		return nil, fmt.Errorf("embedding profile id is required")
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be positive")
	}
	filter := activeFilter()
	filter["parsing_profile_id"] = parsingProfileID
	filter["chunking_profile_id"] = chunkingProfileID
	filter["embedding_profile_ids"] = bson.M{"$ne": embeddingProfileID}

	var documents []po.ChunkDocument
	if err := repository.operations.FindAll(
		ctx,
		chunksCollection,
		filter,
		&documents,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "ordinal", Value: 1}}).SetLimit(int64(limit)),
	); err != nil {
		return nil, repositoryError(err)
	}
	result := make([]domainknowledge.Chunk, 0, len(documents))
	for _, document := range documents {
		result = append(result, knowledgemapper.ChunkDomainFromDocument(document))
	}
	return result, nil
}

func (repository *ChunkRepository) MarkEmbedded(ctx context.Context, chunkIDs []string, embeddingProfileID string, updatedAt time.Time) error {
	ids, err := normalizeIDs(chunkIDs, "chunk")
	if err != nil {
		return err
	}
	if strings.TrimSpace(embeddingProfileID) == "" {
		return fmt.Errorf("embedding profile id is required")
	}
	if len(ids) == 0 {
		return nil
	}
	filter := activeFilter()
	filter["_id"] = bson.M{"$in": ids}
	_, err = repository.operations.UpdateMany(
		ctx,
		chunksCollection,
		filter,
		bson.M{"$addToSet": bson.M{"embedding_profile_ids": embeddingProfileID}, "$set": bson.M{"updated_at": updatedAt}},
	)
	if err != nil {
		return fmt.Errorf("mark chunks embedded for profile %q: %w", embeddingProfileID, repositoryError(err))
	}
	return nil
}

func (repository *ChunkRepository) MarkDeletedByDocument(ctx context.Context, documentID string, deletedAt time.Time, syncSequence int64) error {
	if strings.TrimSpace(documentID) == "" {
		return fmt.Errorf("document id is required")
	}
	if syncSequence < 0 {
		return fmt.Errorf("sync sequence must not be negative")
	}
	filter := activeFilter()
	filter["document_id"] = documentID
	_, err := repository.operations.UpdateMany(
		ctx,
		chunksCollection,
		filter,
		bson.M{"$set": bson.M{
			"deleted":       true,
			"deleted_at":    deletedAt,
			"delete_reason": "document deleted",
			"sync_sequence": syncSequence,
			"updated_at":    deletedAt,
		}},
	)
	if err != nil {
		return fmt.Errorf("mark chunks deleted for document %q: %w", documentID, repositoryError(err))
	}
	return nil
}

func normalizeIDs(ids []string, resource string) ([]string, error) {
	result := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("%s id must not be empty", resource)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}
