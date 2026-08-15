# AI 记忆系统功能实现

本文说明 MyAI 当前 AI 记忆系统的真实实现。阅读对象是需要维护该功能的开发人员，重点解释领域对象、接口、实现类、应用服务、持久化、模型提取、聊天注入、远程协议和 Mobile UI 之间如何协作。

## 1. 功能边界

项目现在有两类容易混淆的“知识”：

| 类型 | 保存内容 | 主要来源 | 检索目的 | 主要存储 |
|---|---|---|---|---|
| 用户资料知识库 | PDF、Word、网页、用户上传文件及其 Chunk | 用户上传 | 回答资料中的事实 | MinIO、Mongo、Milvus、sqlite-vec、FTS5 |
| AI 记忆库 | 目标、方案、结果、痛点、错误原因、经验、偏好 | AgentRun 总结和人工维护 | 复用过去的解决经验 | Mongo 或进程内存 |

AI 记忆不是文件知识库中的一个分类，也不复用 RAG Chunk。两者只在模型生成前同时作为动态上下文进入请求。

当前 AI 记忆系统已经支持：

- AgentRun 结束后异步提取候选记忆。
- 候选记忆人工通过或拒绝。
- 有效记忆新建、编辑、追加版本、逻辑删除和恢复。
- 按全局、工作区、项目和会话作用域过滤。
- 模型生成前检索相关经验并注入当前轮上下文。
- Mobile 中查看有效记忆和待审核候选。
- Mongo 持久化和无 Mongo 时的内存实现。
- DreamRun 领域对象和存储骨架。

当前尚未实现自动 Dream 合并、定时夜间整理和 AI 记忆向量检索。

## 2. 目录与分层

```text
core/domain/memory
  memory.go                 Memory、Revision、Content
  candidate.go              Candidate、CandidateDraft
  job.go                    ExtractionJob
  dream.go                  DreamRun、DreamAction
  types.go                  枚举、Scope、SourceRef

core/port/memory
  repository.go             仓储和 ID 接口
  filters.go                查询条件 POJO
  extractor.go              CandidateExtractor、DreamConsolidator

core/application/memory
  catalog                   CRUD、审核、使用次数
  extraction                异步提取任务
  retrieval                 生成前经验检索

core/adapter/memory/extractor
  model_extractor.go        使用 ChatModelPort 提取候选

core/adapter/persistence
  memory/memory             内存仓储实现
  mongo/memory              Mongo PO、Mapper、Repository

core/remote/agent
  memory_facade.go          Agent 依赖的应用接口
  memory_handlers.go        远程消息 Handler
  memory_payload_mapper.go  Domain -> Protocol DTO

mobile/src
  hooks/useAIMemoryState.ts
  hooks/useAIMemoryActions.ts
  components/knowledge/AIMemoryPanel.tsx
  components/knowledge/KnowledgeWorkspacePanel.tsx
```

对应 Spring Boot 的理解：

| MyAI | Spring Boot 类比 |
|---|---|
| `domain/memory` | Domain Entity、Value Object |
| `port/memory` | Repository Interface、外部能力接口 |
| `application/memory/*/command` | Request Command POJO |
| `application/memory/*/service` | Application Service |
| `adapter/persistence/*` | Repository Implementation |
| `remote/protocol` | Controller DTO |
| `remote/agent/memory_handlers.go` | WebSocket Controller |
| `App.InitMemoryServices` | `@Configuration` 和 Bean 装配 |

## 3. 核心领域对象

### 3.1 Memory 与 Revision

`core/domain/memory/memory.go`：

```go
type Memory struct {
    ID             string
    Title          string
    Kind           Kind
    Scope          Scope
    Tags           []string
    Status         Status
    CurrentVersion int
    Revisions      []Revision
    HumanLocked    bool
    UseCount       int64
    LastUsedAt     *time.Time
    DeletedAt      *time.Time
    DeletionReason string
}
```

编辑记忆时不会覆盖旧内容。`CatalogService.Update` 构造新 `Revision`，再调用：

```go
memory.AppendRevision(revision, now)
```

`AppendRevision` 自动使用 `CurrentVersion + 1`，保留旧 Revision，并把新版本设为当前版本。

### 3.2 Candidate

模型不能直接创建有效记忆。`ModelExtractor` 只返回 `CandidateDraft`，`ExtractionService` 把它变成 `CandidatePending`：

