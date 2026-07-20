package milvus

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type Store struct {
	client           client
	collectionPrefix string
	shards           int32
	locksMu          sync.Mutex
	locks            map[string]*sync.Mutex
}

var _ knowledgeport.VectorStore = (*Store)(nil)

func New(ctx context.Context, config Config) (*Store, error) {
	config = config.normalize()
	if err := config.validate(); err != nil {
		return nil, err
	}
	runtimeClient, err := milvusclient.NewClient(ctx, milvusclient.Config{
		Address:       config.Address,
		Username:      config.Username,
		Password:      config.Password,
		DBName:        config.Database,
		APIKey:        config.APIKey,
		EnableTLSAuth: config.EnableTLS,
	})
	if err != nil {
		return nil, fmt.Errorf("connect Milvus: %w", err)
	}
	return newWithClient(runtimeClient, config), nil
}

func newWithClient(runtimeClient client, config Config) *Store {
	config = config.normalize()
	return &Store{
		client:           runtimeClient,
		collectionPrefix: config.CollectionPrefix,
		shards:           config.Shards,
		locks:            make(map[string]*sync.Mutex),
	}
}

func (store *Store) EnsureIndex(ctx context.Context, definition domainknowledge.VectorIndexDefinition) error {
	if store == nil || store.client == nil {
		return fmt.Errorf("Milvus vector store is not initialized")
	}
	if err := definition.Validate(); err != nil {
		return fmt.Errorf("validate vector index definition: %w", err)
	}
	name := collectionName(store.collectionPrefix, definition.EmbeddingProfileID)
	lock := store.collectionLock(name)
	lock.Lock()
	defer lock.Unlock()

	exists, err := store.client.HasCollection(ctx, name)
	if err != nil {
		return fmt.Errorf("check Milvus collection %q: %w", name, err)
	}
	var collection *entity.Collection
	if !exists {
		if err := store.client.CreateCollection(ctx, collectionSchema(name, definition.Dimensions), store.shards); err != nil {
			return fmt.Errorf("create Milvus collection %q: %w", name, err)
		}
	} else {
		collection, err = store.client.DescribeCollection(ctx, name)
		if err != nil {
			return fmt.Errorf("describe Milvus collection %q: %w", name, err)
		}
		if err := validateCollection(collection, definition.Dimensions); err != nil {
			return err
		}
	}

	desiredIndex, err := buildIndex(definition)
	if err != nil {
		return err
	}
	indexes, err := store.client.DescribeIndex(ctx, name, vectorField)
	if err != nil {
		return fmt.Errorf("describe Milvus index for %q: %w", name, err)
	}
	compatible := len(indexes) == 1 && indexesCompatible(indexes[0], desiredIndex)
	if !compatible && len(indexes) > 0 {
		if collection != nil && collection.Loaded {
			if err := store.client.ReleaseCollection(ctx, name); err != nil {
				return fmt.Errorf("release Milvus collection %q: %w", name, err)
			}
		}
		if err := store.client.DropIndex(ctx, name, vectorField); err != nil {
			return fmt.Errorf("drop Milvus index for %q: %w", name, err)
		}
	}
	if !compatible {
		if err := store.client.CreateIndex(ctx, name, vectorField, desiredIndex, false); err != nil {
			return fmt.Errorf("create Milvus index for %q: %w", name, err)
		}
	}
	if collection == nil || !collection.Loaded || !compatible {
		if err := store.client.LoadCollection(ctx, name, false); err != nil {
			return fmt.Errorf("load Milvus collection %q: %w", name, err)
		}
	}
	return nil
}

func (store *Store) Upsert(ctx context.Context, embeddings []domainknowledge.EmbeddingVector) error {
	if store == nil || store.client == nil {
		return fmt.Errorf("Milvus vector store is not initialized")
	}
	if len(embeddings) == 0 {
		return nil
	}
	groups := make(map[string][]domainknowledge.EmbeddingVector)
	dimensions := make(map[string]int)
	seen := make(map[string]struct{}, len(embeddings))
	for _, embedding := range embeddings {
		if err := embedding.Validate(); err != nil {
			return fmt.Errorf("validate embedding vector %q: %w", embedding.ID, err)
		}
		if _, exists := seen[embedding.ID]; exists {
			return fmt.Errorf("embedding vector id %q is duplicated", embedding.ID)
		}
		seen[embedding.ID] = struct{}{}
		if current, exists := dimensions[embedding.EmbeddingProfileID]; exists && current != embedding.Dimensions {
			return fmt.Errorf("embedding profile %q contains mixed dimensions", embedding.EmbeddingProfileID)
		}
		dimensions[embedding.EmbeddingProfileID] = embedding.Dimensions
		groups[embedding.EmbeddingProfileID] = append(groups[embedding.EmbeddingProfileID], embedding)
	}
	for profileID, group := range groups {
		name := collectionName(store.collectionPrefix, profileID)
		exists, err := store.client.HasCollection(ctx, name)
		if err != nil {
			return fmt.Errorf("check Milvus collection %q: %w", name, err)
		}
		if !exists {
			return fmt.Errorf("Milvus collection %q is missing; call EnsureIndex before Upsert", name)
		}
		if _, err := store.client.Upsert(ctx, name, "", embeddingColumns(group)...); err != nil {
			return fmt.Errorf("upsert %d vectors into Milvus collection %q: %w", len(group), name, err)
		}
	}
	return nil
}

