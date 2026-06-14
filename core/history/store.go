package history

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	_ "github.com/mattn/go-sqlite3"
)

type FileSnapshot struct {
	Path      string
	Size      int64
	Hash      [32]byte
	Content   []byte
	Binary    bool
	TooLarge  bool
	Mode      os.FileMode
	Available bool
}

type Checkpoint struct {
	ID        string
	Workspace string
	SessionID string
	RequestID string
	Title     string
	Reason    string
	CreatedAt time.Time
}

type CheckpointSummary struct {
	ID          string
	Workspace   string
	SessionID   string
	RequestID   string
	Title       string
	Reason      string
	ChangeCount int
	CreatedAt   time.Time
}

type FileChange struct {
	Path       string
	ChangeType string
	Before     *FileSnapshot
	After      *FileSnapshot
	CreatedAt  time.Time
}

type StoredFileChange struct {
	ID           int64
	CheckpointID string
	Path         string
	ChangeType   string
	Before       *FileSnapshot
	After        *FileSnapshot
	CreatedAt    time.Time
}

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(path string) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func DefaultSQLitePath(workspace string) (string, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(abs))))

	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		base, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(base, ".myai")
	} else {
		base = filepath.Join(base, "myai")
	}

	return filepath.Join(base, "workspaces", hex.EncodeToString(hash[:16]), "history.db"), nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) HasBaseline(ctx context.Context, workspace string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1
		FROM workspaces
		WHERE workspace = ?
		LIMIT 1
	`, workspace).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func (s *SQLiteStore) LoadBaseline(ctx context.Context, workspace string) (map[string]FileSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT path, size, hash, content, binary, too_large, mode, available
		FROM workspace_baseline_files
		WHERE workspace = ?
	`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]FileSnapshot)
	for rows.Next() {
		var item FileSnapshot
		var hashText string
		var mode int64
		var binary int
		var tooLarge int
		var available int
		if err := rows.Scan(&item.Path, &item.Size, &hashText, &item.Content, &binary, &tooLarge, &mode, &available); err != nil {
			return nil, err
		}
		hash, err := decodeHash(hashText)
		if err != nil {
			return nil, err
		}
		item.Hash = hash
		item.Binary = binary != 0
		item.TooLarge = tooLarge != 0
		item.Mode = os.FileMode(mode)
		item.Available = available != 0
		result[item.Path] = item
	}
	return result, rows.Err()
}

