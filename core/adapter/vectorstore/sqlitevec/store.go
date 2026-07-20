package sqlitevec

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	domainknowledge "myai/core/domain/knowledge"
	"myai/core/infra/sqliteruntime"
	knowledgeport "myai/core/port/knowledge"
)

type Store struct {
	db      *sql.DB
	writeMu sync.Mutex
}

var _ knowledgeport.VectorStore = (*Store)(nil)

func Open(config Config) (*Store, error) {
	config = config.normalize()
	if err := config.validate(); err != nil {
		return nil, err
	}
	if err := ensureParentDirectory(config.Path); err != nil {
		return nil, err
	}
	sqliteruntime.Configure()
	dsn, err := sqliteruntime.DataSourceName(config.Path)
	if err != nil {
		return nil, fmt.Errorf("build sqlite-vec data source: %w", err)
	}
	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite-vec database: %w", err)
	}
	database.SetMaxOpenConns(config.MaxOpenConnections)
	database.SetMaxIdleConns(config.MaxOpenConnections)
	store := &Store{db: database}
	if err := database.PingContext(context.Background()); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping sqlite-vec database: %w", err)
	}
	if err := store.initialize(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}

func (store *Store) EnsureIndex(ctx context.Context, definition domainknowledge.VectorIndexDefinition) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("sqlite-vec store is not initialized")
	}
	if err := definition.Validate(); err != nil {
		return fmt.Errorf("validate vector index definition: %w", err)
	}
	desired, err := newProfileDefinition(definition)
	if err != nil {
		return err
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()

	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite-vec index transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	current, exists, err := loadProfile(ctx, transaction, desired.ID)
	if err != nil {
		return err
	}
	if exists {
		if current.Dimensions != desired.Dimensions || current.Metric != desired.Metric || current.TableName != desired.TableName {
			return fmt.Errorf(
				"sqlite-vec profile %q is incompatible: existing dimensions=%d metric=%s, requested dimensions=%d metric=%s",
				desired.ID,
				current.Dimensions,
				current.Metric,
				desired.Dimensions,
				desired.Metric,
			)
		}
	} else {
		now := time.Now().UTC().UnixMilli()
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO local_vector_profiles(
				profile_id, dimensions, distance_metric, table_name, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?)
		`, desired.ID, desired.Dimensions, desired.Metric, desired.TableName, now, now); err != nil {
			return fmt.Errorf("save sqlite-vec profile %q: %w", desired.ID, err)
		}
	}
	if _, err := transaction.ExecContext(ctx, vectorTableStatement(desired)); err != nil {
		return fmt.Errorf("create sqlite-vec table for profile %q: %w", desired.ID, err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit sqlite-vec index transaction: %w", err)
	}
	return nil
}

func (store *Store) Upsert(ctx context.Context, embeddings []domainknowledge.EmbeddingVector) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("sqlite-vec store is not initialized")
	}
	if len(embeddings) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(embeddings))
	for _, embedding := range embeddings {
		if err := embedding.Validate(); err != nil {
			return fmt.Errorf("validate sqlite-vec embedding %q: %w", embedding.ID, err)
		}
		if _, exists := seen[embedding.ID]; exists {
			return fmt.Errorf("embedding vector id %q is duplicated", embedding.ID)
		}
		seen[embedding.ID] = struct{}{}
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()

	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite-vec upsert: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	profiles := make(map[string]profileDefinition)
	for _, embedding := range embeddings {
		profile, exists := profiles[embedding.EmbeddingProfileID]
		if !exists {
			profile, exists, err = loadProfile(ctx, transaction, embedding.EmbeddingProfileID)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("sqlite-vec profile %q is missing; call EnsureIndex before Upsert", embedding.EmbeddingProfileID)
			}
			profiles[embedding.EmbeddingProfileID] = profile
		}
		if embedding.Dimensions != profile.Dimensions {
			return fmt.Errorf("embedding %q dimensions %d do not match sqlite-vec profile %q dimensions %d", embedding.ID, embedding.Dimensions, profile.ID, profile.Dimensions)
		}
		if err := upsertEmbedding(ctx, transaction, profile, embedding); err != nil {
			return err
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit sqlite-vec upsert: %w", err)
	}
	return nil
}

func (store *Store) Search(ctx context.Context, query domainknowledge.VectorQuery) ([]domainknowledge.VectorHit, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("sqlite-vec store is not initialized")
	}
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("validate sqlite-vec query: %w", err)
	}
	metric, err := normalizeMetric(query.DistanceMetricID)
	if err != nil {
		return nil, err
	}
	profile, exists, err := loadProfile(ctx, store.db, query.EmbeddingProfileID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("sqlite-vec profile %q is missing", query.EmbeddingProfileID)
	}
	if profile.Metric != metric {
		return nil, fmt.Errorf("sqlite-vec profile %q uses metric %s, not %s", profile.ID, profile.Metric, metric)
	}
	if len(query.Vector) != profile.Dimensions {
		return nil, fmt.Errorf("query dimensions %d do not match sqlite-vec profile %q dimensions %d", len(query.Vector), profile.ID, profile.Dimensions)
	}
	serialized := serializeFloat32(query.Vector)
	knowledgeBaseIDs := query.KnowledgeBaseIDs
	if len(knowledgeBaseIDs) == 0 {
		knowledgeBaseIDs = []string{""}
	}
	merged := make(map[string]domainknowledge.VectorHit)
	for _, knowledgeBaseID := range knowledgeBaseIDs {
		hits, err := searchProfile(ctx, store.db, profile, serialized, knowledgeBaseID, query.TopK)
		if err != nil {
			return nil, err
		}
		for _, hit := range hits {
			current, exists := merged[hit.EmbeddingID]
			if !exists || hit.Distance < current.Distance {
				merged[hit.EmbeddingID] = hit
			}
		}
	}
	hits := make([]domainknowledge.VectorHit, 0, len(merged))
	for _, hit := range merged {
		hit.Score = scoreForDistance(profile.Metric, hit.Distance)
		hits = append(hits, hit)
	}
	sort.SliceStable(hits, func(left, right int) bool {
		if hits[left].Distance == hits[right].Distance {
			return hits[left].EmbeddingID < hits[right].EmbeddingID
		}
		return hits[left].Distance < hits[right].Distance
	})
	if len(hits) > query.TopK {
		hits = hits[:query.TopK]
	}
	for index := range hits {
		hits[index].Rank = index + 1
	}
	return hits, nil
}

func (store *Store) MarkDeleted(ctx context.Context, deletion domainknowledge.VectorDeletion) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("sqlite-vec store is not initialized")
	}
	if len(deletion.EmbeddingIDs) == 0 {
		return nil
	}
	if err := deletion.Validate(); err != nil {
		return fmt.Errorf("validate sqlite-vec deletion: %w", err)
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()

	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite-vec deletion: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	profile, exists, err := loadProfile(ctx, transaction, deletion.EmbeddingProfileID)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	deletedAt := deletion.DeletedAt
	if deletedAt.IsZero() {
		deletedAt = time.Now().UTC()
	}
	seen := make(map[string]struct{}, len(deletion.EmbeddingIDs))
	for _, embeddingID := range deletion.EmbeddingIDs {
		if _, duplicate := seen[embeddingID]; duplicate {
			return fmt.Errorf("vector deletion embedding id %q is duplicated", embeddingID)
		}
		seen[embeddingID] = struct{}{}
		var rowID int64
		var currentProfile string
		var syncSequence int64
		err := transaction.QueryRowContext(ctx, `
			SELECT id, profile_id, sync_sequence
			FROM local_vector_records
			WHERE embedding_id = ?
		`, embeddingID).Scan(&rowID, &currentProfile, &syncSequence)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("load sqlite-vec embedding %q for deletion: %w", embeddingID, err)
		}
		if currentProfile != profile.ID {
			return fmt.Errorf("embedding %q belongs to sqlite-vec profile %q, not %q", embeddingID, currentProfile, profile.ID)
		}
		if syncSequence > deletion.SyncSequence {
			continue
		}
		if _, err := transaction.ExecContext(ctx, "DELETE FROM "+quoteIdentifier(profile.TableName)+" WHERE rowid = ?", rowID); err != nil {
			return fmt.Errorf("remove sqlite-vec payload %q: %w", embeddingID, err)
		}
		if _, err := transaction.ExecContext(ctx, `
			UPDATE local_vector_records
			SET deleted = 1, deleted_at = ?, delete_reason = ?, sync_sequence = ?, updated_at = ?
			WHERE id = ?
		`, deletedAt.UnixMilli(), "logically deleted", deletion.SyncSequence, time.Now().UTC().UnixMilli(), rowID); err != nil {
			return fmt.Errorf("mark sqlite-vec embedding %q deleted: %w", embeddingID, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit sqlite-vec deletion: %w", err)
	}
	return nil
}

func (store *Store) Health(ctx context.Context) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("sqlite-vec store is not initialized")
	}
	var version string
	if err := store.db.QueryRowContext(ctx, `SELECT vec_version()`).Scan(&version); err != nil {
		return fmt.Errorf("check sqlite-vec health: %w", err)
	}
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("sqlite-vec health check returned an empty version")
	}
	return nil
}

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func loadProfile(ctx context.Context, queryer queryer, profileID string) (profileDefinition, bool, error) {
	var profile profileDefinition
	err := queryer.QueryRowContext(ctx, `
		SELECT profile_id, dimensions, distance_metric, table_name
		FROM local_vector_profiles
		WHERE profile_id = ?
	`, strings.TrimSpace(profileID)).Scan(&profile.ID, &profile.Dimensions, &profile.Metric, &profile.TableName)
	if errors.Is(err, sql.ErrNoRows) {
		return profileDefinition{}, false, nil
	}
	if err != nil {
		return profileDefinition{}, false, fmt.Errorf("load sqlite-vec profile %q: %w", profileID, err)
	}
	return profile, true, nil
}

func upsertEmbedding(ctx context.Context, transaction *sql.Tx, profile profileDefinition, embedding domainknowledge.EmbeddingVector) error {
	var rowID int64
	var currentProfile string
	var currentSequence int64
	var currentDeleted bool
	err := transaction.QueryRowContext(ctx, `
		SELECT id, profile_id, sync_sequence, deleted
		FROM local_vector_records
		WHERE embedding_id = ?
	`, embedding.ID).Scan(&rowID, &currentProfile, &currentSequence, &currentDeleted)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load sqlite-vec embedding %q: %w", embedding.ID, err)
	}
	if exists && currentProfile != profile.ID {
		return fmt.Errorf("embedding %q belongs to sqlite-vec profile %q, not %q", embedding.ID, currentProfile, profile.ID)
	}
	if exists && (embedding.SyncSequence < currentSequence ||
		(embedding.SyncSequence == currentSequence && currentDeleted && !embedding.Deletion.Deleted)) {
		return nil
	}
	now := time.Now().UTC().UnixMilli()
	var deletedAt any
	if embedding.Deletion.DeletedAt != nil {
		deletedAt = embedding.Deletion.DeletedAt.UTC().UnixMilli()
	}
	if exists {
		if _, err := transaction.ExecContext(ctx, "DELETE FROM "+quoteIdentifier(profile.TableName)+" WHERE rowid = ?", rowID); err != nil {
			return fmt.Errorf("replace sqlite-vec payload %q: %w", embedding.ID, err)
		}
		if _, err := transaction.ExecContext(ctx, `
			UPDATE local_vector_records SET
				chunk_id = ?, knowledge_base_id = ?, document_id = ?, document_version = ?,
				profile_id = ?, dimensions = ?, deleted = ?, deleted_at = ?, delete_reason = ?,
				sync_sequence = ?, updated_at = ?
			WHERE id = ?
		`, embedding.ChunkID, embedding.KnowledgeBaseID, embedding.DocumentID, embedding.DocumentVersion,
			embedding.EmbeddingProfileID, embedding.Dimensions, embedding.Deletion.Deleted, deletedAt,
			embedding.Deletion.DeleteReason, embedding.SyncSequence, now, rowID); err != nil {
			return fmt.Errorf("update sqlite-vec metadata %q: %w", embedding.ID, err)
		}
	} else {
		result, err := transaction.ExecContext(ctx, `
			INSERT INTO local_vector_records(
				embedding_id, chunk_id, knowledge_base_id, document_id, document_version,
				profile_id, dimensions, deleted, deleted_at, delete_reason, sync_sequence, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, embedding.ID, embedding.ChunkID, embedding.KnowledgeBaseID, embedding.DocumentID,
			embedding.DocumentVersion, embedding.EmbeddingProfileID, embedding.Dimensions,
			embedding.Deletion.Deleted, deletedAt, embedding.Deletion.DeleteReason, embedding.SyncSequence, now)
		if err != nil {
			return fmt.Errorf("insert sqlite-vec metadata %q: %w", embedding.ID, err)
		}
		rowID, err = result.LastInsertId()
		if err != nil {
			return fmt.Errorf("read sqlite-vec row id for %q: %w", embedding.ID, err)
		}
	}
	if embedding.Deletion.Deleted {
		return nil
	}
	serialized := serializeFloat32(embedding.Values)
	statement := fmt.Sprintf(
		"INSERT INTO %s(rowid, knowledge_base_id, deleted, embedding_id, chunk_id, embedding) VALUES (?, ?, ?, ?, ?, ?)",
		quoteIdentifier(profile.TableName),
	)
	if _, err := transaction.ExecContext(ctx, statement, rowID, embedding.KnowledgeBaseID, false, embedding.ID, embedding.ChunkID, serialized); err != nil {
		return fmt.Errorf("insert sqlite-vec payload %q: %w", embedding.ID, err)
	}
	return nil
}

