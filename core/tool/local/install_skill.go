package local

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"myai/core/skill"
	"myai/core/skillhub"
	tooldef "myai/core/tool/tool"
)

var errEmptySkillName = errors.New("skill name is empty")

type skillCatalog interface {
	Reload(ctx context.Context) error
	List() []skill.Skill
}

type InstallSkillTool struct {
	workspace string
	skillRoot string
	runner    skillhub.Runner
	skills    skillCatalog
}

type installSkillArgs struct {
	Name string `json:"name"`
}

type installSkillResult struct {
	Skill      string   `json:"skill"`
	SkillRoot  string   `json:"skill_root"`
	Command    []string `json:"command"`
	WorkDir    string   `json:"work_dir"`
	Output     string   `json:"output"`
	ExitCode   int      `json:"exit_code"`
	Reloaded   bool     `json:"reloaded"`
	SkillCount int      `json:"skill_count"`
}

func NewInstallSkillToolWithWorkspace(workspace string, skillRoot string) *InstallSkillTool {
	return NewInstallSkillToolWithWorkspaceAndSkills(workspace, skillRoot, nil)
}

func NewInstallSkillToolWithWorkspaceAndSkills(workspace string, skillRoot string, skills skillCatalog) *InstallSkillTool {
	return newInstallSkillTool(workspace, skillRoot, nil, skills)
}

func newInstallSkillTool(workspace string, skillRoot string, runner skillhub.Runner, skills skillCatalog) *InstallSkillTool {
	return &InstallSkillTool{
		workspace: workspace,
		skillRoot: skillRoot,
		runner:    runner,
		skills:    skills,
	}
}

func (t *InstallSkillTool) Name() string {
	return "install_skill"
}

func (t *InstallSkillTool) Description() string {
	return "Install a SkillHub skill into the local workspace skill directory. Use only when the user explicitly asks to install a skill by name."
}

func (t *InstallSkillTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name or slug to install, for example pdf-parser.",
			},
		},
		"required": []string{"name"},
	}
}

func (t *InstallSkillTool) Permission() tooldef.Permission {
	return tooldef.PermissionExecute
}

func (t *InstallSkillTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	input, err := normalizeInstallSkillArgs(args)
	if err != nil {
		return "", err
	}

	workspace, err := toolWorkspace(t.workspace)
	if err != nil {
		return "", err
	}
	client := skillhub.NewClient(skillhub.Options{
		Workspace: workspace,
		SkillRoot: t.skillRoot,
		Runner:    t.runner,
	})
	result, err := client.InstallSkill(ctx, skillhub.InstallRequest{Name: input.Name})
	if err != nil {
		return "", err
	}

	skillRoot, err := client.SkillRootPath()
	if err != nil {
		return "", err
	}

	reloaded := false
	skillCount := 0
	if t.skills != nil {
		if err := t.skills.Reload(ctx); err != nil {
			return "", errors.New("reload installed skills: " + err.Error())
		}
		reloaded = true
		skillCount = len(t.skills.List())
	}

	output, err := json.MarshalIndent(installSkillResult{
		Skill:      input.Name,
		SkillRoot:  filepath.ToSlash(relativePath(workspace, skillRoot)),
		Command:    result.Command,
		WorkDir:    filepath.ToSlash(result.WorkDir),
		Output:     result.Output,
		ExitCode:   result.ExitCode,
		Reloaded:   reloaded,
		SkillCount: skillCount,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func normalizeInstallSkillArgs(args json.RawMessage) (installSkillArgs, error) {
	var input installSkillArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return installSkillArgs{}, err
		}
	}

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return installSkillArgs{}, errEmptySkillName
	}
	return input, nil
}
