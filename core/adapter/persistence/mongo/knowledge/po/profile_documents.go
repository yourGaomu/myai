package po

import "time"

type ParsingProfileDocument struct {
	ID            string            `bson:"_id"`
	Name          string            `bson:"name"`
	ParserID      string            `bson:"parser_id"`
	ParserVersion string            `bson:"parser_version"`
	OCR           bool              `bson:"ocr"`
	Options       map[string]string `bson:"options,omitempty"`
	Deleted       bool              `bson:"deleted"`
	DeletedAt     *time.Time        `bson:"deleted_at,omitempty"`
	DeleteReason  string            `bson:"delete_reason,omitempty"`
	CreatedAt     time.Time         `bson:"created_at"`
	UpdatedAt     time.Time         `bson:"updated_at"`
}

type EmbeddingProfileDocument struct {
	ID           string     `bson:"_id"`
	Name         string     `bson:"name"`
	ModelID      string     `bson:"model_id"`
	Provider     string     `bson:"provider"`
	Model        string     `bson:"model"`
	ModelVersion string     `bson:"model_version,omitempty"`
	Dimensions   int        `bson:"dimensions"`
	Normalize    bool       `bson:"normalize"`
	InputMode    string     `bson:"input_mode,omitempty"`
	Deleted      bool       `bson:"deleted"`
	DeletedAt    *time.Time `bson:"deleted_at,omitempty"`
	DeleteReason string     `bson:"delete_reason,omitempty"`
	CreatedAt    time.Time  `bson:"created_at"`
	UpdatedAt    time.Time  `bson:"updated_at"`
}

type ChunkingProfileDocument struct {
	ID              string            `bson:"_id"`
	Name            string            `bson:"name"`
	StrategyID      string            `bson:"strategy_id"`
	StrategyVersion string            `bson:"strategy_version"`
	MaxChunkSize    int               `bson:"max_chunk_size"`
	Overlap         int               `bson:"overlap"`
	Tokenizer       string            `bson:"tokenizer,omitempty"`
	Options         map[string]string `bson:"options,omitempty"`
	Deleted         bool              `bson:"deleted"`
	DeletedAt       *time.Time        `bson:"deleted_at,omitempty"`
	DeleteReason    string            `bson:"delete_reason,omitempty"`
	CreatedAt       time.Time         `bson:"created_at"`
	UpdatedAt       time.Time         `bson:"updated_at"`
}

type IndexProfileDocument struct {
	ID                 string            `bson:"_id"`
	Name               string            `bson:"name"`
	ParsingProfileID   string            `bson:"parsing_profile_id"`
	ChunkingProfileID  string            `bson:"chunking_profile_id"`
	EmbeddingProfileID string            `bson:"embedding_profile_id"`
	DistanceMetricID   string            `bson:"distance_metric_id"`
	VectorIndexOptions map[string]string `bson:"vector_index_options,omitempty"`
	Status             string            `bson:"status"`
	FailureReason      string            `bson:"failure_reason,omitempty"`
	Deleted            bool              `bson:"deleted"`
	DeletedAt          *time.Time        `bson:"deleted_at,omitempty"`
	DeleteReason       string            `bson:"delete_reason,omitempty"`
	CreatedAt          time.Time         `bson:"created_at"`
	UpdatedAt          time.Time         `bson:"updated_at"`
}
