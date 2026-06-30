package skillhub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	DefaultCommand  = "skillhub"
	DefaultSkillDir = "skills"
	InstallScript   = "https://skillhub-1388575217.cos.ap-guangzhou.myqcloud.com/install/install.sh"
)

type Options struct {
	Workspace string
	SkillRoot string
	Command   string
	Registry  string
	Runner    Runner
}

type InstallRequest struct {
	Name      string
	Namespace string
	Force     bool
}

type SearchRequest struct {
	Query string
}

type SearchResult struct {
	OK    bool         `json:"ok"`
	Items []SearchItem `json:"items"`
	Total int          `json:"total"`
}

type SearchItem struct {
	Namespace     string `json:"namespace"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	LatestVersion string `json:"latestVersion"`
	Version       string `json:"version"`
	Summary       string `json:"summary"`
	Description   string `json:"description"`
	Source        string `json:"source"`
}

type AmbiguousSkillError struct {
	Query      string
	Candidates []SearchItem
}

func (e *AmbiguousSkillError) Error() string {
	return fmt.Sprintf("multiple SkillHub skills matched %q; install a specific slug", e.Query)
}

type CommandResult struct {
	Command  []string `json:"command"`
	WorkDir  string   `json:"work_dir"`
	Output   string   `json:"output"`
	ExitCode int      `json:"exit_code"`
}

type Runner interface {
	Run(ctx context.Context, workDir string, name string, args ...string) (CommandResult, error)
}

type Client struct {
	workspace string
	skillRoot string
	command   string
	registry  string
	runner    Runner
}

func NewClient(options Options) *Client {
	command := strings.TrimSpace(options.Command)
	if command == "" {
		command = DefaultCommand
	}

	runner := options.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	return &Client{
		workspace: strings.TrimSpace(options.Workspace),
		skillRoot: strings.TrimSpace(options.SkillRoot),
		command:   command,
		registry:  strings.TrimSpace(options.Registry),
		runner:    runner,
	}
}

func (c *Client) InstallCLI(ctx context.Context) (CommandResult, error) {
	workspace, err := c.WorkspacePath()
	if err != nil {
		return CommandResult{}, err
	}

	var result CommandResult
	script := fmt.Sprintf("curl -fsSL %s | bash -s -- --cli-only", InstallScript)
	bash := c.bashCommand()
	result, err = c.runner.Run(ctx, workspace, bash, "-lc", script)
	if err != nil {
		return result, withInstallCLIHint(err)
	}
	return result, nil
}

func (c *Client) InstallSkill(ctx context.Context, request InstallRequest) (CommandResult, error) {
	namespace, name := normalizeInstallTarget(request.Namespace, request.Name)
	if name == "" {
		return CommandResult{}, errors.New("skill name is empty")
	}

	workspace, err := c.WorkspacePath()
	if err != nil {
		return CommandResult{}, err
	}
	skillRoot, err := c.SkillRootPath()
	if err != nil {
		return CommandResult{}, err
	}
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		return CommandResult{}, err
	}

	args := []string{"install", name, "--dir", skillRoot}
	if namespace != "" && c.command != DefaultCommand {
		args = append(args, "--namespace", namespace)
	}
	if request.Force {
		args = append(args, "--force")
	}
	args = c.withRegistry(args...)
	result, err := c.runSkillHub(ctx, workspace, args...)
	if err != nil {
		return result, withSkillHubHint(err)
	}
	return result, nil
}

func (c *Client) InstallMatchedSkill(ctx context.Context, request InstallRequest) (SearchItem, CommandResult, error) {
	target, _, err := c.ResolveSkill(ctx, request.Name)
	if err != nil {
		return SearchItem{}, CommandResult{}, err
	}

	result, err := c.InstallSkill(ctx, InstallRequest{Name: target.Slug, Namespace: target.Namespace, Force: request.Force})
	if err != nil {
		return target, result, err
	}
	return target, result, nil
}

func (c *Client) Search(ctx context.Context, request SearchRequest) (CommandResult, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return CommandResult{}, errors.New("search query is empty")
	}

	workspace, err := c.WorkspacePath()
	if err != nil {
		return CommandResult{}, err
	}

	args := c.withRegistry("search", query)
	result, err := c.runSkillHub(ctx, workspace, args...)
	if err != nil {
		return result, withSkillHubHint(err)
	}
	return result, nil
}

func (c *Client) SearchItems(ctx context.Context, request SearchRequest) (SearchResult, CommandResult, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return SearchResult{}, CommandResult{}, errors.New("search query is empty")
	}

	workspace, err := c.WorkspacePath()
	if err != nil {
		return SearchResult{}, CommandResult{}, err
	}

	args := c.withRegistry("search", query, "--json")
	result, err := c.runSkillHub(ctx, workspace, args...)
	if err != nil {
		return SearchResult{}, result, withSkillHubHint(err)
	}

	parsed, err := ParseSearchOutput(result.Output)
	if err != nil {
		return SearchResult{}, result, err
	}
	return parsed, result, nil
}

func (c *Client) ResolveSkill(ctx context.Context, query string) (SearchItem, []SearchItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchItem{}, nil, errors.New("skill name is empty")
	}

	namespace, slug, qualified := splitQualifiedSkill(query)
	if qualified {
		return SearchItem{Namespace: namespace, Slug: slug}, nil, nil
	}

	result, _, err := c.SearchItems(ctx, SearchRequest{Query: query})
	if err != nil {
		return SearchItem{}, nil, err
	}
	if len(result.Items) == 0 {
		return SearchItem{}, nil, fmt.Errorf("no SkillHub skill matched %q", query)
	}

	matches := exactSearchMatches(query, result.Items)
	if len(matches) == 1 {
		return matches[0], result.Items, nil
	}
	if len(matches) > 1 {
		if target, ok := uniqueGlobalMatch(matches); ok {
			return target, result.Items, nil
		}
		return SearchItem{}, result.Items, &AmbiguousSkillError{Query: query, Candidates: matches}
	}
	if len(result.Items) == 1 {
		return result.Items[0], result.Items, nil
	}
	return SearchItem{}, result.Items, &AmbiguousSkillError{Query: query, Candidates: result.Items}
}

func ParseSearchOutput(output string) (SearchResult, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return SearchResult{}, errors.New("SkillHub search returned empty output")
	}

	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start < 0 || end < start {
		return SearchResult{}, fmt.Errorf("SkillHub search returned non-JSON output: %s", output)
	}

	var result SearchResult
	if err := json.Unmarshal([]byte(output[start:end+1]), &result); err != nil {
		return SearchResult{}, fmt.Errorf("parse SkillHub search JSON: %w", err)
	}
	if len(result.Items) == 0 {
		var official struct {
			Results []SearchItem `json:"results"`
			Count   int          `json:"count"`
		}
		if err := json.Unmarshal([]byte(output[start:end+1]), &official); err == nil && len(official.Results) > 0 {
			result.OK = true
			result.Items = official.Results
			result.Total = official.Count
		}
	}
	for i := range result.Items {
		if result.Items[i].Summary == "" {
			result.Items[i].Summary = result.Items[i].Description
		}
		if result.Items[i].LatestVersion == "" {
			result.Items[i].LatestVersion = result.Items[i].Version
		}
	}
	return result, nil
}

func (c *Client) SkillRootPath() (string, error) {
	workspace, err := c.WorkspacePath()
	if err != nil {
		return "", err
	}

	root := strings.TrimSpace(c.skillRoot)
	if root == "" {
		root = DefaultSkillDir
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(workspace, root)
	}
	return filepath.Abs(filepath.Clean(root))
}

func (c *Client) WorkspacePath() (string, error) {
	workspace := strings.TrimSpace(c.workspace)
	if workspace == "" {
		current, err := os.Getwd()
		if err != nil {
			return "", err
		}
		workspace = current
	}
	return filepath.Abs(filepath.Clean(workspace))
}

func (c *Client) withRegistry(args ...string) []string {
	next := append([]string(nil), args...)
	if c.registry != "" {
		next = append(next, "--registry", c.registry)
	}
	return next
}

func (c *Client) runSkillHub(ctx context.Context, workspace string, args ...string) (CommandResult, error) {
	if runtime.GOOS == "windows" && c.command == DefaultCommand {
		script := skillHubShellScript(args)
		return c.runner.Run(ctx, workspace, c.bashCommand(), "-lc", script)
	}
	return c.runner.Run(ctx, workspace, c.command, args...)
}

func (c *Client) bashCommand() string {
	if runtime.GOOS != "windows" {
		return "bash"
	}
	for _, candidate := range []string{
		os.Getenv("SKILLHUB_BASH"),
		`D:\Git\bin\bash.exe`,
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "bash"
}

func skillHubShellScript(args []string) string {
	parts := []string{`export PYTHONIOENCODING=utf-8;`, `export PATH="$HOME/.local/bin:$PATH";`}
	if runtime.GOOS == "windows" {
		parts = append(parts, "python", `"$HOME/.skillhub/skills_store_cli.py"`)
	} else {
		parts = append(parts, "skillhub")
	}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return strconv.Quote(value)
}

func normalizeInstallTarget(namespace string, name string) (string, string) {
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	if parsedNamespace, slug, ok := splitQualifiedSkill(name); ok {
		if namespace == "" {
			namespace = parsedNamespace
		}
		name = slug
	}
	return namespace, name
}

func splitQualifiedSkill(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	left, right, ok := strings.Cut(value, "/")
	if !ok {
		return "", value, false
	}
	left = strings.TrimSpace(left)
	right = strings.Trim(strings.TrimSpace(right), "/")
	if left == "" || right == "" || strings.Contains(right, "/") {
		return "", value, false
	}
	return left, right, true
}

func exactSearchMatches(query string, items []SearchItem) []SearchItem {
	query = normalizeSkillMatch(query)
	matches := make([]SearchItem, 0)
	for _, item := range items {
		if query == normalizeSkillMatch(item.Slug) || query == normalizeSkillMatch(item.Namespace+"/"+item.Slug) {
			matches = append(matches, item)
		}
	}
	return matches
}

func uniqueGlobalMatch(items []SearchItem) (SearchItem, bool) {
	var target SearchItem
	count := 0
	for _, item := range items {
		if strings.EqualFold(item.Namespace, "global") {
			target = item
			count++
		}
	}
	return target, count == 1
}

func normalizeSkillMatch(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, workDir string, name string, args ...string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workDir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	result := CommandResult{
		Command:  append([]string{name}, args...),
		WorkDir:  workDir,
		Output:   strings.TrimSpace(output.String()),
		ExitCode: exitCode(err),
	}
	if err != nil {
		return result, commandError(name, result.Output, err)
	}
	return result, nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func commandError(name string, output string, err error) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return fmt.Errorf("%s command not found: %w", name, err)
	}
	if strings.TrimSpace(output) == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, output)
}

func withSkillHubHint(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "command not found") {
		return fmt.Errorf("%w; install SkillHub CLI first with: curl -fsSL %s | bash -s -- --cli-only", err, InstallScript)
	}
	if strings.Contains(strings.ToLower(err.Error()), "already installed") {
		return fmt.Errorf("%w; use --force to overwrite the existing skill", err)
	}
	return fmt.Errorf("%w; SkillHub CLI ran but failed. Check the skill slug, namespace, and registry; you can pass --registry <url>", err)
}

func withInstallCLIHint(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w; manual CLI-only install command: curl -fsSL %s | bash -s -- --cli-only", err, InstallScript)
}
