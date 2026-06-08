package files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myai/core/remote/protocol"
)

func TestListHidesIgnoredSensitiveAndHiddenFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "package main\n")
	writeTestFile(t, root, ".env", "SECRET=value\n")
	writeTestFile(t, root, ".hidden", "hidden\n")
	mkdirTest(t, root, ".git")
	mkdirTest(t, root, "src")

	service := newTestService(t, root)

	result, err := service.List(context.Background(), protocol.FileListPayload{Path: "."})
	if err != nil {
		t.Fatalf("list files failed: %v", err)
	}

	if !hasEntry(result.Entries, "main.go") {
		t.Fatalf("expected main.go in entries: %+v", result.Entries)
	}
	if !hasEntry(result.Entries, "src") {
		t.Fatalf("expected src in entries: %+v", result.Entries)
	}
	if hasEntry(result.Entries, ".env") {
		t.Fatalf("did not expect .env in entries: %+v", result.Entries)
	}
	if hasEntry(result.Entries, ".git") {
		t.Fatalf("did not expect .git in entries: %+v", result.Entries)
	}
	if hasEntry(result.Entries, ".hidden") {
		t.Fatalf("did not expect .hidden in entries: %+v", result.Entries)
	}

	result, err = service.List(context.Background(), protocol.FileListPayload{Path: ".", IncludeHidden: true})
	if err != nil {
		t.Fatalf("list hidden files failed: %v", err)
	}
	if !hasEntry(result.Entries, ".hidden") {
		t.Fatalf("expected .hidden when include_hidden=true: %+v", result.Entries)
	}
	if hasEntry(result.Entries, ".env") || hasEntry(result.Entries, ".git") {
		t.Fatalf("sensitive and ignored names must stay hidden: %+v", result.Entries)
	}
}

func TestServiceRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	service := newTestService(t, root)

	if _, err := service.List(context.Background(), protocol.FileListPayload{Path: ".."}); err == nil {
		t.Fatalf("expected path traversal to be rejected")
	}

	absolute := filepath.Join(root, "main.go")
	writeTestFile(t, root, "main.go", "package main\n")
	if _, err := service.Read(context.Background(), protocol.FileReadPayload{Path: absolute}); err == nil {
		t.Fatalf("expected absolute path to be rejected")
	}

	writeTestFile(t, root, ".env", "SECRET=value\n")
	if _, err := service.Read(context.Background(), protocol.FileReadPayload{Path: ".env"}); err == nil {
		t.Fatalf("expected sensitive file preview to be rejected")
	}
}

func TestReadDetectsBinaryAndTruncatesLargeFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin.dat", string([]byte{0, 1, 2, 3}))
	writeTestFile(t, root, "large.txt", strings.Repeat("x", maxReadBytes+10))

	service := newTestService(t, root)

	binary, err := service.Read(context.Background(), protocol.FileReadPayload{Path: "bin.dat"})
	if err != nil {
		t.Fatalf("read binary file failed: %v", err)
	}
	if !binary.Binary {
		t.Fatalf("expected binary file to be detected")
	}
	if binary.Content != "" {
		t.Fatalf("expected binary content to be omitted")
	}

	large, err := service.Read(context.Background(), protocol.FileReadPayload{Path: "large.txt"})
	if err != nil {
		t.Fatalf("read large file failed: %v", err)
	}
	if !large.Truncated {
		t.Fatalf("expected large file to be truncated")
	}
	if len(large.Content) != maxReadBytes {
		t.Fatalf("expected preview size %d, got %d", maxReadBytes, len(large.Content))
	}
}

func newTestService(t *testing.T, root string) *Service {
	t.Helper()

	service, err := New(root)
	if err != nil {
		t.Fatalf("new file service failed: %v", err)
	}
	return service
}

func writeTestFile(t *testing.T, root string, path string, content string) {
	t.Helper()

	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create test parent directory failed: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file failed: %v", err)
	}
}

func mkdirTest(t *testing.T, root string, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
		t.Fatalf("create test directory failed: %v", err)
	}
}

func hasEntry(entries []protocol.FileEntry, name string) bool {
	for _, entry := range entries {
		if entry.Name == name {
			return true
		}
	}
	return false
}
