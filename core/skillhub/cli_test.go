package skillhub

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls  []fakeCall
	result CommandResult
	err    error
}

type fakeCall struct {
	workDir string
	name    string
	args    []string
}

func (r *fakeRunner) Run(ctx context.Context, workDir string, name string, args ...string) (CommandResult, error) {
	r.calls = append(r.calls, fakeCall{
		workDir: workDir,
		name:    name,
		args:    append([]string(nil), args...),
	})

	result := r.result
	if result.Command == nil {
		result.Command = append([]string{name}, args...)
	}
	if result.WorkDir == "" {
		result.WorkDir = workDir
	}
	return result, r.err
}

func TestInstallSkillRunsSkillHubInstallInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	runner := &fakeRunner{}
	client := NewClient(Options{
		Workspace: workspace,
		Runner:    runner,
	})

	_, err := client.InstallSkill(context.Background(), InstallRequest{Name: "  write-go  "})
	if err != nil {
		t.Fatalf("install skill: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one command call, got %d", len(runner.calls))
	}

	call := runner.calls[0]
	if call.workDir != absPath(t, workspace) {
		t.Fatalf("expected workspace %q, got %q", absPath(t, workspace), call.workDir)
	}
	if call.name != DefaultCommand {
		t.Fatalf("expected command %q, got %q", DefaultCommand, call.name)
	}
	if want := []string{"install", "write-go"}; !reflect.DeepEqual(call.args, want) {
		t.Fatalf("expected args %v, got %v", want, call.args)
	}
	if !pathExists(filepath.Join(workspace, DefaultSkillDir)) {
		t.Fatalf("expected default skill root to be created")
	}
}

func TestInstallSkillSupportsCustomCommandAndRoot(t *testing.T) {
	workspace := t.TempDir()
	runner := &fakeRunner{}
	client := NewClient(Options{
		Workspace: workspace,
		SkillRoot: "custom-skills",
		Command:   "custom-skillhub",
		Runner:    runner,
	})

	_, err := client.InstallSkill(context.Background(), InstallRequest{Name: "demo"})
	if err != nil {
		t.Fatalf("install skill: %v", err)
	}

	call := runner.calls[0]
	if call.name != "custom-skillhub" {
		t.Fatalf("expected custom command, got %q", call.name)
	}
	if !pathExists(filepath.Join(workspace, "custom-skills")) {
		t.Fatalf("expected custom skill root to be created")
	}
}

func TestSearchRunsSkillHubSearch(t *testing.T) {
	workspace := t.TempDir()
	runner := &fakeRunner{}
	client := NewClient(Options{
		Workspace: workspace,
		Runner:    runner,
	})

	_, err := client.Search(context.Background(), SearchRequest{Query: "  go  "})
	if err != nil {
		t.Fatalf("search skill: %v", err)
	}

	call := runner.calls[0]
	if call.name != DefaultCommand {
		t.Fatalf("expected command %q, got %q", DefaultCommand, call.name)
	}
	if want := []string{"search", "go"}; !reflect.DeepEqual(call.args, want) {
		t.Fatalf("expected args %v, got %v", want, call.args)
	}
}

func TestInstallCLIRunsCliOnlyInstaller(t *testing.T) {
	workspace := t.TempDir()
	runner := &fakeRunner{}
	client := NewClient(Options{
		Workspace: workspace,
		Runner:    runner,
	})

	_, err := client.InstallCLI(context.Background())
	if err != nil {
		t.Fatalf("install cli: %v", err)
	}

	call := runner.calls[0]
	if call.name != "bash" {
		t.Fatalf("expected bash command, got %q", call.name)
	}
	if want := []string{"-lc"}; len(call.args) < 2 || !reflect.DeepEqual(call.args[:1], want) {
		t.Fatalf("expected args to start with %v, got %v", want, call.args)
	}
	if !strings.Contains(call.args[1], "--cli-only") || !strings.Contains(call.args[1], InstallScript) {
		t.Fatalf("expected cli-only install script, got %q", call.args[1])
	}
}

func TestValidationErrors(t *testing.T) {
	client := NewClient(Options{Workspace: t.TempDir(), Runner: &fakeRunner{}})

	if _, err := client.InstallSkill(context.Background(), InstallRequest{}); err == nil {
		t.Fatalf("expected empty install name error")
	}
	if _, err := client.Search(context.Background(), SearchRequest{}); err == nil {
		t.Fatalf("expected empty search query error")
	}
}

func TestRunnerErrorsIncludeInstallHint(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	client := NewClient(Options{Workspace: t.TempDir(), Runner: runner})

	_, err := client.InstallSkill(context.Background(), InstallRequest{Name: "demo"})
	if err == nil {
		t.Fatalf("expected install error")
	}
	if !strings.Contains(err.Error(), "install SkillHub CLI first") {
		t.Fatalf("expected install hint, got %v", err)
	}
}

func absPath(t *testing.T, path string) string {
	t.Helper()

	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	return abs
}

func pathExists(path string) bool {
	_, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	_, err = filepath.EvalSymlinks(path)
	return err == nil
}