```text
模型输出
-> CandidateDraft
-> Candidate(status=pending)
-> 人工审核
-> Memory(status=active)
```

候选 ID 由 `jobID + ordinal` 的 SHA-256 生成。相同提取任务重试时会得到相同 Candidate ID，不会重复插入。

### 3.3 Scope

```go
const (
    ScopeGlobal    ScopeType = "global"
    ScopeWorkspace ScopeType = "workspace"
    ScopeProject   ScopeType = "project"
    ScopeSession   ScopeType = "session"
)
```

- `global` 的 Key 必须为空。
- `workspace`、`project`、`session` 必须有 Key。
- 当前自动提取默认使用 Agent 启动时的 workspace 绝对路径。
- 模型返回非法 Scope 时回退到默认 Scope，不让脏枚举导致整个任务失败。

### 3.4 逻辑删除

`Memory.MarkDeleted` 设置：

```text
Status = deleted
DeletedAt = 当前时间
DeletionReason = 删除原因
```

不会物理删除 Mongo 文档。`Memory.Restore` 恢复为 `active` 并清空删除字段。

## 4. 应用启动与依赖装配

入口位于 `core/App.go`。

```text
Application.NewApplication
-> InitMemoryStorage
-> InitMemoryServices
-> InitChatService
```

### 4.1 InitMemoryStorage

Mongo 可用时：

```go
repository := memorymongo.New(app.mongoDb, app.properties.Mongo.Database)
repository.EnsureIndexes(context.Background())
app.memoryStore = repository
```

Mongo 不可用时：

```go
app.memoryStore = aimemory.New()
```

内存模式可运行完整业务流程，但进程重启后数据消失。

### 4.2 InitMemoryServices

此函数创建三个 Application Service：

```text
CatalogService
  负责 CRUD、审核、逻辑删除、版本和使用次数

ExtractionService
  负责持久化提取任务、异步执行和启动恢复

Retrieval ContextService
  负责生成前的相关经验检索和 Prompt 格式化
```

`ExtractionService` 同时注册为 AgentRun 的 `CompletionObserver`；`RetrievalService` 注入聊天生成链路。

## 5. 白天提取链路

### 5.1 AgentRun 完成

聊天和 Plan 都通过 AgentRun CommandService 记录执行过程。装配时使用：

```go
agentrunservice.ObservingCommandService{
    Inner: baseCommands,
    Observer: memoryExtractionService,
}
```

当业务调用 `Finish`：

```text
ObservingCommandService.Finish
-> Inner.Finish
-> Observer.AgentRunCompleted
-> ExtractionService.EnqueueRun
```

仅 `succeeded` 和 `failed` 的 AgentRun 会提取。`paused` 和 `canceled` 不提取。

### 5.2 先持久化任务，再进入线程池

`ExtractionService.EnqueueRun` 的顺序是：

```text
校验 AgentRun
-> 按 agent_run_id + extractor_version 查询已有任务
-> 创建 ExtractionJob(status=pending)
-> SaveExtractionJob
-> Async.Submit(Process)
```

线程池队列满或进程中断时，pending Job 仍保存在 Mongo。下次启动由 `Recover` 重新提交。

Mongo 使用唯一索引：

```text
agent_run_id + extractor_version
```

如果并发创建触发唯一冲突，Service 会重新读取已存在的 Job 并返回，保持接口幂等。

### 5.3 Process

`ExtractionService.Process`：

```text
GetExtractionJob
-> status=running, attempts++
-> GetRun
-> ListEvents
-> CandidateExtractor.Extract
-> CandidateDraft -> Candidate
-> SaveCandidate
-> status=succeeded
```

失败时保存：

```text
status=failed
last_error=错误文本
attempts=本次尝试次数
```

启动恢复最多自动尝试三次。达到三次后不再在每次启动时无限重试；用户后续可以增加显式重试用例，但当前远程协议还没有该入口。

## 6. 模型提取器

实现类：`core/adapter/memory/extractor/model_extractor.go`。

### 6.1 输入限制

- 单个 Event 文本最多 2000 字节。
- 总证据最多 14000 字节。
- UTF-8 截断不会切断多字节字符。
- 常见 API Key、Token、Password、Secret、Bearer 值在入模前替换为 `[REDACTED]`。

