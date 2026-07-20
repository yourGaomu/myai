# RAG 本地优先检索实现

本文说明 MyAI 当前真实存在的 RAG 检索编排，包括 Retrieval 用例、LocalQualityGate、RRF、本地并行检索、Milvus 回退、Mongo Chunk 补全、降级诊断和热点回填。

当前实现同时包含三条入口：底层 `RetrievalService`、面向用户和工具的 `KnowledgeSearchFacade`，以及 Chat 在每条用户消息前执行的 Session RAG 自动检索。RAG Context 以合成消息追加到本轮用户消息前，不改写固定 System Prompt。

## 1. 目录和对象职责

```text
core/application/knowledge/retrieval/
├── api/
│   └── service.go
├── command/
│   └── retrieve.go
├── result/
│   └── retrieve.go
├── port/
│   └── contracts.go
└── service/
    ├── configuration.go
    ├── quality_gate.go
    ├── rrf.go
    ├── retrieval_service.go
    ├── hydrate.go
    ├── cache.go
    └── *_test.go
```

按照 Java/Spring Boot 理解：

| Go 对象 | Spring Boot 对照 | 职责 |
|---|---|---|
| `retrieval/api.Service` | Service 接口 | 定义检索用例入口 |
| `RetrievalService` | ServiceImpl | 编排 Profile、Embedding、Store、RRF 和回填 |
| `LocalQualityGate` Port | Strategy 接口 | 判断本地结果是否足够 |
| `LocalQualityGate` 实现 | StrategyImpl | 默认阈值算法 |
| `RankFusion` Port | Strategy 接口 | 定义排名融合能力 |
| `ReciprocalRankFusion` | StrategyImpl | RRF 默认实现 |
| `command.Retrieve` | Command DTO | 查询和严格模式输入 |
| `result.Retrieve` | Result DTO | Hits 和 Diagnostics 输出 |

Service 只依赖 `core/port/knowledge`，不知道 Milvus SDK、sqlite-vec SQL、Mongo BSON 或 Embedding HTTP 协议。

## 2. API

```go
type Service interface {
    Retrieve(ctx context.Context, command command.Retrieve) (result.Retrieve, error)
}
```

调用示例：

```go
response, err := retrievalService.Retrieve(ctx, command.Retrieve{
    Query: knowledge.RetrievalQuery{
        Text:               "Plan 模式为什么没有执行？",
        KnowledgeBaseIDs:   []string{"kb-project"},
        IndexProfileID:     "index-bge-m3-v1",
        EmbeddingProfileID: "embedding-bge-m3-v1",
        TopK:               8,
    },
    Strict: false,
})
```

`Strict=false` 面向 Chat 自动检索：底层单通道失败时尽量降级，并把错误放进 Diagnostics。

`Strict=true` 面向显式 `knowledge_search`：所有可用通道都失败且没有任何结果时返回 error。

参数或 Profile 不一致属于调用错误，无论是否 Strict 都直接失败。

## 3. Result 和 Diagnostics

```go
type Retrieve struct {
    Hits        []knowledge.RetrievalHit
    Diagnostics Diagnostics
}
```

Diagnostics 包含：

```text
LocalVectorHits
LocalKeywordHits
RemoteVectorHits
RemoteFallback
CacheFillCount
LocalQuality
Warnings[]
```

Warnings 只记录适合开发排错的错误摘要，不保存 API Key、原始向量或完整文档。

## 4. 完整调用链

```text
RetrievalService.Retrieve
-> RetrievalQuery.Validate
-> 校验 TopK <= MaxCandidates
-> ProfileRepository.GetIndexProfile
-> ProfileRepository.GetEmbeddingProfile
-> KnowledgeBaseRepository.Get
-> EmbeddingModelResolver.Resolve
-> EmbeddingProvider.EmbedQuery
-> 并行执行本地 Vector Search 和 Keyword Search
-> RRF 融合本地候选
-> LocalQualityGate.Evaluate
-> 不通过时执行 Milvus Vector Search
-> RRF 融合本地和远程候选
-> ChunkRepository.GetByIDs
-> DocumentRepository.Get
-> 生成 RetrievalHit
-> 回填本地 FTS5 和 sqlite-vec
-> 返回 Hits + Diagnostics
```

RAG 检索没有写入 `llm.Model.Generate`。LLM Adapter 仍然只负责模型协议映射。

## 5. Profile 和 KnowledgeBase 校验

检索前必须满足：

```text
IndexProfile 存在
AND Status == active
AND 未逻辑删除
AND IndexProfile.EmbeddingProfileID == Query.EmbeddingProfileID
AND EmbeddingProfile 存在且未删除
AND 每个 KnowledgeBase 已启用 RAG
AND KnowledgeBase.ActiveIndexProfileID == Query.IndexProfileID
```

这一层防止以下错误：