func searchProfile(ctx context.Context, database *sql.DB, profile profileDefinition, vector []byte, knowledgeBaseID string, topK int) ([]domainknowledge.VectorHit, error) {
	statement := strings.Builder{}
	statement.WriteString("SELECT embedding_id, chunk_id, distance FROM ")
	statement.WriteString(quoteIdentifier(profile.TableName))
	statement.WriteString(" WHERE embedding MATCH ? AND k = ? AND deleted = 0")
	arguments := []any{vector, topK}
	if knowledgeBaseID != "" {
		statement.WriteString(" AND knowledge_base_id = ?")
		arguments = append(arguments, knowledgeBaseID)
	}
	statement.WriteString(" ORDER BY distance ASC")
	rows, err := database.QueryContext(ctx, statement.String(), arguments...)
	if err != nil {
		return nil, fmt.Errorf("search sqlite-vec profile %q: %w", profile.ID, err)
	}
	defer rows.Close()
	hits := make([]domainknowledge.VectorHit, 0, topK)
	for rows.Next() {
		var hit domainknowledge.VectorHit
		if err := rows.Scan(&hit.EmbeddingID, &hit.ChunkID, &hit.Distance); err != nil {
			return nil, fmt.Errorf("scan sqlite-vec profile %q result: %w", profile.ID, err)
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite-vec profile %q results: %w", profile.ID, err)
	}
	return hits, nil
}

func scoreForDistance(metric string, distance float64) float64 {
	switch metric {
	case "cosine":
		return 1 - distance
	case "l2":
		return 1 / (1 + math.Max(distance, 0))
	default:
		return 0
	}
}

func serializeFloat32(values []float32) []byte {
	serialized := make([]byte, len(values)*4)
	for index, value := range values {
		binary.LittleEndian.PutUint32(serialized[index*4:], math.Float32bits(value))
	}
	return serialized
}
