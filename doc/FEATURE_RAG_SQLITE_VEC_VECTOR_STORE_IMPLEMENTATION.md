# RAG sqlite-vec VectorStore 实现

本文说明 MyAI 本地热点向量库的真实实现，包括 Port 与 Adapter 的关系、SQLite Runtime、Schema、EmbeddingProfile 隔离、Upsert、KNN Search、逻辑删除、SyncSequence、并发和 Application 装配。

本文只描述 sqlite-vec Adapter。LocalQualityGate、本地优先回退、RRF 和热点回填已在 `FEATURE_RAG_RETRIEVAL_IMPLEMENTATION.md` 落地；LRU 淘汰仍属于后续阶段。

## 1. 实现位置

```text
core/port/knowledge/vector_store.go
core/domain/knowledge/retrieval.go
core/domain/knowledge/vector_index.go

core/adapter/vectorstore/sqlitevec/
├── config.go
├── path.go
├── schema.go
├── store.go
└── store_test.go

core/infra/sqliteruntime/
└── runtime.go
```

按照 Java/Spring Boot 的习惯理解：

| Go 对象 | Spring Boot 对照 | 职责 |
|---|---|---|
| `knowledge.VectorStore` | Repository/Port 接口 | 定义统一向量存储能力 |
| `sqlitevec.Store` | 本地 Repository 实现类 | 用 sqlite-vec 保存和检索热点向量 |
| `milvus.Store` | 远程 Repository 实现类 | 用 Milvus 保存线上完整向量 |
| `knowledge.EmbeddingVector` | Domain POJO | 向量及业务身份、版本和同步状态 |
| `knowledge.VectorQuery` | Query DTO | 向量检索参数 |
| `knowledge.VectorHit` | Result DTO | 统一检索结果 |
| `sqliteruntime` | Infrastructure Configuration | 统一注册 SQLite Driver 和 WASM Runtime |

Adapter 不定义业务接口，也不把 sqlite-vec 类型泄漏到 Application 或 Domain。

## 2. 统一 VectorStore 接口

本地和远程实现共同实现：

```go
type VectorStore interface {
    EnsureIndex(ctx context.Context, definition VectorIndexDefinition) error
    Upsert(ctx context.Context, embeddings []EmbeddingVector) error
    Search(ctx context.Context, query VectorQuery) ([]VectorHit, error)
    MarkDeleted(ctx context.Context, deletion VectorDeletion) error
    Health(ctx context.Context) error
}
```

因此 RetrievalService 后续只依赖 Port：

```text
RetrievalService
├── Local VectorStore  -> sqlitevec.Store
└── Remote VectorStore -> milvus.Store
```

它不需要在业务代码中拼 sqlite-vec SQL 或调用 Milvus SDK。

## 3. Driver 和 Runtime 选择

当前依赖固定为：

```text
github.com/asg017/sqlite-vec-go-bindings v0.1.6
github.com/ncruces/go-sqlite3 v0.19.0
github.com/tetratelabs/wazero v1.8.1
```

采用 ncruces WASM 绑定的原因：

- Windows 不需要额外安装 `sqlite3.h`；
- 不需要 CGO 和 C 编译器；
- sqlite-vec 已编译进 WASM，不依赖运行机器上的动态扩展文件；
- 可以继续通过 `database/sql` 使用连接池和事务。

sqlite-vec 的 WASM Build 使用 Threads/Atomics。`sqliteruntime.Configure()` 必须在第一条 SQLite 连接创建前执行：

```go
sqlite3.RuntimeConfig = wazero.NewRuntimeConfig().WithCoreFeatures(
    api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
)
```

`core/infra/sqliteruntime` 同时负责：

```text
加载 sqlite-vec WASM
注册 ncruces database/sql Driver
启用 WebAssembly Threads
```

历史记录 SQLite Adapter 也调用该 Runtime。这样项目中不会出现两个包同时以 `sqlite3` 名称注册 Driver 的启动 panic。