- 同维度但不同 Embedding Model 的向量混用；
- 新 IndexProfile 尚在 building 时提前参与检索；
- KnowledgeBase 已切换 Profile，但调用方仍发送旧 Profile；
- 已逻辑删除的知识继续进入模型上下文。

## 6. Query Embedding

当本地或远程存在 VectorStore 时：

```go
provider, err := EmbeddingModelResolver.Resolve(embeddingProfile)
result, err := provider.EmbedQuery(ctx, EmbedRequest{
    EmbeddingProfileID: query.EmbeddingProfileID,
    Inputs: []EmbedInput{{
        ID:   "retrieval-query",
        Text: query.Text,
    }},
})
```

Service 验证：

- 只返回一个 Query Vector；
- `EmbeddingProfileID` 一致；
- Output ID 是 `retrieval-query`；
- 维度、数量和数值通过 `EmbedResult.Validate`。

Provider 失败时不会阻止 FTS5。关键词通道仍然执行，Vector 通道记录 Warning 后降级。

## 7. 本地并行检索

两个本地任务同时执行：

```text
任务 A：sqlite-vec Health -> VectorStore.Search
任务 B：SQLite FTS5 KeywordStore.Search
```

每个任务只向自己的单元素 Channel 写结果，主 goroutine 再统一合并，不共享写入状态。

候选数量不是直接等于 TopK：

```text
candidate_limit = min(TopK * candidate_multiplier, max_candidates)
```

默认：

```text
candidate_multiplier = 3
max_candidates = 100
```

多取候选是为了给 RRF、去重和无效 Chunk 过滤保留空间。

## 8. LocalQualityGate

接口：

```go
type LocalQualityGate interface {
    Evaluate(input LocalQualityInput) (knowledge.LocalQualityDecision, error)
}
```

默认通过条件：

```text
Local VectorStore Health 成功
AND 本地 Vector Search 确认 Profile 可检索
AND 本地 RRF 去重结果数量 >= min_local_results
AND TopVectorScore >= min_local_score
```

默认值：

```text
min_local_results = 3
min_local_score = 0.55
```

`TopVectorScore` 使用 VectorStore 的归一语义，不使用 RRF Score。RRF 的典型分值约为 `1 / (60 + rank)`，不能和 0.55 阈值直接比较。

Decision 保存全部失败原因，例如：

```text
local vector index is unavailable
local index profile does not match the request
local results 1 are below minimum 3
local top vector score 0.4200 is below minimum 0.5500
```

## 9. RRF

不同检索引擎的原始分数不可直接相加：

- sqlite-vec 的 COSINE Score；
- FTS5 的 BM25；
- Milvus 的 COSINE/L2 Score。

统一使用 Reciprocal Rank Fusion：

```text
RRFScore(chunk) = sum(1 / (k + rank_in_list))
```

默认：

```text
k = 60
```

融合规则：

- 同一 RankedList 内重复 Chunk 只贡献一次；
- 多个通道命中同一 Chunk 时累加贡献；
- 按 RRF Score 降序；
- 同分时按最佳 Rank、首次出现顺序和 ChunkID 保证确定性；
- 最终 Rank 从 1 重新生成。

当前参与列表：

```text
Local Vector
Local Keyword
Remote Milvus Vector（仅质量不足时）
```

远程 KeywordStore 尚未实现，因此当前没有 Remote Keyword 列表。

## 10. Milvus 回退

只有本地质量不通过时才执行：

```go
RemoteVectors.Search(ctx, knowledge.VectorQuery{
    EmbeddingProfileID: query.EmbeddingProfileID,
    KnowledgeBaseIDs:   query.KnowledgeBaseIDs,
    DistanceMetricID:   indexProfile.DistanceMetricID,
    Options:            indexProfile.VectorIndexOptions,
    Vector:             queryVector,
    TopK:               candidateLimit,
})
```

本地质量通过时 Milvus 调用次数为 0。该行为有单元测试保护，避免“本地优先”退化成每次本地和远程双查。

Milvus 失败时保留已有本地候选，并写入 Diagnostics。只有 Strict 模式且最终无结果时才返回 error。

## 11. Chunk 和来源补全

VectorHit 和 KeywordHit 只保存 ChunkID。RRF 后统一执行：

```text
ChunkRepository.GetByIDs
-> 按 RRF 顺序建立 Chunk Map
-> 过滤不存在或已删除 Chunk
-> 校验 Parsing/Chunking/Embedding Profile
-> 校验 KnowledgeBase 范围
-> DocumentRepository.Get 获取 FileName
-> 组装 RetrievalHit
```

`SourceLocation` 优先组合：

```text
SourceHeading
page N
bytes start-end
```

Document 元数据读取失败时，仍使用 DocumentID 作为 SourceName，并记录 Warning，不丢弃有效 Chunk 正文。

