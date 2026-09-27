# 自动规划与子智能体机制实现说明

> 本文记录当前项目中“自动规划（Auto Plan）+ 子智能体（Subagent）”的实现方式、调用链、状态流转和移动端协议。
>
> 文档描述以当前源码为准。若文档与代码不一致，应优先以源码和测试结果为准。

## 1. 功能目标

用户只需要在根会话中提出一个完整的开发请求，例如“给安卓端增加后台长连接并补充测试”，系统可以在一条对话链路内完成：

1. 判断这是不是需要修改项目的实现型请求。
2. 自动生成结构化 Plan。
3. 按依赖顺序执行 Plan 步骤。
4. 在某个步骤需要独立调查或实现时，调用 spawn_agent 创建子智能体。
5. 子智能体在独立 Session、独立 Run 和可选隔离 Workspace 中异步执行。
6. 通过 TaskEvent 将状态、推理片段、工具调用和最终结果推送到 Relay/Mobile。
7. 用户可以等待、取消、恢复主任务，或明确应用/丢弃子智能体产生的 ChangeSet。

整体链路如下：

~~~text
用户请求
  -> ChatService 自动意图分类
  -> 自动 Plan 生成（只读规划）
  -> Plan 保存到 Parent Session.CurrentPlan
  -> ExecutionService 执行依赖就绪步骤
  -> 模型生成 / 工具调用
  -> spawn_agent 创建 Child Task
  -> Scheduler 异步调度
  -> Child Session + Child Run
  -> TaskEvent 事件总线
  -> Agent / Relay WebSocket
  -> Mobile 子智能体面板
~~~

主要源码入口：

| 层次 | 入口 |
|---|---|
| 自动规划 | core/service/auto_plan_classifier.go、core/service/chat.go |
| 计划执行 | core/application/chat/plan/service/execution_service.go |
| 子智能体工具 | core/tool/local/subagent_tools.go |
| 子智能体领域对象 | core/domain/subagent/definition.go、core/domain/subagent/task.go |
| 子智能体应用服务 | core/application/subagent/service/task_service.go、task_wait.go |
| 调度器 | core/adapter/subagent/local/scheduler.go |
| 跨进程执行租约 | core/application/subagent/service/task_lease.go、core/adapter/persistence/mongo/subagent/repository/repository.go |
| 子 Session 工厂 | core/adapter/subagent/session/factory.go |
| 事件总线 | core/adapter/subagent/events/bus.go |
| 远程协议 | core/remote/protocol/message.go、core/remote/agent/subagent_handlers.go |
| 移动端状态和操作 | mobile/src/hooks/useSubagentState.ts、useSubagentActions.ts |
| 移动端界面 | mobile/src/components/subagents/SubagentPanel.tsx |

## 2. 自动规划（Auto Plan）

### 2.1 开关与作用范围

service.ChatDependencies.AutoPlanEnabled 控制根用户会话是否自动进入“规划 -> 执行”链路。当前组合配置在 core/composition/chat/configuration.go 中开启：

~~~go
return service.ChatDependencies{
    AutoPlanEnabled: true,
    AutoPlanClassifier: service.ModelAutoPlanClassifier{
        Models: configuration.Models,
        Metadata: configuration.Models,
    },
    // ...
}
~~~

该开关只影响根用户会话：

- 根用户会话可以自动规划并执行。
- session.KindSubagent 会话不会再次触发 Auto Plan，避免子智能体递归进入完整规划流程。
- 明确的问题解释、可行性咨询和普通聊天仍走普通 Chat。

### 2.2 分类结果

分类器返回固定的三类意图：

~~~text
conversation    普通聊天、创作或未充分明确的请求
explanation     为什么、如何、能否、是否等解释/可行性问题
implementation   要求实现、修改、修复、构建或变更项目
~~~

对应结构：

~~~go
type AutoPlanDecision struct {
    Intent     AutoPlanIntent
    ShouldPlan bool
    Confidence float64
    Reason     string
}
~~~

ShouldPlan=true 必须同时满足：

- 意图为 implementation。
- 请求有足够的代码/项目上下文。
- 分类置信度达到阈值。

### 2.3 两级分类策略

1. RuleBasedAutoPlanClassifier

   - 识别中文和英文的实现动作词，例如“实现、添加、修改、修复、重构、升级、构建、implement、add、fix、refactor”等。
   - 检查“代码、项目、接口、安卓、WebSocket、测试”等项目上下文。
   - 对“为什么、如何、是否可以”等问题默认走解释路径。
   - 子智能体会被明确排除在自动规划之外。

2. ModelAutoPlanClassifier

   - 只对规则无法确定、但当前 Session 有项目上下文的短请求调用模型分类。
   - 使用独立、短超时、无工具的模型请求。
   - 只发送最近的用户消息，不把工具输出或历史助手文本作为分类指令。
   - 只接受固定 JSON 形状：

~~~json
{
  "intent": "implementation|explanation|conversation",
  "should_plan": true,
  "confidence": 0.92,
  "reason": "用户明确要求修改项目"
}
~~~

