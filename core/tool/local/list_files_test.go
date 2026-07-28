package local

import (
	"encoding/json"
	"testing"
)

func TestNormalizeListFilesArgsRejectsInvalidJSON(t *testing.T) {
	if _, err := normalizeListFilesArgs(t.TempDir(), json.RawMessage(`{"path":`)); err == nil {
		t.Fatal("expected malformed list_files arguments to fail")
	}
}
