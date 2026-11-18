package service

import (
	"encoding/json"
	"strconv"
	"strings"

	toolcommand "myai/core/application/tool/command"
	"myai/core/pathpolicy"
	tooldef "myai/core/tool/tool"
)

type askApproval int

const (
	askApprovalAuto askApproval = iota
	askApprovalAsk
)

func assessAskApproval(command toolcommand.Permission) askApproval {
	if requiresExplicitApproval(command) {
		return askApprovalAsk
	}
	switch tooldef.NormalizePermission(command.Permission) {
	case tooldef.PermissionRead:
		return askApprovalAuto
	case tooldef.PermissionWrite:
		// Only the local file mutation tools use this path-based exemption.
		if (command.Name == "write_file" || command.Name == "edit_file") && pathInsideWorkspace(command.WorkspaceRoot, argumentPath(command.Arguments)) {
			return askApprovalAuto
		}
	case tooldef.PermissionExecute:
		return assessExecuteApproval(command)
	}
	return askApprovalAsk
}

// External publication and executable declarations need per-call consent even
// in full mode. A workspace path alone is never authorization to publish it.
func requiresExplicitApproval(command toolcommand.Permission) bool {
	if command.ExplicitApproval {
		return true
	}
	switch command.Name {
	case "share_file", "install_skill":
		return true
	}
	for _, field := range []string{"path", "work_dir", "destination", "source", "target"} {
		path := jsonStringField(command.Arguments, field)
		if path == "" {
			continue
		}
		if pathpolicy.IsProtected(path) {
			return true
		}
		if command.WorkspaceRoot != "" && (pathpolicy.Policy{WorkspaceRoot: command.WorkspaceRoot}).RequiresApproval(path) {
			return true
		}
	}
	if command.Name == "shell" {
		for _, word := range strings.Fields(jsonStringField(command.Arguments, "command")) {
			if pathpolicy.IsProtected(strings.Trim(word, `"'`)) {
				return true
			}
		}
	}
	return false
}

func assessExecuteApproval(command toolcommand.Permission) askApproval {
	// Isolated is an execution-boundary fact; it only grants an exemption for
	// shell, never MCP, publication or any other execute tool.
	if command.Name != "shell" || requiresExplicitApproval(command) {
		return askApprovalAsk
	}
	cmdline := jsonStringField(command.Arguments, "command")
	if isSafeReadOnlyCommand(cmdline) {
		return askApprovalAuto
	}
	if command.Isolated {
		tokens, ok := simpleCommandTokens(cmdline)
		if ok && !dangerousSegment(tokens) {
			return askApprovalAuto
		}
	}
	return askApprovalAsk
}

func argumentPath(arguments string) string {
	return jsonStringField(arguments, "path")
}

func jsonStringField(arguments, field string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(arguments)), &payload); err != nil {
		return ""
	}
	value, _ := payload[field].(string)
	return strings.TrimSpace(value)
}

func pathInsideWorkspace(workspaceRoot, rawPath string) bool {
	_, err := pathpolicy.Resolve(workspaceRoot, rawPath)
	return err == nil && !(pathpolicy.Policy{WorkspaceRoot: workspaceRoot}).RequiresApproval(rawPath)
}

// This deliberately accepts one simple command. Quotes, substitutions, shell
// operators, continuations and wildcards are not parsed reliably across the
// supported shells and therefore always require approval.
func simpleCommandTokens(command string) ([]string, bool) {
	command = strings.TrimSpace(command)
	if command == "" || strings.ContainsAny(command, "\r\n\x00\"'`$%&|;<>\\(){}[]!?*~#^") {
		return nil, false
	}
	for _, r := range command {
		if r > 127 || r < 32 && r != '\t' {
			return nil, false
		}
	}
	return strings.Fields(command), true
}

func isSafeReadOnlyCommand(command string) bool {
	tokens, ok := simpleCommandTokens(command)
	if !ok || len(tokens) == 0 {
		return false
	}
	args := tokens[1:]
	switch strings.ToLower(tokens[0]) {
	case "pwd", "whoami", "hostname", "date", "get-date", "get-location":
		return len(args) == 0
	case "ls":
		return allowedFlags(args, "-l", "-a", "-la", "-al", "-lh", "-lah", "--all", "--long")
	case "dir":
		return allowedFlags(args, "/b", "/a", "/ad", "/a-d")
	case "go":
		return len(args) == 1 && args[0] == "version"
	case "node", "python", "python3", "py":
		return len(args) == 1 && args[0] == "--version"
	case "java":
		return len(args) == 1 && args[0] == "-version"
	case "git":
		// Disable configured pagers. Each subcommand has an argument allowlist;
		// a read-looking subcommand can otherwise create files or run programs.
		if len(args) < 2 || args[0] != "--no-pager" {
			return false
		}
		args = args[1:]
		switch args[0] {
		case "status":
			return allowedFlags(args[1:], "--short", "--branch", "--porcelain", "--porcelain=v1", "--porcelain=v2", "--untracked-files=no", "--untracked-files=normal")
		case "log":
			for _, arg := range args[1:] {
				if strings.HasPrefix(arg, "--max-count=") {
					n, err := strconv.Atoi(strings.TrimPrefix(arg, "--max-count="))
					if err != nil || n < 1 || n > 100 {
						return false
					}
				} else if !allowedFlags([]string{arg}, "--oneline", "--no-patch", "--decorate=no") {
					return false
				}
			}
			return true
		case "diff":
			return len(args) >= 3 && args[1] == "--no-ext-diff" && args[2] == "--no-textconv" && allowedFlags(args[3:], "--stat", "--name-only", "--cached", "--staged", "--check")
		case "rev-parse":
			return len(args) == 2 && allowedFlags(args[1:], "--show-toplevel", "--is-inside-work-tree", "--show-prefix")
		case "branch":
			return len(args) == 2 && args[1] == "--show-current"
		}
	}
	return false
}

func allowedFlags(args []string, flags ...string) bool {
	for _, arg := range args {
		found := false
		for _, flag := range flags {
			if arg == flag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func dangerousSegment(tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	switch strings.ToLower(tokens[0]) {
	case "rm", "del", "erase", "rd", "rmdir", "remove-item", "ri", "format", "shutdown", "restart-computer",
		"curl", "wget", "invoke-webrequest", "iwr", "iex", "invoke-expression", "start-process",
		"set-executionpolicy", "chmod", "chown", "sudo", "mkfs", "diskpart", "git":
		return true
	case "npm", "pip", "pip3", "pnpm", "yarn", "go":
		return len(tokens) > 1 && strings.EqualFold(tokens[1], "install")
	}
	return false
}