FTS5 KeywordStore 仍使用 `modernc.org/sqlite`，它的 Driver 名称是 `sqlite`，与这里的 `sqlite3` 不冲突。关键词库和向量库使用独立文件，避免两个 SQLite Runtime 同时写同一个文件。

这些版本涉及 WASM Host ABI，不应只升级其中一个依赖。升级时必须重新运行历史库和 sqlite-vec 的真实文件测试。

## 4. 配置和文件路径

```yaml
rag:
  local:
    enabled: true
    path: .myai/knowledge.db
    vector_path: .myai/knowledge-vectors.db
    max_open_connections: 4
```

字段作用：

| 字段 | 作用 |
|---|---|
| `enabled` | 同时启用本地 FTS5 和 sqlite-vec |
| `path` | FTS5 关键词数据库文件 |
| `vector_path` | sqlite-vec 向量数据库文件 |
| `max_open_connections` | 两个本地 Store 各自的连接池上限 |

`path` 和 `vector_path` 为相对路径时，由 `ViperLoader` 按 Workspace 转换成绝对路径。

当 `path` 为空时，FTS5 默认路径是：

```text
<UserConfigDir>/myai/workspaces/<workspace-hash>/knowledge.db
```

当 `vector_path` 为空时，`sqlitevec.PathBeside(keywordPath)` 生成：

```text
<keyword-directory>/knowledge-vectors.db
```

每个 Workspace 使用独立 Hash 目录，不会把两个项目的热点向量混在一起。

## 5. SQLite 运行参数

`sqlitevec.Open` 创建绝对路径 DSN，并设置：

```text
busy_timeout(5000)
journal_mode(WAL)
synchronous(NORMAL)
foreign_keys(ON)
txlock=immediate
```

含义：

- Writer 最多等待 5 秒，不立即返回 `SQLITE_BUSY`；
- WAL 允许 Reader 和 Writer 更好地并发；
- `NORMAL` 在本地缓存场景平衡持久性和速度；
- 外键保护 Profile 和 Record 的关系；
- 写事务尽早获取写锁，减少事务执行到一半才竞争失败。

Store 内还有 `writeMu`，负责把当前 Go 进程的写操作排队。Reader 仍可通过连接池并发执行。

## 6. 两层 Schema

本地向量库没有把所有内容只放进一个虚拟表，而是分成元数据层和检索载荷层。

### 6.1 Profile 元数据

```sql
CREATE TABLE local_vector_profiles (
    profile_id TEXT PRIMARY KEY,
    dimensions INTEGER NOT NULL,
    distance_metric TEXT NOT NULL,
    table_name TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
```

它记录每个 `EmbeddingProfile` 对应的维度、距离算法和 vec0 表名。

### 6.2 同步和墓碑元数据

```sql
CREATE TABLE local_vector_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    embedding_id TEXT NOT NULL UNIQUE,
    chunk_id TEXT NOT NULL,
    knowledge_base_id TEXT NOT NULL,
    document_id TEXT NOT NULL,
    document_version INTEGER NOT NULL,
    profile_id TEXT NOT NULL,
    dimensions INTEGER NOT NULL,
    deleted INTEGER NOT NULL,
    deleted_at INTEGER,
    delete_reason TEXT NOT NULL,
    sync_sequence INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
```

这一层永久保留字符串 ID、业务来源、同步序号和逻辑删除墓碑。

### 6.3 vec0 检索表

每个 EmbeddingProfile 创建一张独立虚拟表：

```sql
CREATE VIRTUAL TABLE local_vectors_<profile-hash> USING vec0(
    knowledge_base_id TEXT,
    deleted BOOLEAN,
    +embedding_id TEXT,
    +chunk_id TEXT,
    embedding FLOAT[1024] distance_metric=cosine
);
```

表名使用 ProfileID 的 SHA-256 前缀生成，不直接把用户输入拼成 SQL Identifier。

分 Profile 建表解决两个问题：

- sqlite-vec 的向量维度在建表时固定；
- 不同 Embedding Model 即使同为 1024 维，也不能混用向量空间。

## 7. EnsureIndex

调用示例：

