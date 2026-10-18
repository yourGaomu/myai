# MyAI 开发变更记录

## 2026-08-31：自动 Plan、协议模型与远程运行可靠性

本次文档对应的源码已把“自动规划执行”、多协议模型和长连接运行边界补齐，以下行为以当前实现为准：

- `ChatService` 默认启用 `AutoPlanEnabled`，通过结构化分类器区分 conversation、explanation 和 implementation；实现类请求在同一条请求内先以只读 Plan 规划，再执行已捕获的步骤，分类失败时安全回退为普通 Chat，子智能体不会递归触发。
- `AutonomousPlanPrompt`、结构化 JSON Plan、步骤依赖、dependency-ready 批次、最多 3 个并行步骤、`MaxRetries`/`RetryCount` 和 Recovery planner 已进入 Plan 执行链；旧 Markdown Plan 仍兼容并按顺序执行。
- Plan/Step 持久化增加 `Revision`、依赖、重试、错误、时间戳和 `AgentTaskID` 等状态字段；Plan 执行的 AgentRun 通过 `ParentRunID`、`PlanID`、`StepID` 关联规划与步骤运行。
- Agent 连接 Relay 时使用 `Authorization: Bearer`、user/device headers；断线后按 1 秒起步、30 秒封顶的退避策略自动重连，并在每次新连接重新发送 `agent_online`。
- 子智能体改用带序列和父会话范围的 `TaskEvent` 事件流；异步 `subagent_task_event` 使用独立 request ID，普通请求响应仍复用原 request ID。
- 模型 Factory 已支持 OpenAI Chat Completions、Anthropic Messages、Google Generative AI、Mistral Chat 和 Ollama Chat 五种协议；`GenerateRequest` 携带解析后的生成参数。
- Mobile Android 前台 Relay Service 现在接收 WebSocket 地址、用户/设备身份和 client token，支持发送消息、读取连接状态、消息排空和状态/消息事件订阅。

验证命令：

```powershell
go test ./...
go vet ./...
cd mobile
npm run typecheck
```

## 2026-08-15：移动端响应式表单弹层

修复长列表中的内嵌编辑器与当前滚动位置脱节的问题。此前点击 AI 记忆或子智能体条目中的“编辑”后，表单会插入列表顶部，用户必须手动向上滚动才能开始操作。

已实现：

- 新增通用 `ResponsiveFormModal`，统一处理遮罩、键盘避让、安全区、独立内容滚动和固定操作区。
- 小于 700px 的视口使用底部展开的近全屏编辑层；宽屏使用最大宽度 760px 的居中窗口。
- AI 记忆的新建和编辑不再改变列表布局，关闭弹层后保留原来的列表位置。
- 子智能体配置的新建和编辑迁移到同一交互模式，避免重复出现“编辑器在当前位置之外”的问题。
- 在 390×844 和 1440×900 视口完成交互检查，确认长表单可滚动，标题栏与取消/保存操作始终可见。

验证命令：

```powershell
cd mobile
npm run typecheck
```

## 2026-08-22：第三方模型配置与连接测试

新增 OpenAI Chat Completions 兼容模型的运行时添加能力，支持 Provider、Protocol、AuthType、BaseURL 和厂商模型名分离配置。

已实现：

- Mongo 模型配置、YAML 配置、Registry 元数据和 LangChainGo Factory 同步支持协议与认证类型。
- Mobile 设置页支持添加第三方模型、显式选择 Bearer Token/无认证、填写模型默认生成参数。
- Base URL 严格要求为 HTTP/HTTPS API 根地址，不允许凭据、Query、Fragment 或 `/chat/completions`。
- 新增 `model_config_add`、`model_config_add_result`，API Key 永不返回 Mobile，只返回 `has_api_key`。
- 新增 `model_config_test`、`model_config_test_result`；测试创建临时模型并发起最小请求，不持久化、不注册，超时 30 秒。
- Mobile 能把模型操作失败显示在模型设置区域；保存请求失败时保留表单内容，成功后才关闭并清空表单。
- CLI `/model add` 支持认证类型和模型默认生成参数。
- 模型配置新增编辑、删除、启用/禁用和设置默认模型；编辑时 API Key 留空表示保留旧密钥。
- 新增 `model_config_update`、`model_config_delete`、`model_config_enabled_set`、`model_config_default_set` 和统一的 `model_config_mutation_result` 协议，Mobile 统一用服务端最新列表刷新状态。
- 删除或禁用模型前检查会话引用；默认模型不能直接删除或禁用，必须先选择其他默认模型。
- `llm.Client` 的运行时 Registry 增加读写锁和模型移除能力，避免生成请求与模型管理并发读写 Map。
- Model Summary 返回模型默认生成参数，编辑表单复用添加表单并支持启用/禁用、设为默认、编辑和删除。

