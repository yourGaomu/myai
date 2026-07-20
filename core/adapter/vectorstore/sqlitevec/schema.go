package sqlitevec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	domainknowledge "myai/core/domain/knowledge"
)

const (
	profileTable = "local_vector_profiles"
	recordTable  = "local_vector_records"
)

type profileDefinition struct {
	ID         string
	Dimensions int
	Metric     string
	TableName  string
}

func (store *Store) initialize(ctx context.Context) error {
	var version string
	if err := store.db.QueryRowContext(ctx, `SELECT vec_version()`).Scan(&version); err != nil {
		return fmt.Errorf("check sqlite-vec extension: %w", err)
	}
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("sqlite-vec extension returned an empty version")
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS local_vector_profiles (
			profile_id TEXT PRIMARY KEY,
			dimensions INTEGER NOT NULL,
			distance_metric TEXT NOT NULL,
			table_name TEXT NOT NULL UNIQUE,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS local_vector_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			embedding_id TEXT NOT NULL UNIQUE,
			chunk_id TEXT NOT NULL,
			knowledge_base_id TEXT NOT NULL,
			document_id TEXT NOT NULL,
			document_version INTEGER NOT NULL,
			profile_id TEXT NOT NULL,
			dimensions INTEGER NOT NULL,
			deleted INTEGER NOT NULL DEFAULT 0,
			deleted_at INTEGER,
			delete_reason TEXT NOT NULL DEFAULT '',
			sync_sequence INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY(profile_id) REFERENCES local_vector_profiles(profile_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_vector_records_profile
			ON local_vector_records(profile_id, deleted)`,
		`CREATE INDEX IF NOT EXISTS idx_local_vector_records_chunk
			ON local_vector_records(chunk_id, profile_id)`,
		`CREATE INDEX IF NOT EXISTS idx_local_vector_records_sync
			ON local_vector_records(sync_sequence)`,
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize sqlite-vec store: %w", err)
		}
	}
	return nil
}

func newProfileDefinition(definition domainknowledge.VectorIndexDefinition) (profileDefinition, error) {
	metric, err := normalizeMetric(definition.DistanceMetricID)
	if err != nil {
		return profileDefinition{}, err
	}
	return profileDefinition{
		ID:         strings.TrimSpace(definition.EmbeddingProfileID),
		Dimensions: definition.Dimensions,
		Metric:     metric,
		TableName:  vectorTableName(definition.EmbeddingProfileID),
	}, nil
}

func vectorTableName(profileID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(profileID)))
	return "local_vectors_" + hex.EncodeToString(digest[:12])
}

func vectorTableStatement(profile profileDefinition) string {
	return fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(
		knowledge_base_id TEXT,
		deleted BOOLEAN,
		+embedding_id TEXT,
		+chunk_id TEXT,
		embedding FLOAT[%d] distance_metric=%s
	)`, quoteIdentifier(profile.TableName), profile.Dimensions, profile.Metric)
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func normalizeMetric(metricID string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(metricID)) {
	case "l2", "euclidean":
		return "l2", nil
	case "cosine":
		return "cosine", nil
	default:
		return "", fmt.Errorf("unsupported sqlite-vec distance metric %q", metricID)
	}
}
