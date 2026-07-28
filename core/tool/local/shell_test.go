package local

import (
	"encoding/json"
	"testing"

	domainexecution "myai/core/domain/execution"
)

func TestJoinRemoteErrorOmitsEmptyFields(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "", value: "", want: ""},
		{name: "RuntimeError", value: "", want: "RuntimeError"},
		{name: "", value: "command failed", want: "command failed"},
		{name: "RuntimeError", value: "command failed", want: "RuntimeError: command failed"},
	}
	for _, test := range tests {
		if got := joinRemoteError(test.name, test.value); got != test.want {
			t.Fatalf("joinRemoteError(%q, %q) = %q, want %q", test.name, test.value, got, test.want)
		}
	}
}

func TestShellPayloadMakesIsolationExplicit(t *testing.T) {
	payload, err := json.Marshal(shellPayload(domainexecution.RunResult{
		ExecutionEnvironment: "local-host",
		Isolated:             false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["execution_environment"] != "local-host" || decoded["isolated"] != false {
		t.Fatalf("unexpected shell execution metadata: %s", payload)
	}
}
