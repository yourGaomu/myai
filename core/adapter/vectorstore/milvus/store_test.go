package milvus

import (
	"context"
	"strings"
	"testing"
	"time"

	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"

	domainknowledge "myai/core/domain/knowledge"
)

func TestEnsureIndexCreatesCollectionIndexAndLoads(t *testing.T) {
	runtimeClient := &fakeClient{}
	store := newWithClient(runtimeClient, Config{CollectionPrefix: "test_vectors", Shards: 2})
	definition := domainknowledge.VectorIndexDefinition{
		EmbeddingProfileID: "profile/one",
		Dimensions:         3,
		DistanceMetricID:   "cosine",
		Options:            map[string]string{"index_type": "HNSW", "m": "8", "ef_construction": "100"},
	}
	if err := store.EnsureIndex(context.Background(), definition); err != nil {
		t.Fatal(err)
	}
	if runtimeClient.createCollectionCalls != 1 || runtimeClient.createdSchema == nil || runtimeClient.createdShards != 2 {
		t.Fatalf("unexpected collection creation: %#v", runtimeClient)
	}
	if err := validateCollection(&entity.Collection{Schema: runtimeClient.createdSchema}, 3); err != nil {
		t.Fatalf("created schema is invalid: %v", err)
	}
	if runtimeClient.createIndexCalls != 1 || runtimeClient.indexes[0].IndexType() != entity.HNSW {
		t.Fatalf("unexpected index creation: %#v", runtimeClient.indexes)
	}
	if runtimeClient.loadCalls != 1 {
		t.Fatalf("expected collection load, got %d", runtimeClient.loadCalls)
	}
	if strings.Contains(runtimeClient.collectionName, "/") || !strings.HasPrefix(runtimeClient.collectionName, "test_vectors_profile_one_") {
		t.Fatalf("unexpected safe collection name %q", runtimeClient.collectionName)
	}
}

func TestEnsureIndexRebuildsIncompatibleIndexWithoutReembedding(t *testing.T) {
	currentIndex, err := entity.NewIndexFlat(entity.L2)
	if err != nil {
		t.Fatal(err)
	}
	runtimeClient := &fakeClient{
		hasCollection: true,
		collection:    &entity.Collection{Schema: collectionSchema("existing", 2), Loaded: true},
		indexes:       []entity.Index{currentIndex},
	}
	store := newWithClient(runtimeClient, Config{})
	if err := store.EnsureIndex(context.Background(), domainknowledge.VectorIndexDefinition{
		EmbeddingProfileID: "profile-1",
		Dimensions:         2,
		DistanceMetricID:   "cosine",
		Options:            map[string]string{"index_type": "FLAT"},
	}); err != nil {
		t.Fatal(err)
	}
	if runtimeClient.releaseCalls != 1 || runtimeClient.dropIndexCalls != 1 || runtimeClient.createIndexCalls != 1 || runtimeClient.loadCalls != 1 {
		t.Fatalf("expected release/drop/create/load rebuild: %#v", runtimeClient)
	}
	if runtimeClient.upsertCalls != 0 {
		t.Fatal("index rebuild must not rewrite vectors")
	}
}

func TestUpsertMapsAllDomainColumns(t *testing.T) {
	runtimeClient := &fakeClient{hasCollection: true}
	store := newWithClient(runtimeClient, Config{})
	embedding := validMilvusEmbedding("embedding-1", "chunk-1")
	if err := store.Upsert(context.Background(), []domainknowledge.EmbeddingVector{embedding}); err != nil {
		t.Fatal(err)
	}
	if runtimeClient.upsertCalls != 1 || len(runtimeClient.upsertColumns) != 9 {
		t.Fatalf("unexpected upsert columns: %#v", runtimeClient.upsertColumns)
	}
	if got, _ := runtimeClient.column(embeddingIDField).GetAsString(0); got != embedding.ID {
		t.Fatalf("unexpected embedding id %q", got)
	}
	vector, ok := runtimeClient.column(vectorField).(*entity.ColumnFloatVector)
	if !ok || vector.Dim() != 2 || vector.Data()[0][1] != 0.2 {
		t.Fatalf("unexpected vector column: %#v", runtimeClient.column(vectorField))
	}
}

