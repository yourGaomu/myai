package local

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"myai/core/hook"
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
	registry  string
	hooks     *hook.Manager
	runner    skillhub.Runner
	skills    skillCatalog
}

type installSkillArgs struct {
	Name  string `json:"name"`
	Force bool   `json:"force"`
}

type installSkillResult struct {
	Skill      string   `json:"skill"`
	Namespace  string   `json:"namespace,omitempty"`
	Query      string   `json:"query,omitempty"`
	SkillRoot  string   `json:"skill_root"`
	Command    []string `json:"command"`
	WorkDir    string   `json:"work_dir"`
	Output     string   `json:"output"`
	ExitCode   int      `json:"exit_code"`
	Reloaded   bool     `json:"reloaded"`
	SkillCount int      `json:"skill_count"`
	Candidates int      `json:"candidates,omitempty"`
}

func NewInstallSkillToolWithWorkspace(workspace string, skillRoot string) *InstallSkillTool {
	return NewInstallSkillToolWithWorkspaceAndSkills(workspace, skillRoot, nil)
}

func NewInstallSkillToolWithWorkspaceAndSkills(workspace string, skillRoot string, skills skillCatalog) *InstallSkillTool {
	return NewInstallSkillToolWithWorkspaceRegistryAndSkills(workspace, skillRoot, "", skills)
}

func NewInstallSkillToolWithWorkspaceRegistryAndSkills(workspace string, skillRoot string, registry string, skills skillCatalog) *InstallSkillTool {
	return NewInstallSkillToolWithWorkspaceRegistryHooksAndSkills(workspace, skillRoot, registry, nil, skills)
}

func NewInstallSkillToolWithWorkspaceRegistryHooksAndSkills(workspace string, skillRoot string, registry string, hooks *hook.Manager, skills skillCatalog) *InstallSkillTool {
	return newInstallSkillTool(workspace, skillRoot, registry, hooks, nil, skills)
}

func newInstallSkillTool(workspace string, skillRoot string, registry string, hooks *hook.Manager, runner skillhub.Runner, skills skillCatalog) *InstallSkillTool {
	return &InstallSkillTool{
		workspace: workspace,
		skillRoot: skillRoot,
		registry:  registry,
		hooks:     hooks,
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
			"force": map[string]any{
				"type":        "boolean",
				"description": "Overwrite the skill if it is already installed.",
				"default":     false,
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
		Registry:  t.registry,
		Runner:    t.runner,
	})
	// 先解析唯一 Skill 候选再安装，安装完成后立即 reload，使下一轮 Prompt 可以匹配新 Skill。
	target, candidates, err := client.ResolveSkill(ctx, input.Name)
	if err != nil {
		return "", err
	}
	result, err := client.InstallSkill(ctx, skillhub.InstallRequest{Name: target.Slug, Namespace: target.Namespace, Force: input.Force})
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
		t.emitSkillReloaded(ctx, skillCount)
	}

	output, err := json.MarshalIndent(installSkillResult{
		Skill:      target.Slug,
		Namespace:  target.Namespace,
		Query:      input.Name,
		SkillRoot:  filepath.ToSlash(relativePath(workspace, skillRoot)),
		Command:    result.Command,
		WorkDir:    filepath.ToSlash(result.WorkDir),
		Output:     result.Output,
		ExitCode:   result.ExitCode,
		Reloaded:   reloaded,
		SkillCount: skillCount,
		Candidates: len(candidates),
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (t *InstallSkillTool) emitSkillReloaded(ctx context.Context, skillCount int) {
	if t.hooks == nil {
		return
	}
	_ = t.hooks.Emit(ctx, hook.Event{
		Type:       hook.EventSkillReloaded,
		Reason:     "install_skill",
		SkillCount: skillCount,
	})
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
