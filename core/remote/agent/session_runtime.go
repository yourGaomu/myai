package agent

import (
	"context"
	"sync"
)

const defaultRuntimeSessionID = "__default__"

type sessionRuntime struct {
	// mu 串行化同一会话的任务，cancelMu 只保护当前任务的取消函数。
	mu       sync.Mutex
	cancelMu sync.Mutex
	cancel   context.CancelFunc
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
	// cancel 非空表示该 Session 已有任务运行，调用方应返回 busy 而不是并发启动。
	if r.cancel != nil {
		r.cancelMu.Unlock()
		cancel()
		return ctx, cancel, false
	}
	r.cancel = cancel
	r.cancelMu.Unlock()

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
