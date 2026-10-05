# MyAI 前端 Bug 和逻辑错误报告

> 生成时间: 2026-10-03
> 分析范围: React Native 前端代码 (mobile/)
> 发现问题总数: 18

---

## 目录

- [高危问题 (3个)](#高危问题)
- [中危问题 (7个)](#中危问题)
- [低危问题 (8个)](#低危问题)
- [统计总结](#统计总结)

---

## 高危问题

### 🔴 1. WebSocket 连接竞态条件

**文件**: `mobile/src/hooks/useRelayConnection.ts`  
**行号**: 228-238  
**问题类型**: 并发安全 - 竞态条件

**问题描述**:
```typescript
const connect = useCallback(() => {
  const previousSocket = socketRef.current;
  if (previousSocket) {
    if (previousSocket.readyState === WebSocket.OPEN) {
      return;  // 已连接，直接返回
    }
    if (previousSocket.readyState === WebSocket.CONNECTING) {
      return;  // 正在连接，直接返回
    }
  }
  
  // 创建新连接
  const socket = new WebSocket(url);
  socketRef.current = socket;
  // ...
}, [url]);
```

在检查 `readyState` 和实际使用 socket 之间，WebSocket 的状态可能已经改变。没有锁机制保护并发连接请求。如果用户快速切换网络或多次点击连接按钮，可能导致多个 WebSocket 连接同时存在。

**潜在影响**: 
- 多个 WebSocket 连接同时存在
- 消息路由混乱
- 内存泄漏
- 服务端资源浪费

**修复建议**: 
使用 ref 记录连接状态，添加连接锁或使用状态机严格控制连接生命周期。

---

### 🔴 2. Agent Run 事件顺序错乱

**文件**: `mobile/src/hooks/useAgentRunState.ts`  
**行号**: 125-150  
**问题类型**: 数据一致性 - 竞态条件

**问题描述**:
```typescript
function upsertEvent(snapshots: AgentRunSnapshot[], event: AgentRunEvent) {
  const runIndex = snapshots.findIndex((snapshot) => snapshot.run.id === event.run_id);
  
  const base = runIndex >= 0
    ? snapshots[runIndex]
    : { run: placeholderRun(event), events: [] };  // 创建 placeholder
  
  // 追加事件
  const updatedEvents = [...base.events, event];
  
  // ...
}
```

当 `agent_run_event` 在 `agent_run_started` 之前到达时（网络延迟或消息乱序），会创建 placeholder run。但如果后续 `agent_run_started` 到达，现有的 `upsertRun` 逻辑可能会：
1. 直接替换 placeholder，导致已接收的事件丢失
2. 或者保留 placeholder 的事件但 run 信息不完整

**潜在影响**: 
- Agent run 事件丢失
- 事件顺序错乱
- UI 显示不完整的执行状态
- 用户看不到子任务的完整生命周期

**修复建议**: 
在 `upsertRun` 中合并 placeholder 的事件到新的 run，或者使用事件序列号排序和去重。

---

### 🔴 3. 协议消息类型处理遗漏

**文件**: `mobile/src/hooks/useRemoteMessageHandler.ts`  
**行号**: 298-816  
**问题类型**: 功能缺失 - 消息类型处理不完整

**问题描述**:
`protocol.ts` 定义了 168 种消息类型，但 `useRemoteMessageHandler` 的 switch 语句只处理了约 70 种。大量消息类型缺失处理逻辑，这些未处理的消息会进入 default 分支，仅记录日志后被忽略。

**缺失的关键消息类型**:
```typescript
// 用户操作相关
"user_message"              // 第 4 行 - 用户发送消息
"session_new"               // 第 18 行 - 新建会话
"session_load"              // 第 19 行 - 加载会话
"session_delete"            // 第 20 行 - 删除会话

// 模型配置相关
"model_config_add"          // 第 59 行 - 添加模型配置
"model_config_update"       // 第 63 行 - 更新模型配置
"model_config_delete"       // 第 64 行 - 删除模型配置

// 文件操作相关
"file_read"                 // 第 92 行 - 读取文件
"file_write"                // 第 93 行 - 写入文件
"file_list"                 // 第 94 行 - 列出文件

// 权限相关
"permission_ask"            // 第 106 行 - 请求权限
"permission_result"         // 第 107 行 - 权限结果
```

**潜在影响**: 
- 部分核心功能完全不可用
- 用户操作没有响应
- 服务端发送的通知被忽略
- 功能表现与后端实现不一致

**修复建议**: 
1. 遍历 `protocol.ts` 中的所有消息类型
2. 为每种类型添加对应的处理逻辑或明确标记为"不需要前端处理"
3. 添加单元测试验证所有协议消息都有对应的 handler

---

## 中危问题

### 🟡 4. 心跳定时器内存泄漏

**文件**: `mobile/src/hooks/useRelayConnection.ts`  
**行号**: 140-158  
**问题类型**: 内存泄漏 - useEffect 清理函数缺失

**问题描述**:
```typescript
useEffect(() => {
  // ... 设置心跳定时器
  heartbeatTimerRef.current = setInterval(() => {
    if (socketRef.current?.readyState === WebSocket.OPEN) {
      sendPing();
    }
  }, heartbeatIntervalMs);
  
  return () => {
    if (heartbeatTimerRef.current) {
      clearInterval(heartbeatTimerRef.current);
      heartbeatTimerRef.current = null;
    }
    // ... 其他清理
  };
}, [socketRef]);  // 依赖项只有 socketRef
```

清理函数的依赖项只有 `socketRef`，但函数内部使用了 `heartbeatTimerRef` 和 `sendPing`。当其他相关状态变化时（如 `heartbeatIntervalMs`），旧的 timer 可能不会被清理，而是创建了新的 timer。

**潜在影响**: 
- 多个心跳定时器同时运行
- 内存泄漏
- 组件卸载后仍发送心跳包
- 不必要的网络流量

**修复建议**: 
将 `heartbeatIntervalMs` 和其他相关依赖添加到依赖数组，或者使用 ref 稳定化依赖。

---

### 🟡 5. 流式消息关联错误（过时闭包）

**文件**: `mobile/src/hooks/useChatMessages.ts`  
**行号**: 606-621  
**问题类型**: 逻辑错误 - 过时闭包

**问题描述**:
```typescript
function findAssistantID(current: SessionChatState, requestID?: string) {
  if (!requestID) {
    return "";
  }
  
  // 从后往前查找
  for (let index = current.messages.length - 1; index >= 0; index -= 1) {
    const message = current.messages[index];
    
    // 遇到工具调用、工具结果或用户消息时停止
    if (message.role === "tool_call" || message.role === "tool" || message.role === "user") {
      return "";  // 立即返回空字符串
    }
    
    if (message.role === "assistant" && message.requestID === requestID) {
      return message.id;
    }
  }
  
  return "";
}
```

该函数在循环中遇到 `tool_call`、`tool` 或 `user` 消息时立即返回空字符串。但在流式消息场景下，可能存在以下序列：

```
assistant (requestID: req-1)
tool_call (requestID: req-1)
tool (requestID: req-1)
assistant (requestID: req-1)  // 工具调用后继续回答
```

这种情况下，查找第二个 assistant 消息时会因为遇到 tool 消息而返回空字符串，导致后续流式内容无法正确追加到该 assistant 消息。

**潜在影响**: 
- 工具调用后的流式回复创建新消息而不是追加
- 消息列表中出现多个相同 requestID 的 assistant 消息
- 用户体验差：消息显示断开

**修复建议**: 
改进查找逻辑，允许跨越工具调用序列继续查找 assistant 消息，或者使用更明确的消息关联机制。

---

### 🟡 6. Promise 错误处理不完整

**文件**: `mobile/src/hooks/useRemoteRequests.ts`  
**行号**: 216-234  
**问题类型**: 异步处理 - Promise 未正确处理

**问题描述**:
```typescript
void loadCachedSessionHistory(targetSessionID)
  .then(({ meta }) => {
    if (!sendEnvelope("session_history_delta", { ... })) {
      pendingHistorySessionIDRef.current = "";
      stopPending("sessions");
    }
  })
  .catch(() => {
    requestSessionHistoryFull(targetSessionID);  // 返回 boolean
  });
```

使用 `void` 关键字显式忽略 Promise 的返回值。在 catch 块中调用 `requestSessionHistoryFull`，该函数返回 boolean 表示是否成功发送请求，但这个返回值也被忽略了。如果降级到完整历史加载也失败，用户会一直看到加载中状态。

**潜在影响**: 
- 历史消息加载失败时状态不一致
- 用户看到永久的加载中状态
- 没有错误提示
- 难以调试

**修复建议**: 
在所有分支都失败时设置错误状态并清理 pending 状态。

---

### 🟡 7. AppState 监听器可能在组件卸载后执行

**文件**: `mobile/src/screens/MobileAppScreen.tsx`  
**行号**: 841-886  
**问题类型**: 内存泄漏 - 事件监听器清理不完整

**问题描述**:
```typescript
useEffect(() => {
  const subscription = AppState.addEventListener("change", (nextState) => {
    // ...
    
    if (returningToForeground && reconnectOnForegroundRef.current && clientToken) {
      // 延迟重连
      reconnectTimerRef.current = setTimeout(() => {
        reconnectTimerRef.current = null;
        connectRef.current();  // 调用连接函数
      }, 300);
    }
  });
  
  return () => {
    subscription.remove();
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
    }
  };
}, [clientToken, deviceID, /* ... */]);
```

cleanup 函数中清理了 timer，但如果 timer 在 cleanup 执行后、实际触发前已经被设置，或者在 timer 回调执行期间组件卸载，`connectRef.current()` 仍会被调用。

**潜在影响**: 
- 组件卸载后仍尝试建立 WebSocket 连接
- 可能导致内存泄漏
- 状态更新到已卸载的组件

**修复建议**: 
在 timer 回调中检查组件是否仍然挂载，或使用 ref 标记组件挂载状态。

---

### 🟡 8. 乐观更新没有回滚机制

**文件**: `mobile/src/hooks/useChatMessages.ts`  
**行号**: 68-75  
**问题类型**: UI 状态不一致 - 乐观更新未回滚

**问题描述**:
```typescript
const addMessage = useCallback(
  (sessionID: string, role: ChatItem["role"], text: string, requestID?: string, attachments?: ChatAttachment[]) => {
    updateSessionChat(sessionID, (current) => ({
      ...current,
      messages: [...current.messages, { 
        attachments, 
        id: newRequestID(), 
        requestID, 
        createdAt: new Date().toISOString(), 
        role, 
        text 
      }],
    }));
  },
  [updateSessionChat],
);
```

消息被添加到本地状态后，如果发送失败（网络错误、服务端拒绝等），消息仍然显示在 UI 中。没有失败回滚机制或失败标记。

**潜在影响**: 
- 显示未成功发送的消息
- 用户误以为消息已发送
- 重新加载后消息消失，用户困惑
- 需要手动重发消息

**修复建议**: 
1. 为消息添加状态字段（pending/sent/failed）
2. 发送失败时标记消息或从列表中移除
3. 提供重试按钮

---

### 🟡 9. 错误处理不区分具体操作

**文件**: `mobile/src/hooks/useRemoteMessageHandler.ts`  
**行号**: 747-812  
**问题类型**: UI 状态不一致 - 错误状态未正确显示

**问题描述**:
```typescript
case "error": {
  const payload = (message.payload || {}) as ErrorPayload;
  
  // 清理所有 pending 状态
  stopPending("sessions");
  stopPending("models");
  stopPending("settings");
  stopPending("files");
  stopPending("changes");
  stopPending("history");
  stopPending("knowledge");
  stopPending("memories");
  
  // 显示通用错误
  console.error("Received error:", payload.error);
  
  break;
}
```

错误处理中统一清理所有 pending 状态，但没有区分是哪个操作失败。用户无法知道具体哪个操作出错，所有加载中的操作都会停止显示 loading 状态。

**潜在影响**: 
- 用户看不到具体哪个操作失败
- 错误提示不明确
- 难以重试失败的操作
- 其他正常的并发操作被错误地清理

**修复建议**: 
在错误消息中包含操作类型或 requestID，只清理对应操作的 pending 状态。

---

### 🟡 10. 重复连接尝试

**文件**: `mobile/src/screens/MobileAppScreen.tsx`  
**行号**: 827-839  
**问题类型**: 异步处理 - 竞态条件

**问题描述**:
```typescript
useEffect(() => {
  if (!settingsLoaded || !clientToken || AppState.currentState !== "active") {
    return;
  }
  
  const timer = setTimeout(() => {
    if (!socketRef.current) {
      connectRef.current();
    }
  }, 300);
  
  return () => clearTimeout(timer);
}, [clientToken, deviceID, normalizedRelayURL, settingsLoaded, userID]);
```

依赖项包含 `clientToken`, `deviceID`, `normalizedRelayURL`, `userID`, `settingsLoaded`。这些值任何一个变化都会重新设置 timer。如果这些值在短时间内多次变化（例如从 AsyncStorage 批量加载设置），会创建多个 timer，可能导致重复连接尝试。

**潜在影响**: 
- 短时间内多次尝试连接
- 不必要的网络请求
- 可能触发服务端限流

**修复建议**: 
使用 debounce 或者只在真正需要重连时才设置 timer。

---

## 低危问题

### 🟢 11. 子智能体事件序列号可能回退

**文件**: `mobile/src/hooks/useSubagentState.ts`  
**行号**: 98-110  
**问题类型**: 状态更新时机问题

**问题描述**:
```typescript
function upsertTask(current: SubagentTask[], value: SubagentTask) {
  const existing = current.find((item) => item.id === value.id);
  
  if (
    existing?.event_sequence !== undefined &&
    value.event_sequence !== undefined &&
    value.event_sequence < existing.event_sequence
  ) {
    return current;  // 拒绝序列号更小的更新
  }
  
  // ... 更新逻辑
}
```

只比较 `event_sequence`，不检查 `updated_at` 时间戳或其他版本信息。在分布式系统中，序列号可能会重置或出现时钟偏移，导致旧数据覆盖新数据。

**潜在影响**: 
- 子智能体任务状态回退到旧版本
- 显示过时的任务信息

**修复建议**: 
结合时间戳和序列号进行版本比较，或使用向量时钟。

---

### 🟢 12. WebSocket 回调闭包引用旧状态

**文件**: `mobile/src/hooks/useRelayConnection.ts`  
**行号**: 284-355  
**问题类型**: 内存泄漏 - 事件监听器未移除

**问题描述**:
```typescript
const connect = useCallback(() => {
  const socket = new WebSocket(url);
  
  socket.onopen = () => {
    // 回调引用闭包中的状态
    setConnectionState("connected");
    // ...
  };
  
  socket.onmessage = (event) => {
    // 回调引用闭包中的 handler
    handleMessage(event.data);
  };
  
  // ...
}, [url, handleMessage, /* ... */]);
```

WebSocket 的 `onopen`, `onclose`, `onerror`, `onmessage` 回调设置后，如果 socket 未正确关闭，这些回调可能持续引用旧的闭包。如果组件重新渲染但 WebSocket 未重建，回调中引用的状态和函数可能是过时的。

**潜在影响**: 
- 内存泄漏
- 回调使用过时的状态或函数
- 旧的回调函数无法被垃圾回收

**修复建议**: 
在设置新回调前清理旧回调，或使用 ref 确保回调总是访问最新的状态。

---

### 🟢 13. 定时器未正确清理

**文件**: `mobile/src/hooks/useRemoteRequests.ts`  
**行号**: 60-68  
**问题类型**: 内存泄漏 - 定时器未清理

**问题描述**:
```typescript
const stopRemoteStateLoadingLater = useCallback(() => {
  if (remoteStateTimeoutRef.current) {
    clearTimeout(remoteStateTimeoutRef.current);
  }
  
  remoteStateTimeoutRef.current = setTimeout(() => {
    timeoutActions.forEach(stopPending);
    remoteStateTimeoutRef.current = null;
  }, remoteStateTimeoutMs);
}, [stopPending]);
```

`stopRemoteStateLoadingLater` 是一个 callback，但 `remoteStateTimeoutRef` 可能在组件卸载后仍然执行。虽然设置了 timeout 为 null，但 timeout 回调中的 `stopPending` 可能尝试更新已卸载组件的状态。

**潜在影响**: 
- 组件卸载后仍尝试更新状态
- React 警告: "Can't perform a React state update on an unmounted component"

**修复建议**: 
在组件的 cleanup 函数中清理所有 timeout。

---

### 🟢 14. Plan 按钮状态不同步

**文件**: `mobile/src/components/plan/PlanPanel.tsx`  
**行号**: 27-35, 92-98  
**问题类型**: UI 状态不一致

**问题描述**:
```typescript
const canExecute = Boolean(
  clientToken &&
    sessionID &&
    plan &&
    (plan.steps?.length || 0) > 0 &&
    ["draft", "approved", "failed", "canceled"].includes(plan.status) &&
    !pendingPlan,
);

// ...

<Button
  onPress={onExecute}
  disabled={!canExecute}
  loading={pendingPlan}
>
  执行计划
</Button>
```

按钮的 `disabled` 状态和 `loading` 状态分别判断。理论上 `pendingPlan` 为 true 时 `canExecute` 应该为 false，但如果状态更新时序不一致，可能出现按钮显示 "执行中" 但仍可点击的情况（disabled=false, loading=true）。

**潜在影响**: 
- 用户可以在执行中再次点击按钮
- 可能导致重复执行请求

**修复建议**: 
统一按钮状态判断逻辑，确保 loading 时必定 disabled。

---

### 🟢 15. Protocol 类型定义与使用不一致

**文件**: `mobile/src/protocol.ts`  
**行号**: 多处  
**问题类型**: 类型安全 - 类型定义错误

**问题描述**:
许多 payload 类型的字段标记为可选（`?`），但在使用时被当作必需字段直接访问，没有空值检查。

**示例**:
```typescript
// protocol.ts
export interface AssistantDonePayload {
  plan?: Plan;  // 可选字段
  usage?: Usage;
  // ...
}

// 使用处
const payload = message.payload as AssistantDonePayload;
const steps = payload.plan.steps;  // 直接访问，未检查 plan 是否存在
```

**潜在影响**: 
- 运行时类型错误: "Cannot read property 'steps' of undefined"
- 应用崩溃

**修复建议**: 
1. 严格对齐类型定义和后端实际返回
2. 使用可选链操作符 `payload.plan?.steps`
3. 添加类型守卫函数

---

### 🟢 16. useMemo 依赖项过多导致性能问题

**文件**: `mobile/src/hooks/useRemoteResultAppliers.ts`  
**行号**: 144-547  
**问题类型**: 性能问题

**问题描述**:
```typescript
const appliers = useMemo(() => {
  return {
    applySessionListResult,
    applySessionLoadResult,
    applyModelListResult,
    // ... 40+ 个函数
  };
}, [
  // 40+ 个依赖项
  updateSession,
  setCurrentSessionID,
  updateSessionChat,
  addMessage,
  // ...
]);
```

useMemo 包含 40+ 个依赖项，几乎每次渲染都会重新计算。这完全失去了 memoization 的意义，反而增加了依赖比较的开销。

**潜在影响**: 
- 性能下降
- 频繁重新创建对象
- 子组件不必要的重新渲染

**修复建议**: 
拆分 appliers 为多个独立的 useMemo，或者使用 useCallback 单独包装每个函数。

---

### 🟢 17. 消息 ID 碰撞风险

**文件**: `mobile/src/hooks/useChatMessages.ts`  
**行号**: 72, 98, 162, 198, 298, 393  
**问题类型**: 数据一致性

**问题描述**:
```typescript
const addMessage = useCallback((...) => {
  updateSessionChat(sessionID, (current) => ({
    ...current,
    messages: [...current.messages, { 
      id: newRequestID(),  // 生成新 ID
      // ...
    }],
  }));
}, [updateSessionChat]);
```

多处使用 `newRequestID()` 生成消息 ID，但没有检查 ID 是否已存在。虽然 UUID 碰撞概率极低，但在高频消息场景下（例如快速流式响应），如果 ID 生成器使用时间戳，可能产生碰撞。

**潜在影响**: 
- 消息被错误覆盖
- 消息关联错误
- React key warning

**修复建议**: 
在添加消息前检查 ID 是否已存在，或使用更强的 ID 生成策略（crypto.randomUUID）。

---

### 🟢 18. 事件缓存无限增长

**文件**: `mobile/src/hooks/useSubagentState.ts`  
**行号**: 112-119  
**问题类型**: 内存泄漏

**问题描述**:
```typescript
function appendTaskEvent(current: Record<string, SubagentTaskEvent[]>, value: SubagentTaskEvent) {
  const previous = current[value.task_id] || [];
  
  if (previous.some((event) => event.sequence === value.sequence)) {
    return current;  // 去重
  }
  
  // 每个 task 最多保留 256 个事件
  const next = [...previous, value].sort((left, right) => left.sequence - right.sequence);
  return { ...current, [value.task_id]: next.slice(-256) };
}
```

每个 task 最多保留 256 个事件（使用 `slice(-256)`），但 task 本身不会被清理。如果长期运行的应用创建了大量 task，即使每个 task 只保留 256 个事件，总内存占用仍会持续增长。

**潜在影响**: 
- 内存使用持续增长
- 长期运行后应用变慢
- 可能导致 OOM

**修复建议**: 
1. 定期清理已完成的旧 task
2. 设置全局事件总数上限
3. 只保留当前活跃 task 的完整事件，历史 task 只保留摘要

---

## 统计总结

| 严重程度 | 数量 | 占比 |
|---------|------|------|
| 🔴 高危 | 3 | 16.7% |
| 🟡 中危 | 7 | 38.9% |
| 🟢 低危 | 8 | 44.4% |
| **总计** | **18** | **100%** |

### 问题分类统计

| 类别 | 数量 |
|------|------|
| 状态管理问题 | 4 |
| 异步处理问题 | 3 |
| 内存泄漏 | 4 |
| UI 状态不一致 | 3 |
| 协议同步问题 | 2 |
| 性能问题 | 1 |
| 类型安全 | 1 |

---

## 修复优先级建议

### 第一优先级（立即修复）
1. **问题 #3**: 协议消息类型处理遗漏 - 补全所有协议处理分支
2. **问题 #1**: WebSocket 连接竞态 - 添加连接状态锁
3. **问题 #2**: Agent Run 事件顺序错乱 - 合并 placeholder 事件

### 第二优先级（本周内修复）
4. **问题 #5**: 流式消息关联错误 - 改进查找逻辑
5. **问题 #8**: 乐观更新未回滚 - 添加消息状态
6. **问题 #4**: 心跳定时器泄漏 - 修复依赖项
7. **问题 #7**: AppState 监听器泄漏 - 添加挂载检查

### 第三优先级（长期优化）
- 其余低危问题可在日常迭代中逐步修复
- 重点关注内存泄漏和性能优化

---

## 附录：测试建议

为了防止这些 bug，建议增加以下测试：

### 1. 单元测试
- 测试所有 hooks 的 cleanup 函数
- 测试消息处理的各种边界情况
- 测试状态更新的时序

### 2. 集成测试
- 模拟 WebSocket 消息乱序到达
- 测试网络断线重连场景
- 测试快速切换会话

### 3. 性能测试
- 使用 React DevTools Profiler 检查不必要的重新渲染
- 监控内存使用情况
- 测试长时间运行场景

### 4. 协议一致性测试
- 自动对比 `protocol.ts` 和后端协议定义
- 确保所有消息类型都有对应处理
- 验证类型定义与实际使用一致

### 5. 类型检查
```bash
cd mobile
npm run typecheck
```

确保没有类型错误和警告。

---

## 代码质量改进建议

### 1. 添加 ESLint 规则
```json
{
  "rules": {
    "react-hooks/exhaustive-deps": "error",
    "@typescript-eslint/no-floating-promises": "error",
    "@typescript-eslint/no-non-null-assertion": "error"
  }
}
```

### 2. 使用工具检测内存泄漏
- React DevTools
- Chrome DevTools Memory Profiler
- why-did-you-render

### 3. 添加运行时检查
在开发环境中添加以下检查：
- 检测未处理的 Promise rejection
- 检测组件卸载后的状态更新
- 检测异常的重新渲染次数

---

*此报告由自动化分析工具生成，建议结合人工 Code Review 和实际测试进行验证。*
