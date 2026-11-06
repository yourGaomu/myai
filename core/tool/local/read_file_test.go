package local

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReadFileSmallFileCompatibilityAndLineRanges(t *testing.T) {
	root := t.TempDir()
	const text = "第一行\r\nsecond\r\nthird\r\nlast"
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewReadFileToolWithWorkspace(root)
	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"legacy", map[string]any{"path": "notes.txt"}, text},
		{"middle lines", map[string]any{"path": "notes.txt", "offset": 2, "limit": 2}, "second\r\nthird\r\n"},
		{"tail", map[string]any{"path": "notes.txt", "offset": 4}, "last"},
		{"past end", map[string]any{"path": "notes.txt", "offset": 20}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := tool.Call(context.Background(), mustJSON(t, test.args))
			if err != nil {
				t.Fatal(err)
			}
			body, next := splitReadFileContinuation(t, output.Content)
			if body != test.want {
				t.Fatalf("file bytes = %q, want %q", body, test.want)
			}
			if test.name == "middle lines" {
				if next == nil || next.Offset != 4 || next.ByteOffset != 0 {
					t.Fatalf("missing line continuation: %#v", next)
				}
			} else if next != nil {
				t.Fatalf("unexpected continuation: %#v", next)
			}
		})
	}
}

func TestReadFileLongUTF8LineCanBeResumedWithoutLosingBytes(t *testing.T) {
	root := t.TempDir()
	text := strings.Repeat("界", maxReadFileBytes/3+19) + "\r\nnext\n"
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewReadFileToolWithWorkspace(root)
	args := mustJSON(t, map[string]any{"path": "long.txt"})
	var combined strings.Builder
	for page := 0; ; page++ {
		if page > 3 {
			t.Fatal("continuation did not make progress")
		}
		output, err := tool.Call(context.Background(), args)
		if err != nil {
			t.Fatal(err)
		}
		body, next := splitReadFileContinuation(t, output.Content)
		if len(body) > maxReadFileBytes || !utf8.ValidString(body) {
			t.Fatalf("invalid or oversized page: %d bytes", len(body))
		}
		combined.WriteString(body)
		if next == nil {
			break
		}
		if page == 0 && (next.Offset != 1 || next.ByteOffset != int64(len(body))) {
			t.Fatalf("long-line continuation lost its position: %#v", next)
		}
		args = mustJSON(t, next)
	}
	if combined.String() != text {
		t.Fatal("resumed pages changed or omitted file bytes")
	}
}

func TestReadFileByteLimitKeepsRemainingLineLimit(t *testing.T) {
	text := strings.Repeat("a", maxReadFileBytes-2) + "\nsecond\nthird\n"
	limit := int64(2)
	body, next, more, err := readTextRange(context.Background(), strings.NewReader(text), readFileArgs{Path: "x", Offset: 1, Limit: &limit})
	if err != nil || !more || len(body) != maxReadFileBytes || next.Offset != 2 || next.ByteOffset != 1 || next.Limit == nil || *next.Limit != 1 {
		t.Fatalf("byte-capped range = %d bytes, %#v, %v, %v", len(body), next, more, err)
	}
	tail, nextPage, more, err := readTextRange(context.Background(), strings.NewReader(text), next)
	if err != nil || !more || tail != "econd\n" || nextPage.Offset != 3 {
		t.Fatalf("remaining requested line = %q, %#v, %v, %v", tail, nextPage, more, err)
	}
}

func TestReadFileRejectsBinaryAndInvalidRanges(t *testing.T) {
	root := t.TempDir()
	for _, content := range [][]byte{{'a', 0, 'b'}, {0xff, 0xfe}, {'a', 0x01, 'b'}} {
		if err := os.WriteFile(filepath.Join(root, "binary.dat"), content, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewReadFileToolWithWorkspace(root).Call(context.Background(), mustJSON(t, map[string]any{"path": "binary.dat"})); err == nil || !strings.Contains(err.Error(), "binary") {
			t.Fatalf("binary input accepted: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "text.txt"), []byte("界\nlast"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{"path": "text.txt", "offset": -1},
		{"path": "text.txt", "limit": 0},
		{"path": "text.txt", "limit": -1},
		{"path": "text.txt", "byte_offset": -1},
		{"path": "text.txt", "byte_offset": 1},
		{"path": "text.txt", "byte_offset": 9},
		{"path": "."},
	} {
		if _, err := NewReadFileToolWithWorkspace(root).Call(context.Background(), mustJSON(t, args)); err == nil {
			t.Fatalf("invalid range accepted: %#v", args)
		}
	}
}

func TestReadFileStopsReadingLargeStreamAndHonorsCancellation(t *testing.T) {
	// This source represents an arbitrarily large single line and fails if the
	// tool tries to consume more than its page plus one buffered read.
	source := &boundedTextStream{remaining: maxReadFileBytes + readFileBufferBytes}
	body, next, more, err := readTextRange(context.Background(), source, readFileArgs{Offset: 1})
	if err != nil || !more || len(body) != maxReadFileBytes || next.ByteOffset != maxReadFileBytes {
		t.Fatalf("bounded stream page = %d bytes, %#v, %v, %v", len(body), next, more, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := readTextRange(ctx, strings.NewReader("text"), readFileArgs{Offset: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

type boundedTextStream struct{ remaining int }

func (r *boundedTextStream) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, errors.New("attempted to read the whole stream")
	}
	if len(p) > r.remaining {
		p = p[:r.remaining]
	}
	for i := range p {
		p[i] = 'x'
	}
	r.remaining -= len(p)
	return len(p), nil
}

func splitReadFileContinuation(t *testing.T, output string) (string, *readFileArgs) {
	t.Helper()
	const marker = "\n\n[read_file truncated:"
	index := strings.Index(output, marker)
	if index < 0 {
		return output, nil
	}
	const prefix = "Continue with read_file "
	position := strings.Index(output[index:], prefix)
	if position < 0 || !strings.HasSuffix(output, "]") {
		t.Fatalf("unusable continuation notice: %s", output[index:])
	}
	var next readFileArgs
	if err := json.Unmarshal([]byte(output[index+position+len(prefix):len(output)-1]), &next); err != nil {
		t.Fatal(err)
	}
	return output[:index], &next
}

var _ io.Reader = (*boundedTextStream)(nil)