验证命令：

```powershell
go test ./core/application/model/... ./core/adapter/persistence/mongo/mapper ./core/adapter/model/langchaingo ./core/llm ./core/remote/agent ./core/remote/relay ./core/architecture
cd mobile
npm run typecheck
```

已知风险：当前 API Key 仍以明文保存在模型配置数据库中，后续需要接入加密密钥存储或系统 Secret 管理。

### 第二优先级：资料知识库操作区

- 目录创建、知识库创建和目录管理迁移到响应式弹层，目录树数量不会再影响操作区的位置。
- 知识库的“检索测试”和“知识库设置”迁移到独立弹层，使用响应式页签切换。
- 知识库设置中的移动目录、RAG 开关、索引 Profile 和删除操作继续复用原有 Hook 与协议。
- 删除知识库成功后自动关闭设置弹层；取消或完成操作后保留目录树的原滚动位置。

## 2026-07-31：Agent 长任务时间线与历史重放

本次新增独立的 AgentRun 领域，用于记录一次聊天、重新生成或 Plan 执行的完整运行过程。`request_id` 只负责 Relay 请求关联，内部 `run_id` 负责持久化身份，两者不再混用。

核心链路：

```text
TaskService / Plan ExecutionService
  -> AgentRun CommandService
  -> runStreamRecorder
  -> memory 或 Mongo Repository
  -> remote Agent DTO
  -> Relay
  -> mobile useAgentRunState
  -> AgentRunTimeline
```

已实现：

- `AgentRun` 与 `RunEvent` 分别保存运行终态和有序事件，Mongo 使用 `agent_runs`、`agent_run_events` 两个集合。
- 支持 reasoning、工具调用、工具结果、权限、Plan 进度、完成、暂停、失败和取消事件。
- reasoning 实时推送增量，但每轮只持久化一个聚合事件；聚合内容限制为 1MB。
- 工具结果限制为 256KB，工具参数限制为 64KB，并保留截断标记。
- 普通聊天每次创建一个 Run；Plan 的全部步骤复用一个父 Run，同时保留原有步骤文件检查点。
- 新增 `agent_run_started`、`agent_run_event`、`agent_run_completed`、`agent_run_list` 和 `agent_run_list_result` 协议。
- Relay 将 Run 实时消息视为过程事件，聊天请求仍只由 `assistant_done` 结束，避免提前释放请求路由。
- Mobile 使用独立 `useAgentRunState`，不把运行事件混入 `ChatItem`；加载会话历史时同时查询 Run 历史。
- 时间线支持整段收缩、reasoning 的 Fold/Raw/Hide、工具详情展开、Plan 步骤、实时耗时和终态展示。
- 新时间线存在时忽略旧 reasoning/tool 展示协议，保留协议发送以兼容旧客户端。

### Plan 草案阶段修复

- 收紧 Plan 运行时指令：只读工具只用于生成计划前的最小预检，凡是最终交付依赖工具、文件或外部状态，都必须在输出 Plan 后等待用户执行。
- 用户在同一条消息中要求“先规划再执行”时，仍遵守两阶段协议，不能绕过 Mobile 的计划确认。
- 初始计划捕获后向 AgentRun 追加 `plan_update` 事件，并把 Plan 模式的首次生成标记为 `plan` 类型运行。
- Mobile 直接应用 `assistant_done.plan`；收到可执行草案时立即打开 Plan 面板，不再等待 Session 列表刷新后由用户手动寻找入口。

### Plan 执行卡死修复

- AgentRun 是运行时间线的旁路记录，不再允许 Mongo 驱动等待无限阻塞聊天或 Plan 主链路；`Start`、`Append`、`ReplaceEventContent`、`Finish` 统一使用独立的 5 秒持久化超时。
- AgentRun 写入失败或超时只通过 `OnRunError` 记录，Plan 仍会继续进入步骤消息构造、模型生成和工具执行。
- Agent 启动时把上次进程遗留的 `running` Run 收敛为 `failed`，并写入 `Agent restarted before run completed` 终态说明。
- `running` Plan 允许在进程重启后重新执行；已完成或跳过的步骤不会重复执行。
- 新增阻塞仓储回归测试，覆盖 AgentRun 超时后 Plan 仍能进入第一步生成并完成的场景。
- 修复 Plan 进度回调的 Session 自锁：执行链持有 Session 操作锁时不再调用 `sessionSettingsPayload -> ContextStateForSession` 重复获取同一把锁，而是直接发送已有的 `SessionPlanExecuteUpdatePayload` 快照。
- Mobile 按轻量 `session_plan_update {session_id, plan}` 协议直接应用步骤状态，不再把它错误解析为完整 Session 设置响应。

