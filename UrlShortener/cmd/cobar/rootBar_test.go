package cobar

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsInvalidSecurityBeforeCreatingStores(t *testing.T) {
	for _, test := range []struct {
		name    string
		key     string
		limit   string
		wantEnv string
	}{
		{name: "missing key", key: "", limit: "1024", wantEnv: "URL_SHORTENER_API_KEY"},
		{name: "whitespace key", key: " \t", limit: "1024", wantEnv: "URL_SHORTENER_API_KEY"},
		{name: "invalid key", key: "startup-test-key with-space", limit: "1024", wantEnv: "URL_SHORTENER_API_KEY"},
		{name: "zero upload limit", key: "startup-test-key", limit: "0", wantEnv: "URL_SHORTENER_MAX_UPLOAD_BYTES"},
		{name: "negative upload limit", key: "startup-test-key", limit: "-1", wantEnv: "URL_SHORTENER_MAX_UPLOAD_BYTES"},
		{name: "invalid upload limit", key: "startup-test-key", limit: "invalid", wantEnv: "URL_SHORTENER_MAX_UPLOAD_BYTES"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "application.yaml")
			// An unsupported store ensures validation occurs before storage initialization.
			if err := os.WriteFile(path, []byte("store:\n  type: unsupported-test-store\n"), 0600); err != nil {
				t.Fatalf("write test config: %v", err)
			}
			t.Setenv("URL_SHORTENER_CONFIG", path)
			t.Setenv("URL_SHORTENER_STORE", "unsupported-test-store")
			t.Setenv("URL_SHORTENER_API_KEY", test.key)
			t.Setenv("URL_SHORTENER_MAX_UPLOAD_BYTES", test.limit)

			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			defer log.SetOutput(previous)
			err := run(context.Background(), path)
			if err == nil || !strings.Contains(err.Error(), test.wantEnv) || !strings.Contains(err.Error(), "invalid config") {
				t.Fatal("startup did not reject invalid security settings before creating stores")
			}
			if strings.Contains(err.Error(), "startup-test-key") || strings.Contains(output.String(), "startup-test-key") {
				t.Fatal("startup exposed the API key")
			}
			if output.Len() != 0 {
				t.Fatal("startup initialized stores or listeners before security validation")
			}
		})
	}
}
