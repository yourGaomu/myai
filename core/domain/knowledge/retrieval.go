package knowledge

import (
	"fmt"
	"math"
	"strings"
)

type RetrievalChannel string
type RetrievalOrigin string

const (
	RetrievalChannelVector  RetrievalChannel = "vector"
	RetrievalChannelKeyword RetrievalChannel = "keyword"

	RetrievalOriginLocal  RetrievalOrigin = "local"
	RetrievalOriginRemote RetrievalOrigin = "remote"
)

type RetrievalQuery struct {
	Text               string
	KnowledgeBaseIDs   []string
	IndexProfileID     string
	EmbeddingProfileID string
	TopK               int
}

type RetrievalHit struct {
	KnowledgeBaseID string
	DocumentID      string
	ChunkID         string
	DocumentVersion int64
	Text            string
	SourceName      string
	SourceLocation  string
	Score           float64
	Rank            int
	Channel         RetrievalChannel
	Origin          RetrievalOrigin
}

type LocalQualityDecision struct {
	Passed         bool
	Healthy        bool
	ProfileMatched bool
	ResultCount    int
	TopVectorScore float64
	Reasons        []string
}

type LocalQualityInput struct {
	Healthy        bool
	ProfileMatched bool
	ResultCount    int
	TopVectorScore float64
}

type RankedItem struct {
	ChunkID string
	Rank    int
	Score   float64
}

type RankedList struct {
	Channel RetrievalChannel
	Origin  RetrievalOrigin
	Items   []RankedItem
}

type FusedItem struct {
	ChunkID string
	Score   float64
	Rank    int
	Channel RetrievalChannel
	Origin  RetrievalOrigin
}

type EmbeddingVector struct {
	ID                 string
	ChunkID            string
	KnowledgeBaseID    string
	DocumentID         string
	DocumentVersion    int64
	EmbeddingProfileID string
	Dimensions         int
	Values             []float32
	Deletion           Deletion
	SyncSequence       int64
}

type VectorQuery struct {
	EmbeddingProfileID string
	KnowledgeBaseIDs   []string
	DistanceMetricID   string
	Options            map[string]string
	Vector             []float32
	TopK               int
}

type VectorHit struct {
	EmbeddingID string
	ChunkID     string
	Distance    float64
	Score       float64
	Rank        int
}

type KeywordDocument struct {
	ChunkID         string
	KnowledgeBaseID string
	DocumentID      string
	DocumentVersion int64
	Text            string
	Deletion        Deletion
	SyncSequence    int64
}

type KeywordQuery struct {
	Text             string
	KnowledgeBaseIDs []string
	TopK             int
}

type KeywordHit struct {
	ChunkID string
	Score   float64
	Rank    int
}

func (query RetrievalQuery) Validate() error {
	if strings.TrimSpace(query.Text) == "" {
		return fmt.Errorf("retrieval query text is required")
	}
	if strings.TrimSpace(query.IndexProfileID) == "" {
		return fmt.Errorf("index profile id is required")
	}
	if strings.TrimSpace(query.EmbeddingProfileID) == "" {
		return fmt.Errorf("embedding profile id is required")
	}
	if query.TopK < 1 {
		return fmt.Errorf("retrieval top_k must be positive")
	}
	return validateIDs("knowledge base", query.KnowledgeBaseIDs)
}

func (embedding EmbeddingVector) Validate() error {
	if strings.TrimSpace(embedding.ID) == "" {
		return fmt.Errorf("embedding id is required")
	}
	if strings.TrimSpace(embedding.ChunkID) == "" {
		return fmt.Errorf("chunk id is required")
	}
	if strings.TrimSpace(embedding.KnowledgeBaseID) == "" {
		return fmt.Errorf("knowledge base id is required")
	}
	if strings.TrimSpace(embedding.DocumentID) == "" {
		return fmt.Errorf("document id is required")
	}
	if embedding.DocumentVersion < 1 {
		return fmt.Errorf("document version must be at least 1")
	}
	if strings.TrimSpace(embedding.EmbeddingProfileID) == "" {
		return fmt.Errorf("embedding profile id is required")
	}
	if embedding.Dimensions < 1 || len(embedding.Values) != embedding.Dimensions {
		return fmt.Errorf("embedding values must match dimensions")
	}
	if err := validateVector(embedding.Values); err != nil {
		return err
	}
	if embedding.SyncSequence < 0 {
		return fmt.Errorf("sync sequence must not be negative")
	}
	if err := embedding.Deletion.Validate(); err != nil {
		return fmt.Errorf("invalid embedding deletion: %w", err)
	}
	return nil
}

func (query VectorQuery) Validate() error {
	if strings.TrimSpace(query.EmbeddingProfileID) == "" {
		return fmt.Errorf("embedding profile id is required")
	}
	if strings.TrimSpace(query.DistanceMetricID) == "" {
		return fmt.Errorf("distance metric id is required")
	}
	if len(query.Vector) == 0 {
		return fmt.Errorf("query vector is required")
	}
	if err := validateVector(query.Vector); err != nil {
		return err
	}
	if query.TopK < 1 {
		return fmt.Errorf("vector top_k must be positive")
	}
	return validateIDs("knowledge base", query.KnowledgeBaseIDs)
}

func (query KeywordQuery) Validate() error {
	if strings.TrimSpace(query.Text) == "" {
		return fmt.Errorf("keyword query text is required")
	}
	if query.TopK < 1 {
		return fmt.Errorf("keyword top_k must be positive")
	}
	return validateIDs("knowledge base", query.KnowledgeBaseIDs)
}

func (document KeywordDocument) Validate() error {
	if strings.TrimSpace(document.ChunkID) == "" {
		return fmt.Errorf("keyword document chunk id is required")
	}
	if strings.TrimSpace(document.KnowledgeBaseID) == "" {
		return fmt.Errorf("keyword document knowledge base id is required")
	}
	if strings.TrimSpace(document.DocumentID) == "" {
		return fmt.Errorf("keyword document document id is required")
	}
	if document.DocumentVersion < 1 {
		return fmt.Errorf("keyword document version must be at least 1")
	}
	if strings.TrimSpace(document.Text) == "" {
		return fmt.Errorf("keyword document text is required")
	}
	if document.SyncSequence < 0 {
		return fmt.Errorf("keyword document sync sequence must not be negative")
	}
	if err := document.Deletion.Validate(); err != nil {
		return fmt.Errorf("invalid keyword document deletion: %w", err)
	}
	return nil
}

func validateIDs(name string, values []string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s id must not be empty", name)
		}
	}
	return nil
}

func validateVector(values []float32) error {
	for _, value := range values {
		converted := float64(value)
		if math.IsNaN(converted) || math.IsInf(converted, 0) {
			return fmt.Errorf("embedding values must be finite")
		}
	}
	return nil
}