验证命令：

```powershell
go test ./core/application/agentrun/service
go test ./core/application/chat/generation/service
go test ./core/adapter/persistence/mongo/agentrun/mapper
go test ./core/remote/agent ./core/remote/relay

cd mobile
npm run typecheck
```

本文记录已经落地到源码的功能与修复，用于回答以下问题：

- 本次开发修改了什么，以及为什么修改。
- 影响了哪些模块、协议和持久化结构。
- 当前界面实际支持什么，哪些内容仍未展示。
- 开发人员应使用哪些命令验证变更。

最新修订章节置于文档前部，历史章节保留其原有记录顺序。架构原理和完整调用链仍以
[PROJECT_ARCHITECTURE_GUIDE.md](PROJECT_ARCHITECTURE_GUIDE.md) 与
[DEVELOPER_FLOW_GUIDE.md](DEVELOPER_FLOW_GUIDE.md) 为准。

## 2026-07-28：知识库、沙箱、子智能体与核心链路可靠性

对应代码基线：`35ff9025149bf02722ec8fa9effd2c99afac530b`

### 1. 知识库与 RAG

完成的能力：

- 知识库支持分类树，分类和知识库是两个独立领域对象。
- 文档原文存入 MinIO，MongoDB 保存知识库、文档、Chunk、Profile 和索引任务元数据。
- Python 文档处理器通过 gRPC 和 worker pool 执行解析、清洗与分块。
- 远程向量存储使用 Milvus，本地热点向量使用 sqlite-vec，本地关键词检索使用 SQLite FTS5。
- 检索先执行本地向量与关键词通道，并通过 RRF 融合；本地质量不足时回退到 Milvus。
- Session 保存 RAG 模式、知识库范围、`top_k` 等设置，聊天生成前按会话配置注入检索结果。
- 文档、Chunk 和向量采用逻辑删除，不进行自动物理清除。

主要代码位置：

```text
core/domain/knowledge
core/application/knowledge
core/port/knowledge
core/adapter/persistence/mongo/knowledge
core/adapter/vectorstore/milvus
core/adapter/vectorstore/sqlitevec
core/adapter/keywordstore/sqlitefts5
document_processor
```

Mobile 端补齐：

- 分类与知识库树形浏览。
- 新建、修改、移动和删除分类。
- 新建、修改和删除知识库。
- 文档上传、索引状态、失败重试和删除。
- Session RAG 模式、检索范围和 `top_k` 设置。
- 检索预览及来源信息。
- 所有远程操作增加 pending 状态、禁用状态和加载反馈。

主要入口为 `mobile/src/components/knowledge/KnowledgePanel.tsx` 和
`mobile/src/hooks/useKnowledgeActions.ts`。详细实现见所有
`FEATURE_RAG_*_IMPLEMENTATION.md` 文档。

### 2. OpenSandbox 与工作区隔离

命令执行边界重新拆分：

```text
core/domain/execution + core/port/execution
  -> 描述命令请求和本地宿主执行接口

core/domain/sandbox + core/port/sandbox
  -> 只描述真正的隔离环境

core/adapter/execution/local
  -> 在 Agent 所在宿主机执行，isolated=false

core/adapter/sandbox/opensandbox
  -> 通过 OpenSandbox 创建和管理隔离环境
```

本地执行器的 workspace 路径检查不是操作系统隔离。只有选择
`opensandbox` provider 时，命令才运行在远程隔离环境中。OpenSandbox 的
endpoint、API key、镜像、CPU、内存、超时和最大下载体积都由配置对象管理。

新增 Snapshot 工作区作为不依赖 Git 的隔离方案：

- 子任务启动时把源工作区复制到项目内的 `.myai/subagent-snapshots`。
- 子智能体只修改 Snapshot，不直接修改用户源目录。
- 完成后生成 `ChangeSet`，由用户选择应用或丢弃。
- 应用前检查源文件冲突并保存 SQLite checkpoint。
- 应用失败时按 rollback entry 回滚；回滚成功后删除补偿 checkpoint，避免孤儿记录。
- Manifest 先写临时文件并同步到磁盘，再原子替换目标文件。
- Unix 使用 `os.Rename`；Windows 使用带
  `MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH` 的 `MoveFileEx`，不会先删除旧 Manifest。

主要代码位置：

```text
core/adapter/workspace/snapshot
core/adapter/workspace/opensandbox
core/adapter/workspace/router
deploy/opensandbox/docker-compose.yml
```

### 3. 异步子智能体

子智能体由动态 Definition 和异步 Task 组成，不要求每次修改 YAML 后重启：

