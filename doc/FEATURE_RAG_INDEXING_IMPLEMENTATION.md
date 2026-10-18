# RAG 文档索引功能实现

本文说明当前源码中 `IndexingService` 的真实实现。阅读对象是熟悉 Java/Spring Boot、需要快速理解 Go 分层和 RAG 导入链路的开发人员。

本文只描述 IndexingService。Milvus、SQLite FTS5、sqlite-vec、Embedding HTTP Provider、RetrievalService 和热点回填已经实现。Mobile 已提供知识库分类/文档上传、索引状态、失败重试、删除和检索预览，Chat 也可按 Session RAG 设置接入检索；Profile CRUD 和真实 Milvus 服务器 E2E 仍属于未完成边界。

## 1. 当前职责边界

索引应用服务负责把一个已上传的 Document 变成可检索的数据：

```text
IndexingJob
    |
    v
DocumentRepository.Get
    |
    +--> ProfileRepository 加载 Index / Parsing / Chunking / Embedding Profile
    |
    +--> DocumentObjectStore.Open 读取 MinIO 原文
    |
    +--> DocumentProcessor.Process
    |       |
    |       +--> ChunkSink.Accept -> ChunkRepository.SaveAll
    |
    +--> EmbeddingProvider.EmbedDocuments
    |       |
    |       +--> VectorStore.Upsert
    |       +--> ChunkRepository.MarkEmbedded
    |
    +--> KeywordStore.Upsert
    |
    +--> Document ready + IndexingJob completed
```

职责分工如下：

| 层 | 当前职责 |
| --- | --- |
| `domain/knowledge` | Document、Chunk、Profile、IndexingJob、EmbeddingVector 等领域对象及校验 |
| `port/knowledge` | Repository、ObjectStore、DocumentProcessor、Embedding、Vector、Keyword 接口 |
| `application/knowledge/indexing` | Submit/Run 用例、状态机、分页、幂等和错误恢复 |
| `adapter/documentprocessor/grpc` | Go 到 Python 的 gRPC 解析与分块实现 |
| `adapter/persistence/mongo/knowledge` | MongoDB 持久化实现 |
| `adapter/vectorstore/sqlitevec` | 本地热点向量的 sqlite-vec 实现 |
| `application/knowledge/retrieval` | 热点回填、LocalQualityGate 和 RRF |
| 后续检索层 | LRU、远程关键词检索和更多 Embedding Provider |

IndexingService 不知道 MongoDB、MinIO、Milvus 的具体 SDK 类型，只依赖 Port。这与 Spring Boot 中 Service 依赖 Repository/Gateway 接口、由 Configuration 组装实现类是同一思路。

## 2. 目录和入口

```text
core/application/knowledge/indexing/
├── api/service.go                 # 用例接口
├── command/commands.go            # Submit、Run 输入 DTO
├── result/results.go              # Submit、Run 输出 DTO
└── service/
    ├── configuration.go            # 依赖对象和默认配置
    ├── indexing_service.go         # 主状态机
    ├── profiles.go                 # Profile 加载和一致性校验
    ├── embedding.go                # 分页 Embedding 和向量转换
    ├── progress.go                 # 状态保存、失败保存、关键词索引
    └── chunk_sink.go               # Python ChunkBatch 的流式持久化
```

应用入口是接口：

```go
type Service interface {
    Submit(ctx context.Context, command command.Submit) (result.Submit, error)
    Run(ctx context.Context, command command.Run) (result.Run, error)
}
```

实现通过构造函数创建：

```go
service, err := indexingservice.New(indexingservice.Configuration{
    Documents:  documentRepository,
    Profiles:   profileRepository,
    Jobs:       indexingJobRepository,
    Chunks:     chunkRepository,
    Objects:    documentObjectStore,
    Processor:  documentProcessor,
    Embeddings: embeddingResolver,
    Vectors:    vectorStore,
    Keywords:   keywordStore,
    JobIDs:     snowflakeGenerator,
    StableIDs:  contentHashDeriver,
    PageSize:   64,
})
```

`Configuration` 是实现类的依赖对象，不是领域对象，也不是 HTTP/gRPC DTO。`New` 会校验每个必需依赖并把默认 `PageSize` 设为 `64`，默认时钟设为 `time.Now`。

## 3. 用户或后台如何启动索引

当前用例层接收两个明确命令。上传流程或后台调度器先提交任务，再运行任务：

```go
submitted, err := service.Submit(ctx, indexingcommand.Submit{
    DocumentID:     documentID,
    IndexProfileID: indexProfileID,
})
if err != nil {
    return err
}

completed, err := service.Run(ctx, indexingcommand.Run{
    JobID: submitted.Job.ID,
})
```