脱敏是防误传保护，不是完整 DLP 系统。新的密钥格式仍需要扩展规则。

### 6.2 模型参数

提取调用显式使用：

```text
temperature = 0.1
max_output_tokens = 1600
```

它不继承当前 Session 的聊天风格和采样参数，避免用户会话风格改变结构化提取结果。

### 6.3 输出协议

模型必须返回一个 JSON 对象：

```json
{
  "candidates": [
    {
      "title": "提取任务必须先持久化",
      "kind": "experience",
      "scope_type": "workspace",
      "scope_key": "D:\\Go_All\\myai",
      "tags": ["async", "recovery"],
      "goal": "避免线程池任务丢失",
      "applicable_context": "后台任务需要跨重启恢复时",
      "approach": "先保存 pending Job，再提交线程池",
      "result": "启动时可以恢复未完成任务",
      "pain_points": "队列满和进程退出",
      "root_cause": "只保存内存队列",
      "lessons": "队列不是持久化状态",
      "verification": "重启后 Recover 能重新提交",
      "confidence": 0.9
    }
  ]
}
```

最多保留三个候选。日常寒暄、没有复用价值的执行应返回空数组。

## 7. 候选审核与 CRUD

应用入口是 `CatalogService`：

```text
List / Get
Create / Update
Delete / Restore
CreateCandidate / ListCandidates
ApproveCandidate / RejectCandidate
RecordUse
```

### 7.1 通过候选

不指定 `MemoryID`：创建一条新的 Memory 和 Version 1。

指定 `MemoryID`：把候选内容追加为目标 Memory 的新 Revision。

通过后：

```text
Candidate.Status = approved
Candidate.TargetMemoryID = Memory.ID
Memory.HumanLocked = true
```

当前 Mobile 只提供“通过并新建记忆”。合并到已有 Memory 的后端能力已经存在，但 UI 还没有目标记忆选择器。

### 7.2 人工编辑

人工创建和编辑的 Revision 使用：

```text
Author = human
HumanLocked = true
```

Dream 模式未来执行合并时必须尊重 `HumanLocked`，不能静默覆盖人工修正。

## 8. 生成前检索

入口：`AssistantGenerationService.Generate`。

```text
收到最新用户输入
-> MemoryContext.Prepare
-> ContextService.Prepare
-> 查询 active Memory
-> Scope 过滤
-> 标题、标签、内容和中英文二元组评分
-> TopK
-> 格式化 Memory Prompt
-> AgentLoopService.Run
```

默认行为：

- 候选池最多 200 条。
- 最终 TopK 默认 4，最大 8。
- Memory Prompt 最多约 900 Token。
- 过短输入和“你好、hello、谢谢、继续”等不触发。
- failure 类型明确作为警告，不作为推荐方案。

检索命中后通过 `CatalogService.RecordUse` 更新 `UseCount` 和 `LastUsedAt`。统计失败只记录日志，不阻断模型回答。

## 9. MemoryContext 如何进入 AgentLoop

`AssistantGenerationService` 每个 GenerationTask 只检索一次：

```go
generationcommand.Run{
    MemoryContext: prepared.Prompt,
}
```

`AgentLoopService` 的每一轮都调用：

```go
withMemoryContext(snapshot.Messages, command.MemoryContext)
```

插入位置是最新 user message 之前：

```text
固定 system 和历史前缀
-> AI MemoryContext
-> 最新 user message
-> 本轮后续 ToolCall / ToolResult
```

MemoryContext 不写入 `Session.Messages`，因此：

- 不污染用户可见历史。
- 不被 Mongo 当作普通会话消息保存。
- 同一轮所有工具循环复用同一份检索结果。
- 固定历史前缀保持稳定，避免无意义破坏 Prompt Cache。

## 10. Mongo 持久化

集合：

```text
ai_memories
ai_memory_candidates
ai_memory_extraction_jobs
ai_memory_dream_runs
```

关键索引：

```text
ai_memories:
  status + updated_at
  tags
  scope.type + scope.key

ai_memory_candidates:
  status + updated_at
  unique sparse origin_key

ai_memory_extraction_jobs:
  unique agent_run_id + extractor_version
  status + updated_at
```

Mongo PO 在 `core/adapter/persistence/mongo/memory/po`，Domain 和 PO 的转换集中在 `mapper`，Repository 不手写字段转换。

## 11. 远程协议

