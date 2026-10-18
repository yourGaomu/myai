# Plan 模式功能实现说明

> 本文解释 Session 如何切换到 Plan 模式、模型如何生成结构化 Plan、纯文本任务为何可以直接完成，以及用户点击 Execute 后每个步骤如何执行。普通聊天见 [普通聊天消息功能实现](FEATURE_CHAT_MESSAGE_IMPLEMENTATION.md)，模型底层见 [模型生成功能实现](FEATURE_MODEL_GENERATION_IMPLEMENTATION.md)，工具权限见 [工具执行功能实现](FEATURE_TOOL_EXECUTION_IMPLEMENTATION.md)。

## 1. 规划与执行阶段

显式 Plan 模式仍然保留“先规划、再执行”的两个阶段；另外，根用户会话可以通过 `AutoPlanEnabled` 在同一条请求内自动串起这两个阶段。

```text
阶段 A：生成计划
用户消息 -> Plan prompt -> 只读检查 -> 结构化/Markdown Plan -> Capture -> draft/done

阶段 B：执行计划
用户点击 Execute，或 AutoPlan 同请求自动触发 -> 逐步骤生成任务 -> 可写工具 -> running/done/failed/canceled
```

内容创作等安全纯文本任务可以在阶段 A 同时输出 `Plan` 和 `Result`，直接标记 done，不进入阶段 B。

`ChatDependencies.AutoPlanEnabled` 只作用于根用户会话。分类器返回结构化意图（`conversation`、`explanation`、`implementation`）并要求足够置信度；分类失败或信号不足时安全地走普通 Chat。子智能体不会递归触发 AutoPlan。

自动规划使用 `core/application/runtime/service.AutonomousPlanPrompt`，与手动 Plan prompt 分开：它要求先做只读规划，随后由 `ChatService` 调用已有 `PlanExecution`，而不是让一次模型回复自行声称已完成副作用操作。

## 2. 核心对象

| 对象 | 作用 |
|---|---|
| `Session.AgentMode` | 当前 chat/plan 模式 |
| `RuntimeInstructionBuilder` | 生成本轮 Plan 指令 |
| `ModePolicy` | 决定 Plan 限制及工具可见性 |
| `ResponseCommitService` | 在 Plan 模式捕获模型回复 |
| `CaptureService` | Markdown 回复转 `Plan` |
| `plan.Plan/Step` | 领域状态对象 |
| `ExecutionService` | 逐步骤执行已保存 Plan |
| `StateService` | Plan 和 Step 状态转换 |
| `StatePersistenceService` | 保存当前 Plan 快照 |
| `ExecutionInputBuilder` | 将一个步骤变成明确模型输入 |
| `PlanPanel` | 手机展示和执行入口 |

## 3. Plan 数据结构

```go
type Plan struct {
	ID         string
	SessionID  string
	Goal       string
	Status     string
	Revision   int64
	RawContent string
	Steps      []Step
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Step struct {
	ID          string
	Order       int
	Title       string
	Description string
	Dependencies []string
	Status       string
	RetryCount   int
	MaxRetries   int
	LastError    string
	AgentTaskID  string
	StartedAt    *time.Time
	CompletedAt  *time.Time
}
```

结构化 Plan 还接受 JSON `steps` 数组（字段 `id`、`order`、`title`、`description`、`depends_on`/`dependencies`、`max_retries`）；旧会话或普通模型仍可使用 Markdown 列表作为兼容回退。每个 Plan 最多保留 12 个步骤。

Plan 状态：

```text
draft / approved / running / done / failed / canceled
```

Step 状态：

```text
pending / running / done / failed / skipped
```

点击 Execute 后，执行服务会先把可执行 Plan 更新为 `approved`，再进入 `running`。`skipped` 由状态机保留：恢复执行时与 `done` 一样不会重复运行。

## 4. 手机切换模式

SettingsPanel 显示 Chat 和 Plan 两个 mode chip。点击 Plan：

```text
SettingsPanel.onSetAgentMode("plan")
-> useSessionSettingsActions.setAgentMode
-> sendEnvelope("session_mode_set")
```

请求：

