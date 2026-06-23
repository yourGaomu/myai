package skill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultSkillDir = "skills"
	skillFileName   = "SKILL.md"
	maxSkillBytes   = 64 * 1024
)

type Skill struct {
	Name        string
	Description string
	Path        string
	Content     string
	Triggers    []string
	UpdatedAt   time.Time
}

type Manager struct {
	mu       sync.RWMutex
	root     string
	skills   []Skill
	lastScan time.Time
	lastErr  error
}

func NewManager(root string) *Manager {
	root = strings.TrimSpace(root)
	if root == "" {
		root = defaultSkillDir
	}
	return &Manager{root: filepath.Clean(root)}
}

func (m *Manager) Root() string {
	if m == nil {
		return ""
	}
	return m.root
}

func (m *Manager) Reload(ctx context.Context) error {
	if m == nil {
		return nil
	}

	skills, err := scan(ctx, m.root)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastScan = time.Now()
	m.lastErr = err
	if err != nil {
		return err
	}
	m.skills = skills
	return nil
}

func (m *Manager) List() []Skill {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	skills := make([]Skill, len(m.skills))
	copy(skills, m.skills)
	return skills
}

func (m *Manager) Prompt(ctx context.Context) string {
	if m == nil {
		return ""
	}

	if err := m.Reload(ctx); err != nil {
		return ""
	}

	return Prompt(m.List())
}

func (m *Manager) PromptForInput(ctx context.Context, input string) string {
	if m == nil {
		return ""
	}

	if err := m.Reload(ctx); err != nil {
		return ""
	}

	return PromptForInput(m.List(), input)
}

func Prompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Available skills:\n")
	builder.WriteString("The following skill instructions are loaded from local SKILL.md files. Use them when they match the user's task.\n")

	for _, item := range skills {
		builder.WriteString("\n## ")
		builder.WriteString(item.Name)
		if item.Description != "" {
			builder.WriteString("\nDescription: ")
			builder.WriteString(item.Description)
		}
		if len(item.Triggers) > 0 {
			builder.WriteString("\nTriggers: ")
			builder.WriteString(strings.Join(item.Triggers, ", "))
		}
		builder.WriteString("\nPath: ")
		builder.WriteString(filepath.ToSlash(item.Path))
		builder.WriteString("\nInstructions:\n")
		builder.WriteString(strings.TrimSpace(item.Content))
		builder.WriteString("\n")
	}

	return strings.TrimSpace(builder.String())
}

func PromptForInput(skills []Skill, input string) string {
	if len(skills) == 0 {
		return ""
	}

	selected := SelectForInput(skills, input)

	var builder strings.Builder
	writeSkillIndex(&builder, skills)

	if len(selected) > 0 {
		builder.WriteString("\n\nSelected skill instructions:\n")
		builder.WriteString("The following full skill instructions matched the latest user request. Follow them when relevant.\n")
		for _, item := range selected {
			builder.WriteString("\n## ")
			builder.WriteString(item.Name)
			if item.Description != "" {
				builder.WriteString("\nDescription: ")
				builder.WriteString(item.Description)
			}
			if len(item.Triggers) > 0 {
				builder.WriteString("\nTriggers: ")
				builder.WriteString(strings.Join(item.Triggers, ", "))
			}
			builder.WriteString("\nPath: ")
			builder.WriteString(filepath.ToSlash(item.Path))
			builder.WriteString("\nInstructions:\n")
			builder.WriteString(strings.TrimSpace(item.Content))
			builder.WriteString("\n")
		}
	}

	return strings.TrimSpace(builder.String())
}

func SelectForInput(skills []Skill, input string) []Skill {
	if len(skills) == 0 || strings.TrimSpace(input) == "" {
		return nil
	}

	selected := make([]Skill, 0, len(skills))
	for _, item := range skills {
		if skillMatchesInput(item, input) {
			selected = append(selected, item)
		}
	}
	return selected
}

func writeSkillIndex(builder *strings.Builder, skills []Skill) {
	builder.WriteString("Available skill index:\n")
	builder.WriteString("These local skills are available. The full instructions are included only for skills selected by the latest user request.\n")
	for _, item := range skills {
		builder.WriteString("\n- ")
		builder.WriteString(item.Name)
		if item.Description != "" {
			builder.WriteString(": ")
			builder.WriteString(item.Description)
		}
		if len(item.Triggers) > 0 {
			builder.WriteString(" (triggers: ")
			builder.WriteString(strings.Join(item.Triggers, ", "))
			builder.WriteString(")")
		}
		builder.WriteString(" [")
		builder.WriteString(filepath.ToSlash(item.Path))
		builder.WriteString("]")
	}
}

