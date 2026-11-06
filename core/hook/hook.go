package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"myai/core/processpolicy"
	"myai/core/trust"
)

const (
	EventPreToolUse       EventType = "pre_tool_use"
	EventPostToolUse      EventType = "post_tool_use"
	EventSessionChanged   EventType = "session_changed"
	EventSkillReloaded    EventType = "skill_reloaded"
	EventSessionStart     EventType = "session_start"
	EventUserPromptSubmit EventType = "user_prompt_submit"
	EventStop             EventType = "stop"
)

const (
	DecisionContinue Decision = "continue"
	DecisionAllow    Decision = "allow"
	DecisionAsk      Decision = "ask"
	DecisionDeny     Decision = "deny"
)

type EventType string

type Decision string

type Event struct {
	Type          EventType `json:"type"`
	SessionID     string    `json:"session_id,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	ToolName      string    `json:"tool_name,omitempty"`
	ToolArguments string    `json:"tool_arguments,omitempty"`
	Permission    string    `json:"permission,omitempty"`
	Result        string    `json:"result,omitempty"`
	Error         string    `json:"error,omitempty"`
	SkillCount    int       `json:"skill_count,omitempty"`
	Prompt        string    `json:"prompt,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

type Result struct {
	Decision  Decision `json:"decision,omitempty"`
	Message   string   `json:"message,omitempty"`
	Arguments string   `json:"arguments,omitempty"`
}

type Handler interface {
	HandleHook(ctx context.Context, event Event) (Result, error)
}

type Config struct {
	Workspace    string
	CommandHooks []CommandHookConfig
	Trust        *trust.Policy
	// ValidateArguments can enforce the tool schema after each rewrite. The tool
	// execution boundary must also validate the final arguments before use.
	ValidateArguments func(toolName, arguments string) error
}

type CommandHookConfig struct {
	Event   string `mapstructure:"event" json:"event" yaml:"event"`
	Command string `mapstructure:"command" json:"command" yaml:"command"`
	Timeout string `mapstructure:"timeout" json:"timeout" yaml:"timeout"`
	WorkDir string `mapstructure:"work_dir" json:"work_dir" yaml:"work_dir"`
	Enabled *bool  `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
}

type Manager struct {
	// Manager 允许多个 Hook 依次处理同一事件；读取时复制 handler 列表以缩短锁范围。
	mu                sync.RWMutex
	handlers          []Handler
	validateArguments func(string, string) error
}

func NewManager(config Config) (*Manager, error) {
	manager := &Manager{validateArguments: config.ValidateArguments}
	for index, command := range config.CommandHooks {
		if command.Enabled != nil && !*command.Enabled {
			continue
		}
		handler, err := NewCommandHook(command, config.Workspace, WithCommandTrustPolicy(config.Trust))
		if err != nil {
			return nil, fmt.Errorf("configure command hook %d: %w", index+1, err)
		}
		manager.Register(handler)
	}
	return manager, nil
}

func (m *Manager) Register(handler Handler) {
	if m == nil || handler == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.handlers = append(m.handlers, handler)
}

func (m *Manager) PreToolUse(ctx context.Context, event Event) (Result, error) {
	event.Type = EventPreToolUse
	event = normalizeEvent(event)
	results, err := m.handle(ctx, event)
	if err != nil {
		return Result{}, err
	}
	return aggregatePreToolUse(results), nil
}

func (m *Manager) Emit(ctx context.Context, event Event) error {
	event = normalizeEvent(event)
	_, err := m.handle(ctx, event)
	return err
}

func (m *Manager) RunLifecycle(ctx context.Context, event Event) (Result, error) {
	event = normalizeEvent(event)
	if event.Type == "" {
		return Result{}, nil
	}
	results, err := m.handle(ctx, event)
	if err != nil {
		return Result{}, err
	}
	return aggregatePreToolUse(results), nil
}

func (m *Manager) handle(ctx context.Context, event Event) ([]Result, error) {
	if m == nil {
		return nil, nil
	}

	m.mu.RLock()
	handlers := append([]Handler(nil), m.handlers...)
	m.mu.RUnlock()
	if len(handlers) == 0 {
		return nil, nil
	}

	results := make([]Result, 0, len(handlers))
	for index, handler := range handlers {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		result, err := handler.HandleHook(ctx, event)
		if err != nil {
			return results, fmt.Errorf("hook %d: %w", index+1, err)
		}
		result, err = validateResult(result)
		if err != nil {
			return results, fmt.Errorf("hook %d: %w", index+1, err)
		}
		if result.Decision == DecisionDeny {
			results = append(results, result)
			return results, nil
		}
		if strings.TrimSpace(result.Arguments) != "" {
			if event.Type != EventPreToolUse {
				return results, errors.New("argument rewrites are only valid for pre_tool_use")
			}
			if m.validateArguments != nil {
				if err := m.validateArguments(event.ToolName, result.Arguments); err != nil {
					return results, fmt.Errorf("hook %d argument schema validation failed: %w", index+1, err)
				}
			}
			event.ToolArguments = result.Arguments
		}
		results = append(results, result)
	}
	return results, nil
}

func aggregatePreToolUse(results []Result) Result {
	// 决策优先级为 Deny > Ask > Allow > Continue，参数和消息采用最后一个非空结果。
	final := Result{Decision: DecisionContinue}
	for _, result := range results {
		if strings.TrimSpace(result.Arguments) != "" {
			final.Arguments = result.Arguments
		}
		if strings.TrimSpace(result.Message) != "" {
			final.Message = result.Message
		}

		switch normalizeDecision(result.Decision) {
		case DecisionDeny:
			final.Decision = DecisionDeny
			return final
		case DecisionAsk:
			final.Decision = DecisionAsk
		case DecisionAllow:
			if final.Decision == DecisionContinue {
				final.Decision = DecisionAllow
			}
		}
	}
	return final
}

type CommandHook struct {
	event       EventType
	command     string
	timeout     time.Duration
	workDir     string
	trustPolicy *trust.Policy
	request     trust.Request
}

type CommandOption func(*CommandHook)

func WithCommandTrustPolicy(policy *trust.Policy) CommandOption {
	return func(command *CommandHook) { command.trustPolicy = policy }
}

func NewCommandHook(config CommandHookConfig, workspace string, options ...CommandOption) (*CommandHook, error) {
	if config.Enabled != nil && !*config.Enabled {
		return nil, errors.New("hook is disabled")
	}
	event := normalizeEventType(EventType(config.Event))
	if event == "" {
		return nil, errors.New("hook event is empty")
	}
	command := strings.TrimSpace(config.Command)
	if command == "" {
		return nil, errors.New("hook command is empty")
	}

	timeout := 5 * time.Second
	if value := strings.TrimSpace(config.Timeout); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return nil, fmt.Errorf("parse hook timeout: %w", err)
		}
		timeout = parsed
	}
	if timeout <= 0 {
		return nil, errors.New("hook timeout must be positive")
	}

	workspace, err := filepath.Abs(strings.TrimSpace(workspace))
	if err != nil {
		return nil, fmt.Errorf("resolve hook workspace: %w", err)
	}
	workDir := strings.TrimSpace(config.WorkDir)
	if workDir == "" {
		workDir = workspace
	} else if !filepath.IsAbs(workDir) {
		workDir = filepath.Join(workspace, workDir)
	}
	workDir = filepath.Clean(workDir)
	request, err := trust.NewRequest(trust.KindHook, workspace, workspace, struct {
		Event   EventType `json:"event"`
		Command string    `json:"command"`
		Timeout string    `json:"timeout"`
		WorkDir string    `json:"work_dir"`
	}{event, command, timeout.String(), workDir})
	if err != nil {
		return nil, err
	}
	handler := &CommandHook{
		event: event, command: command, timeout: timeout, workDir: workDir, request: request,
	}
	for _, option := range options {
		if option != nil {
			option(handler)
		}
	}
	return handler, nil
}

// ReviewRequests returns exactly the effective declarations used for dispatch,
// without executing a command or granting it trust.
func ReviewRequests(config Config) ([]trust.Request, error) {
	var requests []trust.Request
	for index, command := range config.CommandHooks {
		if command.Enabled != nil && !*command.Enabled {
			continue
		}
		handler, err := NewCommandHook(command, config.Workspace)
		if err != nil {
			return nil, fmt.Errorf("configure command hook %d: %w", index+1, err)
		}
		requests = append(requests, handler.request)
	}
	return requests, nil
}

func (h *CommandHook) HandleHook(ctx context.Context, event Event) (Result, error) {
	if h == nil || h.event != normalizeEventType(event.Type) {
		return Result{}, nil
	}
	if err := h.trustPolicy.Authorize(ctx, h.request); err != nil {
		return Result{}, err
	}

	if h.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.timeout)
		defer cancel()
	}

	// 事件 JSON 通过 stdin 传给外部命令；命令可用 JSON stdout 返回决策和重写参数。
	payload, err := json.Marshal(event)
	if err != nil {
		return Result{}, err
	}

	cmd := hookCommand(ctx, h.command)
	cmd.Dir = h.workDir
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = processpolicy.Environment(nil)

	stdout, stderr := &limitedOutput{}, &limitedOutput{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err = processpolicy.Run(cmd)
	if ctx.Err() != nil {
		return Result{}, fmt.Errorf("hook command interrupted: %w", ctx.Err())
	}
	if err != nil {
		if exitCode(err) == 2 {
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = strings.TrimSpace(stdout.String())
			}
			if message == "" {
				message = "Hook denied this event (exit status 2)."
			}
			return Result{Decision: DecisionDeny, Message: message}, nil
		}
		return Result{}, fmt.Errorf("hook command failed: %w", err)
	}
	if stdout.overflow {
		return Result{}, errors.New("hook stdout exceeds size limit")
	}
	return parseCommandResult(stdout.String())
}

func normalizeEvent(event Event) Event {
	event.Type = normalizeEventType(event.Type)
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	return event
}

func normalizeEventType(event EventType) EventType {
	value := EventType(strings.ToLower(strings.TrimSpace(string(event))))
	switch value {
	case EventPreToolUse, EventPostToolUse, EventSessionChanged, EventSkillReloaded, EventSessionStart, EventUserPromptSubmit, EventStop:
		return value
	default:
		return ""
	}
}

func normalizeDecision(decision Decision) Decision {
	switch normalized := Decision(strings.ToLower(strings.TrimSpace(string(decision)))); normalized {
	case DecisionAllow, DecisionAsk, DecisionDeny, DecisionContinue:
		return normalized
	case "":
		return DecisionContinue
	default:
		return DecisionDeny
	}
}

func validateResult(result Result) (Result, error) {
	normalized := Decision(strings.ToLower(strings.TrimSpace(string(result.Decision))))
	switch normalized {
	case "", DecisionContinue, DecisionAllow, DecisionAsk, DecisionDeny:
		result.Decision = normalizeDecision(normalized)
	default:
		return Result{}, errors.New("hook returned an unknown decision")
	}
	if strings.TrimSpace(result.Arguments) != "" {
		var arguments map[string]json.RawMessage
		if err := json.Unmarshal([]byte(result.Arguments), &arguments); err != nil || arguments == nil {
			return Result{}, errors.New("hook arguments must be a JSON object")
		}
	}
	return result, nil
}

func parseCommandResult(output string) (Result, error) {
	text := strings.TrimSpace(strings.TrimPrefix(output, "\ufeff"))
	if text == "" {
		return Result{Decision: DecisionContinue}, nil
	}
	if !strings.HasPrefix(text, "{") {
		return Result{}, errors.New("hook stdout must contain a single JSON result object")
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	if _, err := decoder.Token(); err != nil {
		return Result{}, errors.New("invalid hook JSON result")
	}
	fields := make(map[string]string, 3)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return Result{}, errors.New("invalid hook JSON result")
		}
		name, ok := token.(string)
		if !ok || (name != "decision" && name != "message" && name != "arguments") {
			return Result{}, errors.New("hook JSON contains an unknown field")
		}
		if _, exists := fields[name]; exists {
			return Result{}, errors.New("hook JSON contains a duplicate field")
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return Result{}, errors.New("invalid hook JSON result")
		}
		valueString, ok := value.(string)
		if !ok {
			return Result{}, errors.New("hook JSON fields must be strings")
		}
		fields[name] = valueString
	}
	if _, err := decoder.Token(); err != nil {
		return Result{}, errors.New("invalid hook JSON result")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return Result{}, errors.New("hook stdout contains trailing output")
	}
	if decision, present := fields["decision"]; present && strings.TrimSpace(decision) == "" {
		return Result{}, errors.New("hook decision must not be empty")
	}
	return validateResult(Result{Decision: Decision(fields["decision"]), Message: fields["message"], Arguments: fields["arguments"]})
}

const maxHookOutputBytes = 1024 * 1024

type limitedOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

func (output *limitedOutput) String() string { return output.buffer.String() }

func (output *limitedOutput) Write(data []byte) (int, error) {
	count := len(data)
	remaining := maxHookOutputBytes - output.buffer.Len()
	if len(data) > remaining {
		data = data[:remaining]
		output.overflow = true
	}
	_, _ = output.buffer.Write(data)
	return count, nil
}

func hookCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		// PowerShell 5.1 otherwise uses the console code page for JSON stdin/stdout.
		preamble := "$utf8 = New-Object System.Text.UTF8Encoding($false); [Console]::InputEncoding = $utf8; [Console]::OutputEncoding = $utf8; $OutputEncoding = $utf8; "
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", preamble+command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