```json
{
  "type": "session_mode_set",
  "request_id": "...",
  "session_id": "session-001",
  "payload": {
    "session_id": "session-001",
    "mode": "plan"
  }
}
```

手机不立即改本地模式，而是设置 `pendingActions.settings`，等待服务端结果。

## 5. Relay 和 Agent 处理模式切换

```text
Relay.handleClientMessage
-> validate token
-> register request
-> forward session_mode_set

Agent.handleRelayMessage
-> handleSessionModeSet
-> 获取 Session runtime.mu
-> 获取 Agent.requestMu
-> ChatService.SetAgentModeForSession
```

Session 锁避免生成过程中同时切换同一会话模式。

## 6. Settings 应用层

调用：

```text
ChatService.SetAgentModeForSession
-> settings.UseCase.SetAgentMode
-> SettingsService.SetAgentMode
-> Load Session
-> Memory.SetAgentModeForSession
-> Persistence.Save
-> Events.SessionChanged(reason="agent_mode")
```

只接受 `chat` 和 `plan`，其他字符串返回 `unsupported agent mode`。

## 7. 手机以服务端结果为准

Agent 返回 `session_mode_set_result`，Payload 包含服务端最新 Session summary、列表和 Context。

手机：

```text
stopPending("settings")
-> applySessionSettings
-> requestSessions
```

因此刷新、重连和多端操作后，模式仍以持久层和 Agent 状态为准。

## 8. 用户在 Plan 模式发送消息

发送链路与 Chat 相同：

```text
user_message
-> Agent.handleUserMessage
-> ChatService.SendMessageStreamForSession
-> AppendUserMessage
-> GenerationTasks.Generate(CapturePlan=true)
```

区别不在协议类型，而在 `Session.AgentMode=plan` 被 RuntimeInstructionBuilder 和 Tool Selection 读取。

## 9. PlanModePrompt

Plan prompt 要求模型：

- 先分析并给出具体执行计划。
- 生成阶段禁止编辑、Shell、安装和不可逆操作。
- 只能使用只读检查工具。
- 必须输出名为 `Plan` 的 Markdown 章节和编号步骤。
- 纯文本安全任务在同一回复增加 `Result` 并完成产出。
- 手动 Plan 的副作用任务在 Plan 后停止，等待用户批准；AutoPlan 由 ChatService 在同一请求内继续执行已捕获的计划。

该 prompt 是代码常量；每次用户发送消息时，`MessageCommandService` 会把本轮构建结果写成 `SyntheticReasonRuntimeInstruction` 消息，再追加真实 user message。

## 10. RuntimeInstructionBuilder

```go
parts := []string{RuntimeTurnBoundaryPrompt}
if ModePolicy.IsPlanMode(agentMode, forceChatMode) {
	parts = append(parts, PlanModePrompt)
}
if skillPrompt != "" {
	parts = append(parts, skillPrompt)
}
return strings.Join(parts, "\n\n")
```

turn boundary 始终存在；Plan 与匹配 Skill 可以同时生效，Skill 指令追加在 Plan 规则之后。

## 11. 缓存前缀为什么稳定

运行时 Plan prompt 与本轮 user 一起进入 Session：

```text
[固定 system]
[历史]
[synthetic system: turn boundary + Plan rules]
[本轮 user]
```

旧 turn 的 Synthetic Message 保持不变；切回 Chat 只影响下一条消息，不改写旧历史。`PrefixHash` 基于当前 turn 之前的已完成历史，因此当前 Plan prompt 的变化不会打乱可缓存前缀。

详细 Context 和缓存指标见 [模型生成功能实现](FEATURE_MODEL_GENERATION_IMPLEMENTATION.md)。

## 12. Plan 生成阶段的工具限制

```text
AgentLoopService
-> Catalog.ToolsForSession
-> SelectionService
-> ModePolicy.IsPlanMode == true
-> 仅保留 PermissionRead
```

即使 Session PermissionMode 为 full，Plan 生成阶段也不应向模型暴露 write/execute 工具。

执行层还有第二道硬门禁：Hook 重写参数之后、PermissionService 之前，Plan 模式仅允许 read 工具。Hook 返回 Allow 也不能绕过该门禁或会话权限；只有执行已批准计划时的 `ForceChatMode=true` 能解除 Plan 门禁。