## 12. 热点回填

只有远程回退产生最终命中且 `cache_remote_results=true` 时执行。

关键词回填：

```text
Remote final Chunk
-> KeywordDocument
-> LocalKeywords.Upsert
```

向量回填：

```text
Remote final Chunk
-> LocalVectors.EnsureIndex
-> EmbeddingProvider.EmbedDocuments
-> 使用 Milvus 返回的 EmbeddingID 构造 EmbeddingVector
-> LocalVectors.Upsert
```

当前 Milvus `VectorStore` Port 只返回命中 ID、距离和分数，不返回原始向量，因此第一版必须使用相同 EmbeddingProfile 重新生成热点向量。

后续应新增独立的 `EmbeddingVectorReader` Port，从 Milvus 批量读取原向量。届时只替换 cache.go 的向量来源，不修改 RetrievalService、Domain 或 sqlite-vec Adapter。

缓存回填错误只进入 Warnings，不影响已经得到的检索结果。

## 13. 配置

```yaml
rag:
  retrieval:
    candidate_multiplier: 3
    max_candidates: 100
    min_local_results: 3
    min_local_score: 0.55
    rrf_k: 60
    cache_remote_results: true
```

约束：

- `candidate_multiplier >= 1`；
- `max_candidates >= 1`；
- `TopK <= max_candidates`；
- `min_local_results >= 1`；
- `min_local_score` 在 `(0, 1]`；
- `rrf_k >= 1`。

## 14. Application 装配

```text
Application.InitKnowledgeStorage
-> 初始化 Mongo Knowledge Repositories
-> 初始化 EmbeddingModelResolver
-> 初始化 Local sqlite-vec / FTS5（可选）
-> 初始化 Remote Milvus（可选）
-> initRetrievalService
-> retrievalservice.New
-> Application.retrievalService
```

Getter：

```go
service := core.GetApp().GetRetrievalService()
```

以下任一结构都可以创建 RetrievalService：

```text
Local Vector + Local Keyword + Remote Vector
Local Vector + Local Keyword
Remote Vector
Local Keyword only
```

Mongo Profile、Chunk、Document、KnowledgeBase Repository 和 Embedding Resolver 是必需依赖。

## 15. knowledge_search Tool 入口

工具目录：

```text
core/tool/local/
├── knowledge_search.go
├── knowledge_search_args.go
├── knowledge_search_result.go
└── knowledge_search_test.go
```

职责保持分离：

| 文件 | 职责 |
|---|---|
| `knowledge_search.go` | Tool 实现、Schema、权限和用例调用 |
| `knowledge_search_args.go` | JSON 参数 DTO、Trim、默认值和 Domain Query 映射 |
| `knowledge_search_result.go` | Retrieval Result 到稳定 JSON DTO 的映射 |
| `knowledge_search_test.go` | Schema、参数、结果、错误和权限测试 |

模型调用参数：

```json
{
  "query": "Plan 模式为什么没有执行？",
  "knowledge_base_ids": ["kb-project"],
  "category_ids": ["category-project"],
  "top_k": 8
}
```

规则：

- `query` 必填；`knowledge_base_ids` 和 `category_ids` 都是可选范围；
- `category_ids` 会展开到所有后代分类下的启用 KnowledgeBase；
- 两种范围都为空表示跨全部启用知识库查询；
- `KnowledgeSearchFacade` 根据每个 KnowledgeBase 的 `ActiveIndexProfileID` 自动分组，模型不需要猜测 IndexProfile 或 EmbeddingProfile；
- `top_k` 省略时默认为 `8`，上限仍由 RetrievalService 的 `max_candidates` 统一校验；
- Agent Loop 执行工具时，如果 Session 配置了 RAG 范围，Session 范围是硬边界，工具参数不能替换或扩大它；Session 为 `off` 时工具直接拒绝；
- Tool 权限为 `PermissionRead`，继续经过现有权限、Hook、TaskRecorder 和 ToolResult 链路；
- Tool 固定传递 `Strict=true`，显式检索在全部通道失败且没有结果时直接返回 error；
- Tool 只依赖 `knowledge/search/api.Service`，不知道 Milvus、sqlite-vec、FTS5、Mongo 或 Embedding Provider 实现。

调用链：

```text
AgentLoopService 收到模型 ToolCall
-> ExecutionService
-> RegisterTools.GetTool("knowledge_search")
-> KnowledgeSearchTool.Call
-> 参数 DTO 映射为 knowledge/search command.Search
-> KnowledgeSearchFacade.Search
-> 按 KnowledgeBase 的 ActiveIndexProfileID 分组
-> 每组调用 retrieval/api.Service.Retrieve(Strict=true)
-> 跨 Profile RRF 融合
-> Result DTO 输出 Hits + Diagnostics JSON
-> ToolResult 进入下一轮模型上下文
```

