package trust

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func requestForTest(t *testing.T, workspace string, declaration any) Request {
	t.Helper()
	request, err := NewRequest(KindHook, workspace, workspace, declaration)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestApprovalsBindDeclarationWorkspaceAndKindAndCanBeRevoked(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(filepath.Join(t.TempDir(), "trust.json"))
	request := requestForTest(t, workspace, map[string]any{"command": "dummy", "env": map[string]string{"DUMMY_VALUE": "placeholder-value"}})
	policy := &Policy{Store: store}
	if err := policy.Authorize(context.Background(), request); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("default trusted: %v", err)
	}
	if err := store.Approve(request); err != nil {
		t.Fatal(err)
	}
	if err := policy.Authorize(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	changed := requestForTest(t, workspace, map[string]any{"command": "dummy", "env": map[string]string{"DUMMY_VALUE": "changed-placeholder"}})
	if err := policy.Authorize(context.Background(), changed); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("changed declaration trusted: %v", err)
	}
	otherWorkspace := requestForTest(t, t.TempDir(), map[string]any{"command": "dummy", "env": map[string]string{"DUMMY_VALUE": "placeholder-value"}})
	if err := policy.Authorize(context.Background(), otherWorkspace); !errors.Is(err, ErrUntrusted) {
		t.Fatal("approval leaked across workspaces")
	}
	otherKind := request
	otherKind.Kind = KindPlugin
	if trusted, err := store.Trusted(otherKind); err != nil || trusted {
		t.Fatal("approval leaked across kinds")
	}
	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "placeholder-value") || strings.Contains(string(raw), "DUMMY_VALUE") || strings.Contains(string(raw), "command") {
		t.Fatal("store persisted declaration values")
	}
	if err := store.Revoke(request); err != nil {
		t.Fatal(err)
	}
	if err := policy.Authorize(context.Background(), request); !errors.Is(err, ErrUntrusted) {
		t.Fatal("revocation ineffective")
	}
}

func TestConfirmationPersistsOnlyExplicitApproval(t *testing.T) {
	request := requestForTest(t, t.TempDir(), map[string]string{"command": "dummy"})
	store := NewStore(filepath.Join(t.TempDir(), "trust.json"))
	for _, approval := range []Approval{{}, {Allowed: true}, {Allowed: true, Persist: true}} {
		policy := &Policy{Store: store, Confirm: func(_ context.Context, got Request) (Approval, error) {
			if got.Digest != request.Digest || len(got.Declaration) == 0 {
				t.Fatal("reviewer did not receive exact declaration")
			}
			return approval, nil
		}}
		err := policy.Authorize(context.Background(), request)
		if (err == nil) != approval.Allowed {
			t.Fatalf("confirmation ignored: %v", err)
		}
		approved, err := store.Trusted(request)
		if err != nil || approved != (approval.Allowed && approval.Persist) {
			t.Fatalf("unexpected persistent approval=%v err=%v", approved, err)
		}
	}
}

func TestWorkspaceCannotSupplyItsOwnTrustStore(t *testing.T) {
	workspace := t.TempDir()
	request := requestForTest(t, workspace, map[string]string{"command": "dummy"})
	store := NewStore(filepath.Join(workspace, "user-trust.json"))
	if err := store.Approve(request); err == nil {
		t.Fatal("workspace store accepted")
	}
	if _, err := store.Trusted(request); err == nil {
		t.Fatal("workspace store trusted")
	}
	link := filepath.Join(t.TempDir(), "linked-workspace")
	if err := os.Symlink(workspace, link); err != nil {
		t.Skip("symlink unavailable")
	}
	store = NewStore(filepath.Join(link, "not-created", "user-trust.json"))
	if err := store.Approve(request); err == nil {
		t.Fatal("workspace store through symlink accepted")
	}
}

func TestConcurrentApprovalsPreserveAllDeclarations(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(t.TempDir(), "trust.json")
	var requests []Request
	for _, command := range []string{"dummy-one", "dummy-two", "dummy-three"} {
		requests = append(requests, requestForTest(t, workspace, map[string]string{"command": command}))
	}
	var group sync.WaitGroup
	for _, request := range requests {
		group.Add(1)
		go func(request Request) {
			defer group.Done()
			if err := NewStore(path).Approve(request); err != nil {
				t.Error(err)
			}
		}(request)
	}
	group.Wait()
	for _, request := range requests {
		if approved, err := NewStore(path).Trusted(request); err != nil || !approved {
			t.Fatalf("approval lost: %v", err)
		}
	}
}

func TestMalformedUserStoreFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"approvals":[]}`+" trailing"), 0600); err != nil {
		t.Fatal(err)
	}
	if approved, err := NewStore(path).Trusted(requestForTest(t, t.TempDir(), map[string]string{"command": "dummy"})); err == nil || approved {
		t.Fatal("malformed store trusted")
	}
}
