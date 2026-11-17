package repository

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	generationcommand "myai/core/application/chat/generation/command"
	modelport "myai/core/port/model"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

func TestPrepareRestrictedSessionsRequireExistingState(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(*session.Session)
		restricted bool
	}{
		{"ordinary", func(*session.Session) {}, false},
		{"readonly", func(s *session.Session) { s.PermissionMode = session.PermissionModeReadonly }, true},
		{"read_write", func(s *session.Session) { s.PermissionMode = session.PermissionModeReadWrite }, true},
		{"execute", func(s *session.Session) { s.PermissionMode = session.PermissionModeExecute }, true},
		{"full", func(s *session.Session) { s.PermissionMode = session.PermissionModeFull }, true},
		{"empty_ceiling", func(s *session.Session) { s.PermissionCeiling = []tooldef.Permission{} }, true},
		{"plan", func(s *session.Session) { s.AgentMode = session.AgentModePlan }, true},
		{"child", func(s *session.Session) { s.ParentSessionID = "parent" }, true},
	}
	writer := Writer{IDs: &sequentialIDs{}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := &session.Session{ID: "session", PermissionMode: session.PermissionModeAsk}
			tc.mutate(current)
			user, err := writer.PrepareUserMessage(generationcommand.PersistUserMessage{SessionID: current.ID, SessionSnapshot: current, Input: "hello"})
			if err != nil {
				t.Fatal(err)
			}
			assistant, err := writer.PrepareAssistantMessage(current, modelport.ChatResult{Content: "answer"}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if user.Metadata.RequiresExisting != tc.restricted || assistant.Metadata.RequiresExisting != tc.restricted {
				t.Fatalf("restricted=%v user=%v assistant=%v", tc.restricted, user.Metadata.RequiresExisting, assistant.Metadata.RequiresExisting)
			}
		})
	}
}

func TestPreparedBatchDoesNotSerializeSessionConfiguration(t *testing.T) {
	current := &session.Session{ID: "session", SystemInstruction: "private-environment-sentinel", WorkspaceRoot: "private-workspace-sentinel", StyleInstruction: "private-style-sentinel"}
	batch, err := (Writer{IDs: &sequentialIDs{}}).PrepareUserMessage(generationcommand.PersistUserMessage{SessionID: current.ID, Input: "actual chat", SessionSnapshot: current})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-environment-sentinel", "private-workspace-sentinel", "private-style-sentinel"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("session configuration entered journal payload")
		}
	}
	if batch.ID == "" || batch.Messages[0].ID == "" || !batch.Metadata.RequiresExisting {
		t.Fatal("batch lacks frozen identity or security prerequisite")
	}
}