func (store *Store) Search(ctx context.Context, query domainknowledge.VectorQuery) ([]domainknowledge.VectorHit, error) {
	if store == nil || store.client == nil {
		return nil, fmt.Errorf("Milvus vector store is not initialized")
	}
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("validate vector query: %w", err)
	}
	metric, err := metricType(query.DistanceMetricID)
	if err != nil {
		return nil, err
	}
	searchParam, err := buildSearchParam(query.Options)
	if err != nil {
		return nil, err
	}
	name := collectionName(store.collectionPrefix, query.EmbeddingProfileID)
	results, err := store.client.Search(
		ctx,
		name,
		nil,
		searchExpression(query),
		[]string{chunkIDField},
		[]entity.Vector{entity.FloatVector(query.Vector)},
		vectorField,
		metric,
		query.TopK,
		searchParam,
	)
	if err != nil {
		return nil, fmt.Errorf("search Milvus collection %q: %w", name, err)
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("Milvus returned %d result sets for one query", len(results))
	}
	if results[0].Err != nil {
		return nil, fmt.Errorf("Milvus vector search failed: %w", results[0].Err)
	}
	chunkIDs := results[0].Fields.GetColumn(chunkIDField)
	if chunkIDs == nil {
		return nil, fmt.Errorf("Milvus search result is missing %s", chunkIDField)
	}
	hits := make([]domainknowledge.VectorHit, 0, results[0].ResultCount)
	for index := 0; index < results[0].ResultCount; index++ {
		embeddingID, err := results[0].IDs.GetAsString(index)
		if err != nil {
			return nil, fmt.Errorf("read Milvus embedding id at rank %d: %w", index, err)
		}
		chunkID, err := chunkIDs.GetAsString(index)
		if err != nil {
			return nil, fmt.Errorf("read Milvus chunk id at rank %d: %w", index, err)
		}
		raw := float64(results[0].Scores[index])
		distance, score := distanceAndScore(metric, raw)
		hits = append(hits, domainknowledge.VectorHit{
			EmbeddingID: embeddingID,
			ChunkID:     chunkID,
			Distance:    distance,
			Score:       score,
			Rank:        index + 1,
		})
	}
	return hits, nil
}

func (store *Store) MarkDeleted(ctx context.Context, deletion domainknowledge.VectorDeletion) error {
	if store == nil || store.client == nil {
		return fmt.Errorf("Milvus vector store is not initialized")
	}
	if len(deletion.EmbeddingIDs) == 0 {
		return nil
	}
	if err := deletion.Validate(); err != nil {
		return fmt.Errorf("validate vector deletion: %w", err)
	}
	deletedAt := deletion.DeletedAt
	if deletedAt.IsZero() {
		deletedAt = time.Now().UTC()
	}
	name := collectionName(store.collectionPrefix, deletion.EmbeddingProfileID)
	columns, err := store.client.Query(ctx, name, nil, idExpression(embeddingIDField, deletion.EmbeddingIDs), []string{
		embeddingIDField,
		chunkIDField,
		knowledgeBaseIDField,
		documentIDField,
		documentVersionField,
		embeddingProfileField,
		vectorField,
		deletedField,
		syncSequenceField,
	})
	if err != nil {
		return fmt.Errorf("query Milvus vectors for logical deletion: %w", err)
	}
	embeddings, err := embeddingsFromColumns(columns)
	if err != nil {
		return err
	}
	if len(embeddings) == 0 {
		return nil
	}
	eligible := embeddings[:0]
	for index := range embeddings {
		if embeddings[index].SyncSequence > deletion.SyncSequence {
			continue
		}
		embeddings[index].Deletion.Deleted = true
		embeddings[index].Deletion.DeletedAt = &deletedAt
		embeddings[index].Deletion.DeleteReason = "logically deleted"
		embeddings[index].SyncSequence = deletion.SyncSequence
		eligible = append(eligible, embeddings[index])
	}
	if len(eligible) == 0 {
		return nil
	}
	if _, err := store.client.Upsert(ctx, name, "", embeddingColumns(eligible)...); err != nil {
		return fmt.Errorf("mark Milvus vectors logically deleted: %w", err)
	}
	return nil
}

func (store *Store) Health(ctx context.Context) error {
	if store == nil || store.client == nil {
		return fmt.Errorf("Milvus vector store is not initialized")
	}
	state, err := store.client.CheckHealth(ctx)
	if err != nil {
		return fmt.Errorf("check Milvus health: %w", err)
	}
	if state == nil || !state.IsHealthy {
		reasons := []string(nil)
		if state != nil {
			reasons = state.Reasons
		}
		return fmt.Errorf("Milvus is unhealthy: %s", strings.Join(reasons, "; "))
	}
	return nil
}

func (store *Store) Close() error {
	if store == nil || store.client == nil {
		return nil
	}
	return store.client.Close()
}

func (store *Store) collectionLock(name string) *sync.Mutex {
	store.locksMu.Lock()
	defer store.locksMu.Unlock()
	if lock, exists := store.locks[name]; exists {
		return lock
	}
	lock := &sync.Mutex{}
	store.locks[name] = lock
	return lock
}

func searchExpression(query domainknowledge.VectorQuery) string {
	parts := []string{
		deletedField + " == false",
		embeddingProfileField + " == " + strconv.Quote(query.EmbeddingProfileID),
	}
	if len(query.KnowledgeBaseIDs) > 0 {
		parts = append(parts, idExpression(knowledgeBaseIDField, query.KnowledgeBaseIDs))
	}
	return strings.Join(parts, " && ")
}

func idExpression(field string, values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}
	return field + " in [" + strings.Join(quoted, ",") + "]"
}

func distanceAndScore(metric entity.MetricType, raw float64) (float64, float64) {
	switch metric {
	case entity.L2:
		distance := math.Max(raw, 0)
		return distance, 1 / (1 + distance)
	case entity.COSINE:
		return 1 - raw, raw
	case entity.IP:
		return -raw, raw
	default:
		return raw, raw
	}
}
