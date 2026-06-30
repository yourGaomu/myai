package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerPromptReloadsSkillFiles(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "write-go", "# Go Writer\nPrefer gofmt after Go edits.")

	manager := NewManager(root)
	first := manager.Prompt(context.Background())
	if !strings.Contains(first, "Prefer gofmt after Go edits.") {
		t.Fatalf("expected first prompt to include initial skill content, got %q", first)
	}

	writeSkill(t, root, "write-go", "# Go Writer\nRun go test for changed packages.")
	second := manager.Prompt(context.Background())
	if !strings.Contains(second, "Run go test for changed packages.") {
		t.Fatalf("expected second prompt to include updated skill content, got %q", second)
	}
	if strings.Contains(second, "Prefer gofmt after Go edits.") {
		t.Fatalf("expected updated prompt to replace old skill content, got %q", second)
	}
}

func TestManagerPromptUsesStableSortedOrder(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "z-last", "# Last\nlast instructions")
	writeSkill(t, root, "a-first", "# First\nfirst instructions")

	manager := NewManager(root)
	prompt := manager.Prompt(context.Background())

	firstIndex := strings.Index(prompt, "## a-first")
	lastIndex := strings.Index(prompt, "## z-last")
	if firstIndex < 0 || lastIndex < 0 {
		t.Fatalf("expected both skills in prompt, got %q", prompt)
	}
	if firstIndex > lastIndex {
		t.Fatalf("expected skills to be sorted by path, got %q", prompt)
	}
}

func TestManagerMissingRootIsEmpty(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), "missing"))

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("expected missing skill root to be allowed, got %v", err)
	}
	if prompt := manager.Prompt(context.Background()); prompt != "" {
		t.Fatalf("expected empty prompt for missing root, got %q", prompt)
	}
}

func TestManagerPromptForInputSelectsMatchedSkillInstructions(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "write-go", "# Go Writer\nTriggers: go, golang, go test\nPrefer gofmt after Go edits.")
	writeSkill(t, root, "design-ui", "# UI Designer\nTriggers: ui, react\nKeep spacing consistent.")

	manager := NewManager(root)
	prompt := manager.PromptForInput(context.Background(), "please write a go test")

	if !strings.Contains(prompt, "Available skill index:") {
		t.Fatalf("expected prompt to include skill index, got %q", prompt)
	}
	if !strings.Contains(prompt, "- write-go") || !strings.Contains(prompt, "- design-ui") {
		t.Fatalf("expected prompt index to include all skills, got %q", prompt)
	}
	if !strings.Contains(prompt, "Selected skill instructions:") {
		t.Fatalf("expected matched prompt to include selected instructions, got %q", prompt)
	}
	if !strings.Contains(prompt, "Prefer gofmt after Go edits.") {
		t.Fatalf("expected matched skill full content, got %q", prompt)
	}
	if strings.Contains(prompt, "Keep spacing consistent.") {
		t.Fatalf("expected unmatched skill full content to be omitted, got %q", prompt)
	}
}

func TestManagerPromptForInputUsesIndexOnlyWhenNothingMatches(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "write-go", "# Go Writer\nTriggers: go, golang\nFull Go instructions.")

	manager := NewManager(root)
	prompt := manager.PromptForInput(context.Background(), "hello")

	if !strings.Contains(prompt, "Available skill index:") {
		t.Fatalf("expected prompt to include skill index, got %q", prompt)
	}
	if strings.Contains(prompt, "Selected skill instructions:") {
		t.Fatalf("expected no selected instructions for unmatched input, got %q", prompt)
	}
	if strings.Contains(prompt, "Full Go instructions.") {
		t.Fatalf("expected full content to be omitted for unmatched input, got %q", prompt)
	}
}

func TestManagerPromptForInputMatchesChineseTrigger(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "file-helper", "# File Helper\nTriggers: 文件, 目录\nFull file instructions.")

	manager := NewManager(root)
	prompt := manager.PromptForInput(context.Background(), "帮我查看文件目录")

	if !strings.Contains(prompt, "Full file instructions.") {
		t.Fatalf("expected chinese trigger to select skill instructions, got %q", prompt)
	}
}

func TestManagerParsesTriggersAndSkipsTriggerLineDescription(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "write-go", "Triggers: go, golang; go test|cobra\n# Go Writer\nPrefer gofmt after Go edits.")

	manager := NewManager(root)
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload skills: %v", err)
	}

	skills := manager.List()
	if len(skills) != 1 {
		t.Fatalf("expected one skill, got %d", len(skills))
	}
	if skills[0].Description != "Go Writer" {
		t.Fatalf("expected description to skip trigger line, got %q", skills[0].Description)
	}

	expected := []string{"go", "golang", "go test", "cobra"}
	if strings.Join(skills[0].Triggers, "|") != strings.Join(expected, "|") {
		t.Fatalf("expected triggers %v, got %v", expected, skills[0].Triggers)
	}
}

func TestManagerParsesSkillHubFrontMatter(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "paper", `---
name: research-paper-reader
description: Read and summarize research papers.
triggers:
  - paper
  - arxiv
---

# Research Paper Reader
Full instructions.`)

	manager := NewManager(root)
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload skills: %v", err)
	}

	skills := manager.List()
	if len(skills) != 1 {
		t.Fatalf("expected one skill, got %d", len(skills))
	}
	if skills[0].Name != "research-paper-reader" {
		t.Fatalf("expected front matter name, got %q", skills[0].Name)
	}
	if skills[0].Description != "Read and summarize research papers." {
		t.Fatalf("expected front matter description, got %q", skills[0].Description)
	}
	if strings.Contains(skills[0].Content, "---") || strings.Contains(skills[0].Content, "description:") {
		t.Fatalf("expected content to exclude front matter, got %q", skills[0].Content)
	}
	if strings.Join(skills[0].Triggers, "|") != "paper|arxiv" {
		t.Fatalf("expected front matter triggers, got %v", skills[0].Triggers)
	}
}

func TestManagerReadsSkillJSONMetadata(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "json-skill", "# JSON fallback\nFull instructions.")
	dir := filepath.Join(root, "json-skill")
	if err := os.WriteFile(filepath.Join(dir, skillJSONName), []byte(`{
  "name": "json-name",
  "description": "Description from skill.json",
  "keywords": ["paper", "research"],
  "triggers": ["read paper"]
}`), 0o644); err != nil {
		t.Fatalf("write skill.json: %v", err)
	}

	manager := NewManager(root)
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload skills: %v", err)
	}

	skills := manager.List()
	if len(skills) != 1 {
		t.Fatalf("expected one skill, got %d", len(skills))
	}
	if skills[0].Name != "json-name" {
		t.Fatalf("expected skill.json name, got %q", skills[0].Name)
	}
	if skills[0].Description != "Description from skill.json" {
		t.Fatalf("expected skill.json description, got %q", skills[0].Description)
	}
	if strings.Join(skills[0].Triggers, "|") != "read paper|paper|research" {
		t.Fatalf("expected merged skill.json triggers and keywords, got %v", skills[0].Triggers)
	}
}

func writeSkill(t *testing.T, root string, name string, content string) {
	t.Helper()

	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, skillFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}
}
