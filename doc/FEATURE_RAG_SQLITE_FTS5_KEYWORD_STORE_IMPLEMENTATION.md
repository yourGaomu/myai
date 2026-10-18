# RAG SQLite FTS5 KeywordStore 实现

本文说明 MyAI 本地关键词索引的真实实现，包括 Driver 选型、Schema、Trigger、并发写入、检索、逻辑删除、SyncSequence 和 Application 装配。

## 1. 实现位置

```text
core/adapter/keywordstore/sqlitefts5/
├── config.go
├── path.go
├── schema.go
├── store.go
└── store_test.go
```

实现接口：

```go
type KeywordStore interface {
    Upsert(ctx context.Context, documents []KeywordDocument) error
    Search(ctx context.Context, query KeywordQuery) ([]KeywordHit, error)
    MarkDeleted(ctx context.Context, chunkIDs []string, syncSequence int64) error
}
```

## 2. Driver 选型

项目原有历史数据库使用：

```text
github.com/mattn/go-sqlite3
```

该 Driver 默认不编译 FTS5，必须额外使用：

```powershell
-tags sqlite_fts5
```

这会导致普通 `go run .` 编译成功、运行到建表时才出现 `no such module: fts5`。因此 Knowledge KeywordStore 使用：

```text
modernc.org/sqlite
```

它的 Windows/Linux 构建默认包含 `SQLITE_ENABLE_FTS5`，不要求 CGO 或额外 Build Tag。Adapter 启动时仍会执行：

```sql
SELECT sqlite_compileoption_used('ENABLE_FTS5');
```

返回值不是 1 时立即失败，不创建半可用 Store。

历史数据库已迁移到 ncruces SQLite Runtime，与 sqlite-vec 共用 `sqlite3` Driver 注册。FTS5 继续使用 `modernc.org/sqlite` 的 `sqlite` Driver；两个知识库 Adapter 使用独立数据库文件。

## 3. 数据库路径

显式配置：

```yaml
rag:
  local:
    enabled: true
    path: .myai/knowledge.db
    max_open_connections: 4
```

相对路径会按 Workspace 解析。Path 为空时，`DefaultPath(workspace)` 生成：

```text
<UserConfigDir>/myai/workspaces/<workspace-hash>/knowledge.db
```

Workspace Hash 避免多个项目共享同一本地知识缓存。

## 4. SQLite 运行参数

数据源为绝对路径并添加：

```text
busy_timeout(5000)
journal_mode(WAL)
synchronous(NORMAL)
foreign_keys(ON)
```

Windows 不能把 `C:` 盘符拼成错误的 URI authority。当前实现使用：

```text
C:/.../knowledge.db?_pragma=...
```

而不是错误的 `file://C:/...`。

## 5. 普通表与 FTS5 表

普通表是元数据事实源：

```sql
CREATE TABLE keyword_documents (
    rowid INTEGER PRIMARY KEY AUTOINCREMENT,
    chunk_id TEXT NOT NULL UNIQUE,
    knowledge_base_id TEXT NOT NULL,
    document_id TEXT NOT NULL,
    document_version INTEGER NOT NULL,
    text TEXT NOT NULL,
    deleted INTEGER NOT NULL DEFAULT 0,
    sync_sequence INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
```

FTS5 使用 External Content 模式：

```sql
CREATE VIRTUAL TABLE keyword_documents_fts USING fts5(
    text,
    content='keyword_documents',
    content_rowid='rowid',
    tokenize='unicode61'
);
```

这样元数据、唯一约束和同步序列不重复存进全文索引。

## 6. Trigger 同步

### INSERT

```sql
INSERT INTO keyword_documents_fts(rowid, text)
VALUES (new.rowid, new.text);
```

### DELETE

```sql
INSERT INTO keyword_documents_fts(keyword_documents_fts, rowid, text)
VALUES ('delete', old.rowid, old.text);
```

### UPDATE OF text

```text
删除旧 FTS 文本
-> 插入新 FTS 文本
```

逻辑删除只修改普通表的 `deleted`，Search 通过 Join 过滤，因此不需要从 FTS5 物理删除正文，符合永久逻辑删除要求。

## 7. Upsert

输入先执行 `KeywordDocument.Validate()`：

```text
ChunkID 非空
KnowledgeBaseID 非空
DocumentID 非空
DocumentVersion >= 1
Text 非空
SyncSequence >= 0
Deletion 合法
```

同一批次重复 ChunkID 会在事务前失败。

Upsert SQL 使用 ChunkID 唯一键：

```sql
INSERT ...
ON CONFLICT(chunk_id) DO UPDATE ...
WHERE excluded.sync_sequence > keyword_documents.sync_sequence
   OR (
       excluded.sync_sequence = keyword_documents.sync_sequence
       AND (keyword_documents.deleted = 0 OR excluded.deleted = 1)
   );
```

规则：

