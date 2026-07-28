package local

import (
	"context"
	"strings"
	"testing"

	domainexecution "myai/core/domain/execution"
)

func TestExecutorReportsHostExecutionWithoutIsolation(t *testing.T) {
	executor, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Run(context.Background(), domainexecution.RunRequest{Command: "go version"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.ExecutionEnvironment != "local-host" || result.Isolated {
		t.Fatalf("unexpected local execution metadata: %#v", result)
	}
}

func TestExecutorDenylistIsOnlyReportedAsLocalPolicy(t *testing.T) {
	executor, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.Run(context.Background(), domainexecution.RunRequest{Command: "Remove-Item -Recurse C:\\"})
	if err == nil || !strings.Contains(err.Error(), "local executor policy") {
		t.Fatalf("expected explicit local policy error, got %v", err)
	}
}