分类阶段不执行工具、不写文件、不修改 Session。分类失败时安全降级为普通 Chat，避免模型分类故障意外触发写操作。

### 2.4 一条消息内的规划与执行

ChatService.SendMessageStreamForSession 的关键流程：

~~~text
加载当前 Session
  -> AutoPlanClassifier.Classify
  -> AppendUserMessage(ForcePlanMode=ShouldPlan)
  -> ShouldPlan=true ? generateAndExecutePlan : 普通生成
~~~

generateAndExecutePlan 会：

1. 克隆当前 Session 作为 planningSession。
2. 将规划 Session 设置为 AgentModePlan。
3. 使用“autonomous planning”原因执行一次只读规划。
4. 规划回复通过 OnPlanUpdate 传递，避免把规划文本拼接到最终助手答复中。
5. 规划结果解析为 Plan 并保存到父 Session。
6. 立即调用 PlanExecution.Execute 执行计划。

因此，用户不需要手动点击一次“生成计划”、再点击一次“执行计划”；但显式 Plan 模式仍然保留，便于用户先审阅计划再执行。

## 3. Plan 数据结构与状态机

Plan 保存在 Session.CurrentPlan。它同时保留模型原始内容和可执行的结构化步骤：

~~~go
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
    ID           string
    Order        int
    Title        string
    Description  string
    Dependencies []string
    Status       string
    RetryCount   int
    MaxRetries   int
    LastError    string
    AgentTaskID  string
    StartedAt    *time.Time
    CompletedAt  *time.Time
}
~~~

Plan 状态：

~~~text
draft -> approved -> running -> done
                         |  \
                         |   -> failed
                         -> canceled（可恢复）
~~~

步骤状态：

~~~text
pending -> running -> done
                    -> failed
                    -> skipped
~~~

计划输入支持两种格式：

- 新格式：JSON steps 数组，支持 id、order、title、description、depends_on/dependencies、max_retries。
- 兼容格式：Plan/计划标题下的 Markdown 列表。

单个 Plan 最多提取 12 个步骤。带依赖关系的新结构化 Plan 可以并行执行；没有依赖元数据的历史 Plan 为保持兼容，仍按顺序执行。

## 4. Plan 执行器

### 4.1 执行入口

核心实现是 core/application/chat/plan/service/execution_service.go 的 ExecutionService.Execute。

执行器首先：

1. 加载 Session.CurrentPlan。
2. 检查计划是否存在、是否有步骤、状态是否可执行。
3. 必要时把 draft 自动批准为 approved。
4. 创建一个父 AgentRun，并写入 ParentRunID、PlanID、总步骤数。
5. 将计划置为 running 并持久化。

### 4.2 依赖就绪批次

每轮执行都会查找 pending 且依赖已满足的步骤：

~~~text
ready = pending steps whose dependencies are done/skipped
  -> 限制本轮并行数量
  -> 全部标记 running
  -> 并行生成
  -> 合并结果
  -> 成功步骤标记 done
  -> 继续下一轮
~~~

当前组合配置为 MaxParallelSteps: 3。但以下情况会自动保持串行：

- 计划没有依赖元数据。
- 计划来自旧版 Markdown 解析，无法确认是结构化计划。
- 并行批次中的多个分支同时失败。

并行执行时，模型和工具运行可以并发；WebSocket 写入、权限询问和 UI 回调通过同步包装器串行化，避免客户端收到交错或破坏顺序的流事件。每个并行步骤的推理和回答先缓冲，步骤完成后按步骤顺序刷新到外层流。

### 4.3 重试与恢复规划

单步骤失败后的处理顺序：

1. 如果 RetryCount < MaxRetries，增加重试次数并重新执行该步骤。
2. 否则调用 RecoveryPlanner，默认实现为 GenerationRecoveryPlanner。
3. Recovery Planner 使用只读 Plan turn 生成替代/后续步骤，不在恢复阶段直接修改文件。
4. 将失败步骤标记为 skipped，合并新的替代步骤，再继续执行。
5. 超过 MaxReplans 或恢复规划失败时，计划进入 failed。

当前组合配置 MaxReplans: 1。取消会被视为可恢复的暂停，保留未完成步骤状态，后续可以继续执行。

## 5. 子智能体工具

工具实现位于 core/tool/local/subagent_tools.go。所有工具都通过当前执行上下文获取父 Session、父 Task、父 Run、Plan、Step、Workspace 和 Model 信息。

### 5.1 工具清单

