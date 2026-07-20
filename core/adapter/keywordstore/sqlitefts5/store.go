package sqlitefts5

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type Store struct {
	db      *sql.DB
	writeMu sync.Mutex
}

var _ knowledgeport.KeywordStore = (*Store)(nil)

func Open(config Config) (*Store, error) {
	config = config.normalize()
	if err := config.validate(); err != nil {
		return nil, err
	}
	if err := ensureParentDirectory(config.Path); err != nil {
		return nil, err
	}
	dsn, err := dataSourceName(config.Path)
	if err != nil {
		return nil, fmt.Errorf("build SQLite FTS5 data source: %w", err)
	}
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite FTS5 database: %w", err)
	}
	database.SetMaxOpenConns(config.MaxOpenConnections)
	database.SetMaxIdleConns(config.MaxOpenConnections)
	store := &Store{db: database}
	if err := database.PingContext(context.Background()); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping SQLite FTS5 database: %w", err)
	}
	if err := store.initialize(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}

func (store *Store) Upsert(ctx context.Context, documents []domainknowledge.KeywordDocument) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("SQLite FTS5 keyword store is not initialized")
	}
	if len(documents) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(documents))
	for _, document := range documents {
		if err := document.Validate(); err != nil {
			return fmt.Errorf("validate keyword document %q: %w", document.ChunkID, err)
		}
		if _, exists := seen[document.ChunkID]; exists {
			return fmt.Errorf("keyword document chunk id %q is duplicated", document.ChunkID)
		}
		seen[document.ChunkID] = struct{}{}
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()

	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SQLite FTS5 upsert: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	statement, err := transaction.PrepareContext(ctx, `
		INSERT INTO keyword_documents(
			chunk_id, knowledge_base_id, document_id, document_version,
			text, deleted, sync_sequence, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chunk_id) DO UPDATE SET
			knowledge_base_id = excluded.knowledge_base_id,
			document_id = excluded.document_id,
			document_version = excluded.document_version,
			text = excluded.text,
			deleted = excluded.deleted,
			sync_sequence = excluded.sync_sequence,
			updated_at = excluded.updated_at
		WHERE excluded.sync_sequence > keyword_documents.sync_sequence
			OR (excluded.sync_sequence = keyword_documents.sync_sequence
				AND (keyword_documents.deleted = 0 OR excluded.deleted = 1))
	`)
	if err != nil {
		return fmt.Errorf("prepare SQLite FTS5 upsert: %w", err)
	}
	defer statement.Close()
	now := time.Now().UTC().UnixMilli()
	for _, document := range documents {
		if _, err := statement.ExecContext(
			ctx,
			document.ChunkID,
			document.KnowledgeBaseID,
			document.DocumentID,
			document.DocumentVersion,
			document.Text,
			document.Deletion.Deleted,
			document.SyncSequence,
			now,
		); err != nil {
			return fmt.Errorf("upsert SQLite FTS5 document %q: %w", document.ChunkID, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit SQLite FTS5 upsert: %w", err)
	}
	return nil
}

func (store *Store) Search(ctx context.Context, query domainknowledge.KeywordQuery) ([]domainknowledge.KeywordHit, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("SQLite FTS5 keyword store is not initialized")
	}
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("validate keyword query: %w", err)
	}
	match, err := matchExpression(query.Text)
	if err != nil {
		return nil, err
	}
	statement := strings.Builder{}
	statement.WriteString(`
		SELECT d.chunk_id, bm25(keyword_documents_fts) AS relevance
		FROM keyword_documents_fts
		INNER JOIN keyword_documents d ON d.rowid = keyword_documents_fts.rowid
		WHERE keyword_documents_fts MATCH ? AND d.deleted = 0
	`)
	arguments := make([]any, 0, len(query.KnowledgeBaseIDs)+2)
	arguments = append(arguments, match)
	if len(query.KnowledgeBaseIDs) > 0 {
		statement.WriteString(" AND d.knowledge_base_id IN (")
		for index, knowledgeBaseID := range query.KnowledgeBaseIDs {
			if index > 0 {
				statement.WriteByte(',')
			}
			statement.WriteByte('?')
			arguments = append(arguments, knowledgeBaseID)
		}
		statement.WriteByte(')')
	}
	statement.WriteString(" ORDER BY relevance ASC LIMIT ?")
	arguments = append(arguments, query.TopK)

	rows, err := store.db.QueryContext(ctx, statement.String(), arguments...)
	if err != nil {
		return nil, fmt.Errorf("search SQLite FTS5: %w", err)
	}
	defer rows.Close()
	hits := make([]domainknowledge.KeywordHit, 0, query.TopK)
	for rows.Next() {
		var chunkID string
		var relevance float64
		if err := rows.Scan(&chunkID, &relevance); err != nil {
			return nil, fmt.Errorf("scan SQLite FTS5 result: %w", err)
		}
		hits = append(hits, domainknowledge.KeywordHit{
			ChunkID: chunkID,
			Score:   math.Max(-relevance, 0),
			Rank:    len(hits) + 1,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SQLite FTS5 results: %w", err)
	}
	return hits, nil
}

func (store *Store) MarkDeleted(ctx context.Context, chunkIDs []string, syncSequence int64) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("SQLite FTS5 keyword store is not initialized")
	}
	if len(chunkIDs) == 0 {
		return nil
	}
	if syncSequence < 0 {
		return fmt.Errorf("keyword deletion sync sequence must not be negative")
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	statement := strings.Builder{}
	statement.WriteString("UPDATE keyword_documents SET deleted = 1, sync_sequence = ?, updated_at = ? WHERE chunk_id IN (")
	arguments := make([]any, 0, len(chunkIDs)+3)
	arguments = append(arguments, syncSequence, time.Now().UTC().UnixMilli())
	seen := make(map[string]struct{}, len(chunkIDs))
	for index, chunkID := range chunkIDs {
		chunkID = strings.TrimSpace(chunkID)
		if chunkID == "" {
			return fmt.Errorf("keyword deletion chunk id must not be empty")
		}
		if _, exists := seen[chunkID]; exists {
			return fmt.Errorf("keyword deletion chunk id %q is duplicated", chunkID)
		}
		seen[chunkID] = struct{}{}
		if index > 0 {
			statement.WriteByte(',')
		}
		statement.WriteByte('?')
		arguments = append(arguments, chunkID)
	}
	statement.WriteString(") AND sync_sequence <= ?")
	arguments = append(arguments, syncSequence)
	if _, err := store.db.ExecContext(ctx, statement.String(), arguments...); err != nil {
		return fmt.Errorf("mark SQLite FTS5 documents deleted: %w", err)
	}
	return nil
}

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

func matchExpression(text string) (string, error) {
	terms := strings.Fields(strings.TrimSpace(text))
	if len(terms) == 0 {
		return "", fmt.Errorf("keyword query text is required")
	}
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " OR "), nil
}
