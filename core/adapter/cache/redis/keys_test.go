package redis

import "testing"

func TestCurrentSessionKey(t *testing.T) {
	if got := currentSessionKey("local"); got != "myai:current_session:local" {
		t.Fatalf("unexpected key: %s", got)
	}
}