| 工具 | 作用 | 是否等待 |
|---|---|---|
| list_subagent_definitions | 查看可用子智能体定义和能力 | 否 |
| start_async_task | 使用指定 Definition 创建后台任务 | 否，立即返回 |
| spawn_agent | Codex 兼容的子智能体创建入口 | 否，立即返回 |
| send_message | 原子写入 AgentMessage 和子任务 mailbox 投影 | 不等待模型消费 |
| followup_task | 在原 ChildSession 中创建新的 Run，保留旧 Run | 等待上一 Run 收尾，不等待新 Run 完成 |
| check_async_task | 查询单个任务状态/结果 | 否 |
| list_async_tasks | 列出当前父会话任务树 | 否 |
| list_agents | 以 agent 视图列出当前父会话子任务 | 否 |
| wait_agent | 等待 `targets` 中任一子任务进入终态，或等待超时 | 是，显式等待 |
| cancel_async_task | 取消排队或运行中的任务 | 调用方不等待模型完成 |
| interrupt_agent | 中断运行中的子智能体 | 调用方不等待模型完成 |
| apply_task_changes | 应用隔离 Workspace 的 ChangeSet | 否 |
| discard_task_changes | 丢弃隔离 Workspace 的 ChangeSet | 否 |

### 5.2 spawn_agent 参数

~~~json
{
  "message": "独立、完整的子任务说明",
  "task_name": "子任务名称",
  "definition_id": "implementer",
  "agent_type": "可选的显示昵称或兼容字段",
  "model": "可选模型 ID"
}
~~~

参数规则：

- message 和 task_name 必填。
- definition_id 为空时优先使用 agent_type，仍为空则默认 researcher。
- model 为空时继承当前执行上下文的模型。
- researcher 是安全的只读默认角色；可写角色必须显式选择。
- 工具返回任务摘要和 task_id，不会把子任务最终结果同步塞回当前模型请求。

start_async_task 是底层显式 Definition 入口；spawn_agent 是对齐 Codex 语义的友好入口。两者最终都调用 subagent.Service.Start。

### 5.3 同一请求禁止立即轮询

创建任务的请求会记录 CreatedRequestID。如果任务仍未结束，check_async_task 在创建它的同一个模型请求中轮询会被拒绝；工具描述也明确要求不要在创建请求中调用查询工具。

这样做是为了保证：

- 创建子智能体是真正的异步操作。
- 父模型不会陷入“创建 -> 立即轮询 -> 再轮询”的忙等循环。
- 子任务有机会在独立 Scheduler worker 中运行。

如果父模型确实需要等待结果，应在后续请求中调用 wait_agent，或由用户在移动端点击等待/继续。

### 5.4 send_message 投递语义

- 非终态任务的消息以 `AgentMessage.ID` 为稳定身份，记录发送者、接收 ChildSession、父 Turn、根 Agent、Kind 和 Trigger。远程入口使用请求 ID；未显式提供 ID 的本地调用生成新 ID。终态 `trigger_turn` 不再额外创建 mailbox envelope，而是把消息 ID作为 follow-up Run 的 RequestID，由 Run 的请求哈希提供同等幂等保证。
- `queue` Trigger 只把消息放入 durable mailbox，在子智能体当前执行边界后消费，不会抢占正在进行的模型调用。
- `trigger_turn` 在非终态任务上沿用 mailbox 投递，并保留触发语义；在成功或失败的终态任务上复用同一 Child Session 创建 follow-up Run。消息 ID作为 Run 的 RequestID，重试时按 RequestID 和内容幂等，不会重复创建 Run。
- `steer_current_turn` 仍未接入运行中模型的实时输入通道，因此会明确拒绝，不会伪装成已经完成实时转向。
- 仓储在同一事务中创建 AgentMessage 并将同 ID 输入放入 Task mailbox；同 ID 同请求重试不会重复入队，元数据或内容不同则拒绝。
- 领取时两处状态同步为 Delivering；模型成功且有非空结果后，在同一事务中移除 mailbox 项并把 AgentMessage 标记 Delivered。模型失败或 panic 时恢复 Pending，保留投递次数及错误。
- 注入 Child Session 的用户消息沿用 AgentMessage ID；如果失败重试时该消息已存在于会话中，不再追加同 ID 的第二份输入。启动时只有 task_result/task_error 会被重新投递到父会话，普通子代理输入由 Task mailbox 恢复。

## 6. Task、Run 与 Child Session

### 6.1 创建时的对象关系

task_service.go 在接收 StartTask 时一次性建立：

1. Task ID：子任务的稳定身份。
2. Run ID：本次执行尝试的身份，初始为第 1 次 Run。
3. Child Session ID：子智能体实际使用的会话。
4. 父关系：ParentSessionID、ParentTaskID、ParentRunID。
5. 计划关系：PlanID、StepID。
6. DefinitionSnapshot：冻结创建时的 Definition 版本。
7. AgentPath、AgentNickname：用于显示嵌套层级和角色名称。
8. Workspace Reference：直接工作区或隔离工作区的引用。

关系图：

~~~text
Parent Session
  |
  +-- Parent AgentRun
        |
        +-- Child Task
              |
              +-- Child Run (当前创建时 sequence=1)
              +-- Child Session
              +-- Workspace Reference
              +-- ChangeSet
~~~

嵌套 spawn_agent 时，当前执行上下文中的 TaskID 用作新任务的 ParentTaskID，而不是使用 Child Session 的 ParentTaskID。这保证孙任务会挂在实际创建它的子任务下面，而不会错误地直接挂到祖父任务。

