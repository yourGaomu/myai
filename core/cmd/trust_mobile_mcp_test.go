package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"myai/core/trust"
)

func TestMobileMCPTrustCLIGrantsOnlyOperatorSelectedEndpoint(t *testing.T) {
	workspace := t.TempDir()
	store := trust.NewStore(filepath.Join(t.TempDir(), "trust.json"))
	pairingID := strings.Repeat("4", 64)
	endpoint := "https://links.example.test/api/mcp"
	run := func(action string, flags ...string) (string, error) {
		t.Helper()
		return executeTrustForTest(t, store, append([]string{"mcp", "mobile-admin", action, "--workspace", workspace,
			"--user", "local", "--device", "pc-local", "--pairing-id", pairingID}, flags...)...)
	}
	if _, err := run("grant"); err == nil {
		t.Fatal("public endpoint must be explicit")
	}
	if _, err := run("grant", "--public-endpoint", endpoint+"?token=test"); err == nil {
		t.Fatal("query-bearing endpoint was allowed")
	}
	if _, err := run("grant", "--public-endpoint", endpoint); err != nil {
		t.Fatal(err)
	}
	if output, err := run("list"); err != nil || !strings.Contains(output, pairingID+"\t"+endpoint) {
		t.Fatalf("unexpected grant list: %s %v", output, err)
	}
	if approvals, err := store.List(workspace, trust.KindMCP); err != nil || len(approvals) != 0 {
		t.Fatalf("grant unexpectedly trusts a declaration: %+v %v", approvals, err)
	}
	if _, err := run("revoke"); err != nil {
		t.Fatal(err)
	}
	if output, err := run("list"); err != nil || output != "" {
		t.Fatalf("revoke did not clear grant: %s %v", output, err)
	}
}
