package repository

import (
	"context"

	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"myai/core/adapter/persistence/mongo/knowledge/po"
)

type fakeOperations struct {
	collection       string
	filter           any
	update           any
	updateOptions    []options.Lister[options.UpdateOneOptions]
	base             po.KnowledgeBaseDocument
	bases            []po.KnowledgeBaseDocument
	document         po.KnowledgeDocument
	documents        []po.KnowledgeDocument
	chunk            po.ChunkDocument
	chunks           []po.ChunkDocument
	parsingProfile   po.ParsingProfileDocument
	embeddingProfile po.EmbeddingProfileDocument
	chunkingProfile  po.ChunkingProfileDocument
	indexProfile     po.IndexProfileDocument
	indexingJob      po.IndexingJobDocument
	indexingJobs     []po.IndexingJobDocument
	syncChanges      []po.SyncChangeDocument
	insertCollection string
	insertDocument   any
	findOneErr       error
	findAllErr       error
	updateErr        error
	updateResult     *gomongo.UpdateResult
	updateCallCount  int
	updateManyFilter any
	updateMany       any
	updateManyErr    error
	updateManyResult *gomongo.UpdateResult
	updateManyCalls  int
}

func (operations *fakeOperations) FindOne(_ context.Context, collection string, filter any, out any, _ ...options.Lister[options.FindOneOptions]) error {
	operations.collection = collection
	operations.filter = filter
	if operations.findOneErr != nil {
		return operations.findOneErr
	}
	switch target := out.(type) {
	case *po.KnowledgeBaseDocument:
		*target = operations.base
	case *po.KnowledgeDocument:
		*target = operations.document
	case *po.ParsingProfileDocument:
		*target = operations.parsingProfile
	case *po.EmbeddingProfileDocument:
		*target = operations.embeddingProfile
	case *po.ChunkingProfileDocument:
		*target = operations.chunkingProfile
	case *po.IndexProfileDocument:
		*target = operations.indexProfile
	case *po.IndexingJobDocument:
		*target = operations.indexingJob
	}
	return nil
}

func (operations *fakeOperations) FindAll(_ context.Context, collection string, filter any, out any, _ ...options.Lister[options.FindOptions]) error {
	operations.collection = collection
	operations.filter = filter
	if operations.findAllErr != nil {
		return operations.findAllErr
	}
	switch target := out.(type) {
	case *[]po.KnowledgeBaseDocument:
		*target = append([]po.KnowledgeBaseDocument(nil), operations.bases...)
	case *[]po.KnowledgeDocument:
		*target = append([]po.KnowledgeDocument(nil), operations.documents...)
	case *[]po.ChunkDocument:
		*target = append([]po.ChunkDocument(nil), operations.chunks...)
	case *[]po.IndexingJobDocument:
		*target = append([]po.IndexingJobDocument(nil), operations.indexingJobs...)
	case *[]po.SyncChangeDocument:
		*target = append([]po.SyncChangeDocument(nil), operations.syncChanges...)
	}
	return nil
}

func (operations *fakeOperations) UpdateOne(_ context.Context, collection string, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*gomongo.UpdateResult, error) {
	operations.collection = collection
	operations.filter = filter
	operations.update = update
	operations.updateOptions = append([]options.Lister[options.UpdateOneOptions](nil), opts...)
	operations.updateCallCount++
	if operations.updateErr != nil {
		return nil, operations.updateErr
	}
	if operations.updateResult != nil {
		return operations.updateResult, nil
	}
	return &gomongo.UpdateResult{MatchedCount: 1}, nil
}

func (operations *fakeOperations) UpdateMany(_ context.Context, collection string, filter any, update any, _ ...options.Lister[options.UpdateManyOptions]) (*gomongo.UpdateResult, error) {
	operations.collection = collection
	operations.updateManyFilter = filter
	operations.updateMany = update
	operations.updateManyCalls++
	if operations.updateManyErr != nil {
		return nil, operations.updateManyErr
	}
	if operations.updateManyResult != nil {
		return operations.updateManyResult, nil
	}
	return &gomongo.UpdateResult{}, nil
}

func (operations *fakeOperations) InsertOne(_ context.Context, collection string, document any) (*gomongo.InsertOneResult, error) {
	operations.insertCollection = collection
	operations.insertDocument = document
	return &gomongo.InsertOneResult{}, nil
}

func (*fakeOperations) DeleteMany(context.Context, string, any) (*gomongo.DeleteResult, error) {
	return &gomongo.DeleteResult{}, nil
}

func (*fakeOperations) Count(context.Context, string, any) (int64, error) {
	return 0, nil
}
