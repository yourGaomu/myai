package mapper

import (
	"testing"

	"myai/core/adapter/persistence/mongo/subagent/po"
	domainsubagent "myai/core/domain/subagent"
)

func TestTaskDomainFromDocumentClearsLegacyCanceledUnreadFlag(t *testing.T) {
	task := TaskDomainFromDocument(po.TaskDocument{
		ID: "task-1", Status: string(domainsubagent.TaskStatusCanceled), Unread: true,
	})
	if task.Unread {
		t.Fatalf("legacy canceled task must load as consumed: %#v", task)
	}
}