func (s *SQLiteStore) ReplaceBaseline(ctx context.Context, workspace string, files map[string]FileSnapshot) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO workspaces(workspace, created_at, updated_at)
		VALUES(?, ?, ?)
		ON CONFLICT(workspace) DO UPDATE SET updated_at = excluded.updated_at
	`, workspace, now, now); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM workspace_baseline_files WHERE workspace = ?`, workspace); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO workspace_baseline_files(
			workspace, path, size, hash, content, binary, too_large, mode, available, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, file := range files {
		if _, err = stmt.ExecContext(
			ctx,
			workspace,
			file.Path,
			file.Size,
			hex.EncodeToString(file.Hash[:]),
			nullableContent(file),
			file.Binary,
			file.TooLarge,
			int64(file.Mode),
			file.Available,
			now,
		); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) SaveCheckpoint(ctx context.Context, checkpoint Checkpoint, changes []FileChange) (string, error) {
	if len(changes) == 0 {
		return "", nil
	}
	if strings.TrimSpace(checkpoint.Workspace) == "" {
		return "", errors.New("checkpoint workspace is empty")
	}
	if strings.TrimSpace(checkpoint.ID) == "" {
		checkpoint.ID = uuid.NewString()
	}
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = time.Now()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := checkpoint.CreatedAt.UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO workspaces(workspace, created_at, updated_at)
		VALUES(?, ?, ?)
		ON CONFLICT(workspace) DO UPDATE SET updated_at = excluded.updated_at
	`, checkpoint.Workspace, now, now); err != nil {
		return "", err
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO checkpoints(id, workspace, session_id, request_id, title, reason, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)
	`, checkpoint.ID, checkpoint.Workspace, checkpoint.SessionID, checkpoint.RequestID, checkpoint.Title, checkpoint.Reason, now); err != nil {
		return "", err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO file_changes(
			checkpoint_id, path, change_type,
			before_hash, after_hash,
			before_content, after_content,
			before_size, after_size,
			before_mode, after_mode,
			before_available, after_available,
			created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return "", err
	}
	defer stmt.Close()

	for _, change := range changes {
		createdAt := change.CreatedAt
		if createdAt.IsZero() {
			createdAt = checkpoint.CreatedAt
		}
		if _, err = stmt.ExecContext(
			ctx,
			checkpoint.ID,
			change.Path,
			change.ChangeType,
			nullableHash(change.Before),
			nullableHash(change.After),
			nullableSnapshotContent(change.Before),
			nullableSnapshotContent(change.After),
			nullableSnapshotSize(change.Before),
			nullableSnapshotSize(change.After),
			nullableSnapshotMode(change.Before),
			nullableSnapshotMode(change.After),
			snapshotAvailable(change.Before),
			snapshotAvailable(change.After),
			createdAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return "", err
		}
	}

	if err = tx.Commit(); err != nil {
		return "", err
	}
	return checkpoint.ID, nil
}

func (s *SQLiteStore) ListCheckpoints(ctx context.Context, workspace string, limit int) ([]CheckpointSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.workspace, c.session_id, c.request_id, c.title, c.reason, c.created_at, COUNT(fc.id)
		FROM checkpoints c
		LEFT JOIN file_changes fc ON fc.checkpoint_id = c.id
		WHERE c.workspace = ?
		GROUP BY c.id, c.workspace, c.session_id, c.request_id, c.title, c.reason, c.created_at
		ORDER BY c.created_at DESC
		LIMIT ?
	`, workspace, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]CheckpointSummary, 0)
	for rows.Next() {
		var item CheckpointSummary
		var createdAt string
		if err := rows.Scan(&item.ID, &item.Workspace, &item.SessionID, &item.RequestID, &item.Title, &item.Reason, &createdAt, &item.ChangeCount); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		item.CreatedAt = parsed
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) LoadCheckpointChanges(ctx context.Context, workspace string, checkpointID string) ([]StoredFileChange, error) {
	checkpointID = strings.TrimSpace(checkpointID)
	if checkpointID == "" {
		return nil, errors.New("checkpoint id is empty")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			fc.id, fc.checkpoint_id, fc.path, fc.change_type,
			fc.before_hash, fc.after_hash,
			fc.before_content, fc.after_content,
			fc.before_size, fc.after_size,
			fc.before_mode, fc.after_mode,
			fc.before_available, fc.after_available,
			fc.created_at
		FROM file_changes fc
		INNER JOIN checkpoints c ON c.id = fc.checkpoint_id
		WHERE c.workspace = ? AND fc.checkpoint_id = ?
		ORDER BY fc.id ASC
	`, workspace, checkpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]StoredFileChange, 0)
	for rows.Next() {
		var item StoredFileChange
		var beforeHash sql.NullString
		var afterHash sql.NullString
		var beforeContent []byte
		var afterContent []byte
		var beforeSize sql.NullInt64
		var afterSize sql.NullInt64
		var beforeMode sql.NullInt64
		var afterMode sql.NullInt64
		var beforeAvailable int
		var afterAvailable int
		var createdAt string
		if err := rows.Scan(
			&item.ID,
			&item.CheckpointID,
			&item.Path,
			&item.ChangeType,
			&beforeHash,
			&afterHash,
			&beforeContent,
			&afterContent,
			&beforeSize,
			&afterSize,
			&beforeMode,
			&afterMode,
			&beforeAvailable,
			&afterAvailable,
			&createdAt,
		); err != nil {
			return nil, err
		}

		item.Before, err = storedSnapshot(item.Path, beforeHash, beforeContent, beforeSize, beforeMode, beforeAvailable != 0)
		if err != nil {
			return nil, err
		}
		item.After, err = storedSnapshot(item.Path, afterHash, afterContent, afterSize, afterMode, afterAvailable != 0)
		if err != nil {
			return nil, err
		}
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) init(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS workspaces (
			workspace TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS workspace_baseline_files (
			workspace TEXT NOT NULL,
			path TEXT NOT NULL,
			size INTEGER NOT NULL,
			hash TEXT NOT NULL,
			content BLOB,
			binary INTEGER NOT NULL,
			too_large INTEGER NOT NULL,
			mode INTEGER NOT NULL,
			available INTEGER NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(workspace, path),
			FOREIGN KEY(workspace) REFERENCES workspaces(workspace) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS checkpoints (
			id TEXT PRIMARY KEY,
			workspace TEXT NOT NULL,
			session_id TEXT,
			request_id TEXT,
			title TEXT,
			reason TEXT,
			created_at TEXT NOT NULL,
			FOREIGN KEY(workspace) REFERENCES workspaces(workspace) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS file_changes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			checkpoint_id TEXT NOT NULL,
			path TEXT NOT NULL,
			change_type TEXT NOT NULL,
			before_hash TEXT,
			after_hash TEXT,
			before_content BLOB,
			after_content BLOB,
			before_size INTEGER,
			after_size INTEGER,
			before_mode INTEGER,
			after_mode INTEGER,
			before_available INTEGER NOT NULL DEFAULT 0,
			after_available INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			FOREIGN KEY(checkpoint_id) REFERENCES checkpoints(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_file_changes_checkpoint ON file_changes(checkpoint_id)`,
		`CREATE INDEX IF NOT EXISTS idx_file_changes_path ON file_changes(path)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("init sqlite history failed: %w", err)
		}
	}
	if err := s.ensureFileChangeColumns(ctx); err != nil {
		return fmt.Errorf("migrate sqlite history failed: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ensureFileChangeColumns(ctx context.Context) error {
	columns, err := s.fileChangeColumns(ctx)
	if err != nil {
		return err
	}

	migrations := map[string]string{
		"before_size":      `ALTER TABLE file_changes ADD COLUMN before_size INTEGER`,
		"after_size":       `ALTER TABLE file_changes ADD COLUMN after_size INTEGER`,
		"before_mode":      `ALTER TABLE file_changes ADD COLUMN before_mode INTEGER`,
		"after_mode":       `ALTER TABLE file_changes ADD COLUMN after_mode INTEGER`,
		"before_available": `ALTER TABLE file_changes ADD COLUMN before_available INTEGER NOT NULL DEFAULT 0`,
		"after_available":  `ALTER TABLE file_changes ADD COLUMN after_available INTEGER NOT NULL DEFAULT 0`,
	}
	for column, statement := range migrations {
		if columns[column] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) fileChangeColumns(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(file_changes)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func nullableContent(file FileSnapshot) any {
	if !file.Available {
		return nil
	}
	return file.Content
}

func nullableHash(file *FileSnapshot) any {
	if file == nil {
		return nil
	}
	return hex.EncodeToString(file.Hash[:])
}

func nullableSnapshotContent(file *FileSnapshot) any {
	if file == nil || !file.Available {
		return nil
	}
	return file.Content
}

func nullableSnapshotSize(file *FileSnapshot) any {
	if file == nil {
		return nil
	}
	return file.Size
}

func nullableSnapshotMode(file *FileSnapshot) any {
	if file == nil {
		return nil
	}
	return int64(file.Mode)
}

func snapshotAvailable(file *FileSnapshot) bool {
	return file != nil && file.Available
}

func storedSnapshot(path string, hashText sql.NullString, content []byte, size sql.NullInt64, mode sql.NullInt64, available bool) (*FileSnapshot, error) {
	if !hashText.Valid {
		return nil, nil
	}

	hash, err := decodeHash(hashText.String)
	if err != nil {
		return nil, err
	}
	item := &FileSnapshot{
		Path:      path,
		Hash:      hash,
		Available: available,
	}
	if size.Valid {
		item.Size = size.Int64
	}
	if mode.Valid {
		item.Mode = os.FileMode(mode.Int64)
	}
	if available {
		item.Content = append([]byte(nil), content...)
	}
	return item, nil
}

func decodeHash(value string) ([32]byte, error) {
	var hash [32]byte
	data, err := hex.DecodeString(value)
	if err != nil {
		return hash, err
	}
	if len(data) != len(hash) {
		return hash, fmt.Errorf("invalid hash length: %d", len(data))
	}
	copy(hash[:], data)
	return hash, nil
}