### 6.2 Child Session 的初始化

core/adapter/subagent/session/factory.go 创建子 Session 时会固定：

- Kind = subagent。
- ParentSessionID、ParentTaskID。
- Definition ID 和 Definition Version。
- 系统提示词和工具白名单。
- EnforceToolAllowlist = true。
- WorkspaceRoot、WorkspaceSandboxID。
- Model、MaxToolRounds。
- 只读能力对应 PermissionModeReadonly，其他能力对应完整权限模式。
- RAGSettings.Mode = off，避免子任务无意读取父会话检索上下文。

子 Session 使用和根会话相同的 Chat Runner，但由于 Session 类型为 subagent，不会递归触发 Auto Plan。

## 7. Definition、权限与 Workspace 隔离

### 7.1 Definition

Definition 是子智能体的能力模板，包含：

~~~go
type Definition struct {
    ID             string
    Name           string
    Description    string
    SystemPrompt   string
    ModelID        string
    AllowedTools   []string
    CapabilityMode CapabilityMode
    IsolationMode  IsolationMode
    MaxTurns       int
    TimeoutSeconds int
    Enabled        bool
    Version        int64
    Source         DefinitionSource
}
~~~

内置 Definition：

| ID | 用途 | 能力 | 隔离 |
|---|---|---|---|
| researcher | 调查代码、知识库和现状 | read_only | direct |
| code-reviewer | 检查缺陷、风险和测试覆盖 | read_only | direct |
| implementer | 执行一个完整实现步骤并验证 | all | snapshot |

Definition 的版本会保存到任务快照中。后续修改 Definition 不会改变已经排队或运行中的任务。

### 7.2 工具白名单和能力模式

子 Session 始终启用工具白名单校验。Definition 同时限制：

- 可见工具名称。
- 能力模式：read_only、read_write、execute、all。
- 最大模型轮数。
- 单任务超时。

工具执行器会把当前 TaskID、PlanID、StepID 和 Workspace 信息写入执行上下文，因此嵌套任务、事件和审计记录可以正确关联。

### 7.3 可写任务必须隔离

服务层拒绝“可写能力 + direct 工作区”的组合：

~~~text
read_only + direct       允许
read_only + snapshot     允许
writable + snapshot      允许
writable + direct        拒绝
~~~

可写子智能体的执行顺序：

~~~text
Prepare isolated workspace
  -> Child Session 在隔离目录执行
  -> Collect ChangeSet
  -> Task 标记 succeeded
  -> 用户/父任务选择 Apply 或 Discard
~~~

失败或取消的隔离工作区会自动清理。成功任务的变更不会自动写回源目录。

## 8. Scheduler 与异步执行

### 8.1 调度器模型

core/adapter/subagent/local/scheduler.go 使用固定数量 worker 和有界队列：

~~~text
Service.Start
  -> Scheduler.Submit(runID, job)
  -> 立即返回任务摘要
  -> worker 从 queue 取 job
  -> 独立 context 执行
~~~

当前调度器的行为：

- 默认至少 2 个 worker。
- 默认队列容量为 32（由构造参数决定）。
- 同一 Run 不能重复提交；同一 Task 的不同 Run 使用不同调度键。Service 负责串行化同一 child 的执行，等待旧 Run 收尾后才接纳新 Run。
- 队列满时，提交失败并将任务标记为失败。
- 外部仍按 task_id 取消，Service 解析 CurrentRunID 后取消对应执行。
- Close() 会取消活动任务、关闭队列并等待 worker 退出。
- worker 捕获 panic，避免单个子任务导致进程崩溃。
- wait_agent 会 Park 当前 Run：释放执行额度，让排队中的后代获得 worker；等待结束后 Unpark 再继续父任务。同一 Run 上的多次等待按引用计数处理。
- active-run 的登记、释放和终态写入均核对 Run 身份，旧 Run 不能结束或移除新 Run。
- queued 事件在调度前发布；调度失败重新读取当前 Task，在保留并发 mailbox/取消状态的基础上提交失败结果。
- Mongo 模式通过以 task_id 为唯一键的执行租约限制同一 Task 只有一个活动 Run owner；租约默认 30 秒，执行期间每约 10 秒续租。内存模式使用相同接口但只在单进程内有效。
- Task/Run 状态转移在 Mongo 事务中检查未过期的 owner/run 租约；失去租约的旧实例不能提交终态。续租失败会取消本地模型/工具执行。
- 启动时和之后每 10 秒扫描未完成任务。仍被其他实例持有的任务跳过；租约释放或过期后重新入队。Scheduler 本身仍是进程内队列。

### 8.2 子任务执行步骤

TaskService.execute 的实际流程：

