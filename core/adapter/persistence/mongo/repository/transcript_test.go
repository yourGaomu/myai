package repository

import (
	"reflect"
	"testing"

	portrepository "myai/core/port/repository"
)

func TestRemovedToolCallIDsKeepsAssetsFromPreservedTurns(t *testing.T) {
	removed := removedToolCallIDs(
		[]string{"call-old", "call-trimmed", "call-trimmed"},
		[]string{"call-old"},
	)
	if !reflect.DeepEqual([]any(removed), []any{"call-trimmed"}) {
		t.Fatalf("unexpected removed tool call ids: %#v", removed)
	}
}

func TestToolCallIDsIncludesEveryRetainedCall(t *testing.T) {
	ids := toolCallIDs([]portrepository.MessageRecord{
		{Role: portrepository.RoleToolCall, ToolCallID: "call-1"},
		{Role: portrepository.RoleToolCall, ToolCallID: "call-2"},
		{Role: portrepository.RoleTool, ToolCallID: "call-2"},
	})
	if !reflect.DeepEqual(ids, []string{"call-1", "call-2"}) {
		t.Fatalf("unexpected retained tool call ids: %#v", ids)
	}
}
