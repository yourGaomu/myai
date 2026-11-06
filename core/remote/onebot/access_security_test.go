package onebot

import (
	"context"
	"errors"
	"testing"

	"myai/core/session"
)

func TestOneBotSessionAccessAllowlistAndRetrieval(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		messageType string
		role        Role
		full        bool
		tools       int
	}{
		{"group", RoleUser, false, 0}, {"group", RoleAdmin, false, 9}, {"group", RoleSuperAdmin, false, 9},
		{"private", RoleUser, false, 0}, {"private", RoleAdmin, false, 9}, {"private", RoleSuperAdmin, true, 0},
	}
	for _, c := range cases {
		t.Run(c.messageType+"-"+string(c.role), func(t *testing.T) {
			chat := newMockChatFacade()
			bot, err := NewBot(Config{}, nil, chat)
			if err != nil {
				t.Fatal(err)
			}
			if err := bot.configureSessionAccess(ctx, "session", Event{MessageType: c.messageType}, c.role); err != nil {
				t.Fatal(err)
			}
			mode := string(session.PermissionModeReadonly)
			if c.full {
				mode = string(session.PermissionModeFull)
			}
			if chat.permissionModes["session"] != mode || chat.enforcedTools["session"] == c.full || len(chat.allowedTools["session"]) != c.tools {
				t.Fatal("OneBot permission or allowlist boundary was not applied")
			}
			if !c.full && chat.ragSettings["session"].Mode != session.RetrievalModeOff {
				t.Fatal("workspace pre-retrieval remained enabled")
			}
			for _, name := range chat.allowedTools["session"] {
				if len(name) < len("onebot_") || name[:len("onebot_")] != "onebot_" {
					t.Errorf("workspace tool %q leaked to OneBot", name)
				}
			}
		})
	}
}

func TestOneBotSecuritySetterFailuresPreventGeneration(t *testing.T) {
	for _, failure := range []string{"permission", "allowlist", "rag"} {
		t.Run(failure, func(t *testing.T) {
			chat := newMockChatFacade()
			sentinel := errors.New("fixture security update failed")
			switch failure {
			case "permission":
				chat.permissionError = sentinel
			case "allowlist":
				chat.allowlistError = sentinel
			case "rag":
				chat.ragError = sentinel
			}
			store := NewMemoryStore()
			if err := store.UpsertUser(context.Background(), UserIdentity{UserID: 1, Role: RoleUser}); err != nil {
				t.Fatal(err)
			}
			bot, err := NewBot(Config{}, store, chat)
			if err != nil {
				t.Fatal(err)
			}
			err = bot.executeTurn(context.Background(), Event{MessageType: "private", UserID: 1}, "hello")
			if !errors.Is(err, sentinel) || chat.generationCalls != 0 {
				t.Fatalf("security failure did not stop generation: err=%v calls=%d", err, chat.generationCalls)
			}
		})
	}
}

// Embedding the narrow public interface intentionally omits security extensions.
type legacyChatFacade struct{ ChatFacade }

func TestOneBotBackendWithoutAllowlistFailsClosed(t *testing.T) {
	chat := newMockChatFacade()
	bot, err := NewBot(Config{}, nil, legacyChatFacade{ChatFacade: chat})
	if err != nil {
		t.Fatal(err)
	}
	if err := bot.configureSessionAccess(context.Background(), "session", Event{MessageType: "group"}, RoleUser); err == nil {
		t.Fatal("backend without allowlist accepted an ordinary conversation")
	}
	if chat.generationCalls != 0 {
		t.Fatal("legacy backend generated a reply")
	}
}

func TestOneBotCreatesCleanSessionsForOldHistoryAndRoleChanges(t *testing.T) {
	ctx := context.Background()
	store, chat := NewMemoryStore(), newMockChatFacade()
	chat.sessions["old-group"], chat.sessions["old-private"] = true, true
	if err := store.UpsertGroup(ctx, GroupConfig{GroupID: 10, SessionID: "old-group", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	user := UserIdentity{UserID: 1, Role: RoleUser, PrivateSessionID: "old-private"}
	if err := store.UpsertUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	bot, err := NewBot(Config{}, store, chat)
	if err != nil {
		t.Fatal(err)
	}
	groupEvent := Event{MessageType: "group", GroupID: 10, UserID: 1}
	groupID, err := bot.ensureSessionForEvent(ctx, groupEvent, &user)
	if err != nil || groupID == "old-group" {
		t.Fatal("pre-policy group history was reused")
	}
	groupAgain, err := bot.ensureSessionForEvent(ctx, groupEvent, &user)
	if err != nil || groupID != groupAgain {
		t.Fatal("safe group session was not reused")
	}
	privateEvent := Event{MessageType: "private", UserID: 1}
	privateID, err := bot.ensureSessionForEvent(ctx, privateEvent, &user)
	if err != nil || privateID == "old-private" {
		t.Fatal("pre-policy private history was reused")
	}
	user.Role = RoleSuperAdmin
	adminID, err := bot.ensureSessionForEvent(ctx, privateEvent, &user)
	if err != nil || adminID == privateID {
		t.Fatal("restricted and unrestricted private histories were mixed")
	}
	user.Role = RoleUser
	restrictedID, err := bot.ensureSessionForEvent(ctx, privateEvent, &user)
	if err != nil || restrictedID == adminID {
		t.Fatal("downgraded user received privileged private history")
	}
	// Persisted policy markers make the migration stable across bot restarts.
	bot, err = NewBot(Config{}, store, chat)
	if err != nil {
		t.Fatal(err)
	}
	groupAfterRestart, err := bot.ensureSessionForEvent(ctx, groupEvent, &user)
	if err != nil || groupAfterRestart != groupID {
		t.Fatal("policy migration was not persisted")
	}
}