1. queued -> running，写入开始时间和 Run 状态。
2. 发布 task.started 事件。
3. 准备隔离 Workspace（direct 模式跳过）。
4. 创建或恢复并复用 Child Session；已有 OpenSandbox 引用直接复用，命令执行时按需重连。
5. 调用 chat.Runner.Run，进入普通 Chat 生成/工具调用循环。
6. 通过任务流把推理、答案、工具调用和工具结果转换成 TaskEvent。
7. 每个工具批次完成后、下一轮模型调用前 claim mailbox，把待处理父输入追加为 Child Session 的 user 消息；此时仍是 Delivering，不取消正在执行的工具。下一次模型成功返回非空结果后才 ack；失败或 panic release。Runner.Run 返回后仍会消费剩余 mailbox（本轮没有工具边界的情况），同样成功 ack、失败 release。
8. 收集隔离 Workspace 的 ChangeSet。
9. 成功则保存结果并标记 succeeded。
10. 超时、取消、模型错误或 panic 则标记 failed 或 canceled。

终态后 Child Session 会先持久化再从内存卸载；后续 follow-up 使用同一个 ChildSessionID 重新加载。Run 的 token usage 单独持久化，Task 摘要中的 usage 是该子任务历次 Run 的累计值，不并入父会话的 usage。

每个任务有 Definition 配置的超时；当前内置实现器默认最多 900 秒，研究/审查任务默认 300 秒。

## 9. TaskEvent 事件流

### 9.1 事件类型

core/port/subagent/event.go 定义了以下事件：

~~~text
task.updated
task.started
task.reasoning
task.answer
task.tool_call
task.tool_result
task.permission
task.completed
task.canceled
task.failed
~~~

每个事件包含：

~~~go
type TaskEvent struct {
    Sequence     uint64
    Kind         string
    Task         subagent.Task
    RunID        string
    Content      string
    ToolName     string
    Arguments    string
    Status       string
    ErrorCode    string
    ErrorMessage string
    Truncated    bool
    Delta        bool
    EmittedAt    time.Time
}
~~~

### 9.2 顺序、历史与重连

core/adapter/subagent/events/bus.go 的 Bus 负责：

- 全局递增 Sequence。
- 内存历史窗口（默认保留最近 512 个事件）。
- 按 ParentSessionID 过滤订阅。
- 新订阅时根据 afterSequence 回放历史。
- 可选接入 TaskEventRepository，把事件持久化到 Mongo 等存储。
- 进程重启时恢复持久化尾部并继续递增序列。
- 慢订阅者不会静默丢事件；其 channel 会被关闭，客户端应使用最后序列号重新订阅。

### 9.3 远程转发

Agent 启动后会订阅子智能体事件，并将事件包装为 subagent_task_event 发往 Relay。终态任务还会通过 subagent_task_result 形式携带完整任务摘要，包含结果、错误、未读状态和 ChangeSet。

Agent 会在进程内记录最后一个成功写入 WebSocket 的 TaskEvent 序列号。连接重建时，使用该序列号调用 `SubscribeTaskEvents("", afterSequence, 32)`，由事件总线先回放断线期间的历史事件，再继续接收实时事件。序列号只有在对应消息成功写入连接后才推进，因此连接写失败不会造成游标跳过。

如果游标已经早于内存窗口，事件总线会从 `TaskEventRepository` 读取持久化日志；如果持久化日志也超出可回放上限，Agent 会发送 `task.snapshot` 快照事件，携带当前任务树摘要，然后从快照期间捕获的最新游标重新订阅，避免全量同步期间跳过新事件。

事件流不是模型请求的阻塞返回值：即使根请求已经结束，后台子任务仍可以继续产生事件并经 Relay 推送到手机端。

## 10. 等待、阻塞、取消与恢复

### 10.1 创建任务是否阻塞

默认不阻塞：

~~~text
spawn_agent/start_async_task
  -> 创建 Task + Run + Child Session ID
  -> Submit 到 Scheduler
  -> 立即返回 task_id
~~~

父模型可以继续当前请求的后续工作，或者结束请求。子任务在独立 worker 中运行。

### 10.2 wait_agent 的语义

wait_agent 是唯一明确的等待入口：

- 参数使用 `targets: string[]`，一次可以等待多个子任务；服务在任一目标进入终态时返回。现有单目标调用仍可使用 `task_id` 作为兼容别名。
- 订阅 TaskEventSource，而不是高频轮询数据库。
- 目标子任务已处于终态时立即返回；如果初始快照中有多个终态目标，会一并返回。
- 收到任一目标任务的终态事件时返回，结果的 `tasks` 包含已观察到的终态目标，`task` 是第一个目标的快捷字段。
- 超时返回所有目标的当前任务快照，并设置 `timed_out=true`；这不等于任务失败，任务可能仍在后台运行。
- 父子任务等待时，父任务暂时变为 waiting_subagents，并让出 Scheduler 执行额度。同一 Run 的最后一个等待者退出后才恢复 running，旧 Run 的等待者不能更改新 Run 状态。
- 每次等待在同一锁内注册并取得独立唤醒 channel；消息会唤醒所有已注册等待者。注册前已存在的 pending 消息也立即唤醒，正在 delivering 的消息不会唤醒自己的等待。
- `waiting_subagents` 状态下可以接收 mailbox 消息，等待结果返回 `woken_by_mailbox=true`。等待结束后，当前工具批次完成、下一轮模型调用前会把 mailbox 内容注入 Child Session。`waiting_permission` 拒绝 mailbox 消息，移动端不显示发送入口。
- 事件流被关闭时返回错误，调用方应使用最后的序列号重新连接/订阅。

