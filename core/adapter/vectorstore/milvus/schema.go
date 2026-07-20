package milvus

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	embeddingIDField      = "embedding_id"
	chunkIDField          = "chunk_id"
	knowledgeBaseIDField  = "knowledge_base_id"
	documentIDField       = "document_id"
	documentVersionField  = "document_version"
	embeddingProfileField = "embedding_profile_id"
	vectorField           = "vector"
	deletedField          = "deleted"
	syncSequenceField     = "sync_sequence"
	maxIDLength           = 512
)

func collectionName(prefix string, embeddingProfileID string) string {
	digest := sha256.Sum256([]byte(embeddingProfileID))
	suffix := hex.EncodeToString(digest[:6])
	safePrefix := sanitizeName(prefix)
	safeProfile := sanitizeName(embeddingProfileID)
	const maxProfileLength = 160
	if len(safeProfile) > maxProfileLength {
		safeProfile = safeProfile[:maxProfileLength]
	}
	return safePrefix + "_" + safeProfile + "_" + suffix
}

func sanitizeName(value string) string {
	var builder strings.Builder
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsDigit(current) || current == '_' {
			builder.WriteRune(current)
		} else {
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		result = "profile"
	}
	first, _ := utf8FirstRune(result)
	if !unicode.IsLetter(first) && first != '_' {
		result = "p_" + result
	}
	return result
}

func utf8FirstRune(value string) (rune, int) {
	for _, current := range value {
		return current, len(string(current))
	}
	return 0, 0
}

func collectionSchema(name string, dimensions int) *entity.Schema {
	return entity.NewSchema().
		WithName(name).
		WithDescription("MyAI knowledge embeddings").
		WithAutoID(false).
		WithDynamicFieldEnabled(false).
		WithField(entity.NewField().WithName(embeddingIDField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxIDLength).WithIsPrimaryKey(true).WithIsAutoID(false)).
		WithField(entity.NewField().WithName(chunkIDField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxIDLength)).
		WithField(entity.NewField().WithName(knowledgeBaseIDField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxIDLength)).
		WithField(entity.NewField().WithName(documentIDField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxIDLength)).
		WithField(entity.NewField().WithName(documentVersionField).WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName(embeddingProfileField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxIDLength)).
		WithField(entity.NewField().WithName(vectorField).WithDataType(entity.FieldTypeFloatVector).WithDim(int64(dimensions))).
		WithField(entity.NewField().WithName(deletedField).WithDataType(entity.FieldTypeBool)).
		WithField(entity.NewField().WithName(syncSequenceField).WithDataType(entity.FieldTypeInt64))
}

func validateCollection(collection *entity.Collection, dimensions int) error {
	if collection == nil || collection.Schema == nil {
		return fmt.Errorf("Milvus collection schema is missing")
	}
	for _, field := range collection.Schema.Fields {
		if field.Name != vectorField {
			continue
		}
		if field.DataType != entity.FieldTypeFloatVector {
			return fmt.Errorf("Milvus vector field has type %s", field.DataType)
		}
		actual, err := strconv.Atoi(field.TypeParams["dim"])
		if err != nil {
			return fmt.Errorf("parse Milvus vector dimensions: %w", err)
		}
		if actual != dimensions {
			return fmt.Errorf("Milvus collection dimensions %d do not match profile dimensions %d", actual, dimensions)
		}
		return nil
	}
	return fmt.Errorf("Milvus collection vector field is missing")
}