- Definition 支持创建、更新、逻辑删除和 MongoDB 持久化。
- Definition 固定模型、系统指令、工具白名单、能力模式、隔离模式、最大轮次和超时。
- Task 保存 Definition 快照，因此 Definition 后续修改不会改变已经运行的任务。
- 子任务建立独立 Child Session，并保留 Parent Session、Task 和 Definition 的关联关系。
- 本地 Scheduler 使用独立 worker/queue 异步执行，主对话结束后任务仍可继续。
- Task 支持 `queued`、`running`、`succeeded`、`failed` 和 `canceled` 等状态。
- 任务完成后保存结果、reasoning、未读状态和工作区 `ChangeSet`。
- 用户可以检查、取消任务，以及应用或丢弃任务产生的文件变更。

Mobile 端新增子智能体设置面板，协议覆盖 Definition 列表与维护、Task
列表、检查、取消、应用和丢弃操作。主要代码位置：

```text
core/domain/subagent
core/application/subagent
core/adapter/subagent
core/adapter/persistence/mongo/subagent
core/remote/agent/subagent_handlers.go
core/tool/local/subagent_tools.go
mobile/src/components/subagents/SubagentPanel.tsx
```

Mongo 更新使用 `$set`/`$unset` 构建更新文档，`_id` 只用于查询条件，不再进入
`$set`。可选字段清空时进入 `$unset`，避免触发 MongoDB immutable field 错误或残留旧值。

### 4. Relay 与 Agent 安全

Relay 不再接受未认证 Agent：

- Relay 启动时配置绑定到 `user/device/token` 的 Agent credential。
- Agent 在 WebSocket Upgrade 请求中发送 Bearer token 和身份 Header。
- Relay 先认证连接，再校验 `agent_online` 和后续消息的身份是否一致。
- `/agents` 使用 Agent credential 时可查看完整列表；配对后的 Client token 只能查看自己的 Agent。
- 浏览器 Origin 默认按同源限制，可通过重复的 `--allowed-origin` 显式放行。
- Relay Web 页面改为使用配对后的 Client token 请求 Agent 状态。

启动参数和示例已同步到 README、开发流程手册和 Agent 启动功能文档。已经在日志、截图或命令行中暴露过的旧 Token 必须轮换。

### 5. 工具、Hook 与模型循环

工具链路修复：

- `PreToolUse` 决策优先级固定为 `Deny > Ask > Allow > Continue`。
- Hook 的 `Allow` 只代表 Hook 放行，不能提升 Session 权限。
- Hook 的 `Ask` 会强制进入权限确认，包括 read 工具和 full 权限会话。
- Hook 改写后的工具参数作为实际 ToolCall 返回，并进入下一轮模型上下文与持久化记录。
- 工具不存在、pre-hook 失败、权限拒绝、超时和取消都会生成结构化 ToolResult。
- 一批工具部分执行后发生错误时，已完成的 ToolCall/ToolResult 会先进入会话，随后终止循环，避免模型重试有副作用的工具。
- 每轮工具执行后重新计算上下文快照；单轮内容超过 Session 窗口时返回明确错误。
- 子智能体额外执行工具白名单与能力模式检查。

### 6. 会话、重新生成与持久化

重新生成不再采用“先删内存、异步落库后继续生成”的弱一致性流程：

1. 保存修剪前的内存 Session 快照。
2. 修剪最后一个 user message 之后的 assistant/tool 消息。
3. 通过 SessionQueue 同步等待 transcript 替换。
4. MongoDB 在事务中替换消息、保存 Session，并只删除已移除 ToolCall 对应的 Asset。
5. 持久化失败时恢复修剪前的内存 Session，并停止模型生成。

多 ToolCall 持久化也已修复：一个 assistant message 中的每个 ToolCall 都会保存为独立记录；加载时把连续 ToolCall 重新聚合成同一条领域消息，保证重启前后上下文一致。

Session 设置更新后不再继续修改旧指针，而是从 MemoryStore 重新读取最新 Session。模型、模式、权限、上下文窗口、生成参数、风格和 RAG 设置都遵循相同规则，降低切换模型或并发设置时出现旧状态覆盖新状态的风险。

### 7. 异步任务与关闭行为

- 底层 `threadpool.Pool` 在队列满时返回 `ErrQueueFull`，不再偷偷创建无管理 goroutine。
- 应用层 `AsyncTaskService` 收到 `ErrQueueFull` 时执行 caller-runs，保证需要持久化的任务不会静默丢失。
- executor 已关闭或不可用时明确返回错误，不执行 caller-runs。
- caller-runs 与 worker 都有 panic 防护。
- 应用关闭流程等待线程池和子智能体 Scheduler 退出。

### 8. 上下文面板与压缩摘要

Mobile 打开“上下文”设置时会主动发送 `session_context_query`，并显示读取中的加载状态；不需要先点击“应用”才能查询。

