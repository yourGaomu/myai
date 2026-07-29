package chat

import (
	"strings"
	"testing"

	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	subagentport "myai/core/port/subagent"
)

func TestContinuationPromptMarksReportAsUntrustedAndPendingChangesAsUnapplied(t *testing.T) {
	prompt, err := continuationPrompt(subagentport.ParentContinuationRequest{Task: domainsubagent.Task{
		ID: "task-1", Title: "Implement feature", Status: domainsubagent.TaskStatusSucceeded,
		Result: "implementation complete",
		ChangeSet: domainworkspace.ChangeSet{
			Status: domainworkspace.ChangeSetStatusPending,
			Files:  []domainworkspace.FileChange{{Path: "core/service/chat.go"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"untrusted subordinate evidence",
		`"task_id": "task-1"`,
		`"change_set_status": "pending"`,
		`"core/service/chat.go"`,
		`"pending_changes_not_applied": true`,
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected continuation prompt to contain %q, got:\n%s", expected, prompt)
		}
	}
}
