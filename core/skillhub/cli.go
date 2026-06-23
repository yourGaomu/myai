package skillhub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	Runner    Runner
}

type InstallRequest struct {
	Name string
}

type SearchRequest struct {
	Query string
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
		runner:    runner,
	}
}

func (c *Client) InstallCLI(ctx context.Context) (CommandResult, error) {
	workspace, err := c.WorkspacePath()
	if err != nil {
		return CommandResult{}, err
	}

	script := fmt.Sprintf("curl -fsSL %s | bash -s -- --cli-only", InstallScript)
	result, err := c.runner.Run(ctx, workspace, "bash", "-lc", script)
	if err != nil {
		return result, withInstallCLIHint(err)
	}
	return result, nil
}

func (c *Client) InstallSkill(ctx context.Context, request InstallRequest) (CommandResult, error) {
	name := strings.TrimSpace(request.Name)
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

	args := []string{"install", name}
	result, err := c.runner.Run(ctx, workspace, c.command, args...)
	if err != nil {
		return result, withSkillHubHint(err)
	}
	return result, nil
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

	args := []string{"search", query}
	result, err := c.runner.Run(ctx, workspace, c.command, args...)
	if err != nil {
		return result, withSkillHubHint(err)
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
	return fmt.Errorf("%w; install SkillHub CLI first with: curl -fsSL %s | bash -s -- --cli-only", err, InstallScript)
}

func withInstallCLIHint(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w; manual CLI-only install command: curl -fsSL %s | bash -s -- --cli-only", err, InstallScript)
}
