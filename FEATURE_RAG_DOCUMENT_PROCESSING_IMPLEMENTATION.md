# RAG 文档解析与分块实现

## 一、职责边界

Go 负责 MinIO 读取、索引任务、并发调度、超时、重试、持久化、Embedding 和向量索引。Python 只负责文档解析与分块。

```text
Go IndexingService
-> DocumentObjectStore.Open
-> DocumentProcessor Port
-> GrpcDocumentProcessor
-> WorkerPool
-> Python ProcessingService
-> ChunkBatch 流
-> Go 校验并生成稳定 ChunkID
-> ChunkRepository
```

Python 不访问 MongoDB、MinIO、Milvus、sqlite-vec、Session 或 IndexingJob。这些对象都属于 Go 应用层和基础设施层。

## 二、Go 对象与目录

接口与实现分别放置：

```text
core/port/knowledge/documentprocessor/
├── processor.go       # DocumentProcessor 接口
├── request.go         # Request DTO
└── sink.go            # ChunkSink 接口

core/adapter/documentprocessor/grpc/
├── processor.go       # gRPC DocumentProcessor 实现
├── mapper.go          # 领域对象与 Protobuf DTO 映射
├── worker.go          # 单个 Python Worker 对象
├── supervisor.go      # Python 进程启动和健康检查
├── worker_pool.go     # Worker 租借、排队和替换
└── config.go          # Adapter 配置
```

`DocumentProcessor.Process` 接收与传输方式无关的 `Request` 和 `ChunkSink`，返回 `knowledge.ProcessingSummary`。生成的 Protobuf 类型不能进入 Domain 或 Application 层。

`ChunkSink.Accept` 每次接收一个有界批次，因此多个并发文档不需要分别在 Go 内存中保存完整的 `[]ChunkDraft`。

## 三、Worker 启动

当 `rag.document_processor.enabled=true` 时，`Application.InitKnowledgeStorage` 调用：

```text
grpcprocessor.New
-> newWorkerPool
-> newWorkerSupervisor
-> workerSupervisor.Start
-> exec.Command 启动 Python
-> 读取 READY JSON
-> 创建 gRPC ClientConn
-> Health RPC
```

所有 Worker 并行预热。只有全部 Worker 启动并通过健康检查后，`DocumentProcessor` 才会进入可用状态。任意 Worker 启动失败都会关闭已经启动的进程，并让应用初始化失败，不会留下半可用的 Pool。

Windows 使用 `127.0.0.1` 随机端口；非 Windows 平台在 `transport=auto` 时使用私有 Unix Socket。Worker 只监听本机地址，并使用 Go 生成的独立 Token 校验每个 RPC。

## 四、并发模型

第一版固定采用：

```text
一个 Python Worker 进程
= 同时处理一个文档
```

多个文档通过多个 Worker 并行处理：

```text
文档 A -> Worker 1
文档 B -> Worker 2
文档 C -> Worker 3
文档 D -> Worker 4
后续文档 -> 有界等待队列
```

`workerPool.acquire` 先占用 admission 名额，再等待空闲 Worker。活跃任务和等待任务的总数不能超过：

```text
worker_count + max_pending_jobs
```

超过上限时返回 `ErrWorkerQueueFull`，不会无限创建 goroutine 或 Python 进程。

Go 收到 `ProcessCompleted` 后不会立即归还 Worker，而是继续读取到 gRPC EOF。EOF 表示 Python 已经退出处理函数并释放单任务锁，可以安全接收下一份文档。这避免了并发场景下的“Completed 已收到，但 Worker 仍然 busy”竞态。

## 五、gRPC v1 协议

协议唯一来源：

```text
api/documentprocessor/v1/document_processor.proto
```

核心方法：

```text
ProcessDocument(stream ProcessRequest) returns (stream ProcessEvent)
```

Go 发送顺序：

```text
ProcessStart
Content 0..N
ProcessInputCompleted
```

Python 返回顺序：

```text
ProcessMetadata
ChunkBatch 1..N
ProcessCompleted
EOF
```

原始文档按 `content_chunk_kb` 分成二进制消息，不使用 Base64，也不把参数放进命令行。Python 将输入流写入本次请求独占的临时文件，使 PDF 和 DOCX Parser 可以使用可 seek 的文件源，同时避免原始文档在内存中再复制一份。

ChunkBatch 同时受两种上限约束：

```text
chunk_batch_size
grpc_max_message_mb
```

因此单个 Profile 即使生成较大的 Chunk，也不会因为固定批次数量而拼出超过 gRPC 上限的响应消息。

## 六、Go 处理流程

`grpcprocessor.Processor.Process` 执行：

```text
校验 Document
-> 校验 ParsingProfile
-> 校验 ChunkingProfile
-> WorkerPool.acquire
-> 创建 ProcessDocument 双向流
-> 发送 Start
-> 从 io.Reader 分块发送原文
-> 发送 InputCompleted
-> 校验 ProcessMetadata
-> 映射并校验 ChunkBatch
-> 调用 ChunkSink.Accept
-> 校验 ProcessCompleted 数量
-> 等待 EOF
-> WorkerPool.release
```

Go 会校验：