这让模型可以读取文件、搜索项目并制定计划，但不能提前实施。

## 13. 模型输出格式

文件修改任务示例：

```markdown
## Plan
1. 检查当前 Session 加载实现。
2. 将加载逻辑拆分到独立服务。
3. 添加单元测试并运行 go test。
```

纯文本任务示例：

```markdown
## Plan
1. 选择春雨和江南意象。
2. 组织四句七言诗。
3. 检查表达和韵律。

## Result
细雨江南入晚烟，...
```

## 14. ResponseCommit 捕获 Plan

模型成功后先保存 Assistant 和 Usage，然后：

```go
if CapturePlan && Session.AgentMode == plan {
	currentPlan := PlanCapturer.Capture(...)
	Memory.SetCurrentPlanForSession(...)
}
```

Chat 模式不会把普通 Markdown 列表误识别为 Plan。

## 15. CaptureService

```go
currentPlan := plan.NewDraft(sessionID, goal, content, now)
if plan.HasResultSection(content) {
	currentPlan.Status = done
	所有 step.Status = done
}
```

`goal` 使用用户本轮原始输入，`RawContent` 保存模型完整回复。

## 16. Step 解析规则

优先寻找规范化后与以下名称完全相等的标题：

```text
plan / execution plan / implementation plan / proposed plan
计划 / 执行计划 / 执行规划 / 规划 / 步骤
```

然后解析：

```text
1. step
1) step
- step
* step
- [ ] step
```

遇到下一个 Markdown heading 停止，最多保留 12 步。

步骤文本可用 ` - `、中文冒号或英文冒号拆分 title 和 description。Title 最长 96 个 rune。

## 17. 解析失败处理

如果没有提取到合法列表，`CurrentPlan` 保留模型原文，但 `Steps` 为空。系统不会根据正文首行编造兜底执行步骤。

手机 Execute 按钮要求至少一个 Step，后端 ExecutionService 也会再次校验，因此格式错误的 Plan 只能查看，不能执行。

## 18. Result 检测

`HasResultSection` 只接受规范化后完全匹配的 heading：

```text
result / final result / output / final output / answer / final answer
结果 / 正文 / 产出 / 成品 / 作品
最终结果
```

检测到后，Plan 和全部 Steps 直接标记 done。

这就是“写一首古诗”能够同一回复先给计划再给正文，并且不需要点击 Execute 的实现。

## 19. Action Plan 为什么停在 draft

手动 Plan 下，文件编辑、Shell、安装等请求按 PlanModePrompt 只能输出 Plan，不输出 Result。Capture 后：

```text
Plan.Status = draft
Steps[*].Status = pending
```

手机 PlanPanel 显示目标、步骤和 Execute 按钮，等待用户显式批准。

启用 AutoPlan 时，`ChatService.SendMessageStreamForSession` 会：

```text
分类实现类请求
-> Clone Session 并以 Plan 模式做只读规划
-> 捕获并持久化 CurrentPlan
-> PlanExecution.Execute(ParentRunID=planning.RunID)
-> 返回合并后的执行结果
```

规划流的正文不会拼进最终回答，但 Plan/Step 进度仍通过回调和 AgentRun 记录；纯文本 Plan+Result 仍可直接完成。

## 20. PlanPanel

`activeSession.current_plan` 是 UI 数据源。

可执行条件：

```ts
clientToken &&
sessionID &&
plan &&
plan.steps.length > 0 &&
["draft", "approved", "failed", "canceled"].includes(plan.status) &&
!pendingPlan
```

Panel 统计 done 数、运行步骤和失败状态，并根据 Step status 渲染进度。

## 21. 点击 Execute

```text
PlanPanel.onExecutePlan
-> useSessionSettingsActions.executePlan
```

发送前手机：

```text
创建 request_id
requestSessionMap[request] = session
清空 active Assistant
清空 permission 和 lastUsage
设置 Session pendingRequestID
startPending("plan")
发送 session_plan_execute
```

请求：

```json
{
  "type": "session_plan_execute",
  "request_id": "...",
  "session_id": "session-001",
  "payload": { "session_id": "session-001" }
}
```

## 22. Relay 路由和终态

Relay 支持：

