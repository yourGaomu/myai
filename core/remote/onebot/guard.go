package onebot

import (
	"context"
	"strings"
	"sync"
)

// MaxPendingQueueSize 定义单个会话在推理期间最多允许排队等待的消息数量，防止恶意刷屏撑爆内存。
const MaxPendingQueueSize = 16

// GuardResult 表示消息经过 SessionGuard 调度后的状态结果。
// 1.1 GuardResultExecuted：当前会话空闲，已直接进入执行流程；
// 1.2 GuardResultQueued：当前会话正在推理中，新消息已进入 Pending 队列等待依次处理；
// 1.3 GuardResultQueueFull：当前会话 Pending 队列已满，新消息被丢弃；
// 1.4 GuardResultCancelled：收到取消指令，已中断当前正在执行的推理任务并清空等待队列；
// 1.5 GuardResultNoActiveTask：收到取消指令，但当前会话没有正在运行的任务。
type GuardResult string

const (
	GuardResultExecuted     GuardResult = "executed"
	GuardResultQueued       GuardResult = "queued"
	GuardResultQueueFull    GuardResult = "queue_full"
	GuardResultCancelled    GuardResult = "cancelled"
	GuardResultNoActiveTask GuardResult = "no_active_task"
)

// QueuedTask 封装一个等待在会话队列中执行的推理闭包及其原始文本。
type QueuedTask struct {
	Text string
	Run  func(ctx context.Context)
}

// sessionSlot 维护单个会话键（如 group:12345 或 private:67890）的并发状态与等待队列。
type sessionSlot struct {
	busy   bool
	cancel context.CancelFunc
	queue  []QueuedTask
}

// SessionGuard 实现推理期并发保护机制（参考 Hermes Agent 的 Active Session Guard）。
// 2.1 保证同一会话在同一时刻最多只有一个 LLM 推理/工具调用在执行，杜绝上下文穿插；
// 2.2 推理期间到达的新消息自动进入内存 Pending 队列，待当前轮次完成后按 FIFO 顺序自动消费；
// 2.3 支持用户发送“停 / 取消 / 算了 / /cancel”实时中断正在挂起的推理协程。
type SessionGuard struct {
	mu    sync.Mutex
	slots map[string]*sessionSlot
}

// NewSessionGuard 创建并初始化一个并发会话看门狗实例。
// 1.1 初始化内部的会话槽位映射表 slots。
func NewSessionGuard() *SessionGuard {
	return &SessionGuard{
		slots: make(map[string]*sessionSlot),
	}
}

// IsCancelCommand 判断用户输入的纯文本是否为中断/取消当前推理的控制指令。
// 1.1 清理首尾空白并转为小写；
// 1.2 匹配中英文常见取消口令（"停"、"停止"、"取消"、"算了"、"打住"、"/cancel"、"stop"、"cancel"）。
func IsCancelCommand(text string) bool {
	cleaned := strings.ToLower(strings.TrimSpace(text))
	switch cleaned {
	case "停", "停止", "取消", "算了", "打住", "/cancel", "/stop", "stop", "cancel":
		return true
	default:
		return false
	}
}

// Submit 向指定 sessionKey 提交一个推理任务或取消指令。
// 3.1 加互斥锁获取或创建该 sessionKey 对应的 sessionSlot；
// 3.2 若输入文本匹配取消指令：
//   - 若当前有任务在运行，则调用 cancel() 终止推理并清空排队队列，返回 GuardResultCancelled；
//   - 若当前无任务运行，则返回 GuardResultNoActiveTask；
// 3.3 若当前会话正忙（slot.busy == true）：
//   - 检查队列长度是否达到 MaxPendingQueueSize，若已满则返回 GuardResultQueueFull；
//   - 否则将任务追加至 slot.queue 尾部，返回 GuardResultQueued；
// 3.4 若当前会话空闲（slot.busy == false）：
//   - 标记 slot.busy = true，并在当前协程（或调用方协程）中循环执行当前任务与后续排队任务。
func (g *SessionGuard) Submit(parentCtx context.Context, sessionKey string, text string, run func(ctx context.Context)) GuardResult {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	key := strings.TrimSpace(sessionKey)
	if key == "" {
		key = "default"
	}

	// 3.1 加锁获取或初始化会话槽位
	g.mu.Lock()
	slot, ok := g.slots[key]
	if !ok {
		slot = &sessionSlot{}
		g.slots[key] = slot
	}

	// 3.2 判断是否为取消指令
	if IsCancelCommand(text) {
		if !slot.busy {
			g.mu.Unlock()
			return GuardResultNoActiveTask
		}
		cancel := slot.cancel
		slot.queue = nil
		g.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return GuardResultCancelled
	}

	// 3.3 若当前会话正忙，则进入 Pending 队列等待
	if slot.busy {
		if len(slot.queue) >= MaxPendingQueueSize {
			g.mu.Unlock()
			return GuardResultQueueFull
		}
		slot.queue = append(slot.queue, QueuedTask{Text: text, Run: run})
		g.mu.Unlock()
		return GuardResultQueued
	}

	// 3.4 当前会话空闲，占用槽位并开始执行
	slot.busy = true
	runCtx, cancel := context.WithCancel(parentCtx)
	slot.cancel = cancel
	g.mu.Unlock()

	g.drainLoop(parentCtx, key, runCtx, cancel, run)
	return GuardResultExecuted
}

// drainLoop 顺序执行首个任务，并在完成后循环检查并消费队列中积压的后续任务。
// 4.1 执行当前任务的闭包函数，并在结束后释放对应的 cancel 句柄；
// 4.2 重新加锁检查 slot.queue 是否还有待处理任务：
//   - 若队列为空或父级 Context 已关闭，则将 slot.busy 置为 false 并清理空闲槽位后退出；
//   - 若队列非空，则弹出队首任务、创建新的可取消 Context 并继续下一轮执行。
func (g *SessionGuard) drainLoop(parentCtx context.Context, key string, initialCtx context.Context, initialCancel context.CancelFunc, initialRun func(ctx context.Context)) {
	currentCtx := initialCtx
	currentCancel := initialCancel
	currentRun := initialRun

	for {
		// 4.1 执行当前轮次的推理任务
		if currentRun != nil {
			currentRun(currentCtx)
		}
		if currentCancel != nil {
			currentCancel()
		}

		// 4.2 检查队列是否还有等待中的后续任务
		g.mu.Lock()
		slot, exists := g.slots[key]
		if !exists || len(slot.queue) == 0 || parentCtx.Err() != nil {
			if exists {
				slot.busy = false
				slot.cancel = nil
				slot.queue = nil
				delete(g.slots, key)
			}
			g.mu.Unlock()
			return
		}

		next := slot.queue[0]
		slot.queue = slot.queue[1:]
		currentCtx, currentCancel = context.WithCancel(parentCtx)
		slot.cancel = currentCancel
		currentRun = next.Run
		g.mu.Unlock()
	}
}

// IsBusy 查询指定 sessionKey 当前是否处于推理繁忙状态。
// 5.1 加锁读取对应槽位的 busy 标志。
func (g *SessionGuard) IsBusy(sessionKey string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	slot, ok := g.slots[strings.TrimSpace(sessionKey)]
	return ok && slot.busy
}

// PendingCount 查询指定 sessionKey 当前排队等待的消息数量。
// 5.1 加锁读取对应槽位 queue 的长度。
func (g *SessionGuard) PendingCount(sessionKey string) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	slot, ok := g.slots[strings.TrimSpace(sessionKey)]
	if !ok {
		return 0
	}
	return len(slot.queue)
}