- RequestID、DocumentID 和 DocumentVersion 是否一致；
- Content-Type 是否一致；
- Parser ID/Version 是否与 ParsingProfile 一致；
- Strategy ID/Version 是否与 ChunkingProfile 一致；
- Chunk ordinal 是否从 0 连续递增；
- Chunk 文本、offset、页码是否合法；
- Completed 的 ChunkCount 是否等于实际接收数量。

`ContentHash` 永远由 Go 根据 Chunk 文本重新计算。Python 不能决定 ChunkID、EmbeddingID 或持久化主键。

## 七、Python 处理流程

```text
DocumentProcessorServicer.ProcessDocument
-> 校验 Worker Token
-> 校验消息顺序和文档大小
-> 写入私有临时文件
-> Protobuf Profile 映射为 Python Model
-> ProcessingService.process
-> ParserRegistry.resolve
-> Parser.parse(DocumentSource)
-> ChunkingStrategyRegistry.resolve
-> ChunkingStrategy.split Iterator
-> 组装并发送 ChunkBatch
-> 删除临时文件
```

Python 内部继续保持接口与实现分离：

```text
document_processor/app/
├── port/                  # Parser、ChunkingStrategy 接口
├── adapter/parser/        # Parser 实现
├── adapter/chunking/      # 分块实现
├── registry/              # 实现注册与选择
├── grpc/                  # gRPC 入口适配器
├── generated/             # Protobuf 生成 DTO
├── models.py
└── service.py             # Parse + Chunk 服务
```

当前 TXT、Markdown 和 HTML 解析 UTF-8 文本；PyMuPDF 和 python-docx 直接读取临时文件。

## 八、中文 Offset 规则

Chunk 的 `start_offset` 和 `end_offset` 统一表示解析后 UTF-8 文本的字节位置，不是 Python Unicode 字符下标。

例如：

```text
文本：你好世界
每块：2 个字符

Chunk 0：你好，offset [0, 6)
Chunk 1：世界，offset [6, 12)
```

每个中文字符在 UTF-8 中占 3 字节。Python 单元测试和 Go 到 Python 的真实集成测试都覆盖了这个场景。

## 九、错误与 Worker 替换

业务错误不会重启 Worker：

```text
InvalidArgument
FailedPrecondition
ResourceExhausted
Unimplemented
```

这些错误表示 Profile、格式或大小不满足要求，Python 进程本身仍然健康。

以下情况会淘汰当前 Worker：

```text
进程退出
gRPC Unavailable
调用超时
调用取消
协议结果不一致
ChunkSink 返回错误
```

淘汰流程：

```text
WorkerPool.release(false)
-> workerSupervisor 关闭旧 Worker
-> 后台启动替代 Worker
-> Health 检查
-> 放回 available channel
```

应用关闭时会等待正在执行的替换协程结束，避免 Go 已退出但新 Python 进程刚刚启动的残留问题。

Adapter 内部不透明重试文档，因为输入是 `io.Reader`，不保证可以重新读取。后续由 `IndexingService` 根据 IndexingJob 重试，并重新调用 `DocumentObjectStore.Open` 获得新的 Reader。

## 十、配置

```yaml
rag:
  document_processor:
    enabled: true
    python_executable: python
    script: ./document_processor/main.py
    transport: auto
    worker_count: 4
    max_pending_jobs: 32
    startup_timeout_seconds: 30
    timeout_seconds: 120
    shutdown_grace_seconds: 5
    max_document_size_mb: 512
    content_chunk_kb: 256
    chunk_batch_size: 64
    grpc_max_message_mb: 8
    temp_directory: ""
```

`worker_count=0` 使用有上限的自动值。每个 Worker 都会独立加载 Python 和 Parser 依赖，因此不能简单设置为 CPU 核心数；需要根据文档类型、CPU 和峰值内存压测。

## 十一、验证

Python：

```powershell
cd document_processor
python -m unittest discover -s tests -v
python -m compileall -q app tests
```

Go：

```powershell
$env:MYAI_RUN_PYTHON_PROCESSOR_TEST = "1"
go test ./core/adapter/documentprocessor/grpc -v
go test ./...
go vet ./...
```

真实集成测试会：

- 启动两个 Python Worker；
- 并发处理四个中文文档；
- 校验 UTF-8 offset 和 Go 计算的 ContentHash；
- 校验 Parser 业务错误不会破坏 Worker；
- 强制杀死一个 Worker，验证 Pool 自动补充替代进程；
- 关闭 Processor 后确认不残留 Python Worker。

## 十二、后续索引链路

gRPC 和 Worker Pool 已经完成，应用层 `IndexingService` 已独立实现。它通过 `ChunkSink` 接收分块，并负责后续状态机、Embedding、向量和关键词索引：

```text
Document
-> DocumentProcessor
-> ChunkRepository
-> EmbeddingProvider
-> Milvus / sqlite-vec
```

详细的调用顺序、对象关系、状态变化、幂等规则和断点位置见 [RAG 文档索引功能实现](FEATURE_RAG_INDEXING_IMPLEMENTATION.md)。IndexingJob 和幂等批量保存规则不能放进 gRPC Adapter。
