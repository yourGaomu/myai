package journal

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"

	outboxencoding "myai/core/adapter/persistence/outbox/encoding"
	outbox "myai/core/port/outbox"
)

const (
	DefaultMaxPending       = 10000
	DefaultMaxEntries       = 20000
	DefaultMaxBytes   int64 = 64 << 20
	DefaultRetainDone       = 10000
)

type Options struct {
	// Scope must be a caller-generated SHA256 of the destination identity. Never
	// pass a connection URI or credentials. Changing destination fails closed.
	Scope      string
	MaxPending int
	MaxEntries int
	MaxBytes   int64
	RetainDone int
}

// Journal owns one SQLite connection with an exclusive process lock. The parent
// must be a dedicated application directory: Open protects it as user-only.
// FULL synchronous DELETE journaling commits before acceptance. MaxBytes bounds
// database pages; a rollback transaction may temporarily need additional space.
// Done/cancelled receipts retain hashes only, for a bounded deduplication window.
// Tombstones do not expire automatically and also count towards MaxEntries.
type Journal struct {
	mu      sync.Mutex
	db      *sql.DB
	options Options
}

func Open(path string, options Options) (*Journal, error) {
	if decoded, err := outboxScope(options.Scope); err != nil || len(decoded) != 32 {
		return nil, errors.New("outbox destination scope must be a SHA256 digest")
	}
	if options.MaxPending == 0 {
		options.MaxPending = DefaultMaxPending
	}
	if options.MaxEntries == 0 {
		options.MaxEntries = DefaultMaxEntries
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = DefaultMaxBytes
	}
	if options.RetainDone == 0 {
		options.RetainDone = DefaultRetainDone
	}
	if options.MaxPending < 1 || options.MaxEntries < 1 || options.MaxBytes < 65536 || options.RetainDone < 0 {
		return nil, errors.New("invalid outbox capacity options")
	}
	if strings.TrimSpace(path) == "" || path == ":memory:" {
		return nil, errors.New("outbox requires a durable file path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("resolve outbox path failed")
	}
	parent := filepath.Dir(absolute)
	if err := rejectLinks(parent); err != nil {
		return nil, err
	}
	_, parentErr := os.Lstat(parent)
	createdParent := errors.Is(parentErr, os.ErrNotExist)
	if parentErr != nil && !createdParent {
		return nil, errors.New("inspect outbox directory failed")
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, errors.New("create outbox directory failed")
	}
	if createdParent {
		if err := protect(parent, true); err != nil {
			return nil, errors.New("protect outbox directory failed")
		}
	} else if err := privateDirectory(parent); err != nil {
		return nil, errors.New("outbox parent must be a private user directory")
	}
	if err := rejectLinks(parent); err != nil {
		return nil, err
	}
	for _, name := range []string{absolute, absolute + "-journal", absolute + "-wal", absolute + "-shm"} {
		info, err := os.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return nil, errors.New("outbox path must contain regular files")
		}
		if err := singleLink(name); err != nil {
			return nil, errors.New("outbox file must not be hard linked")
		}
		if err := protect(name, false); err != nil {
			return nil, errors.New("protect outbox file failed")
		}
	}
	file, err := os.OpenFile(absolute, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errors.New("create outbox database failed")
	}
	if err := file.Close(); err != nil {
		return nil, errors.New("close outbox database file failed")
	}
	if err := protect(absolute, false); err != nil {
		return nil, errors.New("protect outbox database failed")
	}
	urlPath := filepath.ToSlash(absolute)
	if filepath.VolumeName(absolute) != "" && !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	fileURL := &url.URL{Scheme: "file", Path: urlPath}
	db, err := sql.Open("sqlite", fileURL.String())
	if err != nil {
		return nil, errors.New("open outbox database failed")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	j := &Journal{db: db, options: options}
	fail := func(err error) (*Journal, error) { _ = db.Close(); return nil, err }
	// Fail immediately if another owner holds the database. Exclusive locking
	// lasts until Close, including while no backend workers are running.
	for _, statement := range []string{
		"PRAGMA busy_timeout=0", "PRAGMA locking_mode=EXCLUSIVE", "PRAGMA journal_mode=DELETE",
		"PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON", "PRAGMA secure_delete=ON",
		"PRAGMA auto_vacuum=FULL",
	} {
		if _, err := db.Exec(statement); err != nil {
			return fail(errors.New("configure or lock outbox database failed"))
		}
	}
	var pageSize int64
	if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return fail(errors.New("read outbox page size failed"))
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", options.MaxBytes/pageSize)); err != nil {
		return fail(outbox.ErrCapacity)
	}
	var existingObjects int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master").Scan(&existingObjects); err != nil {
		return fail(errors.New("inspect outbox schema failed"))
	}
	freshSchema := existingObjects == 0
	if !freshSchema {
		for key, expected := range map[string]string{"version": "1", "scope": strings.ToLower(options.Scope)} {
			var actual string
			if err := db.QueryRow("SELECT value FROM outbox_meta WHERE key=?", key).Scan(&actual); err != nil || actual != expected {
				return fail(errors.New("outbox destination scope or schema metadata is missing or mismatched"))
			}
		}
	}
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS outbox_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS outbox_entries (
			seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE,
			session_id TEXT NOT NULL, state TEXT NOT NULL, payload BLOB,
			hash TEXT NOT NULL, size INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS outbox_pending ON outbox_entries(state,seq)`,
		`CREATE TABLE IF NOT EXISTS outbox_keys (
			kind TEXT NOT NULL, id TEXT NOT NULL, hash TEXT NOT NULL,
			entry_seq INTEGER NOT NULL REFERENCES outbox_entries(seq) ON DELETE CASCADE,
			PRIMARY KEY(kind,id,entry_seq))`,
		`CREATE TABLE IF NOT EXISTS outbox_sessions (id TEXT PRIMARY KEY, reason TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return fail(errors.New("initialize or lock outbox schema failed"))
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fail(errors.New("begin outbox scope check failed"))
	}
	defer tx.Rollback()
	failTransaction := func(cause error) (*Journal, error) { _ = tx.Rollback(); return fail(cause) }
	for key, expected := range map[string]string{"version": "1", "scope": strings.ToLower(options.Scope)} {
		var actual string
		err := tx.QueryRow("SELECT value FROM outbox_meta WHERE key=?", key).Scan(&actual)
		if errors.Is(err, sql.ErrNoRows) {
			if !freshSchema {
				return failTransaction(errors.New("existing outbox is missing destination scope or schema metadata"))
			}
			if _, err = tx.Exec("INSERT INTO outbox_meta(key,value) VALUES(?,?)", key, expected); err != nil {
				return failTransaction(errors.New("initialize outbox scope failed"))
			}
		} else if err != nil || actual != expected {
			return failTransaction(errors.New("outbox destination scope or schema mismatch"))
		}
	}
	if err := j.prune(context.Background(), tx); err != nil {
		return failTransaction(err)
	}
	if err := tx.Commit(); err != nil {
		return failTransaction(errors.New("commit outbox initialization failed"))
	}
	stats, err := j.stats(context.Background())
	if err != nil {
		return fail(err)
	}
	if stats.Bytes > options.MaxBytes {
		return fail(outbox.ErrCapacity)
	}
	return j, nil
}

