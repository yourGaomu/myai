package milvus

import (
	"fmt"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"

	domainknowledge "myai/core/domain/knowledge"
)

func embeddingColumns(embeddings []domainknowledge.EmbeddingVector) []entity.Column {
	embeddingIDs := make([]string, 0, len(embeddings))
	chunkIDs := make([]string, 0, len(embeddings))
	knowledgeBaseIDs := make([]string, 0, len(embeddings))
	documentIDs := make([]string, 0, len(embeddings))
	documentVersions := make([]int64, 0, len(embeddings))
	embeddingProfileIDs := make([]string, 0, len(embeddings))
	vectors := make([][]float32, 0, len(embeddings))
	deleted := make([]bool, 0, len(embeddings))
	syncSequences := make([]int64, 0, len(embeddings))
	for _, embedding := range embeddings {
		embeddingIDs = append(embeddingIDs, embedding.ID)
		chunkIDs = append(chunkIDs, embedding.ChunkID)
		knowledgeBaseIDs = append(knowledgeBaseIDs, embedding.KnowledgeBaseID)
		documentIDs = append(documentIDs, embedding.DocumentID)
		documentVersions = append(documentVersions, embedding.DocumentVersion)
		embeddingProfileIDs = append(embeddingProfileIDs, embedding.EmbeddingProfileID)
		vectors = append(vectors, append([]float32(nil), embedding.Values...))
		deleted = append(deleted, embedding.Deletion.Deleted)
		syncSequences = append(syncSequences, embedding.SyncSequence)
	}
	return []entity.Column{
		entity.NewColumnVarChar(embeddingIDField, embeddingIDs),
		entity.NewColumnVarChar(chunkIDField, chunkIDs),
		entity.NewColumnVarChar(knowledgeBaseIDField, knowledgeBaseIDs),
		entity.NewColumnVarChar(documentIDField, documentIDs),
		entity.NewColumnInt64(documentVersionField, documentVersions),
		entity.NewColumnVarChar(embeddingProfileField, embeddingProfileIDs),
		entity.NewColumnFloatVector(vectorField, embeddings[0].Dimensions, vectors),
		entity.NewColumnBool(deletedField, deleted),
		entity.NewColumnInt64(syncSequenceField, syncSequences),
	}
}

func embeddingsFromColumns(columns []entity.Column) ([]domainknowledge.EmbeddingVector, error) {
	byName := make(map[string]entity.Column, len(columns))
	for _, column := range columns {
		byName[column.Name()] = column
	}
	embeddingIDs, ok := byName[embeddingIDField].(*entity.ColumnVarChar)
	if !ok {
		return nil, fmt.Errorf("Milvus query is missing %s", embeddingIDField)
	}
	chunkIDs, ok := byName[chunkIDField].(*entity.ColumnVarChar)
	if !ok {
		return nil, fmt.Errorf("Milvus query is missing %s", chunkIDField)
	}
	knowledgeBaseIDs, ok := byName[knowledgeBaseIDField].(*entity.ColumnVarChar)
	if !ok {
		return nil, fmt.Errorf("Milvus query is missing %s", knowledgeBaseIDField)
	}
	documentIDs, ok := byName[documentIDField].(*entity.ColumnVarChar)
	if !ok {
		return nil, fmt.Errorf("Milvus query is missing %s", documentIDField)
	}
	documentVersions, ok := byName[documentVersionField].(*entity.ColumnInt64)
	if !ok {
		return nil, fmt.Errorf("Milvus query is missing %s", documentVersionField)
	}
	profileIDs, ok := byName[embeddingProfileField].(*entity.ColumnVarChar)
	if !ok {
		return nil, fmt.Errorf("Milvus query is missing %s", embeddingProfileField)
	}
	vectors, ok := byName[vectorField].(*entity.ColumnFloatVector)
	if !ok {
		return nil, fmt.Errorf("Milvus query is missing %s", vectorField)
	}
	deleted, ok := byName[deletedField].(*entity.ColumnBool)
	if !ok {
		return nil, fmt.Errorf("Milvus query is missing %s", deletedField)
	}
	syncSequences, ok := byName[syncSequenceField].(*entity.ColumnInt64)
	if !ok {
		return nil, fmt.Errorf("Milvus query is missing %s", syncSequenceField)
	}
	result := make([]domainknowledge.EmbeddingVector, 0, embeddingIDs.Len())
	for index := 0; index < embeddingIDs.Len(); index++ {
		result = append(result, domainknowledge.EmbeddingVector{
			ID:                 embeddingIDs.Data()[index],
			ChunkID:            chunkIDs.Data()[index],
			KnowledgeBaseID:    knowledgeBaseIDs.Data()[index],
			DocumentID:         documentIDs.Data()[index],
			DocumentVersion:    documentVersions.Data()[index],
			EmbeddingProfileID: profileIDs.Data()[index],
			Dimensions:         vectors.Dim(),
			Values:             append([]float32(nil), vectors.Data()[index]...),
			Deletion:           domainknowledge.Deletion{Deleted: deleted.Data()[index]},
			SyncSequence:       syncSequences.Data()[index],
		})
	}
	return result, nil
}
