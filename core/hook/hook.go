package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	EventPreToolUse     EventType = "pre_tool_use"
	EventPostToolUse    EventType = "post_tool_use"
	EventSessionChanged EventType = "session_changed"
	EventSkillReloaded  EventType = "skill_reloaded"
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
	mu       sync.RWMutex
	handlers []Handler
}

func NewManager(config Config) *Manager {
	manager := &Manager{}
	for _, command := range config.CommandHooks {
		handler, err := NewCommandHook(command, config.Workspace)
		if err == nil {
			manager.Register(handler)
		}
	}
	return manager
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
	for _, handler := range handlers {
		result, err := handler.HandleHook(ctx, event)
		if err != nil {
			return results, err
		}
		result.Decision = normalizeDecision(result.Decision)
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
			if final.Decision != DecisionAllow {
				final.Decision = DecisionAsk
			}
		case DecisionAllow:
			if final.Decision == DecisionContinue {
				final.Decision = DecisionAllow
			}
		}
	}
	return final
}

type CommandHook struct {
	event   EventType
	command string
	timeout time.Duration
	workDir string
}

func NewCommandHook(config CommandHookConfig, workspace string) (*CommandHook, error) {
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

	workDir := strings.TrimSpace(config.WorkDir)
	if workDir == "" {
		workDir = strings.TrimSpace(workspace)
	}

	return &CommandHook{
		event:   event,
		command: command,
		timeout: timeout,
		workDir: workDir,
	}, nil
}

func (h *CommandHook) HandleHook(ctx context.Context, event Event) (Result, error) {
	if h == nil || h.event != normalizeEventType(event.Type) {
		return Result{}, nil
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

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err = cmd.Run()
	text := strings.TrimSpace(output.String())
	if err != nil {
		if exitCode(err) == 2 {
			return Result{Decision: DecisionDeny, Message: text}, nil
		}
		return Result{}, fmt.Errorf("hook command failed: %w: %s", err, text)
	}
	if text == "" {
		return Result{}, nil
	}

	var result Result
	if json.Unmarshal([]byte(text), &result) == nil {
		result.Decision = normalizeDecision(result.Decision)
		return result, nil
	}
	return Result{Message: text}, nil
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
	case EventPreToolUse, EventPostToolUse, EventSessionChanged, EventSkillReloaded:
		return value
	default:
		return ""
	}
}

func normalizeDecision(decision Decision) Decision {
	switch Decision(strings.ToLower(strings.TrimSpace(string(decision)))) {
	case DecisionAllow:
		return DecisionAllow
	case DecisionAsk:
		return DecisionAsk
	case DecisionDeny:
		return DecisionDeny
	default:
		return DecisionContinue
	}
}

func hookCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
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
