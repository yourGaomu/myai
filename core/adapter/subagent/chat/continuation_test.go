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

func TestContinuationPromptBoundsLargeTaskReport(t *testing.T) {
	files := make([]domainworkspace.FileChange, maxContinuationChangedFiles+3)
	for index := range files {
		files[index].Path = strings.Repeat("p", maxContinuationPathRunes+10)
	}
	prompt, err := continuationPrompt(subagentport.ParentContinuationRequest{Task: domainsubagent.Task{
		ID: "task-large", Status: domainsubagent.TaskStatusSucceeded,
		Result:    strings.Repeat("结", maxContinuationResultRunes+10),
		ChangeSet: domainworkspace.ChangeSet{Files: files},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"result_truncated": true`) || !strings.Contains(prompt, `"omitted_changed_files": 3`) || !strings.Contains(prompt, "...[truncated]") {
		t.Fatalf("expected bounded report metadata, got:\n%s", prompt)
	}
}
