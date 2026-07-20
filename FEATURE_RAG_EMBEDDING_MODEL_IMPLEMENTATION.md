# RAG Embedding Model 管理实现

本文说明 MyAI 当前 Embedding Model 管家的真实实现，包括配置加载、运行时注册、Profile 解析、OpenAI-compatible API 调用、向量归一化和错误校验。

## 1. 为什么与 Chat Model 分开

Chat Model 和 Embedding Model 的生命周期与职责不同：

```text
Chat Model
    输入消息、工具和生成参数
    输出 reasoning、answer、tool calls

Embedding Model
    输入文档 Chunk 或检索 Query
    输出固定维度向量
```

因此 Embedding Model 不注册到 `llm.Client`，而使用独立对象：

```text
EmbeddingModelRegistry
EmbeddingModelResolver
EmbeddingProvider
```

这相当于 Spring Boot 中为向量模型建立独立的 Bean Registry 和 Gateway，不复用 ChatClient Bean。

## 2. 目录结构

```text
core/
├── port/knowledge/embedding.go
├── adapter/embedding/
│   ├── memory/registry.go
│   └── openaicompatible/
│       ├── config.go
│       └── provider.go
├── application/knowledge/embedding/service/resolver.go
├── config/
│   ├── properties.go
│   ├── loader.go
│   └── mapper.go
└── App.go
```

分层含义：

| 对象 | Spring Boot 对照 | 职责 |
| --- | --- | --- |
| `EmbeddingProvider` | Gateway 接口 | 定义文档和 Query 向量化能力 |
| `memory.Registry` | Bean Registry | 保存运行时 Provider 和公开元数据 |
| `Resolver` | Application Service | 根据 EmbeddingProfile 选择并约束 Provider |
| `openaicompatible.Provider` | GatewayImpl | 调用 OpenAI-compatible Embedding API |
| `EmbeddingModelProperties` | ConfigurationProperties | 承载启动配置和密钥 |
| `Application.InitKnowledgeStorage` | Configuration/Bean 装配 | 创建并注册模型 |

## 3. 核心接口

```go
type EmbeddingProvider interface {
    EmbedDocuments(ctx context.Context, request EmbedRequest) (EmbedResult, error)
    EmbedQuery(ctx context.Context, request EmbedRequest) (EmbedResult, error)
}

type EmbeddingModelRegistry interface {
    Get(modelID string) (EmbeddingProvider, bool)
    GetInfo(modelID string) (EmbeddingModelInfo, bool)
    List() []EmbeddingModelInfo
}

type EmbeddingModelResolver interface {
    Resolve(profile EmbeddingProfile) (EmbeddingProvider, error)
}
```

文档和 Query 使用不同方法，为将来接入要求 `search_document`、`search_query` task type 的模型保留扩展点。

## 4. 配置格式

```yaml
rag:
  embedding:
    models:
      - id: bge-m3
        name: BGE M3
        provider: openai-compatible
        base_url: http://127.0.0.1:11434/v1
        api_key: local-key
        model: bge-m3
        model_version: v1
        dimensions: 1024
        max_input_tokens: 8192
        batch_size: 32
        timeout_seconds: 60
        request_dimensions: false
        preserve_new_lines: false
        enabled: true
```

字段说明：

| 字段 | 作用 |
| --- | --- |
| `id` | Registry 中的唯一 ModelID，也是 EmbeddingProfile.ModelID |
| `provider` | 当前支持 `openai` 和 `openai-compatible` |
| `base_url` | OpenAI-compatible API 根路径，通常以 `/v1` 结尾 |
| `api_key` | 只在配置和 Adapter 构造阶段使用 |
| `model` | 发送给 Embedding API 的模型名 |
| `model_version` | 向量兼容性标识，必须与 Profile 一致 |
| `dimensions` | 期望向量维度 |
| `max_input_tokens` | 模型能力元数据，当前不执行本地 Tokenizer 截断 |
| `batch_size` | LangChainGo 发给 API 的单批文本数量 |
| `timeout_seconds` | 单次 HTTP 请求超时 |
| `request_dimensions` | 是否把 dimensions 字段发送给 API |
| `preserve_new_lines` | 是否保留输入换行符 |
| `enabled` | 缺省为 true；false 时不创建 Provider |

默认值：

```text
provider        = openai-compatible
batch_size      = 32
timeout_seconds = 60
enabled         = true
```

固定维度模型或不接受 `dimensions` 参数的兼容服务，应设置 `request_dimensions: false`。支持可变维度的 OpenAI Embedding 模型可以设为 true。

## 5. 启动注册流程

`core/App.go` 的 `InitKnowledgeStorage` 执行：

```text
ViperLoader
    -> EmbeddingModelProperties
    -> Mapper.EmbeddingProviderConfig
    -> openaicompatible.New
    -> Registry.Set(modelID, provider, info)
    -> Resolver{Registry: registry}
```

禁用模型直接跳过。启用模型如果缺少 API Key、Model、Dimensions，或 Provider 不受支持，应用启动失败并指出具体 ModelID。

App 对外提供：

```go
GetEmbeddingModelRegistry()
GetEmbeddingModelResolver()
```

API Key 不进入 `EmbeddingModelInfo`，因此 Registry 的 `GetInfo/List` 不会向应用用例或未来手机协议暴露密钥。

## 6. Registry 实现

`adapter/embedding/memory.Registry` 内部维护两张 Map：

```text
modelID -> EmbeddingProvider
modelID -> EmbeddingModelInfo
```

所有访问由 `sync.RWMutex` 保护：

- `Set` 使用写锁；
- `Get/GetInfo/List` 使用读锁；
- `List` 按 ModelID 排序，避免 Map 随机顺序影响 UI 和测试；
- 同一个 ModelID 再次 Set 会原子替换 Provider 和元数据。