### 10.3 终态与未读结果

终态包括：

~~~text
succeeded
failed
canceled
~~~

成功或失败会将 Unread=true，提醒父会话/移动端有新结果。取消任务不会标记为未读。

### 10.4 恢复父会话

移动端点击“继续主任务”会发送 subagent_task_resume：

1. 服务校验任务属于当前父 Session。
2. 任务必须是 succeeded 或 failed，并且 Unread=true。
3. 用 claim 防止同一个结果被并发消费两次。
4. 将结构化任务报告作为 Synthetic Message 注入 Parent Session。
5. 以普通 Chat continuation 继续父会话。
6. 成功后清除 Unread；继续失败则恢复 Unread=true。

恢复只会继续主会话上下文，不会自动应用隔离 Workspace 的文件变更。文件变更仍必须明确调用 Apply。

### 10.5 继续子会话

`followup_task` / `subagent_task_followup` 与恢复父会话是不同操作：它复用 ChildSession，为 child 创建新的 Run，并保存旧 Run 历史。

- 仅接纳 succeeded/failed 任务；取消任务不支持续接。父 continuation 消费结果期间不能续接。
- direct 与隔离工作区都支持成功或失败后的续接。Task 会保存原始 SourceRoot；Apply 后下一次 Prepare 把当前快照设为新基线；Discard 或失败清理后按 SourceRoot 重建隔离工作区。
- 续接会同步 ChildSession 的 WorkspaceRoot 和 SandboxID。OpenSandbox 在 Apply/Discard 后释放远程会话，下一轮重新创建。
- 没有可恢复 SourceRoot、且工作区已经消失时，仍会拒绝续接。
- 非空 `request_id` 与原始输入的 SHA-256 摘要一起持久化到 Run；相同 task/request/content 的重试返回当前 Task，不重复创建或调度 Run；同 ID 不同内容报错。重启恢复改写执行指令不会影响原请求身份。空 request_id 不保证幂等，使用新 ID 才表示新一轮请求。
- 输入上限为 20000 个 Unicode 字符。调度失败返回已经持久化的 failed Task。
- 工具摘要和远程摘要包含 `can_followup`，移动端据此显示入口。这是持久化状态的资格判断，实际工作区可用性仍在接纳时检查。
- 本地 Runtime Manager 防止同一进程重复执行；跨进程的活动 Run 由执行租约与带租约校验的 Task/Run 事务约束。取消使用不抢执行租约的条件提交，Apply/Discard/Resume 使用短时操作租约和 `UpdatedAt`/`CurrentRunID` 版本校验；等待状态和部分恢复路径仍保留专用 CAS/调度语义，不能把所有 Task 写入都视为同一种操作。

## 11. Workspace ChangeSet

任务完成后，隔离 Workspace 会生成 ChangeSet，包含：

~~~text
workspace_id
status: pending / applied / discarded / conflict
files[]: path、change_type、before_hash、after_hash、size
checkpoint_id
created_at / applied_at / discarded_at
message
~~~

应用规则：

- apply_task_changes 只允许成功任务。
- direct 模式任务没有隔离变更，不能调用 Apply。
- Apply 时会检查 Workspace 和源目录冲突，并持久化最终状态。
- discard_task_changes 可以丢弃已结束任务的隔离变更。
- 子智能体成功不等于源目录已经修改。

这项设计将“子智能体完成了什么”和“用户是否接受这些文件变更”分开，避免后台任务直接覆盖用户当前工作区。

## 12. Relay 与 Mobile 协议

协议类型定义位于 core/remote/protocol/message.go。当前相关消息如下：

~~~text
subagent_definition_list
subagent_definition_list_result
subagent_definition_create
subagent_definition_update
subagent_definition_delete
subagent_definition_mutation_result

subagent_task_list
subagent_task_list_result
subagent_task_check
subagent_task_message
subagent_task_followup
subagent_task_wait
subagent_task_wait_result
subagent_task_cancel
subagent_task_apply
subagent_task_discard
subagent_task_resume
subagent_task_resume_result
subagent_task_event
subagent_task_result
~~~

`subagent_task_wait` 请求的后端载荷支持 `session_id`、`parent_task_id`、`targets`、`timeout_ms`；返回载荷除了单任务快捷字段 `task` 外，还包含多目标结果数组 `tasks`。Relay 只负责转发，不参与等待判断。

### 12.1 移动端调用

mobile/src/hooks/useSubagentActions.ts 将 UI 操作映射为 Relay 请求：

