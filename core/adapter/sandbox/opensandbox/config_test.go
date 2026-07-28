package opensandbox

import (
	"testing"
	"time"

	domainsandbox "myai/core/domain/sandbox"
)

func TestConnectionConfigAcceptsVersionedEndpoint(t *testing.T) {
	config := Config{
		Endpoint:       "https://sandbox.example.test/v1",
		APIKey:         "secret",
		UseServerProxy: true,
	}.normalize()
	connection, err := config.connectionConfig()
	if err != nil {
		t.Fatal(err)
	}
	if connection.Domain != "sandbox.example.test" || connection.Protocol != "https" {
		t.Fatalf("unexpected connection: %#v", connection)
	}
	if !connection.UseServerProxy || connection.APIKey != "secret" {
		t.Fatalf("unexpected authentication/proxy config: %#v", connection)
	}
}

func TestCreateOptionsApplyDefaultsAndNetworkPolicy(t *testing.T) {
	manager := Manager{config: (Config{
		Image:          "myai-skill:latest",
		CPU:            "1",
		Memory:         "1Gi",
		SandboxTimeout: 2 * time.Minute,
	}).normalize()}
	request := testCreateRequest()
	options := manager.createOptions(request)
	if options.Image != "myai-skill:latest" || options.ResourceLimits["cpu"] != "1" || options.ResourceLimits["memory"] != "1Gi" {
		t.Fatalf("unexpected create options: %#v", options)
	}
	if options.TimeoutSeconds == nil || *options.TimeoutSeconds != 120 {
		t.Fatalf("unexpected sandbox timeout: %#v", options.TimeoutSeconds)
	}
	if options.NetworkPolicy == nil || options.NetworkPolicy.DefaultAction != "deny" {
		t.Fatalf("unexpected network policy: %#v", options.NetworkPolicy)
	}
}

func testCreateRequest() domainsandbox.CreateRequest {
	return domainsandbox.CreateRequest{
		NetworkPolicy: &domainsandbox.NetworkPolicy{DefaultAction: domainsandbox.NetworkActionDeny},
	}
}
