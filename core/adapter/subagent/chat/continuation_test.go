package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"myai/core/adapter/chat/pendinginput"

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
	if !strings.Contains(prompt, `"result_truncated": true`) || !strings.Contains(prompt, "...[truncated]") {
		t.Fatal("large result lacks an explicit truncation marker")
	}
	if utf8.RuneCountInString(prompt) > pendinginput.MaxPendingRunes {
		t.Fatal("complete continuation prompt exceeds the parent queue limit")
	}
	start := strings.Index(prompt, "<subagent_report>\n") + len("<subagent_report>\n")
	end := strings.LastIndex(prompt, "\n</subagent_report>")
	var report struct {
		ChangedFiles        []string `json:"changed_files"`
		OmittedChangedFiles int      `json:"omitted_changed_files"`
	}
	if err := json.Unmarshal([]byte(prompt[start:end]), &report); err != nil {
		t.Fatal(err)
	}
	if report.OmittedChangedFiles < 3 || len(report.ChangedFiles)+report.OmittedChangedFiles != len(files) {
		t.Fatal("completion summary lost its omitted file count")
	}
}