~~~text
刷新配置       -> subagent_definition_list
刷新任务       -> subagent_task_list
查看任务       -> subagent_task_check
发送 follow-up -> subagent_task_message
继续已结束任务 -> subagent_task_followup
等待任务       -> subagent_task_wait (默认 30 秒窗口；后端支持多个 targets)
取消任务       -> subagent_task_cancel
应用变更       -> subagent_task_apply
丢弃变更       -> subagent_task_discard
继续主任务     -> subagent_task_resume
~~~

mobile/src/hooks/useSubagentState.ts 负责：

- 保存 Definition 列表。
- 保存任务列表。
- 按任务 ID 保存事件时间线。
- 用 sequence 去重和排序。
- 保留最近 256 个任务事件。
- 收到完整任务结果时同步更新任务状态。

mobile/src/components/subagents/SubagentPanel.tsx 提供：

- Definition 创建、编辑、删除。
- 能力模式和隔离模式展示。
- 任务状态、未读标识和事件时间线。
- 任务树展示嵌套子任务。
- 等待、取消、继续主任务、Apply、Discard 操作。

## 13. 一次完整请求的示例

用户发送：

~~~text
给 mobile 增加一个设置页，并补充后端接口和测试。
~~~

系统行为：

~~~text
1. AutoPlanClassifier -> implementation / should_plan=true
2. 只读规划模型生成结构化 Plan：
   - 设计接口
   - 修改 Go 后端
   - 修改 Mobile 设置页
   - 补充测试
3. PlanExecutionService 开始执行
4. 某个后端步骤调用 spawn_agent：
   - definition_id=implementer
   - task_name=实现后端接口
   - message=独立、完整的接口实现说明
5. spawn_agent 立即返回 task_id
6. Scheduler 在隔离 Workspace 中执行 Child Session
7. Mobile 收到 queued/running/reasoning/tool/completed 事件
8. 子任务成功后生成 ChangeSet，状态为 pending
9. 用户点击“继续主任务”时，结果报告注入父会话
10. 用户确认后调用 Apply，ChangeSet 才写回源工作区
~~~

如果用户发送的是“为什么要用 WebSocket？”或“这个功能能不能实现？”，分类结果为 explanation，不会自动修改代码。

## 14. 当前实现边界

当前已经支持：

- 根会话自动意图分类。
- 一条请求内自动“规划 -> 执行”。
- Plan 步骤依赖、并行批次、重试和一次恢复规划。
- spawn_agent 创建异步子智能体。
- 嵌套父子任务关系和完整任务树查询。
- 独立 Child Session、Child Run 和 Definition 快照。
- 子任务超时、取消、失败和 panic 保护。
- TaskEvent 实时推送、序列号、回放和可选持久化。
- 等待、状态查询、结果恢复父会话。
- AgentMessage 与 Task mailbox 的原子入队/领取/确认/释放、并行等待唤醒、原子续接接纳，以及以 request_id 去重的独立 follow-up Run。
- Mongo 模式的跨进程执行租约、续租、失租取消和事务化 Task/Run 状态提交；失效租约由周期扫描恢复。
- 终态 Child Session 卸载、后续 follow-up 重载，以及子任务累计/每 Run 的 token usage 持久化。
- wait_agent 等待期间让出 Scheduler worker，避免父任务阻塞导致后代无法运行。
- mailbox 在工具批次完成后、下一轮模型调用前注入 Child Session，保留 claim/ack/release。
- 隔离 Workspace 支持 Apply 后换基线、Discard/失败后重建，以及同一 ChildSession 的多轮修改。
- 可写子智能体的隔离 Workspace 和 ChangeSet Apply/Discard。
- Relay/WebSocket 与 Mobile 面板联动。

需要继续完善或使用时注意：

- 当前没有完全自动的多任务 DAG 结果合并器；父模型需要通过任务结果或 wait_agent 自己汇总。
- spawn_agent 创建后不会在同一模型请求中立即轮询，必须后续请求或显式等待。
- 子智能体成功后不会自动把文件变更写回源目录，必须明确 Apply。
- 内存事件历史有窗口限制；需要跨进程可靠恢复时必须配置 TaskEventRepository。
- 远程 Agent 已按最后成功发送的序列号恢复事件订阅；如果断线时间超过事件总线的内存/持久化回放窗口，会自动推送当前任务树快照并从快照游标继续订阅。
- 配置 TaskEventRepository 时，TaskEvent 序列号由持久化仓库的原子序列分配器统一分配；如果序列分配器暂时不可用，Bus 会跳过该通知，等待后续事件或任务快照修复远程视图。
- Android 后台服务只能尽力保持 Relay 长连接；强行停止应用、厂商电池策略或系统资源回收仍可能终止服务。
- Scheduler 是进程内调度器；进程退出后旧 worker 不会继续运行，Mongo 模式由周期扫描在租约到期后重排任务。没有持久化队列/外部调度器，重排延迟至少受租约和扫描周期影响。
- 恢复扫描遇到 scheduler 队列满或已关闭时会保留任务为 queued、释放恢复租约并等待下一轮重试，不会把暂时的调度器背压错误误记为终态失败。
- `waiting_subagents` 只在进程内保存 waiter；进程重启后的恢复会把它重新排为 running，由新的 Run 重新读取持久化 Task/mailbox/event 状态，避免父任务永久停留在等待状态。
- 跨实例取消已使用不抢执行租约的条件 Task/Run 原子提交，并由 lease owner 周期观察持久化取消状态；Apply/Discard、父会话结果消费等其它 Task-only 写入仍需要更细粒度的 CAS/操作租约，不能把执行租约理解为所有子代理操作都已跨进程安全。
- Apply/Discard/Resume 在获取短时操作租约后会重新读取并校验 Task 版本、CurrentRunID 和终态，避免等待租约期间使用过期快照；工作区适配器仍必须保证同一 Workspace 操作的重试幂等，真实 Mongo/多实例恢复需集成压测。
- 已实现等待让出执行额度；父任务配额、优先级和公平调度仍待补充。
- mailbox 会在工具批次之间注入；不会打断正在执行的工具，也不会在模型生成 token 的过程中插入。
- 父会话完成消息采用 `Pending -> Delivering -> Pending -> Delivered`：恢复器只负责幂等入队，父会话生成循环在成功消费并确认输入后才 ACK，避免进程在入队和实际消费之间崩溃导致结果丢失。