Mobile 发给 Agent 的消息：

```text
ai_memory_list
ai_memory_create
ai_memory_update
ai_memory_delete
ai_memory_restore
ai_memory_candidate_list
ai_memory_candidate_approve
ai_memory_candidate_reject
```

Agent 返回：

```text
ai_memory_list_result
ai_memory_mutation_result
ai_memory_candidate_list_result
ai_memory_candidate_mutation_result
```

调用链示例：

```text
Mobile useAIMemoryActions.createMemory
-> Relay WebSocket
-> Agent.handleAIMemoryCreate
-> MemoryFacade.Create
-> CatalogService.Create
-> Repository.Save
-> ai_memory_mutation_result
-> useRemoteMessageHandler
-> useAIMemoryState.applyMemories
-> AIMemoryPanel 重新渲染
```

所有操作共享 `pendingActions.memory`，请求发送后显示加载状态；收到 result 或 error 时结束 pending。

## 12. Mobile 展示

知识页面第一层：

```text
资料知识库 | AI 记忆
```

AI 记忆第二层：

```text
有效记忆 | 待审核
```

有效记忆支持：

- 本地即时搜索和服务端刷新搜索。
- 新建和编辑。
- 展开查看目标、方案、结果、痛点、根因、经验和验证。
- 查看版本、可信度、标签、作用域和使用次数。
- 逻辑删除、查看已删除和恢复。

待审核支持：

- 查看自动提取内容和来源。
- 通过并创建有效记忆。
- 拒绝候选。

## 13. Dream 模式现状

已经存在：

- `DreamRun`
- `DreamAction`
- `DreamDecision`
- `DreamRunRepository`
- Mongo 和内存存储

尚未存在：

- Dream Application Service。
- 手动触发协议。
- 闲置时间调度器。
- 模型去重、合并、替代和版本决策实现。
- Mobile Dream 执行记录页面。

后续实现时推荐继续遵守：

```text
模型只生成 DreamAction 建议
-> Application Service 校验 HumanLocked、版本和状态
-> 单条应用并记录 Applied/FailureReason
-> DreamRun 保存完整审计结果
```

不要允许模型直接写 Mongo。

## 14. 调试入口

### 14.1 没有生成候选

检查：

1. AgentRun 是否以 `succeeded` 或 `failed` 完成。
2. `ObservingCommandService.Finish` 是否调用 Observer。
3. `ai_memory_extraction_jobs` 是否存在 pending/failed Job。
4. 默认模型是否在 `InitMemoryServices` 时可用。
5. `last_error` 是否为 JSON、模型调用或 Candidate 校验错误。

### 14.2 候选生成但 Mobile 看不到

检查：

```text
ai_memory_candidate_list
-> Agent.handleAIMemoryCandidateList
-> ai_memory_candidate_list_result
-> useRemoteMessageHandler
-> applyAIMemoryCandidates
```

同时确认 Relay 请求路由和 `request_id` 没有提前释放。

### 14.3 回答没有使用记忆

检查：

1. Memory 是否为 `active`。
2. Scope 是否匹配当前 workspace 或 session。
3. 用户输入是否过短或属于跳过词。
4. 标题、标签和正文是否与查询存在词项重叠。
5. `AssistantGenerationService.MemoryContext` 是否注入。
6. `AgentLoopService.withMemoryContext` 是否生成 `SyntheticReasonMemoryContext`。

## 15. 已知限制

- AI 记忆检索目前是轻量词法评分，不是 Embedding 检索。
- 自动提取使用应用启动时的默认 ChatModel，没有独立的记忆模型配置界面。
- 使用次数采用 Get 后 Save 的应用服务流程，高并发下不是原子自增。
- 候选通过时 Memory 和 Candidate 是两次保存，尚未使用 Mongo 事务封装。
- Mobile 尚不支持把候选合并到用户选择的已有 Memory。
- Dream 模式只有领域和持久化骨架。
- 脱敏使用规则匹配，不能代替完整敏感信息检测。

## 16. 验证命令

```powershell
go test ./core/domain/memory
go test ./core/application/memory/...
go test ./core/adapter/memory/...
go test ./core/adapter/persistence/mongo/memory/...
go test ./core/application/chat
go test ./core/remote/agent
go test ./core/architecture
go test ./...

cd mobile
npm run typecheck

git diff --check
```
