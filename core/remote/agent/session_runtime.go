package agent

import (
	"context"
	"strings"
	"sync"
)

const defaultRuntimeSessionID = "__default__"

type sessionRuntime struct {
	// mu 保护读取和修改当前任务状态；它不应阻塞正在运行的任务。
	mu                 sync.Mutex
	cancelMu           sync.Mutex
	cancel             context.CancelFunc
	permMu             sync.Mutex
	allowedTools       map[string]bool
	allowedPermissions map[string]bool
}

type sessionRuntimeManager struct {
	mu       sync.Mutex
	sessions map[string]*sessionRuntime
}

func newSessionRuntimeManager() *sessionRuntimeManager {
	return &sessionRuntimeManager{
		sessions: make(map[string]*sessionRuntime),
	}
}

func (m *sessionRuntimeManager) get(sessionID string) *sessionRuntime {
	if sessionID == "" {
		sessionID = defaultRuntimeSessionID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	runtime := m.sessions[sessionID]
	if runtime == nil {
		runtime = &sessionRuntime{}
		m.sessions[sessionID] = runtime
	}
	return runtime
}

func (r *sessionRuntime) start(parent context.Context) (context.Context, context.CancelFunc, bool) {
	if r == nil {
		return parent, func() {}, false
	}
	ctx, cancel := context.WithCancel(parent)

	r.cancelMu.Lock()
	defer r.cancelMu.Unlock()

	// cancel 非空表示该 Session 已有任务运行，调用方应返回 busy 而不是并发启动。
	if r.cancel != nil {
		cancel()
		return ctx, cancel, false
	}

	r.cancel = cancel

	return ctx, cancel, true
}

func (r *sessionRuntime) finish(cancel context.CancelFunc) {
	if r == nil || cancel == nil {
		return
	}
	r.cancelMu.Lock()
	if r.cancel != nil {
		r.cancel = nil
	}
	r.cancelMu.Unlock()
	cancel()
}

func (r *sessionRuntime) pause() bool {
	// pause 只发出取消信号；业务服务检测 ctx.Done 后保存 canceled/paused 状态。
	if r == nil {
		return false
	}
	r.cancelMu.Lock()
	cancel := r.cancel
	r.cancelMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (r *sessionRuntime) allowPermissionRule(scope, toolName, permission string) {
	if r == nil {
		return
	}
	tool := strings.TrimSpace(toolName)
	perm := strings.TrimSpace(permission)
	r.permMu.Lock()
	defer r.permMu.Unlock()
	switch scope {
	case "session_tool":
		if tool != "" {
			if r.allowedTools == nil {
				r.allowedTools = make(map[string]bool)
			}
			r.allowedTools[tool] = true
		}
	case "session_permission":
		if perm != "" {
			if r.allowedPermissions == nil {
				r.allowedPermissions = make(map[string]bool)
			}
			r.allowedPermissions[perm] = true
		} else if tool != "" {
			if r.allowedTools == nil {
				r.allowedTools = make(map[string]bool)
			}
			r.allowedTools[tool] = true
		}
	}
}

func (r *sessionRuntime) isPermissionAllowed(toolName, permission string) bool {
	if r == nil {
		return false
	}
	tool := strings.TrimSpace(toolName)
	perm := strings.TrimSpace(permission)
	r.permMu.Lock()
	defer r.permMu.Unlock()
	if tool != "" && r.allowedTools[tool] {
		return true
	}
	if perm != "" && r.allowedPermissions[perm] {
		return true
	}
	return false
}

func (r *sessionRuntime) clearPermissionRules() {
	if r == nil {
		return
	}
	r.permMu.Lock()
	r.allowedTools = nil
	r.allowedPermissions = nil
	r.permMu.Unlock()
}
