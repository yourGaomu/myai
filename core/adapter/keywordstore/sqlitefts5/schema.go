package sqlitefts5

import (
	"context"
	"fmt"
)

func (store *Store) initialize(ctx context.Context) error {
	var fts5Enabled int
	if err := store.db.QueryRowContext(ctx, `SELECT sqlite_compileoption_used('ENABLE_FTS5')`).Scan(&fts5Enabled); err != nil {
		return fmt.Errorf("check SQLite FTS5 support: %w", err)
	}
	if fts5Enabled != 1 {
		return fmt.Errorf("SQLite driver was built without FTS5 support")
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS keyword_documents (
			rowid INTEGER PRIMARY KEY AUTOINCREMENT,
			chunk_id TEXT NOT NULL UNIQUE,
			knowledge_base_id TEXT NOT NULL,
			document_id TEXT NOT NULL,
			document_version INTEGER NOT NULL,
			text TEXT NOT NULL,
			deleted INTEGER NOT NULL DEFAULT 0,
			sync_sequence INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_keyword_documents_knowledge_base
			ON keyword_documents(knowledge_base_id, deleted)`,
		`CREATE INDEX IF NOT EXISTS idx_keyword_documents_sync
			ON keyword_documents(sync_sequence)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS keyword_documents_fts USING fts5(
			text,
			content='keyword_documents',
			content_rowid='rowid',
			tokenize='unicode61'
		)`,
		`CREATE TRIGGER IF NOT EXISTS keyword_documents_ai AFTER INSERT ON keyword_documents BEGIN
			INSERT INTO keyword_documents_fts(rowid, text) VALUES (new.rowid, new.text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS keyword_documents_ad AFTER DELETE ON keyword_documents BEGIN
			INSERT INTO keyword_documents_fts(keyword_documents_fts, rowid, text)
			VALUES ('delete', old.rowid, old.text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS keyword_documents_au AFTER UPDATE OF text ON keyword_documents BEGIN
			INSERT INTO keyword_documents_fts(keyword_documents_fts, rowid, text)
			VALUES ('delete', old.rowid, old.text);
			INSERT INTO keyword_documents_fts(rowid, text) VALUES (new.rowid, new.text);
		END`,
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize SQLite FTS5 keyword store: %w", err)
		}
	}
	return nil
}