注册发生在 `Application.InitRegister()`：

```go
if app.knowledgeSearchService != nil {
    localTools = append(localTools, local.NewKnowledgeSearchTool(app.knowledgeSearchService))
}
```

因此 RAG 依赖没有完整装配时不会向模型暴露一个必然失败的工具。`InitKnowledgeStorage -> initRetrievalService -> initKnowledgeSearchService -> InitRegister` 的启动顺序保证正常配置下工具可以被注册。

输出 JSON 包含：

```text
query
count
hits[]: knowledge_base_id / document_id / chunk_id / text / source / score / rank / channel / origin
diagnostics: local hits / remote fallback / cache fill / local quality / warnings
```

## 16. Chat 自动检索和 Session 范围

ChatService 在追加当前用户消息前调用 `chat/retrieval/service.ContextService.Prepare`：

```text
ChatService.SendMessageStreamForSession
-> SessionLoader.Load
-> ContextService.Prepare
-> Session.RAGSettings 归一化
-> off/manual: 跳过
-> auto: DefaultTriggerPolicy 判断是否像知识问题
-> always: 每条非空用户消息都检索
-> KnowledgeSearchFacade.Search
-> ContextFormatter.Format
-> MessageCommandService.AppendUserMessage
   -> SyntheticReasonRAGContext
   -> SyntheticReasonRuntimeInstruction
   -> User Message
-> AgentLoopService.Generate
```

Session RAG 配置保存在 Session 聚合和 Mongo Session Record 中：

```go
type RAGSettings struct {
    Mode             RetrievalMode
    KnowledgeBaseIDs []string
    CategoryIDs      []string
    TopK             int
}
```

因此，知识库范围是会话级配置；检索动作是每条用户消息级动作。一个 Session 可以选择“自动”，另一个 Session 可以关闭；同一 Session 的每一轮根据当前模式和消息内容决定是否调用 Search。空范围不是关闭，而是搜索所有未删除且启用 RAG 的 KnowledgeBase。

RAG Context 被保存为合成消息，方便历史重放和调试；它不改变固定系统提示词，也不把整个知识库永久拼进上下文。

手机端 `KnowledgePanel` 同时提供分类树与目录管理：创建子分类、移动分类、删除空分类、二次确认递归逻辑删除，以及把 KnowledgeBase 移入其他分类或根目录。KnowledgeBase 配置页还可以切换 Active IndexProfile、启停 RAG 和逻辑删除 KnowledgeBase。移动分类或 KnowledgeBase 只更新 Catalog 元数据，不重新解析、分块或向量化。

## 17. 测试覆盖

当前测试覆盖：

- LocalQualityGate 边界和全部失败原因；
- RRF 跨通道去重、分数和稳定排序；
- 本地质量通过时不访问 Milvus；
- 本地不足时 Milvus 回退；
- 远程 Chunk 回填本地 VectorStore 和 KeywordStore；
- 全通道失败时 Strict 模式返回 error；
- Embedding Provider 失败时保留关键词结果；
- IndexProfile 与 EmbeddingProfile 不一致时拒绝检索；
- SourceName 和 RetrievalHit 补全。
- `knowledge_search` Schema、只读权限和默认 `top_k`；
- Tool 参数 Trim、Command 映射和 `Strict=true`；
- Hits、来源和 Diagnostics JSON 映射；
- Retrieval error 原样传播；
- RetrievalService 未装配时不注册 Tool。
- Session 只配置 Category 或只配置 KnowledgeBase 时，模型提供的另一类范围不会绕过 Session 硬边界；
- 分类后代解析、空范围和跨 Profile RRF；
- Session `off/manual/auto/always` 的自动检索触发规则；
- Session 范围和 TopK 进入 Search command，检索片段进入 RAG Prompt。

验证：

```powershell
go test ./core/application/knowledge/retrieval/... -v
go test ./core/tool/local ./core -v
go test ./core/config -v
go test ./...
go vet ./...
go test ./core/architecture
```

## 18. 当前限制

- 自动触发目前是确定性的关键词、问句和长度启发式，尚未接入模型路由器；
- 手机端已经展示检索结果和 AssistantDone diagnostics，但聊天消息内还没有专门的引用卡片；
- 尚未实现远程关键词检索；
- 文档逻辑删除后，Document/Chunk 回填会过滤删除状态；向量层的直接删除同步仍需通过 Embedding ID 补齐；
- 热点回填当前同步执行，尚未放入受控异步队列；
- 尚未实现 LRU、`last_accessed_at` 和容量淘汰；
- 尚未保存 RetrievalRecord 和当时注入的 Context Snapshot；
- CLI 直接检索命令尚未实现，手机端已有知识库和检索测试入口；
- 尚未使用真实 Milvus 服务执行端到端集成测试。
