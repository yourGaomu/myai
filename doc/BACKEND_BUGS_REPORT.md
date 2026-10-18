# MyAI 后端 Bug 和逻辑错误报告

> 生成时间: 2026-10-03
> 分析范围: Go 后端代码 (core/)
> 发现问题总数: 24

---

## 目录

- [高危问题 (5个)](#高危问题)
- [中危问题 (9个)](#中危问题)
- [低危问题 (10个)](#低危问题)
- [统计总结](#统计总结)

---

## 高危问题

### 🔴 1. Session Memory Store - 数据竞态

**文件**: `core/adapter/session/memory/store.go`  
**行号**: 508-527  
**问题类型**: 并发安全 - 数据竞态

**问题描述**:
```go
func (sm *SessionMemory) Current() *session.Session {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    
    if sm.currentSession == nil {
        return nil
    }
    return sm.currentSession  // 返回指针而非副本
}

func (sm *SessionMemory) GetSession(sessionID string) (*session.Session, error) {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    
    sess := sm.sessionByIDLocked(sessionID)
    if sess == nil {
        return nil, fmt.Errorf("session not found: %s", sessionID)
    }
    return sess, nil  // 返回 map 中的指针
}
```

`Current()` 和 `GetSession()` 方法返回的是存储在内存中的 Session 指针，而不是副本。调用方可能在锁外直接修改返回的 Session 对象，导致未同步的写入操作。

**潜在影响**: 
- 多个 goroutine 同时修改同一 Session 导致数据损坏
- 数据竞态检测器会报告 race condition
- 可能导致服务崩溃或状态不一致

**修复建议**: 
返回 Session 的深拷贝，或者改为返回只读视图/不可变对象。

---

### 🔴 2. Session Runtime - Nil 函数指针调用

**文件**: `core/remote/agent/session_runtime.go`  
**行号**: 64-74  
**问题类型**: 并发安全 - 竞态条件

**问题描述**:
```go
func (sr *SessionRuntime) finish() {
    sr.mu.Lock()
    cancel := sr.cancel
    sr.cancel = nil
    sr.mu.Unlock()  // 在此处释放锁
    
    if cancel != nil {
        cancel()  // 锁外调用，可能与 pause() 产生竞态
    }
}
```

`finish` 方法在第 68 行获取锁，将 `cancel` 赋值给局部变量并设置为 nil，然后在第 72 行释放锁。如果另一个 goroutine 在第 72-73 行之间调用 `pause` 方法，可能会访问已被设置为 nil 的 `cancel` 函数指针。

**潜在影响**: 
- Panic: 调用 nil 函数指针导致服务崩溃
- 会话状态不一致

**修复建议**: 
在锁内调用 `cancel()`，或者使用原子操作保护 cancel 函数的访问。

---

### 🔴 3. MongoDB Transaction - 会话泄漏

**文件**: `core/adapter/persistence/mongo/repository/transcript.go`  
**行号**: 34-75  
**问题类型**: 资源泄漏

**问题描述**:
```go
func (r *TranscriptRepository) ReplaceSessionTranscript(
    ctx context.Context,
    sessionID string,
    messages []message.Message,
) error {
    mongoSession, err := r.mongoClient.StartSession()
    if err != nil {
        return fmt.Errorf("start mongo session: %w", err)
    }
    defer mongoSession.EndSession(context.Background())  // 使用 Background context
    
    _, err = mongoSession.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
        // ... 事务操作
    })
    
    return err
}
```

defer 中使用 `context.Background()` 关闭 MongoDB session。如果原始的 `ctx` 已经被取消（例如客户端断开连接），`EndSession` 可能会挂起或超时，因为它使用的是不会被取消的 Background context。

**潜在影响**: 
- MongoDB 会话无法正确关闭
- 连接池资源耗尽
- 最终导致无法建立新的数据库连接

**修复建议**: 
使用带超时的 context，或者使用 `context.WithTimeout(context.Background(), 5*time.Second)`。

---

### 🔴 4. Plan Execution - Goroutine 泄漏

**文件**: `core/application/chat/plan/service/execution_service.go`  
**行号**: 310-326  
**问题类型**: 资源泄漏 - Goroutine 泄漏

**问题描述**:
```go
func (es *ExecutionService) executeReadyBatch(
    ctx context.Context,
    steps []plan.Step,
    // ...
) []stepExecutionResult {
    var waitGroup sync.WaitGroup
    // ...
    
    for _, step := range steps {
        waitGroup.Add(1)
        go func(step plan.Step) {  // 启动 goroutine
            defer waitGroup.Done()
            // ... 执行步骤
        }(step)
    }
    
    waitGroup.Wait()  // 等待所有 goroutine 完成
    return results
}
```

虽然个别任务执行有 panic 恢复机制，但如果 goroutine 在 `defer waitGroup.Done()` 之前 panic（例如在 `go func()` 和 `defer` 之间），`Done()` 不会被调用，导致 `waitGroup.Wait()` 永久阻塞。

**潜在影响**: 
- Goroutine 泄漏
- 内存泄漏
- Plan 执行永久挂起
- 最终导致服务资源耗尽

**修复建议**: 
在 `go func()` 的最外层立即添加 `defer waitGroup.Done()`，并在内部使用 `defer recover()` 捕获所有 panic。

---

### 🔴 5. Session Queue - 检查-执行竞态窗口

**文件**: `core/adapter/chat/generation/session_queue.go`  
**行号**: 42-65  
**问题类型**: 并发安全 - TOCTOU (Time-of-check to time-of-use)

**问题描述**:
```go
func (q *SessionQueue) Submit(sessionID string, task func() error) error {
    q.mu.Lock()
    
    queue := q.getOrCreateLocked(sessionID)
    if queue.running {
        q.mu.Unlock()
        return fmt.Errorf("session %s already has a running task", sessionID)
    }
    
    queue.running = true  // 标记为运行中
    q.mu.Unlock()  // 释放锁
    
    // 在锁外提交任务
    if err := q.Async.Submit(func(ctx context.Context) {
        // ... 执行任务
    }); err != nil {
        // 回滚逻辑
        q.mu.Lock()
        queue.running = false
        q.mu.Unlock()
        return fmt.Errorf("submit task: %w", err)
    }
    
    return nil
}
```

在 `q.mu.Unlock()` (第 56 行) 和 `q.Async.Submit()` (第 58 行) 之间存在竞态窗口。如果 `Async.Submit` 失败，队列状态已被修改为 `running = true`，但任务未实际提交。虽然有回滚逻辑（第 59-66 行），但在高并发场景下，其他 goroutine 可能已经观察到错误的 `queue.running = true` 状态并拒绝新任务。

**潜在影响**: 
- 队列永久标记为"运行中"但实际没有任务在执行
- 该 session 的后续所有任务被拒绝
- 用户消息无法处理

**修复建议**: 
在锁内调用 `Async.Submit`，或者先提交任务成功后再修改状态。

---

## 中危问题

### 🟡 6. Agent Loop Service - 错误被吞没

**文件**: `core/application/chat/generation/service/agent_loop_service.go`  
**行号**: 277-289  
**问题类型**: 错误处理

**问题描述**:
```go
func (s *AgentLoopService) compactIfNeeded(
    ctx context.Context,
    sess *session.Session,
    callback ProgressCallback,
) error {
    // ... 检查是否需要压缩
    
    if err := s.CompactionService.CompactContext(ctx, command); err != nil {
        s.Logger.Error("Auto compaction failed", "session_id", sess.ID, "error", err)
        if callback != nil {
            callback(ChatProgress{
                Type: "warning",
                Message: fmt.Sprintf("上下文压缩失败: %v", err),
            })
        }
        return nil  // 吞没错误，返回 nil
    }
    
    return nil
}
```

压缩失败的错误只通过回调报告给用户，然后返回 nil。这意味着压缩失败不会传播给调用者，可能导致在内存压力下继续执行，最终导致 OOM 或上下文窗口超限。

**潜在影响**: 
- 内存溢出
- 上下文窗口超限导致模型调用失败
- 错误被隐藏，难以调试

**修复建议**: 
返回错误给调用者，让调用者决定是否继续执行。

---

### 🟡 7. Task Service - Context 取消后清理失败

**文件**: `core/application/chat/generation/service/task_service.go`  
**行号**: 89, 200  
**问题类型**: 错误处理 - 上下文使用错误

**问题描述**:
```go
// 第 89 行
defer func() {
    cleanupCtx := context.WithoutCancel(ctx)
    // 使用 cleanupCtx 进行清理
}()

// 第 200 行
defer func() {
    ctx := context.Background()
    // 使用新的 Background context
}()
```

在 defer 中使用 `context.WithoutCancel(ctx)` 或 `context.Background()`。这意味着即使原始请求被取消（例如客户端断开连接），清理操作仍会尝试完成，但可能因为其他资源（如数据库连接、Redis 连接）已经关闭而失败。

**潜在影响**: 
- 资源清理失败
- 部分状态未持久化
- 数据不一致

**修复建议**: 
使用带超时的 context: `context.WithTimeout(context.Background(), 10*time.Second)`。

---

### 🟡 8. Chat Handlers - 入队错误处理不完整

**文件**: `core/remote/agent/chat_handlers.go`  
**行号**: 20-44  
**问题类型**: 错误处理

**问题描述**:
```go
func (a *Agent) enqueueRunningTurnInput(
    conn *connection,
    sessionID string,
    content string,
    messageID string,
) error {
    queue := a.pendingInputQueue
    if queue == nil {
        if err := a.writeRemoteMessage(conn, protocol.NewErrorMessage(
            "pending input queue not available",
        )); err != nil {
            a.logger.Error("Failed to write error", "error", err)
        }
        return fmt.Errorf("pending input queue not available")
    }
    
    // ... 多个错误路径都尝试 writeRemoteMessage
    // 如果 writeRemoteMessage 失败，只记录日志
}
```

方法在多个错误路径（第 23-43 行）中都尝试向连接写入错误消息，但如果 `writeRemoteMessage` 本身失败，只是记录日志而不传播错误。调用方无法知道入队是否真正成功。

**潜在影响**: 
- 客户端可能认为消息已入队，但实际未入队
- 消息丢失
- 用户体验差：发送消息无响应

**修复建议**: 
记录写入失败但仍然返回原始错误，或者使用更可靠的错误通知机制。

---

### 🟡 9. Session Persistence - 并发保存竞态

**文件**: `core/adapter/persistence/chatmessage/repository/writer.go`  
**行号**: 80-101  
**问题类型**: 数据一致性

**问题描述**:
```go
func (w *ChatMessageWriter) SaveAssistantMessage(
    ctx context.Context,
    sess *session.Session,
    msg *message.AssistantMessage,
) error {
    // 先保存消息
    if err := w.saveMessage(ctx, sess.ID, msg); err != nil {
        return fmt.Errorf("save assistant message: %w", err)
    }
    
    // 再保存 session
    if err := w.sessionRepo.Save(ctx, sess); err != nil {
        return fmt.Errorf("save session: %w", err)
    }
    
    return errors.Join(msgErr, sessErr)
}
```

先保存消息（第 91-93 行），然后保存 session（第 96-98 行）。如果在这两个操作之间发生崩溃或错误，会导致消息已持久化但 session 状态未更新。虽然返回 `errors.Join`（第 100 行），但调用方可能无法判断是哪个操作失败以及如何恢复。

**潜在影响**: 
- 数据库中的消息和 session 状态不一致
- Session.Messages 列表与实际持久化的消息不匹配
- 可能导致消息重复或丢失

**修复建议**: 
使用事务包装两个操作，或者设计补偿机制。

---

### 🟡 10. Pending Queue - 消息丢失风险

**文件**: `core/adapter/chat/pendinginput/queue.go`  
**行号**: 142-184  
**问题类型**: 数据一致性 - 消息丢失

**问题描述**:
```go
func (q *Queue) RequeueIdentified(
    sessionID string,
    items []IdentifiedPendingInput,
) error {
    q.mu.Lock()
    defer q.mu.Unlock()
    
    // ... 准备 requeue
    
    totalAfterRestore := restored + len(current.pending)
    if totalAfterRestore > q.MaxPendingMessages {
        return fmt.Errorf(
            "requeue would exceed limit: %d + %d > %d",
            restored, len(current.pending), q.MaxPendingMessages,
        )  // 返回错误但不保存任何内容
    }
    
    // ... 保存逻辑
}
```

在第 179 行检查总数是否超限。如果 `restored + current` 超过 `MaxPendingMessages`，直接返回错误且不保存任何内容（第 180 行）。这意味着如果因为队列满而无法 requeue，这些消息就彻底丢失了。

**潜在影响**: 
- 在高负载或长时间断线重连场景下消息丢失
- 用户的输入无法恢复
- 没有降级策略（例如丢弃最旧的消息）

**修复建议**: 
实现 FIFO 策略，丢弃最旧的消息为新消息腾出空间，或者持久化到磁盘。

---

### 🟡 11. Plan Execution - 并行失败处理不完整

**文件**: `core/application/chat/plan/service/execution_service.go`  
**行号**: 209-228  
**问题类型**: 逻辑错误 - 状态转换不完整

**问题描述**:
```go
func (es *ExecutionService) handleBatchFailures(
    results []stepExecutionResult,
    currentStep *int,
) bool {
    var firstFailure *stepExecutionResult
    terminalFailures := []stepExecutionResult{}
    
    for i := range results {
        if results[i].Err != nil {
            if firstFailure == nil {
                firstFailure = &results[i]
            }
            // ...
            terminalFailures = append(terminalFailures, results[i])
        }
    }
    
    if firstFailure != nil {
        *currentStep = firstFailure.Index  // 只设置第一个失败步骤
        // ... 标记所有 terminal 失败
        return true
    }
    
    return false
}
```

当多个并行步骤失败时（第 209 行），代码将所有失败步骤标记为 terminal（第 220-224 行），但 `currentStep` 变量（用于报告进度）只设置为 `firstFailure.Index`（第 212 行）。这意味着如果步骤 3、5、7 同时失败，进度只会显示到步骤 3。

**潜在影响**: 
- 用户界面显示不准确的进度信息
- 重试逻辑可能基于错误的当前步骤
- 难以理解执行失败的全貌

**修复建议**: 
返回所有失败步骤的信息，或者使用更复杂的进度模型。

---

### 🟡 12. Agent Loop - Pending Input 处理时机问题

**文件**: `core/application/chat/generation/service/agent_loop_service.go`  
**行号**: 70-73, 134  
**问题类型**: 逻辑错误

**问题描述**:
```go
func (s *AgentLoopService) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
    // ...
    canDrainPending := false  // 初始为 false
    
    for iteration := 0; iteration < maxIterations; iteration++ {
        // 第一轮不会 drain
        if canDrainPending && s.PendingInputQueue != nil {
            // drain pending input
        }
        
        // ... 模型生成
        
        canDrainPending = true  // 从第二轮开始才设置为 true
    }
    
    // 达到上限后再次 drain
    if s.PendingInputQueue != nil {
        // drain pending input again
    }
}
```

`canDrainPending` 在第 91 行设置为 true，意味着从第二轮开始才会 drain pending input（第 71-73 行）。但在达到工具轮数上限后（第 134 行），再次 drain pending input。这意味着如果第一轮就达到上限，pending input 会在未进入循环的情况下被 drain，时机不确定。

**潜在影响**: 
- Pending 消息处理时机不确定
- 可能丢失或重复处理消息
- 逻辑难以理解和维护

**修复建议**: 
明确定义 pending input 的处理策略和时机。

---

### 🟡 13. Agent Run Repository - 事件序列号竞态

**文件**: `core/adapter/persistence/mongo/agentrun/repository/repository.go`  
**行号**: 79-94  
**问题类型**: 数据一致性

**问题描述**:
```go
func (r *AgentRunRepository) NextEventSequence(ctx context.Context, runID string) (int, error) {
    // ...
    
    result := r.database.Collection(r.CollectionName).FindOneAndUpdate(
        ctx,
        bson.M{"_id": runID},
        bson.M{"$inc": bson.M{"event_sequence": 1}},
        options.FindOneAndUpdate().SetReturnDocument(options.After),
    )
    
    // ...
    return doc.EventSequence, nil
}
```

`NextEventSequence` 使用 `FindOneAndUpdate` 原子递增序列号（第 84-89 行）。这确保了序列号本身不会重复，但如果两个 goroutine 同时为同一个 runID 调用此方法并获取序列号 N 和 N+1，然后第二个 goroutine 先调用 `SaveEvent` 保存序列号 N+1 的事件，而第一个 goroutine 后调用保存序列号 N 的事件，最终数据库中的事件顺序会被打乱。

**潜在影响**: 
- 事件时间线顺序错误
- 影响 Agent Run 的重放和调试
- 用户看到的执行历史不准确

**修复建议**: 
在事件记录中额外保存时间戳，并在查询时按时间戳排序；或者使用单线程写入。

---

### 🟡 14. Pending Input Queue - 重复检查逻辑缺陷

**文件**: `core/adapter/chat/pendinginput/queue.go`  
**行号**: 90-103  
**问题类型**: 逻辑错误

**问题描述**:
```go
func (q *Queue) Append(sessionID string, content string, messageID string) error {
    q.mu.Lock()
    defer q.mu.Unlock()
    
    queue := q.getOrCreateLocked(sessionID)
    
    // 重复检查
    if messageID == "" {
        // 如果 messageID 为空，跳过重复检查
    } else {
        for _, item := range queue.pending {
            if item.MessageID == messageID {
                return nil  // 已存在，忽略
            }
        }
    }
    
    // 追加到队列
    queue.pending = append(queue.pending, PendingInput{
        Content:   content,
        MessageID: messageID,
    })
    
    return nil
}
```

在第 92-96 行检查重复 ID，然后在第 102 行追加。虽然有锁保护，但逻辑本身存在问题：如果 `messageID` 为空（第 93 行条件为真），会跳过重复检查继续追加，可能导致相同内容的多个无 ID 条目。

**潜在影响**: 
- 队列包含重复消息
- 用户输入被多次处理
- 业务逻辑错误

**修复建议**: 
对于空 messageID 的情况，使用内容哈希或生成唯一 ID 进行重复检查。

---

## 低危问题

### 🟢 15. Agent Run Repository - Database Nil 检查不足

**文件**: `core/adapter/persistence/mongo/agentrun/repository/repository.go`  
**行号**: 79-94  
**问题类型**: 空指针风险

**问题描述**:
在 `NextEventSequence` 方法中，第 80-82 行检查 `r.database == nil`，但在第 84 行直接访问 `r.database.Collection`。如果在检查和访问之间 `r.database` 被设置为 nil（理论上不应该发生，但缺乏并发保护），会 panic。

**潜在影响**: 服务崩溃

**修复建议**: 将 database 的空检查移到锁保护范围内，或者在初始化后设为只读。

---

### 🟢 16. Session Memory Store - Map 未初始化检查

**文件**: `core/adapter/session/memory/store.go`  
**行号**: 596-606  
**问题类型**: 空指针风险

**问题描述**:
`sessionByIDLocked` 方法在第 601 行直接访问 `sm.session[sessionID]`，假设 map 已初始化。虽然构造函数确实初始化了 map（第 52 行），但如果通过其他方式创建 Store（如零值），会 panic。

**潜在影响**: 服务崩溃

**修复建议**: 添加 nil 检查或使用构造函数强制初始化。

---

### 🟢 17. Persistence Service - Nil Session 未检查

**文件**: `core/application/session/persistence/service/persistence_service.go`  
**行号**: 104-193  
**问题类型**: 空指针风险

**问题描述**:
`BuildSessionRecord` 函数在第 123 行访问 `current.ID`，但实际的 nil 检查逻辑复杂且可能遗漏边界情况。如果 `current == nil` 且 `command.Current == nil`，会在第 123 行之前尝试访问 nil 指针。

**潜在影响**: 服务崩溃

**修复建议**: 提前进行清晰的 nil 检查，明确所有输入参数的约束。

---

### 🟢 18. Session Queue - SubmitAndWait 取消不生效

**文件**: `core/adapter/chat/generation/session_queue.go`  
**行号**: 71-98  
**问题类型**: 逻辑错误

**问题描述**:
```go
func (q *SessionQueue) SubmitAndWait(sessionID string, task func() error) error {
    result := make(chan error, 1)
    
    // ...
    
    // 注释说明不能返回 ctx.Err()
    // 但这意味着即使上下文取消，也会一直等待
    select {
    case err := <-result:
        return err
    }
}
```

`SubmitAndWait` 在第 97 行阻塞等待 result channel。注释提到不能返回 `ctx.Err()`（第 94-97 行），意味着即使上下文取消，也会一直等待。虽然 channel 是 buffered 容量为 1（第 81 行），实际是安全的，但在异常情况下调用方可能无限期阻塞。

**潜在影响**: 在异常情况下，调用方无限期阻塞

**修复建议**: 添加 context.Done() 分支，在超时后返回错误。

---

### 🟢 19. Chat Handlers - Send Error Channel 溢出

**文件**: `core/remote/agent/chat_handlers.go`  
**行号**: 140-146  
**问题类型**: 错误丢失

**问题描述**:
```go
sendErrCh := make(chan error, 1)

go func() {
    select {
    case sendErrCh <- err:
    default:
        // 第二个错误会被默默丢弃
    }
}()
```

`sendErrCh` 是容量为 1 的 buffered channel（第 140 行）。在并发发送多个消息时（第 142-147 行），第二个错误会因为 channel 满而被 default 分支丢弃。

**潜在影响**: 错误可能被默默丢弃，调试困难

**修复建议**: 使用更大容量的 channel 或者使用错误列表。

---

### 🟢 20. Plan Execution - Context.WithoutCancel 挂起风险

**文件**: `core/application/chat/plan/service/execution_service.go`  
**行号**: 126, 599  
**问题类型**: 资源清理问题

**问题描述**:
```go
defer func() {
    cleanupCtx := context.WithoutCancel(ctx)
    // 确保 cleanup 完成
    es.Finish(cleanupCtx, sessionID, planID)
}()
```

在 defer 中使用 `context.WithoutCancel`（第 126 行）确保 cleanup 完成，但这意味着即使用户取消操作，Finish 调用仍会尝试完成所有 I/O。在高延迟网络环境或数据库响应慢的情况下，可能导致关闭服务时挂起。

**潜在影响**: 服务关闭缓慢或挂起

**修复建议**: 使用带短超时的 context。

---

### 🟢 21-24. 其他低危问题

以下问题影响较小，可在日常维护中逐步修复：

- **21**: Permission Waiters 超时处理不完整
- **22**: Model Generation Context 构建冗余
- **23**: LLM Client Pool 资源管理
- **24**: Workspace History 快照清理策略

---

## 统计总结

| 严重程度 | 数量 | 占比 |
|---------|------|------|
| 🔴 高危 | 5 | 20.8% |
| 🟡 中危 | 9 | 37.5% |
| 🟢 低危 | 10 | 41.7% |
| **总计** | **24** | **100%** |

### 问题分类统计

| 类别 | 数量 |
|------|------|
| 并发安全问题 | 4 |
| 错误处理问题 | 3 |
| 空指针问题 | 3 |
| 逻辑错误 | 4 |
| 数据一致性问题 | 4 |
| 资源管理问题 | 4 |
| 其他问题 | 2 |

---

## 修复优先级建议

### 第一优先级（立即修复）
1. **问题 #1**: Session Memory Store 数据竞态
2. **问题 #2**: Session Runtime nil 调用
3. **问题 #3**: MongoDB 会话泄漏
4. **问题 #4**: Plan execution goroutine 泄漏
5. **问题 #5**: Session queue 竞态条件

### 第二优先级（本周内修复）
6. **问题 #6**: Agent Loop 错误被吞没
7. **问题 #9**: Session Persistence 竞态
8. **问题 #10**: Pending Queue 消息丢失
9. **问题 #13**: Agent Run 序列号竞态

### 第三优先级（长期优化）
- 其余低危问题可在日常迭代中逐步修复

---

## 附录：测试建议

为了防止这些 bug，建议增加以下测试：

1. **并发测试**: 使用 `-race` flag 运行所有测试
2. **压力测试**: 模拟高并发场景，测试队列和锁机制
3. **故障注入**: 测试数据库连接失败、超时等异常情况
4. **资源泄漏检测**: 使用 pprof 监控 goroutine 和内存泄漏
5. **集成测试**: 测试 Plan 执行、消息持久化等端到端流程

---

*此报告由自动化分析工具生成，建议结合人工 Code Review 进行验证。*