当前 App 已初始化 Mongo Knowledge Repository、MinIO ObjectStore、gRPC DocumentProcessor、EmbeddingModelResolver，并可按配置初始化 Milvus VectorStore 和 SQLite FTS5 KeywordStore。上述依赖全部存在时会最终装配 IndexingService。Mobile 上传/索引/重试/删除/检索预览和 Chat RAG 已接入；Profile CRUD 与真实 Milvus 服务器验证仍未完成，因此不能把当前状态表述为已完成真实生产级端到端 RAG。所有基础设施继续由 Composition/Application 初始化层创建，`IndexingService` 不直接 `new` Mongo、Milvus 或 SQLite 客户端。

## 4. Submit 流程

入口是 `service/indexing_service.go` 的 `IndexingService.Submit`。

### 4.1 输入校验

方法先 Trim `DocumentID` 和 `IndexProfileID`，然后加载 Document 和 IndexProfile：

```text
校验 document_id 非空
校验 index_profile_id 非空
DocumentRepository.Get(document_id)
Document.Validate()
拒绝已逻辑删除 Document
ProfileRepository.GetIndexProfile(index_profile_id)
IndexProfile.Validate()
要求 IndexProfile.Status == active
```

`IndexProfile` 必须是 active，不能使用 building、failed 或 retired Profile 进行正式索引。ParsingProfile、ChunkingProfile 和 EmbeddingProfile 会在 Run 阶段重新加载，这样任务执行时仍会核对当前配置。

### 4.2 创建 IndexingJob

提交阶段不读取原文、不启动 Python、不调用 Embedding。它只创建一个可恢复任务：

```go
job := domainknowledge.IndexingJob{
    ID:              configuration.JobIDs.NewID(),
    KnowledgeBaseID: document.KnowledgeBaseID,
    DocumentID:      document.ID,
    IndexProfileID:  profile.ID,
    Stage:           domainknowledge.IndexingStageParse,
    Status:          domainknowledge.IndexingJobStatusPending,
    CreatedAt:       now,
    UpdatedAt:       now,
}
```

ID 由 `IDGenerator` 提供，当前设计使用雪花 ID。Chunk 和 Embedding 不使用这个随机任务 ID，而使用 `StableIDDeriver` 根据业务身份稳定派生。

## 5. Run 流程

入口是 `IndexingService.Run`。它按以下顺序执行：

```text
1. 校验 job_id
2. 进程内注册 running[job_id]
3. IndexingJobRepository.Get
4. DocumentRepository.Get
5. 校验 Job 与 Document 的 ID、KnowledgeBaseID 一致
6. 加载并校验四类 Profile
7. 保存 running + parse + document parsing
8. Open 原文并调用 DocumentProcessor
9. ChunkSink 分批保存 Chunk
10. 保存 embed + document embedding
11. 分页读取 Chunk，生成 Embedding
12. VectorStore.Upsert
13. ChunkRepository.MarkEmbedded
14. 保存 index + document indexing
15. 分页写入 KeywordStore
16. 保存 completed + document ready
```

同一 `IndexingService` 实例内，同一个 JobID 同时只能有一个 Run。`running` 是进程内保护，不能替代 Mongo 分布式锁；如果未来启动多个 Go 进程，需要在 Job Repository 增加带条件的 claim/lease，避免跨进程重复执行。

## 6. Profile 加载和一致性

`profiles.go` 依次加载：

```text
IndexProfile
    -> ParsingProfile
    -> ChunkingProfile
    -> EmbeddingProfile
```

每个返回对象都必须通过领域 `Validate`，并且返回对象的 ID 必须等于 IndexProfile 中保存的引用 ID。Parsing、Chunking 和 Embedding Profile 不能是逻辑删除状态。

这一步不把三个 Profile 合并成一个巨型配置对象。`IndexProfile` 只是组合引用：

```text
IndexProfile.parsing_profile_id
IndexProfile.chunking_profile_id
IndexProfile.embedding_profile_id
```

这样切换 EmbeddingProfile 时可以复用相同 Chunk，只为缺失 Profile 生成向量。

## 7. Parse 和 Chunk 阶段

### 7.1 打开原文

`Run` 通过 `DocumentObjectStore.Open(document.ObjectKey)` 得到新的 `io.ReadCloser`。每次重试都会重新 Open，而不是让 gRPC Adapter 在内部重放不可重读的 `io.Reader`。

### 7.2 ChunkSink

`chunkPersistenceSink.Accept` 收到一个 `[]ChunkDraft` 后：