func rejectLinks(path string) error {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("inspect outbox path failed")
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return errors.New("outbox path must not traverse links")
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
	}
	return nil
}

func outboxScope(value string) ([]byte, error) {
	if len(value) != 64 {
		return nil, errors.New("invalid scope")
	}
	var decoded []byte
	for index := 0; index < len(value); index += 2 {
		var b byte
		for _, c := range value[index : index+2] {
			b <<= 4
			switch {
			case c >= '0' && c <= '9':
				b += byte(c - '0')
			case c >= 'a' && c <= 'f':
				b += byte(c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				b += byte(c - 'A' + 10)
			default:
				return nil, errors.New("invalid scope")
			}
		}
		decoded = append(decoded, b)
	}
	return decoded, nil
}

func (j *Journal) Append(ctx context.Context, batch outbox.Batch) (outbox.Entry, error) {
	frozen, payload, err := outboxencoding.Freeze(batch)
	if err != nil {
		return outbox.Entry{}, err
	}
	if int64(len(payload)) > j.options.MaxBytes {
		return outbox.Entry{}, outbox.ErrCapacity
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.db == nil {
		return outbox.Entry{}, outbox.ErrClosed
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return outbox.Entry{}, journalError("begin append", err)
	}
	defer tx.Rollback()
	var blocked int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM outbox_sessions WHERE id=?", frozen.SessionID).Scan(&blocked); err != nil {
		return outbox.Entry{}, journalError("check session barrier", err)
	}
	if blocked > 0 {
		return outbox.Entry{}, outbox.ErrBlocked
	}
	var seq int64
	var state, hash string
	err = tx.QueryRowContext(ctx, "SELECT seq,state,hash FROM outbox_entries WHERE id=?", frozen.ID).Scan(&seq, &state, &hash)
	if err == nil {
		if hash != outbox.Hash(payload) {
			return outbox.Entry{}, outbox.ErrConflict
		}
		if state != outbox.Pending && state != outbox.Done {
			return outbox.Entry{}, outbox.ErrBlocked
		}
		return outbox.Entry{Sequence: seq, Batch: frozen, State: state}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return outbox.Entry{}, journalError("find receipt", err)
	}
	if err := j.prune(ctx, tx); err != nil {
		return outbox.Entry{}, err
	}
	var pending, entries int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM outbox_entries WHERE state='pending'").Scan(&pending); err != nil {
		return outbox.Entry{}, journalError("count pending", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM outbox_entries)+(SELECT count(*) FROM outbox_sessions)").Scan(&entries); err != nil {
		return outbox.Entry{}, journalError("count entries", err)
	}
	if pending >= j.options.MaxPending || entries >= j.options.MaxEntries {
		return outbox.Entry{}, outbox.ErrCapacity
	}
	type key struct{ kind, id, hash string }
	var keys []key
	add := func(kind, id string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return errors.New("encode outbox identity failed")
		}
		hash := outbox.Hash(encoded)
		var conflict int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM outbox_keys WHERE kind=? AND id=? AND hash<>?", kind, id, hash).Scan(&conflict); err != nil {
			return journalError("check identity", err)
		}
		if conflict > 0 {
			return outbox.ErrConflict
		}
		keys = append(keys, key{kind, id, hash})
		return nil
	}
	for _, record := range frozen.Messages {
		if err := add("message", record.ID, outboxencoding.Message(record)); err != nil {
			return outbox.Entry{}, err
		}
	}
	for _, record := range frozen.Assets {
		if err := add("asset", record.ID, record); err != nil {
			return outbox.Entry{}, err
		}
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO outbox_entries(id,session_id,state,payload,hash,size) VALUES(?,?,'pending',?,?,?)", frozen.ID, frozen.SessionID, payload, outbox.Hash(payload), len(payload))
	if err != nil {
		return outbox.Entry{}, journalError("append record", err)
	}
	seq, err = result.LastInsertId()
	if err != nil {
		return outbox.Entry{}, journalError("read appended sequence", err)
	}
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO outbox_keys(kind,id,hash,entry_seq) VALUES(?,?,?,?)", key.kind, key.id, key.hash, seq); err != nil {
			return outbox.Entry{}, journalError("append identity", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return outbox.Entry{}, journalError("commit append", err)
	}
	return outbox.Entry{Sequence: seq, Batch: frozen, State: outbox.Pending}, nil
}