- 新序列覆盖旧序列；
- 旧序列不能覆盖新数据；
- 相同序列允许活动文档更新正文；
- 相同序列不允许活动数据复活已删除墓碑；
- 相同序列的删除可以保持删除状态。

一批文档在同一个 SQLite Transaction 中提交。

## 8. 为什么写入需要显式排队

SQLite 是单写者数据库。即使 WAL 允许读写并发，多个连接同时开启写事务仍可能返回：

```text
SQLITE_BUSY: database is locked
```

Store 使用：

```go
writeMu sync.Mutex
```

串行化 `Upsert` 和 `MarkDeleted`。Search 不持有该锁，仍可并发读取。

这与 SQLite 的物理写入能力一致：Adapter 提前排队，避免把随机 Busy 错误交给业务层。`database/sql` 仍保留多个连接服务并发读取。

## 9. Search

输入：

```go
KeywordQuery{
    Text:             "Redis vector",
    KnowledgeBaseIDs: []string{"kb-a", "kb-b"},
    TopK:             8,
}
```

用户文本不会直接拼接进 SQL。每个空白分隔 Term 会被转成 FTS5 Phrase，内部双引号转义，再通过参数绑定传入：

```text
"Redis" OR "vector"
```

查询：

```sql
SELECT d.chunk_id, bm25(keyword_documents_fts) AS relevance
FROM keyword_documents_fts
INNER JOIN keyword_documents d
    ON d.rowid = keyword_documents_fts.rowid
WHERE keyword_documents_fts MATCH ?
  AND d.deleted = 0
  AND d.knowledge_base_id IN (?, ?)
ORDER BY relevance ASC
LIMIT ?;
```

KnowledgeBaseID 只生成占位符，不进入 SQL 字符串。

返回：

```go
KeywordHit{
    ChunkID: chunkID,
    Score:   max(-bm25, 0),
    Rank:    index + 1,
}
```

后续 RRF 主要使用 Rank，Score 用于诊断和本地质量判断。

## 10. 中文检索

第一版 Tokenizer 是 SQLite 内置 `unicode61`。它支持 Unicode 文本和中文存储，但不提供中文词语级分词能力。

例如原文中使用空格或标点分隔的中文词可以正常检索。连续长中文句子的召回质量不如 jieba 等专用分词器。后续可以新增 Tokenizer Adapter 或在写入前提供可配置的中文分词文本，不能把分词逻辑硬编码进 KeywordStore Port。

## 11. 逻辑删除

```go
MarkDeleted(ctx, []string{"chunk-1", "chunk-2"}, syncSequence)
```

SQL：

```sql
UPDATE keyword_documents
SET deleted = 1,
    sync_sequence = ?,
    updated_at = ?
WHERE chunk_id IN (...)
  AND sync_sequence <= ?;
```

旧删除事件不能覆盖更新记录。Chunk 正文和 FTS Token 永久保留，但 Search 永远过滤 deleted。

## 12. App 装配

当 `rag.local.enabled=true`：

```text
Application.InitKnowledgeStorage
-> resolve knowledge.db path
-> sqlitefts5.Open
-> Application.keywordStore
```

Application.Close 会关闭数据库连接。

App 对外提供：

```go
GetKeywordStore()
```

## 13. IndexingService 最终装配

以下依赖全部存在时，App 创建 IndexingService：

```text
Mongo DocumentRepository
Mongo ProfileRepository
Mongo IndexingJobRepository
Mongo ChunkRepository
MinIO DocumentObjectStore
Python DocumentProcessor
EmbeddingModelResolver
Milvus VectorStore
SQLite FTS5 KeywordStore
Snowflake IDGenerator
StableIDDeriver
```

缺少任一项时 `GetIndexingService()` 返回 nil，避免半条索引链路运行。

Snowflake Node 来自：

```yaml
rag:
  id_node: 0
```

多机器部署必须使用不同的 0-1023 NodeID。

## 14. 测试

测试使用真实临时 SQLite 文件，已经覆盖：

- 当前 Driver 确实启用 FTS5；
- Upsert 和 BM25 Search；
- KnowledgeBase 过滤；
- UPDATE Trigger 替换旧 Token；
- stale Upsert 拒绝；
- stale Delete 拒绝；
- 逻辑删除过滤；
- 双引号和中文文本；
- 十二个并发 Writer 不出现 SQLITE_BUSY。

验证：

```powershell
go test ./core/adapter/keywordstore/sqlitefts5 -v
go test ./...
go vet ./...
go test ./core/architecture
```

## 15. 当前限制

- `unicode61` 不是中文专业分词器；
- 本地 KeywordStore 目前是完整写入，不含 LRU 容量淘汰；
- sqlite-vec、本地优先 RetrievalService、热点回填、LocalQualityGate、RRF 和 Chat RAG 接入已实现；
- LRU 和远程关键词检索仍未实现；
- Mobile 已提供知识库文档导入、索引状态、重试、删除和检索预览，当前没有独立 CLI 检索命令。