```text
client -> agent: session_plan_execute
agent -> client: session_plan_update 0..n
agent -> client: assistant_delta/tool_*/permission_ask
agent -> client: assistant_done
agent -> client: session_plan_execute_result
```

对 `session_plan_execute`，只有 `session_plan_execute_result` 或 `error` 是 Relay 终态。中间的 `assistant_done` 不删除 request route。

## 23. Agent 并发与取消

```text
processPlanExecuteMessage
-> Decode SessionPlanExecutePayload
-> resolveSessionID
-> runtime.mu.Lock
-> runtime.start(cancel context)
-> handleSessionPlanExecute
```

Plan 执行与普通消息共享 Session runtime，因此同一会话不能同时执行其他生成任务。`session_pause` 调用同一个 cancel。

## 24. ChatService Plan Facade

```go
ChatService.ExecutePlanStreamForSession(
	ctx, sessionID, stream, onPlanUpdate,
)
```

它把 callback 包装为 `UpdateSink`，再调用 `PlanExecution.Execute`，最后把应用结果映射为统一 `ChatResponse`。

## 25. ExecutionService 前置校验

必须具备：

```text
ModelProvider
SessionLoader
MessageAppender
GenerationTaskService
非空 session_id
Session.CurrentPlan
至少一个 Step
Plan.Status 属于 draft / approved / failed / canceled
```

然后 Clone CurrentPlan，避免直接共享外部指针。

`running`、`done` 和未知状态都会被拒绝。`failed`、`canceled` 允许用户从未完成步骤继续执行。

## 26. Plan 开始状态

```go
currentPlan = State.Approve(currentPlan)
currentPlan = State.Start(currentPlan)
```

结果：

```text
Plan.Status = approved，并立即保存和推送
Plan.Status = running
done/skipped Step 保持原状态
running/failed/pending Step 重置为 pending
```

如果 Plan 原本已经是 `approved`，不会重复保存批准状态。进入 running 后再次 `savePlanState`，手机可以依次看到批准与启动状态。

## 27. 每步执行状态机

ExecutionService 按依赖就绪批次执行 Step；没有依赖的旧 Markdown Plan 仍按顺序串行执行，声明依赖的结构化 Plan 可并行。

```text
选择 dependency-ready batch（默认最多 3 个并行步骤）
-> 每个 Step 检查 ctx
-> Step = running
-> save + update
-> BuildStepInput
-> AppendUserMessage
-> PersistUserMessage
-> GenerationTasks.Generate(ForceChatMode=true)
-> 成功: Step = done -> save + update
-> 失败: 按 MaxRetries 重试；耗尽后调用 Recovery planner 或标记 Step/Plan failed
```

每个 Step：

```text
检查 ctx
-> Step = running
-> save + update
-> BuildStepInput
-> AppendUserMessage
-> PersistUserMessage
-> GenerationTasks.Generate(ForceChatMode=true)
-> 成功: Step = done -> save + update
-> 失败: 按 MaxRetries 重试；耗尽后调用 Recovery planner 或标记 Step/Plan failed
```

全部步骤成功后：

```text
Plan = done
-> save + final update
```

## 28. ExecutionInputBuilder

每步生成一条新的 user 消息，格式包含：

```text
Execute the approved plan step 2/3.

Goal:
原始用户目标

Current step:
当前标题和描述

Full plan:
1. [done] ...
2. [running] ...
3. [pending] ...

Focus on the current step...
```

这让模型知道全局目标，但只处理当前步骤。

## 29. ForceChatMode 的作用

```go
GenerationTask{
	ForceChatMode: true,
	Reason: "execute plan step",
}
```

虽然 Session.AgentMode 仍是 plan，但 ModePolicy 将本轮按 Chat 处理：

- 不注入 PlanModePrompt，避免再次生成新计划。
- 允许按 Session PermissionMode 暴露写入和执行工具。
- Skill 仍可根据步骤输入匹配。

## 30. 每步仍是完整生成任务

每个 Step 都会：

- 创建独立内部任务 ID。
- 创建独立 TaskRecorder 检查点。
- 调用完整 AssistantGenerationService。
- 可能执行多轮工具。
- 提交一条 Assistant 到 Session。
- 异步保存 user 和 assistant。

