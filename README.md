# MyAI

MyAI 是一个 Go + Expo React Native 实现的 AI 编程助手，支持命令行聊天、手机远程控制、会话与模型管理、Plan 模式、工具调用、权限审批、文件预览和工作区变更恢复。

可先打开 [PROJECT_ARCHITECTURE_INTRO.html](PROJECT_ARCHITECTURE_INTRO.html) 查看动画导览，再阅读 [PROJECT_ARCHITECTURE_GUIDE.md](PROJECT_ARCHITECTURE_GUIDE.md) 了解完整架构、Spring Boot 对照和调用链。需要维护或排错时，查看 [DEVELOPER_FLOW_GUIDE.md](DEVELOPER_FLOW_GUIDE.md)；需要确认近期代码变化时，查看 [DEVELOPMENT_CHANGELOG.md](DEVELOPMENT_CHANGELOG.md)。

准备开发知识库与 RAG 时，阅读 [RAG 架构设计](RAG_ARCHITECTURE_DESIGN.md) 和 [RAG 本地优先检索实现](FEATURE_RAG_RETRIEVAL_IMPLEMENTATION.md)。当前支持分类树、Session 级检索范围、自动/手动/每轮模式、MinIO 原文、Mongo 元数据、Milvus 远程向量和 sqlite-vec 本地热点索引。

AI 自身总结的目标、方案、结果、痛点和经验保存在独立的 AI 记忆库，不与用户上传资料混在一起。实现细节见 [AI 记忆系统功能实现](FEATURE_AI_MEMORY_IMPLEMENTATION.md)。

## 功能实现文档

- [Agent 启动功能实现](FEATURE_AGENT_STARTUP_IMPLEMENTATION.md)
- [普通聊天消息功能实现](FEATURE_CHAT_MESSAGE_IMPLEMENTATION.md)
- [模型生成功能实现](FEATURE_MODEL_GENERATION_IMPLEMENTATION.md)
- [工具执行功能实现](FEATURE_TOOL_EXECUTION_IMPLEMENTATION.md)
- [Plan 模式功能实现](FEATURE_PLAN_MODE_IMPLEMENTATION.md)
- [AI 记忆系统功能实现](FEATURE_AI_MEMORY_IMPLEMENTATION.md)
- [RAG 文档解析与分块实现](FEATURE_RAG_DOCUMENT_PROCESSING_IMPLEMENTATION.md)
- [RAG 文档索引功能实现](FEATURE_RAG_INDEXING_IMPLEMENTATION.md)
- [RAG Embedding Model 管理实现](FEATURE_RAG_EMBEDDING_MODEL_IMPLEMENTATION.md)
- [RAG Milvus VectorStore 实现](FEATURE_RAG_MILVUS_VECTOR_STORE_IMPLEMENTATION.md)
- [RAG SQLite FTS5 KeywordStore 实现](FEATURE_RAG_SQLITE_FTS5_KEYWORD_STORE_IMPLEMENTATION.md)
- [RAG sqlite-vec VectorStore 实现](FEATURE_RAG_SQLITE_VEC_VECTOR_STORE_IMPLEMENTATION.md)
- [RAG 本地优先检索实现](FEATURE_RAG_RETRIEVAL_IMPLEMENTATION.md)

## Run

```powershell
go run . help
go run . chat
```

可用的顶层命令包括：`agent`、`chat`、`client`、`completion`、`help`、`relay`、`sandbox` 和 `skill`。其中 `completion`、`help` 由 Cobra 提供，`sandbox` 还包含 `doctor` 子命令。

When MongoDB persistence is enabled, run MongoDB as a replica set. A single-node
replica set is sufficient. Subagent `Task + Run` transitions and transcript
replacement use MongoDB transactions; standalone MongoDB rejects these flows.
For an Agent outside the MongoDB Docker network, include
`replicaSet=rs0&directConnection=true` in the connection URI.

## Remote Agent With Files And Changes

Start the relay:

```powershell
go run . relay --addr 0.0.0.0:18080 --agent-token "replace-with-a-strong-token" --agent-user local --agent-device pc-local
```

When using the Expo Web client from a local development server, allow its
browser origins on the Relay (native Android clients do not send an Origin):

```powershell
$env:MYAI_RELAY_ALLOWED_ORIGINS = "http://localhost:*,http://127.0.0.1:*"
```

Start the PC agent and choose the workspace that clients can preview:

```powershell
go run . agent --server ws://127.0.0.1:18080/ws/agent --relay-token "replace-with-a-strong-token" --user local --device pc-local --workspace D:\Go_All\myai
```

The Relay binds each Agent token to its configured `user/device` identity. For multiple Agents, repeat `--agent-credential "user/device=token"` instead of sharing one token across devices.

After pairing the Android app, open `Files` to browse and preview files from that workspace, or open `Changes` to inspect changes compared with the SQLite workspace history baseline, preview diffs, and revert restorable files.

## Current Commands

- `myai agent`: starts the PC Agent and connects to Relay
- `myai chat`: starts a simple interactive chat loop
- `myai client`: starts the remote client simulator
- `myai completion`: generates shell completion scripts
- `myai help`: shows command help
- `myai relay`: starts the mobile/Agent Relay
- `myai sandbox doctor`: runs a temporary OpenSandbox smoke test
- `myai skill`: lists, searches, reloads, or installs Skills

Inside chat:

- `/help`: shows chat commands
- `/exit`: leaves chat

## Main Capabilities

- OpenAI Chat Completions、Anthropic Messages、Google Generative AI、Mistral Chat 和 Ollama Chat 协议及流式响应
- Chat and Plan session modes
- Local tools, permission approval, Skills, Hooks, and MCP tools
- MongoDB persistence, Redis current-session cache, and SQLite workspace history
- Relay-based mobile pairing and remote Agent control
- Mobile session, file, change, model, context, Plan, and hierarchical knowledge-base management
- AI experience memory extraction, review, versioning, retrieval, and Mobile CRUD
