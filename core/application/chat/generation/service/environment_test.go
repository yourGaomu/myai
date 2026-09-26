package service

import (
	"strings"
	"testing"
	"time"
)

func TestEnvironmentContextPromptIncludesWeekdayWorkspaceAndZone(t *testing.T) {
	now := time.Date(2026, 9, 18, 15, 4, 5, 0, time.FixedZone("CST", 8*3600))
	prompt := environmentContextPrompt(now, `D:\Go_All\myai`)
	for _, want := range []string{
		"Friday",
		"星期五",
		"2026-09-18 15:04:05",
		"CST",
		"UTC+8",
		`Workspace: D:\Go_All\myai`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, prompt)
		}
	}
}

func TestEnvironmentContextPromptOmitsEmptyWorkspace(t *testing.T) {
	now := time.Date(2026, 9, 18, 15, 4, 5, 0, time.UTC)
	prompt := environmentContextPrompt(now, "  ")
	if strings.Contains(prompt, "Workspace:") {
		t.Fatalf("expected empty workspace to be omitted, got %q", prompt)
	}
	if !strings.Contains(prompt, "Friday") || !strings.Contains(prompt, "星期五") {
		t.Fatalf("expected Friday in UTC, got %q", prompt)
	}
}