当前面板展示：

- Full、Selected、Summary、Prefix、Cacheable token 数。
- 选中消息数、摘要版本、摘要和前缀哈希。
- 后端实际持久化的完整 `Session.Summary` 字符串。
- 上一次自动压缩的 before/after token 等统计信息。

需要特别区分：面板会完整展示**已保存的摘要文本**，但不会展示**模型最终请求的全部内容**。当前没有展示选中消息正文、固定 system prompt、cacheable prefix 正文和 Plan snapshot，只提供相应统计或哈希。`summary_tokens=0`、`summary_version=0`、`has_summary=false` 表示尚未生成摘要，而不是摘要显示不完整。

摘要生成本身受以下上限约束：

```text
已有摘要输入：最多 600 tokens
新历史输入：最多 1800 tokens
新摘要输出：最多 768 tokens
Plan snapshot：最多 512 tokens
```

因此，“完整展示摘要”表示 UI 没有再次截断后端保存的 `Summary` 字符串，不表示摘要是压缩前历史的无损副本。

### 9. 验证结果

本次代码基线已通过：

```powershell
go test ./...
go vet ./...
go test ./core/architecture

cd mobile
npm run typecheck

node --check core/remote/relay/web/app.js
git diff --check
```

`git diff --check` 仅报告仓库已有的 LF/CRLF 转换警告，没有空白错误。

## 2026-07-28：子智能体结果手动恢复主会话

完成第一版 Parent Session 恢复链路：

```text
子智能体成功或失败
-> Task.Unread=true
-> Mobile 显示“继续主任务”
-> subagent_task_resume
-> 主会话隐藏注入结构化子智能体报告
-> 强制 Chat 模式恢复模型生成
-> assistant_delta / tool / permission / assistant_done
-> subagent_task_resume_result
```

- `Unread` 表示终态结果尚未被 Parent Session 消费。Resume 开始前先将其设为 `false`，防止重复请求；模型失败或任务取消时恢复为 `true`，允许用户重试。
- 子智能体报告以 `SyntheticReasonSubagentResult` 用户角色消息进入模型上下文，但消息查询会过滤所有 Synthetic Message，因此 Mobile 历史不会出现伪造的用户发言。
- 报告包含任务 ID、标题、状态、结果或错误、ChangeSet 状态与文件路径，并明确标记子智能体输出只是低优先级证据，不能覆盖系统规则、权限和原始用户要求。
- Resume 使用 `ForceChatMode=true` 且不捕获 Plan，避免 Plan 模式下再次生成计划草案。
- Relay 在收到 `assistant_done` 后仍保留 Resume 请求路由，只在收到 `subagent_task_resume_result` 后释放，确保 Mobile 能同时完成聊天流和任务按钮状态。
- Snapshot 文件变更仍由用户单独“应用变更”或“丢弃”，恢复主会话不会自动修改源工作区。

当前边界：不自动恢复 Parent Session，不实现多任务 DAG、依赖策略或多个子智能体结果的自动合并。

## 2026-07-29：限制单轮工具结果占用的模型上下文

修复子智能体在同一轮并行读取多个大文件后失败的问题：

```text
工具执行得到完整输出
-> 根据 Session ContextWindow 计算当前轮剩余 Prompt 预算
-> 只截断送回模型的 ToolResult 副本
-> 完整输出继续用于 Hook、流式回调和审计记录
-> Mongo 同时保存完整输出和受限 Prompt 输出
-> 会话重载继续使用受限版本构建模型上下文
```

- 预算扣除固定 System/Summary/Plan、当前用户轮、ToolCall 参数和回复预留空间，并且最多允许单批工具结果占窗口的一半。
- 当前轮已积累的 ToolResult 越多，后续工具批次可用预算越小，避免多轮工具调用再次撑破窗口。
- `ExecutionEntry.Content/Error` 保留完整审计内容；`PromptContent/PromptError/PromptTruncated` 表示模型实际收到的受限版本。
- Mongo `MessageDocument` 增加对应的 `tool_prompt_*` 可选字段；旧文档没有这些字段时继续使用原始 ToolResult，兼容现有数据。
- Token 估算改为计算结构化 `ToolResult.PromptContent()`，包括状态和错误字段，不再只估算正文。

部署验证同时确认：配置 MongoDB 持久化时必须启用副本集，单节点副本集即可。子智能体 `Task + Run` 原子保存依赖 MongoDB 事务，standalone 模式不支持该语义。

验证命令：

```powershell
go test ./core/contextmgr ./core/adapter/tool/executor
go test ./core/application/session/... ./core/adapter/persistence/mongo/mapper ./core/adapter/persistence/toolrecords/mapper
go test ./...
go vet ./...
go test ./core/architecture
git diff --check
```