func scan(ctx context.Context, root string) ([]Skill, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = defaultSkillDir
	}

	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill root is not a directory: %s", root)
	}

	paths := make([]string, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if entry.IsDir() {
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if entry.Name() == skillFileName {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	skills := make([]Skill, 0, len(paths))
	for _, path := range paths {
		item, err := readSkill(root, path)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(item.Content) == "" {
			continue
		}
		skills = append(skills, item)
	}
	return skills, nil
}

func readSkill(root string, path string) (Skill, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Skill{}, err
	}
	if info.Size() > maxSkillBytes {
		return Skill{}, fmt.Errorf("skill file is too large: %s", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}

	name := filepath.Base(filepath.Dir(path))
	if rel, err := filepath.Rel(root, filepath.Dir(path)); err == nil && rel != "." {
		name = filepath.ToSlash(rel)
	}

	text := strings.TrimSpace(string(content))
	return Skill{
		Name:        name,
		Description: firstHeadingOrLine(text),
		Path:        path,
		Content:     text,
		Triggers:    parseTriggers(text),
		UpdatedAt:   info.ModTime(),
	}, nil
}

func firstHeadingOrLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isTriggerLine(line) {
			continue
		}
		line = strings.TrimPrefix(line, "#")
		return strings.TrimSpace(line)
	}
	return ""
}

func parseTriggers(text string) []string {
	seen := make(map[string]struct{})
	triggers := make([]string, 0)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "-* ")

		value, ok := triggerLineValue(line)
		if !ok {
			continue
		}

		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '|'
		}) {
			part = strings.Trim(strings.TrimSpace(part), "`\"'")
			if !usefulTerm(part) {
				continue
			}
			key := strings.ToLower(part)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			triggers = append(triggers, part)
		}
	}
	return triggers
}

func isTriggerLine(line string) bool {
	_, ok := triggerLineValue(strings.TrimLeft(strings.TrimSpace(line), "-* "))
	return ok
}

func triggerLineValue(line string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(line))
	for _, prefix := range []string{"triggers:", "trigger:", "keywords:", "keyword:"} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(line[len(prefix):]), true
		}
	}
	return "", false
}

func skillMatchesInput(item Skill, input string) bool {
	input = normalizeMatchText(input)
	if input == "" {
		return false
	}

	terms := skillMatchTerms(item)
	for _, term := range terms {
		if matchTerm(input, term) {
			return true
		}
	}
	return false
}

func skillMatchTerms(item Skill) []string {
	terms := make([]string, 0, 8+len(item.Triggers))
	terms = append(terms, item.Triggers...)
	terms = append(terms, item.Name)
	terms = append(terms, filepath.Base(item.Name))
	for _, part := range strings.FieldsFunc(item.Name, func(r rune) bool {
		return r == '/' || r == '\\' || r == '-' || r == '_' || r == '.'
	}) {
		terms = append(terms, part)
	}
	if item.Description != "" {
		terms = append(terms, item.Description)
	}
	return uniqueUsefulTerms(terms)
}

func uniqueUsefulTerms(terms []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if !usefulTerm(term) {
			continue
		}
		key := normalizeMatchText(term)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, term)
	}
	return result
}

func matchTerm(input string, term string) bool {
	term = normalizeMatchText(term)
	if term == "" {
		return false
	}
	if strings.Contains(term, " ") || containsNonASCII(term) {
		return strings.Contains(input, term)
	}

	words := strings.Fields(input)
	for _, word := range words {
		if word == term {
			return true
		}
	}
	return false
}

func containsNonASCII(text string) bool {
	for _, r := range text {
		if r > 127 {
			return true
		}
	}
	return false
}

func normalizeMatchText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	replacer := strings.NewReplacer("-", " ", "_", " ", "/", " ", "\\", " ", ".", " ", ":", " ", ",", " ", ";", " ", "|", " ", "(", " ", ")", " ", "[", " ", "]", " ")
	return strings.Join(strings.Fields(replacer.Replace(text)), " ")
}

func usefulTerm(term string) bool {
	term = strings.TrimSpace(term)
	if len([]rune(term)) < 2 {
		return false
	}
	switch strings.ToLower(term) {
	case "a", "an", "and", "or", "the", "this", "that", "skill", "skills":
		return false
	default:
		return true
	}
}