```go
err := localVectorStore.EnsureIndex(ctx, knowledge.VectorIndexDefinition{
    EmbeddingProfileID: "bge-m3-v1",
    Dimensions:         1024,
    DistanceMetricID:   "cosine",
})
```

真实流程：

```text
VectorIndexDefinition.Validate
-> sqlitevec.normalizeMetric
-> 生成稳定 vec0 TableName
-> 开启写事务
-> 查询 local_vector_profiles
-> 已存在时校验 Dimensions + Metric + TableName
-> 不存在时保存 Profile 元数据
-> CREATE VIRTUAL TABLE IF NOT EXISTS
-> Commit
```

当前本地实现支持：

```text
cosine
l2 / euclidean
```

`ip/inner_product` 会明确返回不支持错误，不会静默换成另一种算法。

同一 ProfileID 再次传入不同维度或 Metric 会失败。正确做法是创建新的 EmbeddingProfileID，而不是原地改变向量空间。

## 8. Upsert

输入对象：

```go
type EmbeddingVector struct {
    ID                 string
    ChunkID            string
    KnowledgeBaseID    string
    DocumentID         string
    DocumentVersion    int64
    EmbeddingProfileID string
    Dimensions         int
    Values             []float32
    Deletion           Deletion
    SyncSequence       int64
}
```

批量 Upsert 的处理顺序：

```text
校验所有 EmbeddingVector
-> 拒绝批次内重复 EmbeddingID
-> 获取进程内写锁
-> 开启一个 SQLite 事务
-> 按 Profile 加载维度和 Metric
-> 查询当前 Record 的 SyncSequence/Deleted
-> 判断本次变更是否可应用
-> 更新 local_vector_records
-> 删除旧 vec0 Payload
-> 非删除状态时写入新 vec0 Payload
-> Commit
```

float32 向量使用 Little Endian BLOB 传给 sqlite-vec，避免使用 JSON 数组增加解析和空间开销。

## 9. SyncSequence 和幂等规则

当前规则与本地 KeywordStore 一致：

```text
incoming.sequence < current.sequence
-> 跳过

incoming.sequence == current.sequence
且 current.deleted == true
且 incoming.deleted == false
-> 跳过，禁止同序号复活

incoming.sequence > current.sequence
-> 应用新状态
```

因此网络重试可以重复发送同一批 Upsert，不会创建重复向量。旧设备晚到的同步事件也不能覆盖新墓碑。

EmbeddingID 已存在但属于另一个 Profile 时会直接失败，防止在错误的 vec0 表中留下孤立载荷。

## 10. Search

查询示例：

```go
hits, err := localVectorStore.Search(ctx, knowledge.VectorQuery{
    EmbeddingProfileID: "bge-m3-v1",
    KnowledgeBaseIDs:   []string{"kb-project"},
    DistanceMetricID:   "cosine",
    Vector:             queryVector,
    TopK:               10,
})
```

核心 SQL：

```sql
SELECT embedding_id, chunk_id, distance
FROM local_vectors_<profile-hash>
WHERE embedding MATCH ?
  AND k = ?
  AND deleted = 0
  AND knowledge_base_id = ?
ORDER BY distance ASC;
```

sqlite-vec 当前只支持有限的 Metadata KNN 条件。多个 KnowledgeBaseID 不拼接不可靠的 `IN` 表达式，而是每个 KnowledgeBase 执行一次 TopK，再按 EmbeddingID 去重并做全局距离排序。

Score 转换与 Milvus Adapter 保持同一语义：

```text
COSINE: score = 1 - distance
L2:     score = 1 / (1 + distance)
```

最终重新生成从 1 开始的全局 Rank。

## 11. 逻辑删除

调用：

```go
err := localVectorStore.MarkDeleted(ctx, knowledge.VectorDeletion{
    EmbeddingProfileID: "bge-m3-v1",
    EmbeddingIDs:       []string{"embedding-123"},
    SyncSequence:       82,
    DeletedAt:          time.Now().UTC(),
})
```

流程：

