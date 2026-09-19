package service

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"unicode"

	toolcommand "myai/core/application/tool/command"
	tooldef "myai/core/tool/tool"
)

type askApproval int

const (
	askApprovalAuto askApproval = iota
	askApprovalAsk
)

func assessAskApproval(command toolcommand.Permission) askApproval {
	switch tooldef.NormalizePermission(command.Permission) {
	case tooldef.PermissionRead:
		return askApprovalAuto
	case tooldef.PermissionWrite:
		if pathInsideWorkspace(command.WorkspaceRoot, argumentPath(command.Arguments)) {
			return askApprovalAuto
		}
		return askApprovalAsk
	default:
		return assessExecuteApproval(command)
	}
}

func assessExecuteApproval(command toolcommand.Permission) askApproval {
	cmdline := commandLine(command.Name, command.Arguments)
	if isSafeReadOnlyCommand(cmdline) {
		return askApprovalAuto
	}
	if command.Isolated && cmdline != "" && !isDangerousCommand(cmdline) {
		return askApprovalAuto
	}
	return askApprovalAsk
}

func argumentPath(arguments string) string {
	return jsonStringField(arguments, "path")
}

func commandLine(name, arguments string) string {
	if strings.TrimSpace(name) == "shell" {
		return jsonStringField(arguments, "command")
	}
	return strings.TrimSpace(arguments)
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
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	rawPath = strings.TrimSpace(rawPath)
	if workspaceRoot == "" || rawPath == "" {
		return false
	}
	workspace, err := filepath.Abs(filepath.Clean(workspaceRoot))
	if err != nil {
		return false
	}
	target := rawPath
	if !filepath.IsAbs(target) {
		target = filepath.Join(workspace, target)
	}
	target, err = filepath.Abs(filepath.Clean(target))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(workspace, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isSafeReadOnlyCommand(command string) bool {
	if strings.TrimSpace(command) == "" {
		return false
	}
	for _, segment := range splitCommandSegments(command) {
		if !safeReadOnlySegment(segment) {
			return false
		}
	}
	return true
}

func isDangerousCommand(command string) bool {
	if strings.TrimSpace(command) == "" {
		return false
	}
	for _, segment := range splitCommandSegments(command) {
		if dangerousSegment(segment) {
			return true
		}
	}
	return false
}

func splitCommandSegments(command string) []string {
	replacer := strings.NewReplacer(
		"&&", "\n",
		"||", "\n",
		"|", "\n",
		";", "\n",
		"`", "\n",
	)
	parts := strings.Split(replacer.Replace(command), "\n")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments = append(segments, part)
	}
	if len(segments) == 0 {
		return []string{strings.TrimSpace(command)}
	}
	return segments
}

func safeReadOnlySegment(segment string) bool {
	tokens := commandTokens(segment)
	if len(tokens) == 0 {
		return false
	}
	first := strings.ToLower(tokens[0])
	switch first {
	case "date", "time", "pwd", "cd", "dir", "ls", "hostname", "whoami", "get-date", "get-location", "set-location", "get-childitem", "gci":
		return !hasRedirection(segment)
	case "git":
		if len(tokens) < 2 {
			return false
		}
		switch strings.ToLower(tokens[1]) {
		case "status", "diff", "log", "rev-parse", "branch":
			return !hasRedirection(segment)
		default:
			return false
		}
	case "go", "python", "python3", "py", "node", "java":
		return versionQuery(tokens[1:]) && !hasRedirection(segment)
	default:
		return false
	}
}

func dangerousSegment(segment string) bool {
	tokens := commandTokens(segment)
	if len(tokens) == 0 {
		return false
	}
	first := strings.ToLower(tokens[0])
	switch first {
	case "rm", "del", "erase", "rd", "rmdir", "remove-item", "ri", "format", "shutdown", "restart-computer",
		"curl", "wget", "invoke-webrequest", "iwr", "iex", "invoke-expression", "start-process",
		"set-executionpolicy", "chmod", "chown", "sudo", "mkfs", "diskpart":
		return true
	case "npm", "pip", "pip3", "pnpm", "yarn":
		return len(tokens) > 1 && strings.EqualFold(tokens[1], "install")
	case "go":
		return len(tokens) > 1 && strings.EqualFold(tokens[1], "install")
	}
	lower := strings.ToLower(segment)
	return strings.Contains(lower, "invoke-expression") || strings.Contains(lower, "iex(")
}

func versionQuery(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	flag := strings.ToLower(tokens[0])
	return flag == "--version" || flag == "-version" || flag == "-v"
}

func hasRedirection(segment string) bool {
	return strings.ContainsAny(segment, "<>")
}

func commandTokens(segment string) []string {
	fields := strings.FieldsFunc(segment, func(r rune) bool {
		return unicode.IsSpace(r)
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, `"'`)
		if field != "" {
			tokens = append(tokens, field)
		}
	}
	return tokens
}