```text
ChunkDraft
    -> 构造 ChunkIdentity
    -> StableIDDeriver.ChunkID
    -> 组装 domainknowledge.Chunk
    -> Chunk.Validate
    -> ChunkRepository.SaveAll
```

稳定身份包含：

```text
DocumentID
DocumentVersion
ParsingProfileID
ChunkingProfileID
Ordinal
ContentHash
```

当前 ContentHash 由 Go gRPC Adapter 根据 Chunk 文本计算，Python 不决定 Mongo/Milvus 的主键。Mongo `SaveAll` 使用 Upsert；已有 Chunk 的 `EmbeddingProfileIDs` 只在插入时初始化，更新文本时不会把已完成的 Embedding 状态清空。

### 7.3 状态切换

Processor 返回第一个 ChunkBatch 前，Sink 的 `onFirstBatch` 回调会保存：

```text
IndexingJob: running / chunk
Document:    chunking
```

Parse 阶段没有批次时仍由 Processor 的 Summary 校验保证失败，不会把空文档标记 ready。

## 8. ProcessingSummary 二次校验

当前 gRPC Adapter 已校验 Python 返回的 Metadata，但 Application 层仍在 `processDocument` 返回后再次校验：

```text
Summary.Validate()
Summary.DocumentID       == Document.ID
Summary.DocumentVersion  == Document.Version
Summary.ContentType      == Document.ContentType
Summary.ParserID/Version == ParsingProfile.ParserID/Version
Summary.ChunkingStrategy == ChunkingProfile.Strategy/Version
Summary.ChunkCount       == Sink 实际保存数量
```

这是接口隔离后的必要防线。未来替换本地 Processor 或测试 Fake 时，错误的解析实现不能静默写入当前 IndexProfile。

## 9. 分页 Embedding

Embedding 阶段不会一次性读取整篇文档所有 Chunk。游标初始为 `afterOrdinal = -1`：

```text
ListByDocumentPage(afterOrdinal, PageSize)
    -> 检查 ordinal 连续递增
    -> 过滤已经包含目标 EmbeddingProfileID 的 Chunk
    -> EmbedDocuments(当前缺失批次)
    -> 校验 EmbeddingResult
    -> VectorStore.Upsert
    -> ChunkRepository.MarkEmbedded
    -> 保存 CompletedChunks
    -> afterOrdinal = 本页最后 ordinal
```

Chunk 页面必须从 ordinal 0 连续到 `TotalChunks - 1`。分页仓储使用 Mongo：

```text
ordinal > afterOrdinal
sort ordinal ASC
limit PageSize
```

### 9.1 EmbeddingResult 校验

`embedding.go` 会拒绝以下结果：

- Profile ID 与目标 EmbeddingProfile 不同；
- Dimensions 与 Profile.Dimensions 不同；
- 输出数量与输入 Chunk 数量不同；
- 输出 ID 重复；
- 某个 Chunk 没有对应输出；
- Vector 维度错误；
- Vector 含 NaN 或 Inf。

向量转换为 `domainknowledge.EmbeddingVector`，其 ID 为：

```text
EmbeddingID = StableIDDeriver.EmbeddingID(ChunkID, EmbeddingProfileID)
```

因此相同 Chunk 和相同 EmbeddingProfile 重试时会 Upsert 同一个向量；不同 EmbeddingProfile 即使维度相同，也会得到不同 EmbeddingID，不能混用。

### 9.2 为什么顺序必须固定

源码明确保证：

```text
EmbedDocuments
-> VectorStore.Upsert
-> ChunkRepository.MarkEmbedded
```

如果先 MarkEmbedded，之后 Milvus 写入失败，重试会误以为 Chunk 已完成而跳过向量生成。当前测试会记录事件序列并要求每页都是 `vector,mark`。

## 10. Keyword 索引阶段

Embedding 阶段完成后，Job 进入 `index`，Document 进入 `indexing`。服务再次按相同 ordinal 游标分页读取 Chunk，组装：

```go
domainknowledge.KeywordDocument{
    ChunkID:         chunk.ID,
    KnowledgeBaseID: chunk.KnowledgeBaseID,
    DocumentID:      chunk.DocumentID,
    DocumentVersion: chunk.DocumentVersion,
    Text:            chunk.Text,
    Deletion:        chunk.Deletion,
    SyncSequence:    chunk.SyncSequence,
}
```

然后调用 `KeywordStore.Upsert`。当前 App 可装配 SQLite FTS5 实现；未来也可以增加线上实现，但 Application 不出现 SQLite 类型。

Keyword 阶段失败时，任务从当前状态进入 failed。重试会重复 Upsert，要求具体 Store 也采用幂等主键。

