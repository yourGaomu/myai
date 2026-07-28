package cmd

import "testing"

func TestConfiguredRelayAgentCredentialsBindsEveryTokenToAnIdentity(t *testing.T) {
	previousToken := relayAgentToken
	previousUser := relayAgentUser
	previousDevice := relayAgentDevice
	previousCredentials := relayCredentials
	t.Cleanup(func() {
		relayAgentToken = previousToken
		relayAgentUser = previousUser
		relayAgentDevice = previousDevice
		relayCredentials = previousCredentials
	})

	relayAgentToken = "primary-token"
	relayAgentUser = "local"
	relayAgentDevice = "pc-local"
	relayCredentials = []string{"local/laptop=secondary-token"}

	credentials, err := configuredRelayAgentCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(credentials) != 2 {
		t.Fatalf("expected two credentials, got %#v", credentials)
	}
	if credentials[0].UserID != "local" || credentials[0].DeviceID != "pc-local" || credentials[0].Token != "primary-token" {
		t.Fatalf("unexpected primary credential: %#v", credentials[0])
	}
	if credentials[1].UserID != "local" || credentials[1].DeviceID != "laptop" || credentials[1].Token != "secondary-token" {
		t.Fatalf("unexpected additional credential: %#v", credentials[1])
	}
}

func TestConfiguredRelayAgentCredentialsRejectsDuplicateIdentity(t *testing.T) {
	previousToken := relayAgentToken
	previousUser := relayAgentUser
	previousDevice := relayAgentDevice
	previousCredentials := relayCredentials
	t.Cleanup(func() {
		relayAgentToken = previousToken
		relayAgentUser = previousUser
		relayAgentDevice = previousDevice
		relayCredentials = previousCredentials
	})

	relayAgentToken = "primary-token"
	relayAgentUser = "local"
	relayAgentDevice = "pc-local"
	relayCredentials = []string{"local/pc-local=other-token"}

	if _, err := configuredRelayAgentCredentials(); err == nil {
		t.Fatal("expected duplicate identity to be rejected")
	}
}
