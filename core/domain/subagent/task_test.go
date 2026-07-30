package subagent

import (
	"testing"
	"time"
)

func TestMarkCanceledDoesNotCreateUnreadResult(t *testing.T) {
	task := Task{Status: TaskStatusRunning, Unread: true}
	if err := task.MarkCanceled("stopped", time.Now()); err != nil {
		t.Fatal(err)
	}
	if task.Unread {
		t.Fatalf("canceled task must not expose an unconsumable unread result: %#v", task)
	}
}