## 11. 状态机

### IndexingJob

```text
pending
   |
   v
running / parse
   |
running / chunk
   |
running / embed
   |
running / index
   |
completed / completed
```

任意阶段失败：

```text
running / current-stage -> failed / current-stage
```

失败信息写入 `IndexingJob.LastError`，当前未完成数量写入 `FailedChunks`。下次 Run 会清除错误并重新从 Parse 开始；ChunkID 和 EmbeddingID 的稳定性保证重复步骤可以 Upsert 和跳过已完成向量。

### Document

```text
uploaded -> parsing -> chunking -> embedding -> indexing -> ready
```

失败时保存：

```text
Document.Status = failed
Document.FailureReason = 错误文本
```

失败保存使用 `context.WithoutCancel(ctx)` 和短超时，避免用户取消请求后连失败状态都无法写入。若失败状态本身持久化失败，返回值会通过 `errors.Join` 同时保留业务错误和持久化错误。

## 12. 重试和幂等场景

### 场景一：Python 在分块后进程崩溃

1. 已收到的 Chunk 已通过 `SaveAll` 写入 Mongo。
2. Job/Document 被保存为 failed。
3. 重试重新从 MinIO Open 原文。
4. 相同 Chunk 使用相同 ChunkID，Mongo Upsert 覆盖同一记录。
5. 新的 Chunk 页面继续完成。

### 场景二：Milvus Upsert 成功后 Go 进程在 MarkEmbedded 前退出

1. Mongo 仍显示 Chunk 缺少目标 Profile。
2. 重试再次调用 Embedding。
3. Milvus 使用相同 EmbeddingID Upsert，结果幂等。
4. 成功后再 MarkEmbedded。

当前实现没有在 VectorStore 成功后、MarkEmbedded 前引入跨存储事务；这是故意的，通过稳定 ID 和幂等 Upsert 换取可恢复性。

### 场景三：已有旧 EmbeddingProfile，切换新 Profile

```text
Chunk.EmbeddingProfileIDs = [embedding-old]
目标 Profile = embedding-new
    -> missing 判断为 true
    -> 生成 embedding-new
    -> 写入新的 EmbeddingID
    -> addToSet embedding-new
```

旧向量不会被覆盖，也不需要重新解析或重新分块。

## 13. 错误传播和调试断点

建议按以下顺序断点：

| 位置 | 观察内容 |
| --- | --- |
| `IndexingService.Submit` | Document、IndexProfile、生成的 Job |
| `Run` 保存 parse 前 | Job/Document 初始状态 |
| `chunkPersistenceSink.Accept` | Draft ordinal、ChunkID、SaveAll 批次 |
| `processDocument` 返回后 | ProcessingSummary 和 sink.count |
| `embedChunks` | afterOrdinal、missing Chunk 数、CompletedChunks |
| `embeddingVectors` | Profile ID、维度、稳定 EmbeddingID |
| `VectorStore.Upsert` 前后 | 向量写入是否成功 |
| `ChunkRepository.MarkEmbedded` | 是否发生在 Vector Upsert 之后 |
| `indexKeywords` | FTS 批次和 ordinal 连续性 |
| `fail` | LastError、FailureReason、最终持久化结果 |
| `Run` 最后保存 | ready/completed 的完整对象 |

如果看到 Document 已是 ready 但 Job 不是 completed，优先检查最后一次 `saveProgress` 的两个 Repository 是否只成功了一个；当前 Mongo 实现还不是事务边界，诊断时要分别查看两条记录。

## 14. 当前实现限制和下一步

已完成：

- Chunk 分页仓储；
- Parse/Chunk/Embed/Index/Completed 状态机；
- Python 流式 Chunk 持久化；
- Embedding 结果严格校验；
- Vector Upsert 后 MarkEmbedded；
- 失败状态和进程内同 Job 并发保护；
- 稳定 ID、分页重试和已有 Profile 跳过；
- 应用层单元测试。

尚未完成：

- sqlite-vec LRU 和增量同步；
- 跨 Go 进程的 Job claim/lease；
- Index Profile 的完整 CRUD；
- 独立 CLI 导入/检索命令；
- 真实 Milvus 服务器端到端集成测试。

这些实现应继续放在对应 Adapter/Composition 目录，不能把 SDK 调用塞进当前 `service` 包。

## 15. 验证命令

```powershell
gofmt -w core/application/knowledge/indexing
go test ./core/application/knowledge/indexing/...
go test ./core/adapter/persistence/mongo/knowledge/repository
go test ./...
go vet ./...
go test ./core/architecture
git diff --check
```