func TestSearchFiltersLogicalDeletionAndMapsScores(t *testing.T) {
	runtimeClient := &fakeClient{searchResults: []milvusclient.SearchResult{{
		ResultCount: 2,
		IDs:         entity.NewColumnVarChar(embeddingIDField, []string{"embedding-1", "embedding-2"}),
		Fields:      milvusclient.ResultSet{entity.NewColumnVarChar(chunkIDField, []string{"chunk-1", "chunk-2"})},
		Scores:      []float32{0.9, 0.7},
	}}}
	store := newWithClient(runtimeClient, Config{})
	hits, err := store.Search(context.Background(), domainknowledge.VectorQuery{
		EmbeddingProfileID: "profile-1",
		KnowledgeBaseIDs:   []string{"knowledge-a", "knowledge-b"},
		DistanceMetricID:   "cosine",
		Options:            map[string]string{"index_type": "FLAT"},
		Vector:             []float32{0.1, 0.2},
		TopK:               2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(runtimeClient.searchExpression, "deleted == false") || !strings.Contains(runtimeClient.searchExpression, `knowledge_base_id in ["knowledge-a","knowledge-b"]`) {
		t.Fatalf("unexpected search expression %q", runtimeClient.searchExpression)
	}
	if len(hits) != 2 || hits[0].EmbeddingID != "embedding-1" || hits[0].ChunkID != "chunk-1" || hits[0].Rank != 1 {
		t.Fatalf("unexpected search hits: %#v", hits)
	}
	if hits[0].Score < 0.899 || hits[0].Distance < 0.099 || hits[0].Distance > 0.101 {
		t.Fatalf("unexpected cosine score mapping: %#v", hits[0])
	}
}

func TestMarkDeletedQueriesAndUpsertsTombstones(t *testing.T) {
	runtimeClient := &fakeClient{queryResult: milvusclient.ResultSet(embeddingColumns([]domainknowledge.EmbeddingVector{
		validMilvusEmbedding("embedding-1", "chunk-1"),
	}))}
	store := newWithClient(runtimeClient, Config{})
	deletedAt := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	if err := store.MarkDeleted(context.Background(), domainknowledge.VectorDeletion{
		EmbeddingProfileID: "profile-1",
		EmbeddingIDs:       []string{"embedding-1"},
		SyncSequence:       9,
		DeletedAt:          deletedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(runtimeClient.queryExpression, `embedding_id in ["embedding-1"]`) {
		t.Fatalf("unexpected deletion expression %q", runtimeClient.queryExpression)
	}
	deleted, ok := runtimeClient.column(deletedField).(*entity.ColumnBool)
	if !ok || !deleted.Data()[0] {
		t.Fatalf("expected logical deletion column: %#v", runtimeClient.column(deletedField))
	}
	sequences, ok := runtimeClient.column(syncSequenceField).(*entity.ColumnInt64)
	if !ok || sequences.Data()[0] != 9 {
		t.Fatalf("expected updated sync sequence: %#v", runtimeClient.column(syncSequenceField))
	}
}

func TestMarkDeletedSkipsStaleSyncSequence(t *testing.T) {
	embedding := validMilvusEmbedding("embedding-1", "chunk-1")
	embedding.SyncSequence = 10
	runtimeClient := &fakeClient{queryResult: milvusclient.ResultSet(embeddingColumns([]domainknowledge.EmbeddingVector{embedding}))}
	store := newWithClient(runtimeClient, Config{})
	if err := store.MarkDeleted(context.Background(), domainknowledge.VectorDeletion{
		EmbeddingProfileID: "profile-1",
		EmbeddingIDs:       []string{"embedding-1"},
		SyncSequence:       9,
	}); err != nil {
		t.Fatal(err)
	}
	if runtimeClient.upsertCalls != 0 {
		t.Fatal("stale deletion must not overwrite a newer vector")
	}
}

func TestHealthRejectsUnhealthyMilvus(t *testing.T) {
	store := newWithClient(&fakeClient{health: &entity.MilvusState{IsHealthy: false, Reasons: []string{"standby"}}}, Config{})
	if err := store.Health(context.Background()); err == nil || !strings.Contains(err.Error(), "standby") {
		t.Fatalf("expected unhealthy error, got %v", err)
	}
}

func validMilvusEmbedding(id string, chunkID string) domainknowledge.EmbeddingVector {
	return domainknowledge.EmbeddingVector{
		ID: id, ChunkID: chunkID, KnowledgeBaseID: "knowledge-1", DocumentID: "document-1", DocumentVersion: 1,
		EmbeddingProfileID: "profile-1", Dimensions: 2, Values: []float32{0.1, 0.2}, SyncSequence: 3,
	}
}

type fakeClient struct {
	hasCollection         bool
	collection            *entity.Collection
	collectionName        string
	createdSchema         *entity.Schema
	createdShards         int32
	indexes               []entity.Index
	createCollectionCalls int
	createIndexCalls      int
	dropIndexCalls        int
	loadCalls             int
	releaseCalls          int
	upsertCalls           int
	upsertColumns         []entity.Column
	searchExpression      string
	searchResults         []milvusclient.SearchResult
	queryExpression       string
	queryResult           milvusclient.ResultSet
	health                *entity.MilvusState
}

func (client *fakeClient) Close() error { return nil }

func (client *fakeClient) HasCollection(context.Context, string) (bool, error) {
	return client.hasCollection, nil
}

func (client *fakeClient) CreateCollection(_ context.Context, schema *entity.Schema, shards int32, _ ...milvusclient.CreateCollectionOption) error {
	client.createCollectionCalls++
	client.createdSchema = schema
	client.createdShards = shards
	client.collectionName = schema.CollectionName
	client.hasCollection = true
	return nil
}

func (client *fakeClient) DescribeCollection(context.Context, string) (*entity.Collection, error) {
	return client.collection, nil
}

func (client *fakeClient) LoadCollection(_ context.Context, name string, _ bool, _ ...milvusclient.LoadCollectionOption) error {
	client.loadCalls++
	client.collectionName = name
	return nil
}

func (client *fakeClient) ReleaseCollection(context.Context, string, ...milvusclient.ReleaseCollectionOption) error {
	client.releaseCalls++
	return nil
}

func (client *fakeClient) CreateIndex(_ context.Context, name string, _ string, index entity.Index, _ bool, _ ...milvusclient.IndexOption) error {
	client.createIndexCalls++
	client.collectionName = name
	client.indexes = []entity.Index{index}
	return nil
}

func (client *fakeClient) DescribeIndex(context.Context, string, string, ...milvusclient.IndexOption) ([]entity.Index, error) {
	return client.indexes, nil
}

func (client *fakeClient) DropIndex(context.Context, string, string, ...milvusclient.IndexOption) error {
	client.dropIndexCalls++
	client.indexes = nil
	return nil
}

func (client *fakeClient) Upsert(_ context.Context, name string, _ string, columns ...entity.Column) (entity.Column, error) {
	client.upsertCalls++
	client.collectionName = name
	client.upsertColumns = columns
	return columns[0], nil
}

func (client *fakeClient) Search(_ context.Context, name string, _ []string, expression string, _ []string, _ []entity.Vector, _ string, _ entity.MetricType, _ int, _ entity.SearchParam, _ ...milvusclient.SearchQueryOptionFunc) ([]milvusclient.SearchResult, error) {
	client.collectionName = name
	client.searchExpression = expression
	return client.searchResults, nil
}

func (client *fakeClient) Query(_ context.Context, _ string, _ []string, expression string, _ []string, _ ...milvusclient.SearchQueryOptionFunc) (milvusclient.ResultSet, error) {
	client.queryExpression = expression
	return client.queryResult, nil
}

func (client *fakeClient) CheckHealth(context.Context) (*entity.MilvusState, error) {
	if client.health == nil {
		return &entity.MilvusState{IsHealthy: true}, nil
	}
	return client.health, nil
}

func (client *fakeClient) column(name string) entity.Column {
	for _, column := range client.upsertColumns {
		if column.Name() == name {
			return column
		}
	}
	return nil
}
