package trust

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMobileMCPGrantsAreScopedAndRegrantChangesGeneration(t *testing.T) {
	workspace, root := t.TempDir(), t.TempDir()
	store := NewMobileMCPStore(filepath.Join(root, "mcp-mobile.json"))
	id := strings.Repeat("a", 64)
	endpoint := "https://example.test/api/mcp"
	scope := MobileMCPScope{workspace, "user", "device"}
	get := func(scope MobileMCPScope) (generation string) {
		t.Helper()
		if err := store.WithGrants(scope, id, func(grants []MobileMCPGrant) error {
			for _, grant := range grants {
				if grant.Endpoint == endpoint {
					generation = grant.Generation
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return generation
	}
	if err := store.Grant(scope, id, endpoint); err != nil {
		t.Fatal(err)
	}
	old := get(scope)
	if old == "" || get(MobileMCPScope{workspace, "user", "different"}) != "" || get(MobileMCPScope{t.TempDir(), "user", "device"}) != "" {
		t.Fatal("grant missing or crosses scope")
	}
	if err := store.Revoke(scope, id); err != nil {
		t.Fatal(err)
	}
	if get(scope) != "" {
		t.Fatal("revoked grant remains")
	}
	if err := store.Grant(scope, id, endpoint); err != nil {
		t.Fatal(err)
	}
	if generation := get(scope); generation == "" || generation == old {
		t.Fatal("grant generation reused")
	}
	for _, unsafe := range []string{endpoint + "?", endpoint + "#", "https://user:password@example.test/api/mcp", endpoint + "/%73ecret"} {
		if err := store.Grant(scope, id, unsafe); err == nil {
			t.Fatal("unsafe endpoint allowed")
		}
	}
}
