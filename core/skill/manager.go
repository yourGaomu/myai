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
		builder.WriteString("\nPath: ")
		builder.WriteString(filepath.ToSlash(item.Path))
		builder.WriteString("\nInstructions:\n")
		builder.WriteString(strings.TrimSpace(item.Content))
		builder.WriteString("\n")
	}

	return strings.TrimSpace(builder.String())
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
		UpdatedAt:   info.ModTime(),
	}, nil
}

func firstHeadingOrLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "#")
		return strings.TrimSpace(line)
	}
	return ""
}
