package repository

import (
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"

	mongotemplate "myai/core/adapter/persistence/mongo/template"
	knowledgeport "myai/core/port/knowledge"
)

const (
	knowledgeBasesCollection      = "knowledge_bases"
	knowledgeCategoriesCollection = "knowledge_categories"
	documentsCollection           = "knowledge_documents"
	chunksCollection              = "knowledge_chunks"
	parsingProfilesCollection     = "parsing_profiles"
	embeddingProfilesCollection   = "embedding_profiles"
	chunkingProfilesCollection    = "chunking_profiles"
	indexProfilesCollection       = "index_profiles"
	indexingJobsCollection        = "knowledge_indexing_jobs"
	syncChangesCollection         = "knowledge_sync_changes"
)

func activeFilter() bson.M {
	return bson.M{"$or": bson.A{
		bson.M{"deleted": bson.M{"$exists": false}},
		bson.M{"deleted": false},
	}}
}

func activeIDFilter(id string) bson.M {
	filter := activeFilter()
	filter["_id"] = id
	return filter
}

func repositoryError(err error) error {
	if errors.Is(err, mongotemplate.ErrNotFound) || errors.Is(err, gomongo.ErrNoDocuments) {
		return knowledgeport.ErrNotFound
	}
	return err
}

func matchedOrNotFound(result *gomongo.UpdateResult, err error) error {
	if err != nil {
		return repositoryError(err)
	}
	if result == nil || result.MatchedCount == 0 {
		return knowledgeport.ErrNotFound
	}
	return nil
}