## 2026-07-29：Mobile 子智能体任务抽屉

优化 Mobile 设置页的后台任务展示，避免任务要求、执行结果、变更文件和操作按钮长期占用较大的纵向空间：

- 每个任务默认收起，标题栏固定展示任务名称、Definition ID、中文状态、变更文件数和展开箭头。
- 收起状态保留最多两行的结果或错误摘要；存在未消费结果时显示提示圆点。
- 点击整个标题栏可展开或收起详情，Android 和其他平台使用 `LayoutAnimation` 提供抽屉过渡效果。
- 展开后继续展示完整任务要求、执行结果、错误、变更文件和原有操作按钮，业务协议与后端状态机不变。
- `waiting_subagents` 和 `waiting_permission` 作为运行中状态处理，仍可在详情中取消任务。
- 标题栏增加无障碍按钮语义、动态展开状态和操作说明。

涉及文件：

```text
mobile/src/components/subagents/SubagentPanel.tsx
```

验证命令：

```powershell
cd mobile
npm run typecheck

git diff --check
```

## 2026-07-29：会话顺序、子任务恢复与 Relay 一致性修复

修复持久化与远程连接链路中的五组一致性问题：

### 1. 持久化消息顺序

- `MessageRecord` 和 Mongo `MessageDocument` 增加 `sequence` 整数顺序字段，避免 BSON datetime 只有毫秒精度时丢失同一批消息的纳秒顺序。
- 普通用户轮、工具调用、工具结果和 Assistant 消息在持久化 Adapter 边界生成精确顺序；重新生成整份 transcript 时重新生成完整顺序。
- Mongo 查询按 `sequence -> created_at -> _id` 排序。旧文档没有 `sequence` 时仍可按原时间和 ID 稳定读取，不要求手工迁移。

### 2. 子任务恢复幂等

- `AppendUserMessage` 增加仅用于合成消息的去重选项；同一 `SyntheticReason + 内容` 已存在时复用当前 Session，不重复追加和持久化报告。
- 普通用户消息不参与去重，用户连续发送相同文本仍会形成不同对话轮次。
- 子任务报告限制 Result、Error、文件数量和单个路径长度，并在 JSON 中写入截断与省略数量，避免超长报告占满父会话上下文。
- 新取消任务不再设置 `Unread=true`；历史 Mongo canceled 文档即使保存了错误未读标记，加载时也会自动归一化为已读。

### 3. 重新生成持久化终态

- `SessionQueue.SubmitAndWait` 在入队前尊重取消；任务一旦被执行器接受，就等待数据库写入返回明确成功或失败。
- transcript 事务使用保留 Context Value、但不继承请求取消的独立超时 Context，防止调用方收到取消后事务仍提交，造成内存回滚和数据库新 transcript 并存。

### 4. Relay 重复 Agent 上线

- 同一 user/device 的新 Agent 上线会替换并主动关闭旧 Peer。
- Agent 每条非注册消息都必须来自 Registry 当前 Peer；旧连接不能继续转发 delta、done、事件或离线消息。
- 旧连接退出时仍通过 Peer 身份比较保护新连接，不会误删新 Registry 记录。

验证命令：

```powershell
go test ./...
go vet ./...
go test ./core/architecture
git diff --check
```

## 2026-08-15：AI 经验记忆系统

新增独立于用户资料知识库的 AI 记忆库，用于保存 Agent 从实际任务中总结出的目标、方案、结果、痛点、错误原因、经验和偏好。

### 1. 领域与应用服务

- 新增 `Memory`、不可变 `Revision`、`Candidate`、`ExtractionJob`、`DreamRun` 等领域对象。
- `CatalogService` 支持 CRUD、追加版本、逻辑删除、恢复、候选审核和使用次数统计。
- 人工创建或编辑会设置 `HumanLocked=true`，为后续 Dream 模式保留人工保护边界。
- Dream 模式当前只完成领域对象和持久化骨架，尚未实现自动合并和夜间调度。

### 2. 白天提取

- AgentRun 以 succeeded 或 failed 完成后，通过 CompletionObserver 异步创建提取任务。
- 提取任务先持久化再提交线程池，进程重启后可恢复 pending、running 和可重试 failed Job。
- `agent_run_id + extractor_version` 唯一；并发冲突时回读已有任务。
- failed Job 最多自动尝试三次，避免每次启动无限重试。
- 模型输入限制单 Event 和总证据长度，并对常见 API Key、Token、Password、Secret 和 Bearer 值脱敏。
- 相同 Job 重试使用确定性 Candidate ID，避免重复候选。

### 3. 生成前检索

