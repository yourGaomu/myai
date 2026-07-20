# RAG Milvus VectorStore 实现

本文说明 MyAI 当前 Milvus 线上向量存储 Adapter 的真实实现，包括 Collection、索引生命周期、批量 Upsert、检索、逻辑删除、配置和边界约束。

## 1. 实现位置

```text
core/adapter/vectorstore/milvus/
├── client.go       # 业务实际使用的窄 Milvus Client 接口
├── config.go       # 连接和 Collection 配置
├── schema.go       # Collection 名称与 Schema
├── index.go        # Metric、索引和 SearchParam 映射
├── columns.go      # Domain 与 Milvus Column 映射
├── store.go        # VectorStore 实现
└── store_test.go   # 不依赖真实服务器的契约测试
```

Port 位于：

```text
core/port/knowledge/vector_store.go
```

Milvus SDK 类型只存在于 Adapter，Domain、Application 和 Port 不引用 SDK。

## 2. 为什么增加 EnsureIndex

原接口只有：

```go
Upsert(ctx, embeddings)
```

但创建 Milvus 索引必须知道：

```text
EmbeddingProfileID
Dimensions
DistanceMetricID
IndexType
HNSW/IVF 参数
```

这些信息存在 `IndexProfile`，不在 `EmbeddingVector`。因此 Port 增加：

```go
EnsureIndex(ctx context.Context, definition VectorIndexDefinition) error
```

领域对象：

```go
type VectorIndexDefinition struct {
    EmbeddingProfileID string
    Dimensions         int
    DistanceMetricID   string
    Options            map[string]string
}
```

IndexingService 在调用 Embedding 和 Vector Upsert 前执行 EnsureIndex。这样修改 HNSW、IVF 或距离算法时可以只重建 Milvus 检索结构，不重新调用 Embedding Model。

## 3. IndexingService 调用顺序

```text
EmbeddingModelResolver.Resolve
-> VectorStore.EnsureIndex
-> EmbeddingProvider.EmbedDocuments
-> VectorStore.Upsert
-> ChunkRepository.MarkEmbedded
```

EnsureIndex 失败时 Job 和 Document 进入 failed，不会继续消耗 Embedding API。

## 4. Collection 隔离

一个 EmbeddingProfile 对应一个 Collection：

```text
knowledge_vectors_<safe_profile_id>_<profile_hash>
```

ProfileID 可能包含 `/`、`-`、空格或 Unicode。Adapter 会：

1. 把不适合 Milvus 名称的字符替换为 `_`；
2. 限制可读名称长度；
3. 添加 SHA-256 截断 Hash，避免清洗后碰撞；
4. 保证名称以字母或下划线开头。

例如：

```text
ProfileID: profile/bge-m3:v1
Collection: knowledge_vectors_profile_bge_m3_v1_a13f...
```

不同模型即使都是 1024 维，也使用不同 Collection。

## 5. Collection Schema

当前字段：

```text
embedding_id         VarChar(512), Primary Key, AutoID=false
chunk_id             VarChar(512)
knowledge_base_id    VarChar(512)
document_id          VarChar(512)
document_version     Int64
embedding_profile_id VarChar(512)
vector               FloatVector(dimensions)
deleted              Bool
sync_sequence        Int64
```

Milvus 不保存 Chunk 正文。Search 返回 ChunkID 后，RetrievalService 再从 Mongo ChunkRepository 批量加载文本和来源信息。

EnsureIndex 遇到已存在 Collection 时会读取 Schema，并拒绝不同维度的 Profile 使用同一 Collection。

## 6. 索引类型

`VectorIndexDefinition.Options` 使用通用字符串 Map，Application 不出现 Milvus 枚举。

### AUTOINDEX

```text
index_type=AUTOINDEX
search_level=1
```

这是默认值，适合先让 Milvus 管理具体索引实现。

### FLAT

```text
index_type=FLAT
```

适合数据量较小、要求精确召回的场景。

### HNSW

```text
index_type=HNSW
m=16
ef_construction=200
ef=64
```

`m` 和 `ef_construction` 用于建索引，`ef` 用于查询。

### IVF_FLAT

```text
index_type=IVF_FLAT
nlist=1024
nprobe=16
```

`nlist` 用于建索引，`nprobe` 用于查询。

非法整数、非正整数和不支持的 IndexType 会在调用 SDK 前失败。

## 7. 距离算法

领域 ID 到 Milvus Metric 映射：

| Domain ID | Milvus |
| --- | --- |
| `cosine` | `COSINE` |
| `l2`、`euclidean` | `L2` |
| `ip`、`inner_product`、`dot` | `IP` |

其他值会被拒绝。距离算法仍由 IndexProfile 决定，不写死在 Adapter 配置中。

## 8. EnsureIndex 状态机

每个 Collection 有独立 `sync.Mutex`：

```text
lock(collection)
-> HasCollection
-> 不存在：CreateCollection
-> 已存在：DescribeCollection + dimensions 校验
-> DescribeIndex
-> 索引兼容：保留
-> 索引不兼容：ReleaseCollection -> DropIndex -> CreateIndex
-> LoadCollection
-> unlock
```