// Pending quarantines malformed envelopes and blocks their sessions, allowing
// unrelated sessions to recover. Invalid payloads stay private for inspection;
// Stats/Flush report them and they cannot silently disappear or replay.
func (j *Journal) Pending(ctx context.Context) ([]outbox.Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.db == nil {
		return nil, outbox.ErrClosed
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, journalError("begin replay scan", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT seq,id,session_id,payload,hash,state FROM outbox_entries WHERE state NOT IN ('done','cancelled','invalid') ORDER BY seq")
	if err != nil {
		return nil, journalError("scan pending", err)
	}
	type invalid struct {
		seq     int64
		session string
	}
	var bad []invalid
	var entries []outbox.Entry
	for rows.Next() {
		var seq int64
		var id, session, hash, state string
		var payload []byte
		if err := rows.Scan(&seq, &id, &session, &payload, &hash, &state); err != nil {
			_ = rows.Close()
			return nil, journalError("read pending", err)
		}
		var batch outbox.Batch
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&batch)
		var trailing any
		if err == nil && !errors.Is(decoder.Decode(&trailing), io.EOF) {
			err = errors.New("trailing data")
		}
		if state != outbox.Pending || err != nil || outbox.Hash(payload) != hash || batch.ID != id || batch.SessionID != session || batch.Validate() != nil {
			bad = append(bad, invalid{seq, session})
			continue
		}
		entries = append(entries, outbox.Entry{Sequence: seq, Batch: batch, State: state})
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, journalError("finish replay scan", err)
	}
	for _, entry := range bad {
		if _, err := tx.ExecContext(ctx, "UPDATE outbox_entries SET state='invalid' WHERE seq=?", entry.seq); err != nil {
			return nil, journalError("quarantine record", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO outbox_sessions(id,reason) VALUES(?,'invalid') ON CONFLICT(id) DO UPDATE SET reason='invalid'", entry.session); err != nil {
			return nil, journalError("quarantine session", err)
		}
	}
	var ready []outbox.Entry
	for _, entry := range entries {
		var blocked int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM outbox_sessions WHERE id=?", entry.Batch.SessionID).Scan(&blocked); err != nil {
			return nil, journalError("check replay barrier", err)
		}
		if blocked == 0 {
			ready = append(ready, entry)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, journalError("commit replay scan", err)
	}
	return ready, nil
}

func (j *Journal) Ack(ctx context.Context, seq int64) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.db == nil {
		return outbox.ErrClosed
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return journalError("begin ack", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE outbox_entries SET state='done',payload=NULL,size=0 WHERE seq=? AND state='pending'", seq)
	if err != nil {
		return journalError("ack record", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return journalError("check ack", err)
	}
	if count == 0 {
		var state string
		if err := tx.QueryRowContext(ctx, "SELECT state FROM outbox_entries WHERE seq=?", seq).Scan(&state); err != nil || state != outbox.Done {
			return errors.New("outbox ack does not name a pending record")
		}
	}
	if err := j.prune(ctx, tx); err != nil {
		return err
	}
	return journalError("commit ack", tx.Commit())
}

func (j *Journal) Block(ctx context.Context, sessionID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.db == nil {
		return outbox.ErrClosed
	}
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("session id is required")
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return journalError("begin barrier", err)
	}
	defer tx.Rollback()
	if err := j.prune(ctx, tx); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM outbox_entries)+(SELECT count(*) FROM outbox_sessions WHERE id<>?)", sessionID).Scan(&count); err != nil {
		return journalError("count barriers", err)
	}
	if count >= j.options.MaxEntries {
		return outbox.ErrCapacity
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO outbox_sessions(id,reason) VALUES(?,'lifecycle') ON CONFLICT(id) DO NOTHING", sessionID); err != nil {
		return journalError("persist barrier", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE outbox_entries SET state='cancelled',payload=NULL,size=0 WHERE session_id=? AND state='pending'", sessionID); err != nil {
		return journalError("cancel pending", err)
	}
	if err := j.prune(ctx, tx); err != nil {
		return err
	}
	return journalError("commit barrier", tx.Commit())
}

func (j *Journal) Resume(ctx context.Context, sessionID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.db == nil {
		return outbox.ErrClosed
	}
	var invalid int
	if err := j.db.QueryRowContext(ctx, "SELECT count(*) FROM outbox_entries WHERE session_id=? AND state='invalid'", sessionID).Scan(&invalid); err != nil {
		return journalError("check quarantine", err)
	}
	if invalid > 0 {
		return outbox.ErrInvalidRecord
	}
	_, err := j.db.ExecContext(ctx, "DELETE FROM outbox_sessions WHERE id=? AND reason='lifecycle'", sessionID)
	return journalError("resume session", err)
}

func (j *Journal) Blocked(ctx context.Context, sessionID string) (bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.db == nil {
		return false, outbox.ErrClosed
	}
	var count int
	err := j.db.QueryRowContext(ctx, "SELECT count(*) FROM outbox_sessions WHERE id=?", sessionID).Scan(&count)
	return count > 0, journalError("check barrier", err)
}

func (j *Journal) Stats(ctx context.Context) (outbox.Stats, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.db == nil {
		return outbox.Stats{}, outbox.ErrClosed
	}
	return j.stats(ctx)
}

func (j *Journal) stats(ctx context.Context) (outbox.Stats, error) {
	var stats outbox.Stats
	for _, item := range []struct {
		state  string
		target *int
	}{{"pending", &stats.Pending}, {"done", &stats.Done}, {"cancelled", &stats.Cancelled}, {"invalid", &stats.Invalid}} {
		if err := j.db.QueryRowContext(ctx, "SELECT count(*) FROM outbox_entries WHERE state=?", item.state).Scan(item.target); err != nil {
			return stats, journalError("count records", err)
		}
	}
	if err := j.db.QueryRowContext(ctx, "SELECT count(*) FROM outbox_sessions").Scan(&stats.Blocked); err != nil {
		return stats, journalError("count sessions", err)
	}
	if err := j.db.QueryRowContext(ctx, "SELECT COALESCE(sum(size),0) FROM outbox_entries").Scan(&stats.PayloadBytes); err != nil {
		return stats, journalError("count payload bytes", err)
	}
	var pages, size int64
	if err := j.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return stats, journalError("count database pages", err)
	}
	if err := j.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&size); err != nil {
		return stats, journalError("read database page size", err)
	}
	stats.Bytes = pages * size
	return stats, nil
}

func (j *Journal) prune(ctx context.Context, tx *sql.Tx) error {
	retain := j.options.RetainDone
	if retain >= j.options.MaxEntries {
		retain = j.options.MaxEntries - 1
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM outbox_entries WHERE seq IN
		(SELECT seq FROM outbox_entries WHERE state IN ('done','cancelled') ORDER BY seq DESC LIMIT -1 OFFSET ?)`, retain)
	return journalError("prune receipts", err)
}

func (j *Journal) Close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.db == nil {
		return nil
	}
	err := j.db.Close()
	j.db = nil
	return journalError("close journal", err)
}

// SQLite errors must not interpolate paths, scope, record bodies or SQL values.
func journalError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var coded interface{ Code() int }
	if errors.As(err, &coded) && coded.Code()&255 == 13 {
		return outbox.ErrCapacity
	}
	return fmt.Errorf("outbox %s failed", operation)
}