注册时要求 Map Key 与 `EmbeddingModelInfo.ID` 一致，并执行领域 `EmbeddingModelInfo.Validate()`。

## 7. Resolver 的作用

IndexingService 不直接调用 Registry：

```go
provider, err := embeddingResolver.Resolve(embeddingProfile)
```

Resolver 会校验：

```text
EmbeddingProfile.Validate
Profile 未逻辑删除
ModelID 已注册
EmbeddingModelInfo 存在并启用
Profile.ModelID      == Info.ID
Profile.Provider     == Info.Provider
Profile.Model        == Info.Model
Profile.ModelVersion == Info.ModelVersion
Profile.Dimensions   == Info.Dimensions
```

仅维度相同不能通过。例如两个模型都是 1024 维，但 Model 或 ModelVersion 不同，Resolver 仍会拒绝混用。

Resolver 返回的不是裸 Provider，而是绑定当前 Profile 的 `profileProvider`。后续请求必须携带同一个 `EmbeddingProfileID`，结果也必须返回同一个 ProfileID 和 Dimensions。

## 8. L2 归一化

当 `EmbeddingProfile.Normalize=true` 时，Resolver 在 Provider 返回后执行：

```text
norm = sqrt(v1^2 + v2^2 + ... + vn^2)
normalized[i] = vector[i] / norm
```

归一化前会复制底层 Provider 返回的 Vector，不修改 Provider 持有的切片。以下情况会失败：

- Vector 包含 NaN；
- Vector 包含正负无穷；
- Vector 全部为 0，无法归一化。

Normalize 是 EmbeddingProfile 的组成部分。修改该值应创建新的 Profile，不能把旧向量直接标记为兼容。

## 9. OpenAI-compatible Provider

Adapter 使用 LangChainGo 的 OpenAI Client 和 Embedder：

```text
openai.New
    WithToken
    WithBaseURL
    WithEmbeddingModel
    WithHTTPClient
    WithEmbeddingDimensions（可选）

embeddings.NewEmbedder
    WithBatchSize
    WithStripNewLines
```

底层共享 `http.Client` 和默认 Transport 连接池，可以安全处理多个并发索引请求。每个 Provider 对应一个配置模型，不在每个 Chunk 批次重新创建 HTTP Client。

请求协议：

```http
POST /v1/embeddings
Authorization: Bearer <api-key>
Content-Type: application/json
```

```json
{
  "input": ["first chunk", "second chunk"],
  "model": "bge-m3",
  "dimensions": 1024
}
```

只有 `request_dimensions=true` 时才发送 dimensions。

## 10. DTO 映射

Application 输入：

```go
EmbedRequest{
    EmbeddingProfileID: "profile-bge-m3-v1",
    Inputs: []EmbedInput{
        {ID: "chunk-1", Text: "..."},
        {ID: "chunk-2", Text: "..."},
    },
}
```

Adapter 只把 Text 发送给 API。返回时按原输入顺序恢复 ID：

```go
EmbedResult{
    EmbeddingProfileID: "profile-bge-m3-v1",
    Dimensions: 1024,
    Outputs: []EmbedOutput{
        {ID: "chunk-1", Vector: vector1},
        {ID: "chunk-2", Vector: vector2},
    },
}
```

Provider 会拒绝重复 InputID、Query 多输入、API 返回数量错误、维度错误和非有限向量。

## 11. Document 与 Query

文档索引调用：

```go
provider.EmbedDocuments(ctx, request)
```

检索 Query 调用：

```go
provider.EmbedQuery(ctx, EmbedRequest{
    EmbeddingProfileID: profile.ID,
    Inputs: []EmbedInput{{
        ID: queryID,
        Text: userQuery,
    }},
})
```

Query 当前严格要求一个输入。Provider 返回统一的 `EmbedResult`，便于后续 RetrievalService 使用相同维度和 Profile 校验逻辑。

## 12. 错误位置与调试

| 位置 | 常见错误 |
| --- | --- |
| `ViperLoader.Map` | YAML 字段映射错误 |
| `Mapper.EmbeddingProviderConfig` | 时间、批大小映射错误 |
| `openaicompatible.New` | API Key、模型、维度非法 |
| `Registry.Set` | ModelID 和 Info.ID 不一致 |
| `Resolver.Resolve` | Profile 与运行时模型不兼容 |
| `Provider.EmbedDocuments` | HTTP、限流、响应数量或维度错误 |
| `profileProvider.prepareResult` | ProfileID、维度、归一化错误 |
| `IndexingService.embeddingVectors` | Chunk ID 覆盖不完整或重复 |

不要在错误日志中打印 API Key 或完整文档文本。

## 13. 当前限制

已经完成：

- 多模型 YAML 配置；
- 线程安全 Registry；
- Profile Resolver；
- OpenAI-compatible Provider；
- 文档与 Query 两类调用；
- 可选 dimensions 参数；
- Profile L2 归一化；
- App 启动注册和只读 Getter；
- Fake 与本地 HTTP 契约测试。

尚未完成：

- Embedding Model 配置的 MongoDB 动态增删改；
- CLI 和手机端模型管理；
- Jina、Voyage、HuggingFace 等独立 Adapter；
- Provider 级限流、指数退避和指标统计；
- 本地 Tokenizer 的 MaxInputTokens 预检查；
- sqlite-vec 本地热点 VectorStore。

Provider 扩展时应新增 Adapter，并只在 Composition Root 注册实现，不能向 `EmbeddingProvider` 接口加入供应商 DTO。

## 14. 验证

```powershell
go test ./core/adapter/embedding/...
go test ./core/application/knowledge/embedding/...
go test ./core/config
go test ./...
go vet ./...
go test ./core/architecture
```