多个文档并发使用同一个 EmbeddingProfile 时，只有一个协程创建或重建 Collection。不同 Profile 的 Collection 可以并行处理。

索引兼容比较包含 IndexType 及构建参数。索引变化不会调用 Upsert，也不会重新向量化已有 Chunk。

## 9. 批量 Upsert

`Upsert` 会：

1. 校验每个 `EmbeddingVector`；
2. 拒绝本批次重复 EmbeddingID；
3. 按 EmbeddingProfileID 分组；
4. 拒绝同 Profile 混合维度；
5. 检查 Collection 已通过 EnsureIndex 创建；
6. 映射为 Milvus Columns；
7. 调用 SDK `Upsert`。

稳定主键：

```text
EmbeddingID = hash(ChunkID + EmbeddingProfileID)
```

因此进程在 Milvus Upsert 成功后、Mongo MarkEmbedded 前退出，重试仍会覆盖同一个 Entity，不产生重复向量。

## 10. Search

输入：

```go
VectorQuery{
    EmbeddingProfileID: "profile-bge-v1",
    KnowledgeBaseIDs:   []string{"kb-1", "kb-2"},
    DistanceMetricID:   "cosine",
    Options:            map[string]string{"index_type": "HNSW", "ef": "64"},
    Vector:             queryVector,
    TopK:               8,
}
```

Milvus 表达式：

```text
deleted == false
&& embedding_profile_id == "profile-bge-v1"
&& knowledge_base_id in ["kb-1","kb-2"]
```

字符串通过 `strconv.Quote` 构造，不直接拼接未转义 ID。

结果只读取：

```text
embedding_id
chunk_id
raw distance/score
```

转换规则：

| Metric | Distance | Score |
| --- | --- | --- |
| L2 | `max(raw, 0)` | `1 / (1 + distance)` |
| COSINE | `1 - raw` | `raw` |
| IP | `-raw` | `raw` |

Rank 从 1 开始，保持 Milvus 返回顺序。

## 11. 永久逻辑删除

原 `MarkDeleted(embeddingIDs, syncSequence)` 无法定位 Profile Collection，因此接口改为：

```go
type VectorDeletion struct {
    EmbeddingProfileID string
    EmbeddingIDs       []string
    SyncSequence       int64
    DeletedAt          time.Time
}
```

Milvus 没有 SQL 风格的部分字段 Update。Adapter 使用：

```text
Query 完整 Entity
-> 过滤 SyncSequence 更新的 Entity
-> deleted=true
-> sync_sequence=删除事件序列
-> 完整 Upsert
```

不会调用 Milvus 物理 Delete。原向量永久保留，但所有正常 Search 都过滤 `deleted == false`。

乱序同步规则：

```text
existing.sync_sequence > deletion.sync_sequence
    -> 跳过旧删除事件
```

因此较旧的删除消息不能覆盖已经同步的新版本。

## 12. 配置

```yaml
rag:
  milvus:
    enabled: true
    address: 127.0.0.1:19530
    username: ""
    password: ""
    database: default
    api_key: ""
    enable_tls: false
    collection_prefix: knowledge_vectors
    shards: 1
```

默认 `enabled=false`。关闭时 App 不创建 Milvus Client，也不访问服务器。

启用后：

```text
Application.InitKnowledgeStorage
-> milvus.New
-> client.NewClient
-> Application.vectorStore
```

Application.Close 会关闭 SDK 连接。

## 13. 健康检查

```go
err := vectorStore.Health(ctx)
```

内部调用 SDK `CheckHealth`。Milvus 返回 unhealthy 时会带上 Reasons，不把连接失败伪装成空检索结果。

## 14. 测试

Adapter 定义只包含实际使用方法的窄 `client` 接口，测试 Fake 不需要实现官方 SDK 的全部大型接口。

当前覆盖：

- 创建 Collection、Schema、Index 并 Load；
- 不兼容 Metric 时 Release/Drop/Create/Load；
- 重建索引不 Upsert Vector；
- Domain 到九个 Milvus Column 的映射；
- Search 逻辑删除与 KnowledgeBase 过滤；
- COSINE Score/Distance 转换；
- 逻辑删除完整 Entity Upsert；
- 旧 SyncSequence 删除跳过；
- unhealthy 状态传播。

验证命令：

```powershell
go test ./core/adapter/vectorstore/milvus -v
go test ./...
go vet ./...
go test ./core/architecture
```

## 15. 当前限制

- 当前未配置可访问的 Milvus 地址，因此尚未执行真实服务器集成测试；
- IndexType 第一版支持 AUTOINDEX、FLAT、HNSW、IVF_FLAT；
- 一个 EmbeddingProfile 同一时刻维护一个活动向量索引结构；
- stale Upsert 的通用 compare-and-set 尚未实现，目前删除路径已保护 SyncSequence；
- sqlite-vec、RetrievalService 和本地热点回填已完成，但 Milvus 原向量读取 Port 尚未实现；
- IndexingService 已可在完整配置下装配，但尚无用户侧上传入口。

真实服务器启用前应先在测试 Collection 验证 Milvus 版本、认证、TLS、AUTOINDEX 可用性和资源占用。