### 14.1 设计检查记录（2026-09-18）

下面保留当时的检查记录；其中调度恢复和父会话输入队列的状态已随后续实现改变，应以本节上方的当前边界为准。

1. 调度器是进程内的。`Scheduler.Submit` 使用自己的 `context.Background()`，默认 2 个 worker、队列 32。Mongo 模式现在能在租约过期后扫描重排，但调度队列本身仍不可恢复。
2. 父任务取消不会自动取消仍在跑的子任务。子任务生命周期不跟着父 Turn。
3. 父会话中途插话和子任务 mailbox 是两套执行队列，但现均有 AgentMessage 持久化身份；子任务 mailbox 是消息的 Task 投影，并在工具批次之间注入。
4. `spawn_agent` 立即返回 `task_id`。父模型必须再调 `wait_agent`，否则主回复里只有任务编号。
5. 只读角色 `researcher` 的工具列表包含 `spawn_agent`。有深度 4、扇出 8 的限制，但仍可能套出一串只读子任务。
6. 多数 Task 操作共用 `service.mu`。子任务变多时，创建、等待和 Apply 会互相等待。
7. 子任务成功不会自动 Apply。这是刻意的，不要改成成功即写回源目录。

## 15. 相关测试与验证

2026-09-13：工具批次间 mailbox 注入后，`go test -p 1 ./...`、`go vet ./...`、移动端 `npm run typecheck`、`git diff --check` 均通过。本轮未执行 race 检查或真实 OpenSandbox 服务集成测试。

`core/application/subagent/service/task_concurrency_test.go` 覆盖等待唤醒、等待让出 worker、Run 身份隔离、续接去重、调度失败、工作区准备并发更新，以及 Apply/Discard/失败后的真实磁盘快照续接。

2026-09-27：子代理相关包的定向测试、全量 `go test -p 1 ./...`、`go vet` 和 `git diff --check` 通过；新增 `task_lease_test.go` 覆盖跨实例租约冲突、旧 owner 写入拒绝、失租取消及任务恢复重试，新增 mailbox/envelope 测试覆盖幂等入队和模型失败重投。全量测试使用 D 盘 Go 临时目录以规避 Windows 默认 C 盘构建目录空间不足；Mongo 原子租约和消息事务尚未在真实多实例 Mongo 环境做集成压测。

修改子智能体机制后，建议在 D:\Go_All\myai 执行：

~~~powershell
go test ./core/application/subagent/... ./core/adapter/subagent/... ./core/remote/agent/...
go test ./core/application/chat/plan/...
git diff --check
~~~

重点测试覆盖：

- core/service/chat_auto_plan_test.go：自动规划分类和失败降级。
- core/application/chat/plan/service/execution_service_test.go：步骤依赖、并行、重试、恢复和取消。
- core/application/subagent/service/task_service_test.go：创建、执行、取消、恢复和 ChangeSet。
- core/application/subagent/service/task_wait_test.go：事件驱动等待和超时。
- core/remote/relay/server_test.go：子任务事件与 Resume 路由。
- mobile/src/hooks/useSubagentState.ts：客户端事件去重和状态合并。

## 16. 维护原则

1. 新增子智能体能力时，先修改 Definition/权限模型，再暴露工具和远程协议。
2. 所有后台任务必须携带父 Session、父 Task、父 Run、Plan 和 Step 元数据。
3. 事件序列号不可复用或回退；客户端以序列号去重和重连。
4. 可写子智能体不得绕过 Workspace 隔离直接修改源目录。
5. 自动规划分类失败必须 fail closed，回到普通 Chat。
6. 子智能体结果和文件变更是两个独立确认点，不要把“任务成功”当成“变更已应用”。
7. 修改协议时同时更新 Go payload、Relay 路由、Agent handler、Mobile protocol、Hook 和 UI。
