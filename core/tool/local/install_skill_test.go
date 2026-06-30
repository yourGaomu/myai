package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"myai/core/skill"
	"myai/core/skillhub"
)

type installSkillFakeRunner struct {
	t       *testing.T
	skill   string
	root    string
	calls   []installSkillFakeCall
	result  skillhub.CommandResult
	err     error
	install bool
}

type installSkillFakeCall struct {
	workDir string
	name    string
	args    []string
}

func (r *installSkillFakeRunner) Run(ctx context.Context, workDir string, name string, args ...string) (skillhub.CommandResult, error) {
	r.calls = append(r.calls, installSkillFakeCall{
		workDir: workDir,
		name:    name,
		args:    append([]string(nil), args...),
	})

	isInstall := hasSkillHubArg(args, "install")
	if r.install && isInstall {
		skillDir := filepath.Join(r.root, r.skill)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			r.t.Fatalf("create fake skill dir failed: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Demo skill\nTriggers: demo\n"), 0o644); err != nil {
			r.t.Fatalf("write fake SKILL.md failed: %v", err)
		}
	}

	result := r.result
	if hasSkillHubArg(args, "search") && result.Output == "" {
		result.Output = `{"query":"demo","count":1,"results":[{"slug":"demo","name":"Demo skill","description":"Demo skill","version":"1.0.0","source":"community"}],"warnings":[]}`
	}
	if result.Command == nil {
		result.Command = append([]string{name}, args...)
	}
	if result.WorkDir == "" {
		result.WorkDir = workDir
	}
	return result, r.err
}

func TestInstallSkillToolInstallsAndReloadsSkillCatalog(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "skills")
	manager := skill.NewManager(root)
	runner := &installSkillFakeRunner{
		t:       t,
		skill:   "demo",
		root:    root,
		install: true,
	}
	tool := newInstallSkillTool(workspace, root, "", nil, runner, manager)

	output, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"name":  " demo ",
		"force": true,
	}))
	if err != nil {
		t.Fatalf("install skill failed: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected search and install skillhub commands, got %d", len(runner.calls))
	}
	search := runner.calls[0]
	assertInstallSkillHubArgs(t, search, []string{"search", "demo", "--json"})
	call := runner.calls[1]
	assertInstallSkillHubArgs(t, call, []string{"install", "demo", "--dir", root, "--force"})

	var result installSkillResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode install result failed: %v", err)
	}
	if !result.Reloaded {
		t.Fatalf("expected skill catalog to be reloaded, output=%s", output)
	}
	if result.Skill != "demo" || result.Namespace != "" {
		t.Fatalf("expected installed skill metadata, got %+v", result)
	}
	if result.SkillCount != 1 {
		t.Fatalf("expected one loaded skill, got %d: %s", result.SkillCount, output)
	}
	if len(manager.List()) != 1 {
		t.Fatalf("expected manager to contain installed skill")
	}
}

func hasSkillHubArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return strings.Contains(strings.Join(args, " "), strconv.Quote(value))
}

func assertInstallSkillHubArgs(t *testing.T, call installSkillFakeCall, want []string) {
	t.Helper()

	if call.name == skillhub.DefaultCommand {
		if !reflect.DeepEqual(call.args, want) {
			t.Fatalf("expected args %v, got %v", want, call.args)
		}
		return
	}

	for _, arg := range want {
		if !hasSkillHubArg(call.args, arg) {
			t.Fatalf("expected command=%q args=%v to contain %q for %v", call.name, call.args, arg, want)
		}
	}
}