- 每个 GenerationTask 只检索一次，AgentLoop 的所有工具轮次复用同一份 MemoryContext。
- 支持 global、workspace、project、session 作用域，默认 TopK 为 4，Prompt 限制约 900 Token。
- MemoryContext 插入最新 user message 之前，不写入 `Session.Messages`，保持历史与缓存前缀稳定。
- failure 记忆只作为警告，不作为推荐方案。
- 命中后通过 Catalog 用例更新使用次数；统计失败不阻断模型回答。

### 4. Mongo、协议与 Mobile

- 新增 `ai_memories`、`ai_memory_candidates`、`ai_memory_extraction_jobs`、`ai_memory_dream_runs` 集合和索引。
- Mongo 不可用时回退到进程内存仓储。
- 新增 `ai_memory_*` 远程协议和 Agent Handler。
- Mobile 知识页增加“资料知识库 / AI 记忆”，AI 记忆内部提供“有效记忆 / 待审核”。
- 支持搜索、新建、编辑新版本、逻辑删除、查看已删除、恢复、通过候选和拒绝候选，并提供统一加载反馈。

完整调用链见 [FEATURE_AI_MEMORY_IMPLEMENTATION.md](FEATURE_AI_MEMORY_IMPLEMENTATION.md)。

验证命令：

```powershell
go test ./core/domain/memory ./core/application/memory/... ./core/adapter/memory/...
go test ./core/adapter/persistence/mongo/memory/... ./core/application/chat ./core/remote/agent
go test ./core/architecture ./core

cd mobile
npm run typecheck
```

## 2026-08-15：AI 记忆生产级一致性与失败恢复

补齐 AI 记忆第一批生产级缺口，重点保证候选审批的一致性、使用计数的并发正确性，以及提取失败后的人工恢复能力。

### 1. 候选审批与使用统计

- `CandidateApprovalRepository` 将 Memory 保存和 Candidate 状态更新收口为一个仓储操作。
- Mongo Adapter 使用事务保存审批结果，并通过 `status=pending` 条件更新阻止同一候选被重复审批；内存 Adapter 在同一把锁内完成相同行为。
- `UsageRepository.RecordUse` 使用 Mongo `$inc` 或内存锁完成原子自增，避免并发命中同一条记忆时丢失计数。
- 不存在的 Memory 返回 `ErrNotFound`，已逻辑删除的 Memory 保持幂等，不再增加使用次数。

### 2. 提取失败任务恢复

- Extraction 应用服务新增失败任务查询和手动重试用例；手动重试只接受 `failed` Job，恢复为 `pending` 后重新提交线程池。
- 手动重试保留累计 `Attempts`，清空 `LastError` 和 `CompletedAt`，并且不受三次自动重试上限限制。
- 新增 `ai_memory_extraction_job_list`、`ai_memory_extraction_job_retry` 及对应结果协议，Relay、Agent Handler 和 DTO Mapper 已完整接通。
- Mobile 的“AI 记忆 / 待审核”页展示失败任务的 AgentRun ID、尝试次数和最后错误，并提供带加载反馈的“重试”按钮。

### 3. 尚未纳入本批的能力

- 提取模型仍复用默认模型，尚未支持独立的记忆模型配置。
- Mobile 尚未支持把候选合并到指定的已有 Memory。
- Dream 仍只有领域对象和持久化骨架，尚未实现质检、去重合并和调度。

验证命令：

```powershell
go test ./...
go test ./core/architecture

cd mobile
npm run typecheck

git diff --check
```

## 2026-08-16：AI 记忆独立模型、候选合并与手动 Dream

完成 AI 记忆第二优先级能力：提取和 Dream 不再必须绑定默认聊天模型，Mobile 可以把候选合并到已有记忆，并提供可审计的手动 Dream 质检流程。

### 1. 独立模型配置

- 新增 `memory.extraction.model_id` 和 `memory.dream.model_id`；留空时分别回退到应用默认模型。
- `Application.InitMemoryServices` 从 Model Registry 独立获取两个 `ChatModelPort`。某一模型不可用时只禁用对应能力，不影响 AI 记忆 CRUD 和生成前检索。
- `ModelExtractor.Version()` 加入模型 ID 的稳定哈希，切换提取模型后不会错误复用旧模型对应的 ExtractionJob。
- 提取仍固定使用低温度和 1600 输出 Token；Dream 固定使用低温度和 3000 输出 Token，不继承 Session 风格与生成参数。

### 2. 候选人工合并

- Mobile 待审核页新增“合并到已有”，使用响应式弹层搜索和选择 active Memory，并展示目标标题、目标摘要和当前版本。
- `ai_memory_candidate_approve` 通过可选 `memory_id` 区分新建和合并，继续复用 Catalog Application Service。
- 人工审批使用 `HumanApprove=true`，生成新 Revision 后保持 `HumanLocked=true`，允许用户显式纠正已锁定记忆。
- Mongo 使用事务同时更新 Candidate 和 Memory；候选必须仍为 `pending`，版本条件可以阻止并发覆盖。

