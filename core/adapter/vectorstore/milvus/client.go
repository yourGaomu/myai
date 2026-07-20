package milvus

import (
	"context"

	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

type client interface {
	Close() error
	HasCollection(ctx context.Context, collectionName string) (bool, error)
	CreateCollection(ctx context.Context, schema *entity.Schema, shards int32, options ...milvusclient.CreateCollectionOption) error
	DescribeCollection(ctx context.Context, collectionName string) (*entity.Collection, error)
	LoadCollection(ctx context.Context, collectionName string, async bool, options ...milvusclient.LoadCollectionOption) error
	ReleaseCollection(ctx context.Context, collectionName string, options ...milvusclient.ReleaseCollectionOption) error
	CreateIndex(ctx context.Context, collectionName string, fieldName string, index entity.Index, async bool, options ...milvusclient.IndexOption) error
	DescribeIndex(ctx context.Context, collectionName string, fieldName string, options ...milvusclient.IndexOption) ([]entity.Index, error)
	DropIndex(ctx context.Context, collectionName string, fieldName string, options ...milvusclient.IndexOption) error
	Upsert(ctx context.Context, collectionName string, partitionName string, columns ...entity.Column) (entity.Column, error)
	Search(ctx context.Context, collectionName string, partitions []string, expression string, outputFields []string, vectors []entity.Vector, vectorField string, metricType entity.MetricType, topK int, searchParam entity.SearchParam, options ...milvusclient.SearchQueryOptionFunc) ([]milvusclient.SearchResult, error)
	Query(ctx context.Context, collectionName string, partitions []string, expression string, outputFields []string, options ...milvusclient.SearchQueryOptionFunc) (milvusclient.ResultSet, error)
	CheckHealth(ctx context.Context) (*entity.MilvusState, error)
}
