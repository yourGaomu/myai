package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSessionOperationCoordinatorSerializesSameSession(t *testing.T) {
	coordinator := newSessionOperationCoordinator()
	firstUnlock, err := coordinator.lock(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func(), 1)
	go func() {
		unlock, lockErr := coordinator.lock(context.Background(), "session-1")
		if lockErr == nil {
			acquired <- unlock
		}
	}()

	select {
	case <-acquired:
		t.Fatal("second operation acquired the same session too early")
	case <-time.After(50 * time.Millisecond):
	}
	firstUnlock()
	select {
	case unlock := <-acquired:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("second operation did not acquire the released session")
	}
}

func TestSessionOperationCoordinatorAllowsDifferentSessionsAndCancellation(t *testing.T) {
	coordinator := newSessionOperationCoordinator()
	unlock, err := coordinator.lock(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	otherUnlock, err := coordinator.lock(context.Background(), "session-2")
	if err != nil {
		t.Fatal(err)
	}
	otherUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := coordinator.lock(ctx, "session-1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected canceled wait, got %v", err)
	}
}

// TestSessionOperationCoordinatorReentrantLockingWithContext 验证 context 标记重入锁不会发生自死锁。
// 1.1 外部首先成功获取 session-1 的锁；
// 1.2 将 session-1 标记到 context 中（模拟 SendMessageStream 传递给 Tool 的上下文）；
// 1.3 在相同的 context 下再次请求 session-1 锁，应当立即返回成功且不阻塞；
// 1.4 无标记的其它协程/context 依然被正常阻塞互斥。
func TestSessionOperationCoordinatorReentrantLockingWithContext(t *testing.T) {
	coordinator := newSessionOperationCoordinator()
	ctx := context.Background()

	// 1.1 首次获取锁
	firstUnlock, err := coordinator.lock(ctx, "session-1")
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	defer firstUnlock()

	// 1.2 将会话标记注入 context
	lockedCtx := withSessionOperationLocked(ctx, normalizeSessionOperationKey("session-1"))

	// 1.3 重入调用应当立即返回成功且不阻塞
	reentrantUnlock, err := coordinator.lock(lockedCtx, "session-1")
	if err != nil {
		t.Fatalf("reentrant lock failed: %v", err)
	}
	reentrantUnlock()

	// 1.4 无标记的 context 依然会被正常互斥阻挡
	timedCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := coordinator.lock(timedCtx, "session-1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded for un-marked context, got: %v", err)
	}
}
