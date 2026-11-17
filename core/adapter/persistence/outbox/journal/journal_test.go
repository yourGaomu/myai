package journal

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	outbox "myai/core/port/outbox"
	repository "myai/core/port/repository"
)

func testOptions() Options { return Options{Scope: outbox.Hash([]byte("test destination"))} }
func testPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "private", "outbox.db")
}
func testBatch(id, session string) outbox.Batch {
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	return outbox.Batch{Version: outbox.Version, ID: "batch-" + id, SessionID: session, Messages: []repository.MessageRecord{{ID: id, SessionID: session, Role: repository.RoleUser, Content: "chat " + id, CreatedAt: at, Sequence: at.UnixNano()}}}
}
func mustOpen(t *testing.T, path string, options Options) *Journal {
	t.Helper()
	j, err := Open(path, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}

func TestJournalReopenPendingDoneAndStableIdentity(t *testing.T) {
	ctx := context.Background()
	path := testPath(t)
	j := mustOpen(t, path, testOptions())
	first := testBatch("one", "session")
	entry, err := j.Append(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := j.Append(ctx, first)
	if err != nil || duplicate.Sequence != entry.Sequence {
		t.Fatalf("duplicate=%v err=%v", duplicate, err)
	}
	other := testBatch("two", "session")
	second, err := j.Append(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Ack(ctx, entry.Sequence); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j = mustOpen(t, path, testOptions())
	pending, err := j.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Sequence != second.Sequence || pending[0].Batch.Messages[0].ID != "two" {
		t.Fatalf("pending=%v", pending)
	}
	done, err := j.Append(ctx, first)
	if err != nil || done.State != outbox.Done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	var payload []byte
	if err := j.db.QueryRow("SELECT payload FROM outbox_entries WHERE seq=?", entry.Sequence).Scan(&payload); err != nil || len(payload) != 0 {
		t.Fatalf("ACK retained payload err=%v", err)
	}
}

func TestJournalRejectsEnvelopeAndRecordPayloadConflicts(t *testing.T) {
	j := mustOpen(t, testPath(t), testOptions())
	ctx := context.Background()
	batch := testBatch("message", "session")
	if _, err := j.Append(ctx, batch); err != nil {
		t.Fatal(err)
	}
	batch.Messages[0].Content = "changed"
	if _, err := j.Append(ctx, batch); err != outbox.ErrConflict {
		t.Fatalf("envelope conflict=%v", err)
	}
	batch.ID = "different batch"
	if _, err := j.Append(ctx, batch); err != outbox.ErrConflict {
		t.Fatalf("record conflict=%v", err)
	}
	batch.Messages[0].Content = "chat message"
	if _, err := j.Append(ctx, batch); err != nil {
		t.Fatalf("identical immutable record rejected: %v", err)
	}
}

func TestJournalQuarantinesBadRecordAndOnlyBlocksItsSession(t *testing.T) {
	ctx := context.Background()
	path := testPath(t)
	j := mustOpen(t, path, testOptions())
	bad, err := j.Append(ctx, testBatch("bad", "broken"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(ctx, testBatch("later", "broken")); err != nil {
		t.Fatal(err)
	}
	good, err := j.Append(ctx, testBatch("good", "healthy"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.db.Exec("UPDATE outbox_entries SET payload=? WHERE seq=?", []byte(`{"Version":1,"unexpected":"secret"}`), bad.Sequence); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j = mustOpen(t, path, testOptions())
	pending, err := j.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Sequence != good.Sequence {
		t.Fatalf("pending=%v", pending)
	}
	stats, err := j.Stats(ctx)
	if err != nil || stats.Invalid != 1 || stats.Blocked != 1 {
		t.Fatalf("stats=%v err=%v", stats, err)
	}
	if err := j.Resume(ctx, "broken"); err != outbox.ErrInvalidRecord {
		t.Fatalf("quarantine resume=%v", err)
	}
	if _, err := j.Append(ctx, testBatch("new", "broken")); err != outbox.ErrBlocked {
		t.Fatalf("quarantine append=%v", err)
	}
}

func TestJournalCapacityRefusalDoesNotLoseAcceptedRecords(t *testing.T) {
	ctx := context.Background()
	path := testPath(t)
	options := testOptions()
	options.MaxPending = 1
	j := mustOpen(t, path, options)
	if _, err := j.Append(ctx, testBatch("first", "session")); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(ctx, testBatch("refused", "session")); err != outbox.ErrCapacity {
		t.Fatalf("capacity error=%v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j = mustOpen(t, path, options)
	pending, err := j.Pending(ctx)
	if err != nil || len(pending) != 1 || pending[0].Batch.Messages[0].ID != "first" {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
}

func TestJournalByteCapAndWriteFailureAreAtomic(t *testing.T) {
	ctx := context.Background()
	options := testOptions()
	options.MaxBytes = 65536
	j := mustOpen(t, testPath(t), options)
	batch := testBatch("big", "session")
	batch.Messages[0].Content = strings.Repeat("x", 65000)
	if _, err := j.Append(ctx, batch); err != outbox.ErrCapacity {
		t.Fatalf("byte cap error=%v", err)
	}
	if _, err := j.db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(ctx, testBatch("rejected", "session")); err == nil {
		t.Fatal("read-only journal accepted append")
	}
	stats, err := j.Stats(ctx)
	if err != nil || stats.Pending != 0 {
		t.Fatalf("stats=%v err=%v", stats, err)
	}
}

func TestJournalScopeMismatchAndExclusiveOwner(t *testing.T) {
	path := testPath(t)
	j := mustOpen(t, path, testOptions())
	if second, err := Open(path, testOptions()); err == nil {
		_ = second.Close()
		t.Fatal("second journal owner succeeded")
	}
	command := exec.Command(os.Args[0], "-test.run=^TestJournalProcessLockHelper$", "--", path)
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("second process acquired journal: %s %v", result, err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	options := testOptions()
	options.Scope = outbox.Hash([]byte("other destination"))
	if second, err := Open(path, options); err == nil {
		_ = second.Close()
		t.Fatal("changed destination scope opened")
	}
	j = mustOpen(t, path, testOptions())
	if _, err := j.Append(context.Background(), testBatch("still usable", "session")); err != nil {
		t.Fatal(err)
	}
}

func TestJournalProcessLockHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	journal, err := Open(os.Args[len(os.Args)-1], testOptions())
	if err == nil {
		_ = journal.Close()
		t.Fatal("journal is already held by another process")
	}
}

func TestJournalTombstoneSurvivesRestartAndDoneWindowIsBounded(t *testing.T) {
	ctx := context.Background()
	path := testPath(t)
	options := testOptions()
	options.RetainDone = 2
	j := mustOpen(t, path, options)
	for index := 0; index < 7; index++ {
		entry, err := j.Append(ctx, testBatch(fmt.Sprint(index), "session"))
		if err != nil {
			t.Fatal(err)
		}
		if err := j.Ack(ctx, entry.Sequence); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := j.Append(ctx, testBatch("pending", "session")); err != nil {
		t.Fatal(err)
	}
	if err := j.Block(ctx, "session"); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j = mustOpen(t, path, options)
	pending, err := j.Pending(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	if _, err := j.Append(ctx, testBatch("new", "session")); err != outbox.ErrBlocked {
		t.Fatalf("blocked append=%v", err)
	}
	stats, err := j.Stats(ctx)
	if err != nil || stats.Done+stats.Cancelled > 2 || stats.Blocked != 1 {
		t.Fatalf("stats=%v err=%v", stats, err)
	}
	if err := j.Resume(ctx, "session"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(ctx, testBatch("new", "session")); err != nil {
		t.Fatal(err)
	}
}

func TestJournalMissingMetadataCannotReassignPendingScope(t *testing.T) {
	for _, statement := range []string{"DELETE FROM outbox_meta WHERE key='scope'", "DELETE FROM outbox_meta WHERE key='version'", "DROP TABLE outbox_meta"} {
		t.Run(statement, func(t *testing.T) {
			path := testPath(t)
			j := mustOpen(t, path, testOptions())
			entry, err := j.Append(context.Background(), testBatch("original", "session"))
			if err != nil {
				t.Fatal(err)
			}
			if err := j.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(statement); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			var before []byte
			if err := db.QueryRow("SELECT payload FROM outbox_entries WHERE seq=?", entry.Sequence).Scan(&before); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			options := testOptions()
			options.Scope = outbox.Hash([]byte("changed destination"))
			if opened, err := Open(path, options); err == nil {
				_ = opened.Close()
				t.Fatal("existing pending database was reclaimed")
			}
			db, err = sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var after []byte
			var state string
			if err := db.QueryRow("SELECT payload,state FROM outbox_entries WHERE seq=?", entry.Sequence).Scan(&after, &state); err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || state != outbox.Pending {
				t.Fatal("scope refusal changed pending payload")
			}
			var tables int
			if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='outbox_meta'").Scan(&tables); err != nil {
				t.Fatal(err)
			}
			if statement == "DROP TABLE outbox_meta" {
				if tables != 0 {
					t.Fatal("scope refusal recreated missing metadata")
				}
			} else {
				var count int
				if err := db.QueryRow("SELECT count(*) FROM outbox_meta WHERE value=?", options.Scope).Scan(&count); err != nil || count != 0 {
					t.Fatal("scope refusal initialized a new destination")
				}
			}
		})
	}
}

func TestJournalPendingSurvivesAbruptProcessExit(t *testing.T) {
	path := testPath(t)
	command := exec.Command(os.Args[0], "-test.run=^TestJournalAbruptProcessHelper$", "--", path)
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("journal child failed: %s %v", result, err)
	}
	j := mustOpen(t, path, testOptions())
	pending, err := j.Pending(context.Background())
	if err != nil || len(pending) != 1 || pending[0].Batch.Messages[0].ID != "crash" {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	stats, err := j.Stats(context.Background())
	if err != nil || stats.Done != 1 {
		t.Fatalf("stats=%v err=%v", stats, err)
	}
}

func TestJournalAbruptProcessHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	j, err := Open(os.Args[len(os.Args)-1], testOptions())
	if err != nil {
		t.Fatal(err)
	}
	done, err := j.Append(context.Background(), testBatch("done", "session"))
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Ack(context.Background(), done.Sequence); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(context.Background(), testBatch("crash", "session")); err != nil {
		t.Fatal(err)
	}
	// Deliberately skip Close and test cleanups, as a crashed process would.
	os.Exit(0)
}

func TestJournalRejectsFileAndAncestorLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(target, "original.db")
	if err := os.WriteFile(original, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	hard := filepath.Join(target, "hard.db")
	if err := os.Link(original, hard); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	if j, err := Open(hard, testOptions()); err == nil {
		_ = j.Close()
		t.Fatal("hardlink accepted")
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Logf("symlink unavailable: %v", err)
		return
	}
	if j, err := Open(filepath.Join(link, "nested", "outbox.db"), testOptions()); err == nil {
		_ = j.Close()
		t.Fatal("symlink ancestor accepted")
	}
}
