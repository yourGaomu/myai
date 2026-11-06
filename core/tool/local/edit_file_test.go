package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFilePreservesRealMultilineFileNewlines(t *testing.T) {
	for _, test := range []struct {
		name, before, oldText, newText, want string
		all                                  bool
		count                                int
	}{
		{
			name:    "Windows file with LF request",
			before:  "package main\r\n\r\nfunc main() {\r\n\tprintln(\"旧值\")\r\n}\r\n",
			oldText: "func main() {\n\tprintln(\"旧值\")\n}",
			newText: "func main() {\n\tprintln(\"新值\")\n\tprintln(\"done\")\n}",
			want:    "package main\r\n\r\nfunc main() {\r\n\tprintln(\"新值\")\r\n\tprintln(\"done\")\r\n}\r\n", count: 1,
		},
		{
			name:   "LF file with Windows request",
			before: "header\nfirst\nsecond\nfooter\n", oldText: "first\r\nsecond\r\n",
			newText: "replaced\r\ninserted\r\n", want: "header\nreplaced\ninserted\nfooter\n", count: 1,
		},
		{
			name:   "insert multiline at single-line Windows match",
			before: "head\r\nMARKER\r\ntail\r\n", oldText: "MARKER", newText: "one\ntwo",
			want: "head\r\none\r\ntwo\r\ntail\r\n", count: 1,
		},
		{
			name:   "insert at final Windows line without terminator",
			before: "head\r\nMARKER", oldText: "MARKER", newText: "one\ntwo",
			want: "head\r\none\r\ntwo", count: 1,
		},
		{
			name:   "replace all mixed spellings without changing surrounding bytes",
			before: "pre\r\na\r\nb\r\nmiddle\na\nb\nend", oldText: "a\nb", newText: "x\ny",
			want: "pre\r\nx\r\ny\r\nmiddle\nx\ny\nend", all: true, count: 2,
		},
		{
			name:   "delete Windows multiline span",
			before: "head\r\nfirst\r\nsecond\r\nlast", oldText: "first\nsecond\n", newText: "",
			want: "head\r\nlast", count: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "source.txt")
			if err := os.WriteFile(path, []byte(test.before), 0o600); err != nil {
				t.Fatal(err)
			}
			store := newTestHistoryStore(t)
			tool := NewEditFileToolWithWorkspaceAndRecorder(root, newTestRecorder(t, root, store))
			output, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
				"path": "source.txt", "old_text": test.oldText, "new_text": test.newText, "replace_all": test.all,
			}))
			if err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(path)
			if err != nil || string(actual) != test.want {
				t.Fatalf("on-disk bytes = %q, want %q (error %v)", actual, test.want, err)
			}
			var result editFileResult
			if err := json.Unmarshal([]byte(output.Content), &result); err != nil {
				t.Fatal(err)
			}
			if result.Replacements != test.count || result.Bytes != len(test.want) || result.CheckpointID == "" || result.HistoryError != "" {
				t.Fatalf("unexpected edit result: %#v", result)
			}
			changes, err := store.LoadCheckpointChanges(context.Background(), absPath(t, root), result.CheckpointID)
			if err != nil || len(changes) != 1 || changes[0].Before == nil || changes[0].After == nil || string(changes[0].Before.Content) != test.before || string(changes[0].After.Content) != test.want {
				t.Fatalf("checkpoint did not capture exact before/after bytes: %#v, %v", changes, err)
			}
		})
	}
}

func TestEditFileRejectsMissingAndAmbiguousMultilineMatches(t *testing.T) {
	for _, test := range []struct{ name, before, oldText, wantError string }{
		{"missing", "one\r\ntwo\r\n", "one\nthree", "not found"},
		{"duplicate Windows", "one\r\ntwo\r\none\r\ntwo\r\n", "one\ntwo", "appears 2 times"},
		{"duplicate across newline styles", "one\r\ntwo\r\none\ntwo\n", "one\r\ntwo", "appears 2 times"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "source.txt")
			if err := os.WriteFile(path, []byte(test.before), 0o600); err != nil {
				t.Fatal(err)
			}
			store := newTestHistoryStore(t)
			tool := NewEditFileToolWithWorkspaceAndRecorder(root, newTestRecorder(t, root, store))
			if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
				"path": "source.txt", "old_text": test.oldText, "new_text": "changed\nvalue",
			})); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("expected %q, got %v", test.wantError, err)
			}
			actual, err := os.ReadFile(path)
			if err != nil || string(actual) != test.before {
				t.Fatalf("rejected edit changed file bytes: %q, %v", actual, err)
			}
			checkpoints, err := store.ListCheckpoints(context.Background(), absPath(t, root), 10)
			if err != nil || len(checkpoints) != 0 {
				t.Fatalf("rejected edit created a checkpoint: %#v, %v", checkpoints, err)
			}
		})
	}
}
