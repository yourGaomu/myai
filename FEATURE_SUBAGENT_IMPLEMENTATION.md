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
| send_message | 将消息持久化到排队、运行或等待子任务的 mailbox | 不等待模型消费 |
| followup_task | 在原 ChildSession 中创建新的 Run，保留旧 Run | 等待上一 Run 收尾，不等待新 Run 完成 |
| check_async_task | 查询单个任务状态/结果 | 否 |
| list_async_tasks | 列出当前父会话任务树 | 否 |
| list_agents | 以 agent 视图列出当前父会话子任务 | 否 |
| wait_agent | 等待子任务进入终态或超时 | 是，显式等待 |
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
- active-run 的登记、释放和终态写入均核对 Run 身份，旧 Run 不能结束或移除新 Run。
- queued 事件在调度前发布；调度失败重新读取当前 Task，在保留并发 mailbox/取消状态的基础上提交失败结果。

### 8.2 子任务执行步骤

TaskService.execute 的实际流程：

1. queued -> running，写入开始时间和 Run 状态。
2. 发布 task.started 事件。
3. 准备隔离 Workspace（direct 模式跳过）。
4. 创建或恢复并复用 Child Session；已有 OpenSandbox 引用直接复用，命令执行时按需重连。
5. 调用 chat.Runner.Run，进入普通 Chat 生成/工具调用循环。
6. 通过任务流把推理、答案、工具调用和工具结果转换成 TaskEvent。
7. Runner.Run 返回后 claim mailbox，逐条执行后续输入，成功 ack，失败或 panic release；检查最终结果非空。
8. 收集隔离 Workspace 的 ChangeSet。
9. 成功则保存结果并标记 succeeded。
10. 超时、取消、模型错误或 panic 则标记 failed 或 canceled。

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

- 订阅 TaskEventSource，而不是高频轮询数据库。
- 子任务已处于终态时立即返回。
- 收到目标任务的终态事件时返回。
- 超时返回当前任务快照，并设置 timed_out=true；这不等于任务失败，任务可能仍在后台运行。
- 父子任务等待时，父任务暂时变为 waiting_subagents。同一 Run 的最后一个等待者退出后才恢复 running，旧 Run 的等待者不能更改新 Run 状态。
- 每次等待在同一锁内注册并取得独立唤醒 channel；消息会唤醒所有已注册等待者。注册前已存在的 pending 消息也立即唤醒，正在 delivering 的消息不会唤醒自己的等待。
- `waiting_subagents` 状态下可以接收 mailbox 消息，等待结果返回 `woken_by_mailbox=true`。这只结束等待；消息内容实际要到当前 `Runner.Run` 返回后才进入下一次执行，不保证在每次工具调用之间注入。`waiting_permission` 拒绝 mailbox 消息，移动端不显示发送入口。
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
- direct 工作区支持成功或失败后的续接；隔离工作区只允许成功且 ChangeSet 为 pending/conflict 的任务，还要查询 Workspace Manager 的实际状态。
- 已应用、已丢弃、缺失或执行失败后清理的隔离工作区目前必须新建任务。尚未实现保留源目录身份、重建工作区及新一轮变更基线。
- 非空 `request_id` 与原始输入的 SHA-256 摘要一起持久化到 Run；相同 task/request/content 的重试返回当前 Task，不重复创建或调度 Run；同 ID 不同内容报错。重启恢复改写执行指令不会影响原请求身份。空 request_id 不保证幂等，使用新 ID 才表示新一轮请求。
- 输入上限为 20000 个 Unicode 字符。调度失败返回已经持久化的 failed Task。
- 工具摘要和远程摘要包含 `can_followup`，移动端据此显示入口。这是持久化状态的资格判断，实际工作区可用性仍在接纳时检查。
- 当前锁和执行所有权是单 Service 进程内机制，尚不保证多个进程同时接管同一 Task 时的唯一执行。

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

### 12.1 移动端调用

mobile/src/hooks/useSubagentActions.ts 将 UI 操作映射为 Relay 请求：

~~~text
刷新配置       -> subagent_definition_list
刷新任务       -> subagent_task_list
查看任务       -> subagent_task_check
发送 follow-up -> subagent_task_message
继续已结束任务 -> subagent_task_followup
等待任务       -> subagent_task_wait (默认 30 秒窗口)
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
- 持久化 mailbox、并行等待唤醒、原子续接接纳，以及以 request_id 去重的独立 follow-up Run。
- 可写子智能体的隔离 Workspace 和 ChangeSet Apply/Discard。
- Relay/WebSocket 与 Mobile 面板联动。

需要继续完善或使用时注意：

- 当前没有完全自动的多任务 DAG 结果合并器；父模型需要通过任务结果或 wait_agent 自己汇总。
- spawn_agent 创建后不会在同一模型请求中立即轮询，必须后续请求或显式等待。
- 子智能体成功后不会自动把文件变更写回源目录，必须明确 Apply。
- 内存事件历史有窗口限制；需要跨进程可靠恢复时必须配置 TaskEventRepository。
- Android 后台服务只能尽力保持 Relay 长连接；强行停止应用、厂商电池策略或系统资源回收仍可能终止服务。
- Scheduler 是进程内调度器，进程退出时任务不能依赖它继续执行；需要跨进程任务恢复时应增加持久化队列/外部调度器。
- 固定 worker 在 wait_agent 阻塞期间仍被占用；若所有 worker 都等待尚在排队的后代任务，后代可能一直等到父任务超时才开始执行。后续需要等待期间让出执行额度，并加入父任务配额和公平调度。
- mailbox 尚未接入工具轮次间的即时输入机制；隔离工作区完成 Apply/Discard 后的同会话续接也仍待开发。

## 15. 相关测试与验证

2026-09-13：最终代码的 `go test -p 1 ./...`、`go vet ./...`、移动端 `npm run typecheck`、`git diff --check` 均通过。首次并行全量运行中，本地执行器的 `go version` 用例超时；独立复跑和后续全量串行测试均通过。本轮未执行 race 检查或真实 OpenSandbox 服务集成测试。

`core/application/subagent/service/task_concurrency_test.go` 覆盖等待唤醒、Run 身份隔离、续接去重、调度失败、工作区准备并发更新，以及真实磁盘快照的续接边界。

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