```text
校验 ProfileID、EmbeddingIDs、SyncSequence
-> 查询每个 Record
-> current.sequence > incoming.sequence 时跳过
-> 从 vec0 删除可检索 Payload
-> local_vector_records 保留 deleted=true 墓碑
-> 保存 deleted_at + reason + sync_sequence
-> Commit
```

这里没有物理删除业务记录。删除的是可重建的本地热点向量载荷，永久墓碑仍保留在元数据表中，用于防止旧同步事件复活数据。MongoDB、MinIO 和 Milvus 中的线上事实数据不受影响。

## 12. Application 装配

启动链路：

```text
core.InitApp
-> Application.InitKnowledgeStorage
-> ViperLoader 读取 rag.local
-> sqlitefts5.Open(keywordPath)
-> sqlitevec.PathBeside 或显式 vector_path
-> sqlitevec.Open(vectorPath)
-> Application.keywordStore
-> Application.localVectorStore
```

两个 Getter 的含义不同：

```go
GetVectorStore()      // 远程 Milvus，线上完整向量
GetLocalVectorStore() // 本地 sqlite-vec，热点缓存向量
```

`Application.Close()` 会分别关闭 Milvus、sqlite-vec 和 FTS5。

当前 `IndexingService` 仍把向量写入 `GetVectorStore()`，即 Milvus，不会把全部向量双写到本地。`RetrievalService` 已在远程回退后把最终热点回填 sqlite-vec；完整增量同步仍未实现。

## 13. 完整场景

假设查询“Plan 模式为什么没有执行”：

```text
1. RetrievalService 使用 EmbeddingModelResolver 生成 Query Vector
2. 调用 GetLocalVectorStore().Search
3. 本地命中满足 LocalQualityGate 时直接使用
4. 本地不足时调用 GetVectorStore().Search 查询 Milvus
5. 远程结果进入 RRF
6. 选中的远程 Chunk 向量回填 sqlite-vec
7. 下次相近查询优先命中本地
```

当前本文完成第 2 步使用的 Adapter 和第 6 步需要的 Upsert 能力。第 3 到第 6 步的编排见 `FEATURE_RAG_RETRIEVAL_IMPLEMENTATION.md`。

## 14. 测试

测试使用真实临时 SQLite 文件和真实 sqlite-vec WASM，覆盖：

- Runtime 和 vec0 扩展能真实启动；
- Profile 建表；
- COSINE 和 L2 Search；
- KnowledgeBase 过滤；
- 批量 Upsert；
- 逻辑删除后不再返回；
- stale Upsert 不能复活墓碑；
- 同一 ProfileID 的维度冲突被拒绝；
- 十六个并发 Writer 不产生 `SQLITE_BUSY` 或丢失记录；
- 历史 SQLite Adapter 在统一 Runtime 下仍可读写。

验证命令：

```powershell
go test ./core/adapter/vectorstore/sqlitevec -v
go test ./core/adapter/persistence/sqlite/history/repository -v
go test ./core/config -v
go test ./...
go vet ./...
go test ./core/architecture
```

## 15. 当前限制

- sqlite-vec Adapter 和 RetrievalService 已完成，远程最终命中可以回填热点；
- 当前只支持 COSINE 和 L2，不支持 Milvus 的 IP Metric；
- 本地容量上限、`last_accessed_at` 和 LRU 淘汰尚未实现；
- 本地损坏后的隔离、Snapshot 和增量恢复尚未实现；
- LocalQualityGate、本地优先回退、RRF 和 ChatService 的 Session RAG 接入已实现；
- vec0 当前没有暴露 HNSW/IVF 等可调 ANN Index，`VectorIndexDefinition.Options` 不参与本地建表；
- WASM 和原生 Milvus 的性能对比仍需要使用真实知识量级做 Benchmark；
- Mobile 已提供知识库管理、文档导入、索引进度、重试、删除和检索预览；当前仍未提供独立 CLI 检索命令。

下一阶段可继续实现独立 CLI 检索命令、检索触发规则的可配置化和 LRU。不要在 ChatService 中直接调用两个 VectorStore，否则会把检索策略重新耦合进聊天主链路。
