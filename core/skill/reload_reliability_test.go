package skill

import (
	"context"
	"strings"
	"testing"
)

func TestScanDoesNotLoseConcurrentInvalidation(t *testing.T) {
	manager := NewManager(t.TempDir())
	if err := manager.reload(context.Background(), func(context.Context, string) ([]Skill, error) {
		manager.invalidate() // watcher observes another edit after files were read
		return []Skill{{Name: "old", Content: "old"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !manager.dirty {
		t.Fatal("scan erased a newer change event")
	}
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.dirty {
		t.Fatal("successful current scan remained dirty")
	}
}

func TestFailedScanKeepsLastValidPrompt(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "good", "# Good\nKeep the valid instructions.")
	manager := NewManager(root)
	if !strings.Contains(manager.Prompt(context.Background()), "valid instructions") {
		t.Fatal("initial skill missing")
	}
	writeSkill(t, root, "broken", strings.Repeat("x", maxSkillBytes+1))
	if !strings.Contains(manager.Prompt(context.Background()), "valid instructions") {
		t.Fatal("failed scan removed existing skills from prompt")
	}
}
