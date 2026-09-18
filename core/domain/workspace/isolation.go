package workspace

import (
	"fmt"
	"strings"
)

type IsolationMode string

const (
	IsolationModeDirect      IsolationMode = "direct"
	IsolationModeSnapshot    IsolationMode = "snapshot"
	IsolationModeGitWorktree IsolationMode = "git_worktree"
	IsolationModeOpenSandbox IsolationMode = "opensandbox"
)

func NormalizeIsolationMode(mode IsolationMode) IsolationMode {
	switch mode {
	case IsolationModeDirect, IsolationModeSnapshot, IsolationModeGitWorktree, IsolationModeOpenSandbox:
		return mode
	default:
		return IsolationModeSnapshot
	}
}

func ValidateIsolationMode(mode IsolationMode) error {
	if strings.TrimSpace(string(mode)) == "" {
		return nil
	}
	switch mode {
	case IsolationModeDirect, IsolationModeSnapshot, IsolationModeGitWorktree, IsolationModeOpenSandbox:
		return nil
	default:
		return fmt.Errorf("unsupported workspace isolation mode %q", mode)
	}
}

type Reference struct {
	ID         string
	Mode       IsolationMode
	Root       string
	SourceRoot string
	SandboxID  string
}
