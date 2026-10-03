package service

import (
	"context"
	"strings"
	"sync"
)

const globalSessionOperationKey = "__global__"

// lockedSessionContextKey 用于在 context.Context 中追踪当前协程已持有的会话锁集合。
type lockedSessionContextKey struct{}

// normalizeSessionOperationKey 规范化会话键，空字符串回退为全局锁键。
// 1.1 去除首尾空白；
// 1.2 若为空则返回 globalSessionOperationKey。
func normalizeSessionOperationKey(sessionID string) string {
	key := strings.TrimSpace(sessionID)
	if key == "" {
		return globalSessionOperationKey
	}
	return key
}

// withSessionOperationLocked 将指定会话键标记为当前 Context 已持锁。
// 2.1 浅拷贝既有已锁集合并添加当前键；
// 2.2 返回携带新集合的子 Context。
func withSessionOperationLocked(ctx context.Context, key string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	existing, _ := ctx.Value(lockedSessionContextKey{}).(map[string]bool)
	m := make(map[string]bool, len(existing)+1)
	for k, v := range existing {
		m[k] = v
	}
	m[key] = true
	return context.WithValue(ctx, lockedSessionContextKey{}, m)
}

// isSessionOperationLocked 检查当前 Context 是否在调用链上游已持有指定会话的操作锁。
// 3.1 从 Context 提取已锁集合并判定 key 是否存在。
func isSessionOperationLocked(ctx context.Context, key string) bool {
	if ctx == nil {
		return false
	}
	m, _ := ctx.Value(lockedSessionContextKey{}).(map[string]bool)
	return m != nil && m[key]
}

type sessionOperationCoordinator struct {
	mu      sync.Mutex
	entries map[string]*sessionOperationEntry
}

type sessionOperationEntry struct {
	gate chan struct{}
	refs int
}

func newSessionOperationCoordinator() *sessionOperationCoordinator {
	return &sessionOperationCoordinator{entries: make(map[string]*sessionOperationEntry)}
}

// lock 对指定 sessionID 申请操作锁。
// 4.1 规范化 sessionID 键；
// 4.2 若当前 context 已持有该会话的操作锁（如工具调用在生成流程内回调），直接返回空操作解锁闭包，避免自我死锁；
// 4.3 否则按信号量排队等待获取互斥通行令牌。
func (coordinator *sessionOperationCoordinator) lock(ctx context.Context, sessionID string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	key := normalizeSessionOperationKey(sessionID)

	// 4.2 若在同一调用树上游已持有该会话锁，作为可重入锁放行
	if isSessionOperationLocked(ctx, key) {
		return func() {}, nil
	}

	coordinator.mu.Lock()
	entry := coordinator.entries[key]
	if entry == nil {
		entry = &sessionOperationEntry{gate: make(chan struct{}, 1)}
		entry.gate <- struct{}{}
		coordinator.entries[key] = entry
	}
	entry.refs++
	coordinator.mu.Unlock()

	select {
	case <-ctx.Done():
		coordinator.releaseReference(key, entry)
		return nil, ctx.Err()
	case <-entry.gate:
		var once sync.Once
		return func() {
			once.Do(func() {
				entry.gate <- struct{}{}
				coordinator.releaseReference(key, entry)
			})
		}, nil
	}
}

func (coordinator *sessionOperationCoordinator) releaseReference(key string, entry *sessionOperationEntry) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && coordinator.entries[key] == entry {
		delete(coordinator.entries, key)
	}
}