因此一个三步 Plan 会在会话历史中增加三组“执行步骤 user + assistant/tool”记录。

## 31. Plan 状态如何保存

```text
ExecutionService.savePlanState
-> StatePersistenceService.Save
-> StateRepository.SaveCurrentPlan
-> 更新 Session.CurrentPlan clone
-> UpdateSink.PlanUpdated clone
-> Events.SessionChanged(reason="plan")
```

当前 Adapter 把 Plan 写入内存 Session，并通过 Session Persistence 保存整个 SessionRecord。

每次保存更新 `UpdatedAt`。

## 32. session_plan_update

Agent 的 `onPlanUpdate` callback 构造最新 `SessionSettingsResultPayload` 并发送：

```text
session_plan_update
```

手机只调用 `applySessionSettings`，不结束 pending。PlanPanel 因 active Session 更新而展示 running/done/failed Step。

## 33. 多步骤结果合并

`ResponseCombiner`：

```text
Content: 各步骤回答用空行拼接
Reasoning: 各步骤 reasoning 用空行拼接
Usage: 逐字段相加
ToolCalls: 使用最后一步值
Context/Compact: 使用最后一步
```

最终 ChatResponse 代表整个 Plan 执行汇总。

## 34. Agent 最终消息顺序

成功时：

```text
1. assistant_done
   完整合并 Content/Reasoning/Usage/Context/Plan

2. session_plan_execute_result
   最新 Session、Plan、列表、Context 和完成消息
```

`assistant_done` 完成聊天气泡；`execute_result` 才释放 Plan pending 和 Relay request route。

## 35. 手机终态处理

收到 `assistant_done`：

- 完成 Assistant 气泡。
- 清除 Session `pendingRequestID` 和 requestSessionMap。
- 刷新远程状态。
- 但不执行 `stopPending("plan")`。

收到 `session_plan_execute_result`：

```text
stopPending("plan")
-> applySessionSettings
-> requestSessions
```

因此 Plan 按钮的 Executing 状态持续到真正最终结果。

## 36. 失败处理

步骤生成失败：

```text
Step = failed
Plan = failed
savePlanState(background)
-> 返回 error
-> Agent TypeError
-> Relay 将 error 视为终态
-> Mobile stopPending("plan") 并显示错误
```

已经 done 的步骤保持 done，失败步骤标记 failed，后续步骤保持 pending。

用户重新点击 Execute 时，ExecutionService 保留 `done/skipped`，并从第一个未完成步骤继续，不会重复执行已经成功的步骤。

## 37. 取消处理

每步开始前检查 `ctx.Err()`。取消时：

```text
Plan.Status = canceled
-> background save
-> 返回 context.Canceled
```

Agent 识别取消后依次发送：

```text
paused assistant_done
session_plan_execute_result（包含 canceled Plan）
```

`session_plan_execute_result` 释放手机 pending 状态和 Relay request route，取消请求不会滞留到连接断开。

## 38. 纯文本任务完整示例

用户在 Plan 模式输入：

```text
写一首描写春雨的七言绝句
```

链路：

```text
PlanModePrompt
-> 模型输出 Plan + Result
-> ResponseCommitService
-> CaptureService.NewDraft
-> HasResultSection = true
-> Plan/Steps = done
-> assistant_done 包含正文和 done Plan
-> PlanPanel Execute disabled
```

## 39. 文件修改完整示例

用户：

```text
重构 Session 加载逻辑并添加测试
```

阶段 A：

```text
Plan 模式只读工具检查源码
-> 输出 Plan，无 Result
-> CurrentPlan=draft
```

阶段 B：

```text
用户 Execute
-> Plan approved -> running
-> Step 1 running -> ForceChat generation -> done
-> Step 2 running -> edit_file/permission -> done
-> Step 3 running -> shell go test/permission -> done
-> Plan done
-> assistant_done
-> session_plan_execute_result
```

## 40. Prompt Cache 示例

模式切换不会这样修改历史：

```text
错误做法: Session.Messages[0] = PlanPrompt + SystemPrompt
```

实际做法：

```text
固定 Session system 不变
本轮先追加 Synthetic runtime message，再构建 Snapshot
下一轮 Chat 追加自己的 turn boundary/runtime message，不重写旧消息
```

