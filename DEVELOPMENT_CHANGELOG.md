# MyAI 开发变更记录

本文记录已经落地到源码的功能与修复，用于回答以下问题：

- 本次开发修改了什么，以及为什么修改。
- 影响了哪些模块、协议和持久化结构。
- 当前界面实际支持什么，哪些内容仍未展示。
- 开发人员应使用哪些命令验证变更。

记录按日期倒序排列。架构原理和完整调用链仍以
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

## 后续记录约定

后续每次跨模块功能或重要缺陷修复，都应在提交前追加一个日期章节，至少记录：

1. 用户可观察到的行为变化。
2. 关键领域对象、应用服务、Adapter 和协议入口。
3. 持久化或兼容性影响。
4. 已知限制与未完成项。
5. 实际执行过的验证命令。
