package subagent

import (
	"testing"
	"time"

	domainworkspace "myai/core/domain/workspace"
)

func TestCanFollowupAllowsClosedIsolatedWorkspaces(t *testing.T) {
	pending := Task{Status: TaskStatusSucceeded, Workspace: domainworkspace.Reference{Mode: domainworkspace.IsolationModeSnapshot}, ChangeSet: domainworkspace.ChangeSet{Status: domainworkspace.ChangeSetStatusApplied}}
	if !pending.CanFollowup() {
		t.Fatal("applied isolated workspace should remain eligible for follow-up")
	}
	failed := Task{Status: TaskStatusFailed, Workspace: domainworkspace.Reference{Mode: domainworkspace.IsolationModeSnapshot}, ChangeSet: domainworkspace.ChangeSet{Status: domainworkspace.ChangeSetStatusDiscarded}}
	if !failed.CanFollowup() {
		t.Fatal("failed isolated workspace should remain eligible for follow-up")
	}
	if (Task{Status: TaskStatusCanceled}).CanFollowup() {
		t.Fatal("canceled tasks cannot be followed up")
	}
}

func TestMarkCanceledDoesNotCreateUnreadResult(t *testing.T) {
	task := Task{Status: TaskStatusRunning, Unread: true}
	if err := task.MarkCanceled("stopped", time.Now()); err != nil {
		t.Fatal(err)
	}
	if task.Unread {
		t.Fatalf("canceled task must not expose an unconsumable unread result: %#v", task)
	}
}
