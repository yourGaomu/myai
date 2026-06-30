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
	assertSkillHubArgs(t, call, []string{"install", "write-go", "--dir", filepath.Join(absPath(t, workspace), DefaultSkillDir)})
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
	if want := []string{"install", "demo", "--dir", filepath.Join(absPath(t, workspace), "custom-skills")}; !reflect.DeepEqual(call.args, want) {
		t.Fatalf("expected args %v, got %v", want, call.args)
	}
	if !pathExists(filepath.Join(workspace, "custom-skills")) {
		t.Fatalf("expected custom skill root to be created")
	}
}

func TestSkillHubSupportsCustomRegistry(t *testing.T) {
	workspace := t.TempDir()
	runner := &fakeRunner{}
	client := NewClient(Options{
		Workspace: workspace,
		Registry:  "https://registry.example.test",
		Runner:    runner,
	})

	if _, err := client.Search(context.Background(), SearchRequest{Query: "go"}); err != nil {
		t.Fatalf("search skill: %v", err)
	}
	assertSkillHubArgs(t, runner.calls[0], []string{"search", "go", "--registry", "https://registry.example.test"})

	if _, err := client.InstallSkill(context.Background(), InstallRequest{Name: "demo"}); err != nil {
		t.Fatalf("install skill: %v", err)
	}
	assertSkillHubArgs(t, runner.calls[1], []string{"install", "demo", "--dir", filepath.Join(absPath(t, workspace), DefaultSkillDir), "--registry", "https://registry.example.test"})
}

func TestInstallSkillSupportsNamespace(t *testing.T) {
	workspace := t.TempDir()
	runner := &fakeRunner{}
	client := NewClient(Options{
		Workspace: workspace,
		Command:   "custom-skillhub",
		Runner:    runner,
	})

	if _, err := client.InstallSkill(context.Background(), InstallRequest{Name: "global/demo"}); err != nil {
		t.Fatalf("install skill: %v", err)
	}

	if want := []string{"install", "demo", "--dir", filepath.Join(absPath(t, workspace), DefaultSkillDir), "--namespace", "global"}; !reflect.DeepEqual(runner.calls[0].args, want) {
		t.Fatalf("expected args %v, got %v", want, runner.calls[0].args)
	}
}

func TestInstallSkillSupportsForce(t *testing.T) {
	workspace := t.TempDir()
	runner := &fakeRunner{}
	client := NewClient(Options{
		Workspace: workspace,
		Runner:    runner,
	})

	if _, err := client.InstallSkill(context.Background(), InstallRequest{Name: "demo", Force: true}); err != nil {
		t.Fatalf("install skill: %v", err)
	}

	assertSkillHubArgs(t, runner.calls[0], []string{"install", "demo", "--dir", filepath.Join(absPath(t, workspace), DefaultSkillDir), "--force"})
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
	assertSkillHubArgs(t, call, []string{"search", "go"})
}

func TestSearchItemsParsesSkillHubJSON(t *testing.T) {
	workspace := t.TempDir()
	runner := &fakeRunner{result: CommandResult{Output: `{"ok":true,"items":[{"namespace":"global","slug":"research-paper-reader","summary":"Read papers"}],"total":1}`}}
	client := NewClient(Options{
		Workspace: workspace,
		Runner:    runner,
	})

	result, _, err := client.SearchItems(context.Background(), SearchRequest{Query: "paper"})
	if err != nil {
		t.Fatalf("search items: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Slug != "research-paper-reader" {
		t.Fatalf("expected parsed item, got %+v", result)
	}
	assertSkillHubArgs(t, runner.calls[0], []string{"search", "paper", "--json"})
}

func TestResolveSkillRequiresSpecificSlugForAmbiguousKeyword(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Output: `{"ok":true,"items":[{"namespace":"global","slug":"research-paper-reader"},{"namespace":"global","slug":"paper-deconstructor"}],"total":2}`}}
	client := NewClient(Options{Workspace: t.TempDir(), Runner: runner})

	_, candidates, err := client.ResolveSkill(context.Background(), "paper")
	if err == nil {
		t.Fatalf("expected ambiguous skill error")
	}
	if _, ok := err.(*AmbiguousSkillError); !ok {
		t.Fatalf("expected ambiguous skill error, got %T %v", err, err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected candidates, got %v", candidates)
	}
}

func TestResolveSkillUsesExactSlugMatch(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Output: `{"ok":true,"items":[{"namespace":"global","slug":"research-paper-reader"},{"namespace":"global","slug":"paper-deconstructor"}],"total":2}`}}
	client := NewClient(Options{Workspace: t.TempDir(), Runner: runner})

	item, _, err := client.ResolveSkill(context.Background(), "research-paper-reader")
	if err != nil {
		t.Fatalf("resolve skill: %v", err)
	}
	if item.Namespace != "global" || item.Slug != "research-paper-reader" {
		t.Fatalf("expected exact matched item, got %+v", item)
	}
}

func TestResolveSkillAcceptsQualifiedSlugWithoutSearch(t *testing.T) {
	runner := &fakeRunner{}
	client := NewClient(Options{Workspace: t.TempDir(), Runner: runner})

	item, _, err := client.ResolveSkill(context.Background(), "global/research-paper-reader")
	if err != nil {
		t.Fatalf("resolve skill: %v", err)
	}
	if item.Namespace != "global" || item.Slug != "research-paper-reader" {
		t.Fatalf("expected qualified item, got %+v", item)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no search command for qualified slug, got %d", len(runner.calls))
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
	if call.name == "" {
		t.Fatalf("expected bash command")
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
	if !strings.Contains(err.Error(), "SkillHub CLI ran but failed") {
		t.Fatalf("expected registry or slug hint, got %v", err)
	}
}

func TestCommandNotFoundErrorsIncludeInstallHint(t *testing.T) {
	runner := &fakeRunner{err: errors.New("skillhub command not found")}
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

func assertSkillHubArgs(t *testing.T, call fakeCall, want []string) {
	t.Helper()

	if call.name == DefaultCommand {
		if !reflect.DeepEqual(call.args, want) {
			t.Fatalf("expected args %v, got %v", want, call.args)
		}
		return
	}

	if len(call.args) != 2 || call.args[0] != "-lc" {
		t.Fatalf("expected bash -lc args for %v, got command=%q args=%v", want, call.name, call.args)
	}
	if !strings.Contains(call.args[1], `export PYTHONIOENCODING=utf-8;`) || !strings.Contains(call.args[1], `export PATH="$HOME/.local/bin:$PATH";`) {
		t.Fatalf("expected official skillhub bootstrap, got %q", call.args[1])
	}
	for _, arg := range want {
		quoted := shellQuote(arg)
		if !strings.Contains(call.args[1], quoted) {
			t.Fatalf("expected shell script %q to contain %q for args %v", call.args[1], quoted, want)
		}
	}
}