Synthetic runtime message 会进入 `Session.Messages` 并持久化，但历史查询会过滤它；因此固定 system 和已持久化历史不会因模式开关被重写，Mobile 也不会看到伪造的普通用户消息。

## 41. 问题清理结果

本轮记录的六项问题均已解决：

1. Execute 会显式保存 `approved`，再进入 `running`。
2. 前后端都校验可执行状态；`running`、`done` 和未知状态不能执行。
3. `failed/canceled` 重试保留 `done/skipped`，从第一个未完成步骤恢复。
4. 取消会发送 paused `assistant_done` 和终态 `session_plan_execute_result`。
5. Step 解析失败不再创建虚构步骤，空步骤 Plan 无法执行。
6. Plan/Result 章节只按受支持的完整标题匹配，不再使用宽泛的子串判断。

## 42. 推荐断点

| 文件 | 函数 | 观察内容 |
|---|---|---|
| `useSessionSettingsActions.ts` | `setAgentMode/executePlan` | request、pending、Session |
| `session_settings_handlers.go` | `handleSessionModeSet` | 锁、Payload、返回 Session |
| `settings_service.go` | `SetAgentMode` | 模式校验和内存更新 |
| `runtime_instruction_builder.go` | `Build` | Plan prompt 与 ForceChat |
| `response_commit_service.go` | `Commit` | CapturePlan 条件 |
| `capture_service.go` | `Capture` | draft/done 判断 |
| `core/plan/plan.go` | `ExtractSteps/HasResultSection` | Markdown 解析 |
| `chat/plan/execution_service.go` | `Execute` | Step 循环和状态 |
| `state_service.go` | 状态方法 | Clone 后状态转换 |
| `chat_handlers.go` | `handleSessionPlanExecute` | update/done/result 顺序 |
| `useRemoteMessageHandler.ts` | Plan cases | 手机 pending 释放 |

## 43. 测试

重点测试：

```text
TestSettingsServiceSetAgentMode
TestRuntimeInstructionBuilderAddsPlanPrompt
TestRuntimeInstructionBuilderForceChatSkipsPlanPrompt
TestModePolicyPlanModeOnlyAllowsReadTools
TestContextSnapshotServiceRuntimePromptDoesNotChangeCacheablePrefix
TestResponseCommitServiceCapturesPlanInPlanMode
TestCaptureServiceMarksContentOnlyPlanDone
TestCaptureServiceKeepsActionPlanDraft
TestExecutionInputBuilderBuildsFocusedStepInput
TestPlanExecutionServiceExecutesStepsAndCombinesResults
TestPlanExecutionServiceMarksStepFailedWhenGenerationFails
TestRelayForwardsPlanExecutionUntilFinalResult
```

运行：

```powershell
go test ./core/application/runtime ./core/application/plan
go test ./core/application/chat ./core/application/session
go test ./core/remote/relay ./core/remote/agent
cd mobile
npm run typecheck
```

人工验收：

```text
Chat -> Plan -> Chat 模式切换
写古诗：Plan + Result，同轮 done
代码任务：只生成 draft
Execute：逐步 update、工具审批、最终 result
步骤失败：Plan failed
执行中 Pause：Plan canceled
重连后 CurrentPlan 保持
```

## 44. 源码索引

```text
mobile/src/hooks/useSessionSettingsActions.ts
mobile/src/hooks/useRemoteMessageHandler.ts
mobile/src/components/settings/SettingsPanel.tsx
mobile/src/components/plan/PlanPanel.tsx
core/remote/agent/session_settings_handlers.go
core/remote/agent/chat_handlers.go
core/application/runtime/service/
core/application/plan/service/
core/application/chat/plan/service/
core/application/session/settings/service/
core/plan/plan.go
core/remote/protocol/message.go
core/remote/relay/server.go
```

## 45. 阅读检查

阅读后应能回答：模式如何持久化、Plan prompt 为什么不打乱缓存、生成阶段为何只读、Result 如何让纯文本 Plan 直接 done、Execute 如何逐步运行、ForceChatMode 为什么必要、update/done/execute_result 有何区别、失败和取消如何保存状态。