### 3. 手动 Dream 质检

- 新增 `application/memory/dream` 的 command、result、api 和 service，模型能力通过 `DreamConsolidator` 接口与 `ModelConsolidator` 实现隔离。
- 模型只生成 `create`、`merge`、`keep_both`、`reject` 或 `needs_review` 建议，不能直接访问仓储。
- Application Service 再次校验 Candidate、目标状态、`HumanLocked` 和目标版本，并通过 CatalogService 应用动作。
- `create` 和 `keep_both` 创建新 Memory，`merge` 追加 Revision，`reject` 更新候选状态，`needs_review` 保持 pending。
- 自动 `supersede` 在具备跨多 Memory 的原子持久化前保持禁用；非法或未应用动作写入 `FailureReason`。
- 同一进程的 Dream Run 串行执行；运行开始即保存审计记录，模型、仓储或 JSON 失败时也能查询失败终态。

### 4. 远程协议与 Mobile 审计

- 新增 `ai_memory_dream_run`、`ai_memory_dream_list` 及对应结果协议，Relay 和 Agent Handler 已接通。
- Mobile AI 记忆增加 Dream 页签，可手动执行、查看运行计数，并展开每条 Action 的候选标题、目标标题、模型理由、应用状态和失败原因。
- Dream 成功返回最新 Runs、Memories 和 pending Candidates，三个页签同步刷新。
- `DreamAction` 的可读标题和完整审计字段已进入 Mongo PO、Mapper 和内存实现。

当前边界：尚未实现闲置或夜间自动调度、分布式 Dream 锁、自动 `supersede` 和 AI 记忆向量检索；独立模型配置目前需要修改 YAML 或环境变量并重启 Agent。

验证命令：

```powershell
go test ./...
go vet ./...
go test ./core/architecture

cd mobile
npm run typecheck

git diff --check
```

## 2026-08-16：Mobile 会话级生成参数与回复风格

补齐已有生成参数能力的远程控制和 Mobile 设置入口，同一个模型现在可以按 Session 使用不同采样参数和表达风格。

### 1. 远程协议与应用层复用

- 新增 `session_generation_query`、`session_generation_set`、`session_style_set` 及对应结果协议。
- Relay 将三类请求纳入客户端 Token 鉴权、请求路由和响应类型匹配；Agent 使用现有会话运行时锁和请求锁处理，避免与同一会话的其他设置并发覆盖。
- Agent 通过 `ChatService` Facade 调用既有 Session Settings Application Service，继续执行参数校验、内存更新、持久化和 `SessionChanged` 事件发布，没有从远程层直接修改领域对象。
- 保存成功后重新计算并返回 Session 覆盖、Model 默认和 Effective Settings，Mobile 不使用本地乐观值冒充服务端状态。

### 2. Mobile 设置界面

- 设置页新增“生成”分区，进入分区、切换 Session 或切换模型时主动查询最新配置。
- 支持 Temperature、Top P、最大输出 Token 的单项继承、完整替换和全部继承，并展示最终生效值。
- Model 没有配置默认值时明确显示“模型未设置”和系统兜底值，不再把 `0.7`、`1.0`、`2048` 误标成模型默认值。
- 空输入发送 JSON `null` 表示继承，显式数值 `0` 保持为零；客户端范围校验和后端领域校验共同生效。
- 回复风格支持保存和清除，限制 2000 个 Unicode 字符；参数和风格采用独立保存协议。
- 查询和保存期间显示 loading、禁止重复点击，成功与服务端错误都在当前生成设置区域反馈。

### 3. 边界与兼容性

- 旧 Session 文档缺少生成字段时自然恢复为继承，不需要数据库迁移。
- Session 参数优先级保持为 `Session 显式覆盖 > Model 默认值 > System 兜底值`。
- 回复风格仍只进入 Runtime Instruction，不写入聊天历史，不改变固定 System Prompt 和历史缓存前缀。
- Mobile 当前没有编辑 Model 默认生成参数的独立界面，模型级默认值仍通过模型配置入口维护。

验证命令：

```powershell
go test ./...
go vet ./...
go test ./core/architecture

cd mobile
npm run typecheck

git diff --check
```

## 后续记录约定

后续每次跨模块功能或重要缺陷修复，都应在提交前追加一个日期章节，至少记录：

1. 用户可观察到的行为变化。
2. 关键领域对象、应用服务、Adapter 和协议入口。
3. 持久化或兼容性影响。
4. 已知限制与未完成项。
5. 实际执行过的验证命令。
