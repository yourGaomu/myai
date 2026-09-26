# 模型生成功能实现说明

> 本文解释 MyAI 如何把一条已经进入 Session 的用户消息转换为模型请求，并最终得到 Assistant 回答。普通消息的手机、Relay、Agent 链路见 [普通聊天消息功能实现](FEATURE_CHAT_MESSAGE_IMPLEMENTATION.md)，工具细节见 [工具执行功能实现](FEATURE_TOOL_EXECUTION_IMPLEMENTATION.md)，Plan 特有行为见 [Plan 模式功能实现](FEATURE_PLAN_MODE_IMPLEMENTATION.md)。

## 1. 功能目标

模型生成不是一次简单的 HTTP 调用，而是以下流程的组合：

```text
选择会话模型
-> 构建本轮运行时指令
-> 检查并压缩上下文
-> 选择可见工具
-> 构建消息快照
-> 调用模型并推送流式内容
-> 可选执行工具并再次调用模型
-> 汇总 reasoning 和 TokenUsage
-> 提交 Assistant 到内存 Session
-> 异步持久化
```

## 2. Java/Spring Boot 对照

| Go 对象 | Spring Boot 类比 | 作用 |
|---|---|---|
| `ChatModelPort` | 模型 Gateway 接口 | 隔离具体模型 SDK |
| `llm.Model` | Gateway 实现类 | 通过协议 Adapter 调用 LangChainGo 模型 provider |
| `llm.Client` | Bean Registry | 保存 model ID 到运行时模型的映射 |
| `TaskService` | 带审计切面的应用 Service | 创建任务 ID 和文件检查点 |
| `AssistantGenerationService` | UseCase Service | 编排一次完整回答 |
| `AgentLoopService` | Agent 执行 Service | 模型与工具多轮循环 |
| `SnapshotService` | Context Builder | 生成本轮模型消息快照 |
| `ResponseCommitService` | 聚合根提交 Service | 写入 Assistant、Usage 和可选 Plan |
| `Persistence` | 异步 Repository Adapter | 保存回答和当前 Session |

## 3. 入口参数

`ChatService` 最终调用：

```go
GenerationTasks.Generate(ctx, generationcommand.GenerationTask{
	Session:     current,
	LatestInput: latestInput,
	Title:       title,
	Reason:      reason,
	Stream:      stream,
	CapturePlan: true,
})
```

| 字段 | 作用 |
|---|---|
| `Session` | 模型、模式、权限、消息、摘要和窗口配置 |
| `LatestInput` | Plan 捕获目标和任务记录；Skill/Plan runtime 已在消息命令阶段生成 |
| `Title` | 新会话标题或 Plan 执行标题 |
| `Reason` | 检查点审计原因 |
| `Stream` | reasoning、answer、tool 和 permission 回调 |
| `CapturePlan` | 成功后是否允许捕获结构化 Plan |
| `ForceChatMode` | Plan 执行时跳过 Plan 限制 |

## 4. 总调用链

```text
ChatService.SendMessageStreamForSession
-> MessageCommandService.AppendUserMessage
   -> RuntimeInstructionProvider.Prompt
   -> AddUserTurnTo(Synthetic runtime + User)
-> ChatService.generateAssistantForSession
-> TaskService.Generate
   -> RequestIDGenerator.NewRequestID
   -> TaskRecorderFactory.NewTaskRecorder
   -> recorder.Attach(ctx)
   -> AssistantGenerationService.Generate
      -> ModelProvider.GetModel
      -> CompactService.CompactIfNeeded
      -> AgentLoopService.Run
         -> SnapshotService.Snapshot
         -> ToolCatalog.ToolsForSession
         -> ChatModelPort.Generate
         -> 0..n 次 ToolExecutor.Execute
      -> ResponseCommitService.Commit
      -> Persistence.PersistAssistant
      -> Persistence.PersistCurrentSession
      -> Context Query
   -> recorder.Save + Close
```

## 5. 模型如何在启动时注册

调用链：

```text
Application.InitClient
-> llm.NewClient
-> model.BootstrapService.Bootstrap
-> Repository.ListConfigs 或 YAML seed
-> adapter/model/langchaingo.Factory.CreateModel
-> Registry.SetModelInfo
```

核心接口：

```go
type ChatModelPort interface {
	Generate(ctx context.Context, request GenerateRequest) (ChatResult, error)
}

type Registry interface {
	GetModel(name string) ChatModelPort
	HasModel(name string) bool
	ListModels() []ModelInfo
}
```

`llm.Client` 内部有两个 map：

```go
type Client struct {
	models map[string]modelport.ChatModelPort
	infos  map[string]ModelInfo
}
```

`models` 用于运行，`infos` 用于手机模型列表和默认模型展示。

## 6. 模型配置来源

优先级：

```text
Mongo 已有模型配置
-> 使用 Mongo

Mongo 没有配置
-> 使用 application.yaml seed
-> Mongo 可用时保存 seed

Mongo 不可用
-> 直接使用 YAML seed
```

每个启用配置由 `adapter/model/langchaingo.Factory` 按 `Protocol` 选择 Adapter 创建。当前内置协议为：

| 协议值 | 典型服务 | 认证/地址说明 |
|---|---|---|
| `openai-chat-completions` | OpenAI、DeepSeek、兼容 OpenAI 的网关 | HTTP API 根地址，可 Bearer 或无认证 |
| `anthropic-messages` | Anthropic/Claude Messages API | 配置 `AuthType=bearer` 和 API key，使用 Anthropic 原生消息格式 |
| `google-generative-ai` | Google Gemini Generative AI | 配置 `AuthType=bearer` 和 API key，使用 Google 默认原生生成接口；不支持自定义 `BaseURL` |
| `mistral-chat` | Mistral Chat API | 配置 `AuthType=bearer` 和 API key，使用 Mistral 原生接口 |
| `ollama-chat` | Ollama 本地服务 | 无认证，使用 Ollama Chat API |

Provider 标签只是展示和筛选用的厂商字符串，不能替代协议选择。

例如 OpenAI Chat Completions Adapter：

```go
openai.New(
	openai.WithToken(config.APIKey),
	openai.WithBaseURL(config.BaseURL),
	openai.WithModel(config.ModelName),
)
```

其他协议由各自 Adapter 负责 endpoint、认证头和请求/响应映射；不能假设所有 provider 都共享 OpenAI-compatible 的 URL 或消息格式。

## 7. TaskService 的职责

源码：`core/application/chat/generation/service/task_service.go`

```go
requestID := s.RequestIDs.NewRequestID()
recorder := s.newRecorder(TaskRecord{
	Title: command.Title, Reason: command.Reason,
	SessionID: command.Session.ID, RequestID: requestID,
})
ctx = recorder.Attach(ctx)
```

这里的 `requestID` 是内部任务 ID，不是手机协议 `request_id`。它用于：

- 关联文件修改检查点。
- 关联工具执行记录和共享 Asset。
- 描述一次生成任务的标题和原因。

`defer` 保证成功或失败时都尝试保存和关闭 Recorder。

## 8. AssistantGenerationService

源码：`core/application/chat/generation/service/assistant_generation_service.go`

它验证以下依赖：

```text
Session
ModelProvider
AgentRunner
ResponseCommitter
```

然后根据 Session 模型选择运行时对象：

```go
model := s.Models.GetModel(command.Session.Model)
if model == nil {
	return error("model not found")
}
```

因此模型是 Session 级设置，不是每次都使用应用默认值。

## 9. 固定系统提示词

每个新 Session 的第一条消息来自 `core/session/Session.go`：

```text
[system] You are myai, a local AI coding assistant...
```

它包含工作区、安全、工具使用和最终回复规则。该消息随 Session 生命周期保存于内存聚合中，Chat/Plan 切换不会改写它。

固定 system 的目标是：

- 所有会话共享核心行为。
- 历史前缀保持稳定。
- 模式切换不污染历史。

## 10. 运行时提示词

运行时提示词在进入 `AssistantGenerationService` 之前，由 `MessageCommandService.AppendUserMessage` 计算：

```go
runtimeInstruction := s.RuntimeInstructions.Prompt(
	ctx, current, command.Input, command.ForceChatMode,
)
s.Memory.AddUserTurnTo(current.ID, runtimeInstruction, command.Input)
```

`RuntimeInstructionBuilder` 合并两种动态内容：

```text
PlanModePrompt（仅 Plan 生成阶段）
+
根据 LatestInput 匹配的 Skill prompt
```

普通 Chat 且没有 Skill 时仍返回一个轻量 turn boundary，用于声明旧 runtime 指令只作用于各自紧随的 user message，避免旧 Plan/Style 规则泄漏到新 turn。

## 11. 动态提示词如何插入

`AddUserTurnTo` 在同一内存锁中按顺序追加两条消息：

```text
[固定 system]
[旧历史]
[system: Runtime instructions for this turn, SyntheticReason=runtime_instruction]
[最新 user]
```

Synthetic Message 和真实 user message 随后由 `UserMessagePersistence` 以相同顺序写入 Mongo。它不出现在手机历史列表中，但会在会话重载时恢复到 `Session.Messages`，供后续轮次和 Prompt Cache 使用。

## 12. Context Snapshot

```go
SnapshotService.Snapshot(current)
-> contextmgr.BuildSnapshot(current.Messages, ...)
```

`Snapshot` 包含：

```go
type Snapshot struct {
	Info     Info
	Messages []domainmessage.Message
	Prefix   []domainmessage.Message
}
```

| 字段 | 含义 |
|---|---|
| `Messages` | 实际发给模型的消息 |
| `Prefix` | 固定 system、可选 summary 和已经完成的历史 turn |
| `Info` | Token、窗口、摘要和前缀哈希信息 |

## 13. Prompt Cache 指标

`BuildSnapshot` 计算：

```text
PrefixTokens
CacheableTokens
PrefixHash
SummaryHash
```

当前 `Prefix` 是实际选中消息中“当前 turn 之前”的稳定部分。当前 turn 的 Synthetic runtime message 和 user message不计入，但已经完成的旧 turn 会计入，因此下一轮可以继续复用完整历史前缀。

provider 实际缓存命中量来自返回的 `PromptCachedTokens`，项目内哈希用于观察本地前缀是否稳定，两者不是同一个指标。

## 14. 消息窗口选择

Token 预算：

```text
ContextWindowK * 1000
```

选择顺序：

1. 固定保留 system 和 summary。
2. 将最近消息划分为完整消息块。
3. 从最新向旧选择，直到达到预算。
4. Assistant tool call 与其后 Tool result 作为一个块，不拆开。

如果一个最新块本身超过预算，仍保留该块，确保本轮输入不会完全消失。

## 15. 自动压缩

触发条件：

```text
Snapshot.Truncated == true
或
SelectedTokens >= WindowK * 1000 * 0.70
```

`CompactService` 保留最近 8 个消息块，将更旧的完整块交给 `SummaryService`，再更新：

```text
Session.Summary
Session.CompactedMessages
```

压缩错误通过 `OnCompactError` 记录，但不会中止当前模型请求。返回的 `CompactInfo` 只有压缩成功时才反映触发结果。

## 16. AgentLoopService

每轮执行：

```go
result, err := command.Model.Generate(ctx, GenerateRequest{
	Messages: Snapshot(...).Messages,
	Tools:    ToolsForSession(...),
	Stream:   command.Stream,
})
```

如果 `ToolCalls` 为空，返回最终结果。如果存在 ToolCalls：

```text
执行工具
-> ToolCall + ToolResult 追加到 Session.Messages
-> 重新构建快照
-> 再次调用模型
```

同一 turn 内不会重新计算 runtime instruction；工具轮次始终复用 user message 前已经持久化的那条 Synthetic Message。

详细工具规则见 [工具执行功能实现](FEATURE_TOOL_EXECUTION_IMPLEMENTATION.md)。

## 17. 工具轮数上限

默认：

```go
const DefaultMaxToolRounds = 32
```

达到 32 轮后，系统额外进行一次不带 Tools 的模型请求，强制产生最终回答。正常情况在模型不再请求工具、Stop Hook 不续跑、且没有中途插话时结束。

## 18. 多轮结果汇总

每轮 Usage：

```go
totalUsage = totalUsage.Add(result.Usage)
```

多轮 reasoning：

```go
strings.Join(reasoningParts, "\n")
```

最终正文使用最后一轮 `ChatResult.Content`；中间工具轮的正文不会拼入最终 Content。

## 19. 模型请求 DTO

```go
type GenerateRequest struct {
	Messages []domainmessage.Message
	Tools    []Tool
	Stream   ChatStreamHandler
	Settings generation.ResolvedSettings
}

type ChatResult struct {
	Content   string
	Reasoning string
	Usage     TokenUsage
	ToolCalls []ToolCall
}
```

应用层只依赖这些稳定 DTO，不依赖 LangChainGo 类型。

## 20. LangChainGo 映射

`message_mapper.go` 映射：

| 领域角色/Part | LangChainGo |
|---|---|
| system | `ChatMessageTypeSystem` |
| user | `ChatMessageTypeHuman` |
| assistant | `ChatMessageTypeAI` |
| tool | `ChatMessageTypeTool` |
| text | `TextContent` |
| tool call | `llms.ToolCall` |
| tool result | `llms.ToolCallResponse` |

工具定义由 `tool_mapper.go` 转成 `llms.Tool` 和 `FunctionDefinition`。

## 21. Provider 调用参数

生成参数在 `AssistantGenerationService` 中按以下优先级解析：

```text
Session 显式覆盖 > Model 默认值 > System 兜底值
```

`nil` 表示继承上一级，因此 `temperature=0` 仍是有效的显式设置。系统兜底值为：

```text
temperature = 0.7
top_p = 1.0
max_output_tokens = 2048
```

解析后的 `generation.ResolvedSettings` 放入 `GenerateRequest.Settings`，AgentLoop 的每一轮都传递同一份参数。`llm.Model` 将其映射为 LangChainGo 调用选项：

```go
llms.WithTemperature(settings.Temperature)
llms.WithTopP(settings.TopP)
llms.WithMaxTokens(settings.MaxOutputTokens)
```

存在工具时额外设置：

```go
llms.WithTools(tools)
llms.WithToolChoice("auto")
```

模型默认值来自模型配置和 Registry 元数据，Session 覆盖值保存在 Session 聚合及其持久化记录中。上下文压缩等非 Session 模型调用显式使用 `generation.SystemDefaults()`，不依赖 Adapter 隐藏默认值。

### 21.1 Mobile 如何查询和修改会话参数

Mobile 进入设置页的“生成”分区时，以及当前 Session 或模型发生变化时，会主动查询一次服务端状态：

```text
SettingsPanel
-> useSessionSettingsActions.requestGenerationPreferences
-> session_generation_query
-> Relay 校验 client_token 并转发
-> Agent.handleSessionGenerationQuery
-> ChatService.SessionPreferencesForSession
-> session_generation_query_result
-> useRemoteMessageHandler
-> useSessionGenerationState.applyPreferences
-> SettingsPanel 展示 Session 覆盖、模型默认、系统兜底和最终生效值
```

查询结果的核心 DTO 为：

```json
{
  "preferences": {
    "session_id": "session-123",
    "session_overrides": {
      "temperature": 0,
      "top_p": null,
      "max_output_tokens": 4096
    },
    "model_defaults": {
      "temperature": null,
      "top_p": 0.9,
      "max_output_tokens": null
    },
    "effective": {
      "temperature": 0,
      "top_p": 0.9,
      "max_output_tokens": 4096
    },
    "style_instruction": "使用简洁中文，先给结论。"
  }
}
```

其中 `session_overrides` 和 `model_defaults` 保留可空字段，`effective` 一定是经过三级解析后的完整数值。界面会分别标明“模型未设置”和“系统兜底”，不会把系统值误显示为模型配置。

### 21.2 添加第三方模型

当前新增模型支持 Factory 已注册的五种协议。这里的 `BaseURL` 是对应协议的 API 根地址；OpenAI Chat Completions 例如：

```text
https://api.example.com/v1
```

它不是协议名，运行时由 LangChainGo 在根地址后拼接 `/chat/completions`。因此不能把完整的
`/chat/completions` 地址再次作为 `BaseURL` 保存。模型配置中的几个概念必须分开：

| 字段 | 含义 |
|---|---|
| `Provider` | 厂商标签，可填写 `deepseek`、`ollama` 等自由文本 |
| `Protocol` | 选择具体请求协议，例如 `openai-chat-completions`、`anthropic-messages`、`google-generative-ai`、`mistral-chat` 或 `ollama-chat` |
| `AuthType` | `bearer` 或 `none` |
| `BaseURL` | API 根地址，不包含凭据、Query 或 Fragment |
| `ModelName` | 厂商识别的模型名 |

保存调用链为：

```text
Mobile SettingsPanel
-> model_config_add
-> Relay
-> Agent.handleModelConfigAdd
-> ChatService.AddModelConfig
-> ModelConfigService.AddConfig
-> Factory.CreateModel
-> Mongo SaveConfig
-> Registry.SetModelInfo
-> model_config_add_result
```

新增模型只有在 Factory 创建成功、Mongo 保存成功后才进入运行时 Registry。`ModelSummary` 只返回
`has_api_key`，不会把原始 API Key 返回给手机端；当前 API Key 仍以配置数据库中的明文保存，这是待后续接入加密密钥存储的安全风险。

保存前的连接测试使用独立的 `model_config_test` 协议。它复用相同的校验和 Factory，但只创建临时模型，
使用一次最小请求，并设置 30 秒超时；测试成功不会写 Mongo，也不会注册模型。这样“地址是否可用”和“是否保存模型”
是两个明确的用例：

```text
Mobile -> model_config_test
       -> ConfigService.TestConfig
       -> transient Factory model.Generate
       -> model_config_test_result { success, latency_ms, message }
```

Mobile 的认证方式必须显式选择。`Bearer Token` 需要 API Key；`无认证`适用于本地兼容服务，Factory 会移除
`Authorization` 请求头。模型默认 `temperature`、`top_p` 和 `max_output_tokens` 可以在添加表单中填写，
留空表示继续使用系统兜底值。

### 21.3 模型配置生命周期

模型保存后仍然可以从 Mobile 设置页管理。四个管理操作都经过 Relay 和 Agent，最终由同一个
`ModelConfigService` 修改 Mongo 配置并同步运行时 Registry：

```text
model_config_update       -> UpdateConfig       -> 重建启用模型 -> SaveConfig -> SetModelInfo
model_config_enabled_set  -> SetEnabled        -> 校验会话引用 -> 创建或移除运行时模型
model_config_default_set  -> SetDefault        -> 更新所有 is_default -> 更新新建会话默认模型
model_config_delete       -> DeleteConfig      -> 校验默认值和会话引用 -> 删除配置和 Registry
```

编辑模型时，`api_key` 留空不会覆盖旧密钥；如果用户明确切换为 `none`，则会清除旧密钥。
默认模型不能直接删除或禁用，正在被任意会话使用的模型也不能删除或禁用，避免已有 Session
在下一轮生成时突然失去模型。`llm.Client` 内部使用读写锁，生成请求读取 Registry 的同时，
模型管理请求可以安全地更新或删除 Map。

所有管理操作都返回 `model_config_mutation_result` 和完整模型列表，Mobile 以服务端列表为准，
不在本地推测默认值、启用状态或删除结果。

参数保存使用独立协议：

```text
session_generation_set
-> Agent.handleSessionGenerationSet
-> ChatService.SetGenerationSettingsForSession
-> Session Settings UseCase
-> MemoryStore + Session persistence + SessionChanged event
-> 重新查询 SessionPreferences
-> session_generation_set_result
```

Mobile 空输入框会被编码为 JSON `null`，表示继承；字符串 `"0"` 会被编码为数值 `0`，表示显式覆盖。因此 Temperature 的继承和零温度不会混淆：

```json
{
  "settings": {
    "temperature": 0,
    "top_p": null,
    "max_output_tokens": null
  }
}
```

“继承”只清空当前输入框，点击“应用参数”后才保存；“全部继承”会立即发送三个 `null`。Mobile 先校验 Temperature `[0,2]`、Top P `[0,1]`、最大输出 Token `[1,131072]`，后端领域层仍会再次校验，客户端校验不能替代服务端约束。

回复风格单独使用 `session_style_set`，避免参数有效但风格超过 2000 字符时出现一半成功、一半失败的状态。成功结果会回传最新 Preferences 和可见提示；服务端错误通过统一 `error` 协议回到当前设置区域。保存期间按钮进入 loading 并禁止重复提交。

当前 Mobile 可以选择模型并修改 Session 覆盖值，但没有编辑 Model 默认生成参数的独立界面。Model 默认值仍由模型配置入口维护。`StyleInstruction` 会作为本轮 Synthetic Runtime Instruction 写入并持久化到 `Session.Messages`，但历史查询会过滤 Synthetic Message，Mobile 不会把它展示为普通聊天消息。

## 22. 流式正文

Provider 每产生一个 chunk：

```go
builder.WriteString(text)
handler.OnAnswer(text)
```

`builder` 保存完整正文，`OnAnswer` 立即向上游推送。Agent 把它映射为 `assistant_delta.content`。

如果 provider 没有实际流式回调，Adapter 会从最终 response 读取 Content 并调用一次 `OnAnswer`。

## 23. 流式 Reasoning

`WithStreamingReasoningFunc` 将 reasoning chunk 写入独立 builder 并调用 `OnReasoning`。

非流式 provider 则依次读取：

```text
Choice.ReasoningContent
GenerationInfo["ThinkingContent"]
```

最终 reasoning 会进入 `ChatResult` 和持久化 MessageRecord。

## 24. ToolCall chunk 过滤

存在 Tools 时，流式 callback 会尝试把 chunk 解析为 ToolCall JSON 数组。识别为工具调用时，不把该 chunk 当正文发给手机，避免显示内部函数参数。

真正 ToolCalls 从最终 `ContentResponse.Choices[0].ToolCalls` 映射。

## 25. TokenUsage

从 `GenerationInfo` 读取：

```text
PromptTokens
CompletionTokens
TotalTokens
ReasoningTokens
PromptCachedTokens
```

`Available` 只有 provider 至少返回 prompt、completion 或 total 字段时才为 true。数值转换支持常见整数和浮点类型。

## 26. ResponseCommitService

模型成功后：

```go
Memory.AddAssistantMessageTo(sessionID, result.Content)
Memory.AddUsageTo(sessionID, result.Usage)
```

内存先提交，之后才异步持久化。Plan 模式且 `CapturePlan=true` 时还会捕获结构化 Plan，详见 [Plan 模式功能实现](FEATURE_PLAN_MODE_IMPLEMENTATION.md)。

## 27. 异步持久化

```text
PersistAssistant
-> ThreadPool
-> 保存 Assistant MessageRecord
-> 保存 SessionRecord

PersistCurrentSession
-> ThreadPool
-> Redis CurrentSessionCache.Save
```

持久化有默认 10 秒超时。失败只记录日志，不回滚已经提交的内存回答。

## 28. GenerationResponse

```go
type GenerationResponse struct {
	SessionID string
	Result    ChatResult
	Context   contextmgr.Info
	Compact   CompactInfo
	Plan      *Plan
}
```

Agent 最终将其映射为 `assistant_done`。ContextInfo 是最终提交后的快照信息，因此消息数和 Token 估算包含新 Assistant。

## 29. 错误传播

会中止生成并返回远程 error：

- Session、ModelProvider、AgentRunner 等必需依赖为空。
- Session.Model 在 Registry 中不存在。
- Context 构建或模型调用失败。
- Tool Executor 返回基础设施错误。
- ResponseCommit 写内存失败。
- 流式 WebSocket 写失败。

不会中止：

- 自动压缩失败。
- Assistant、当前 Session 等异步持久化失败。
- TaskRecorder 保存或关闭失败。

## 30. 问题清理结果与剩余风险

已解决：生成参数不再由 `llm.Model` 写死。Temperature、TopP 和 MaxOutputTokens 已支持系统兜底、模型默认值和 Session 覆盖值三级解析。

仍需注意：

1. ToolCall chunk 依赖 JSON 形状识别；在启用 Tools 时，极少数正文若刚好是类似 ToolCall 的 JSON 数组，可能被过滤。
2. 自动压缩失败后会继续尝试生成，极端情况下 provider 仍可能因上下文超限返回错误。
3. 内存提交与异步数据库保存不是事务；进程在写入完成前退出可能丢失最新持久化记录。

## 31. 具体示例

输入：

```text
解释 main.go 的启动流程
```

无 Skill、Chat 模式时：

```text
runtime instruction = turn boundary
Snapshot = fixed system + selected history + synthetic boundary + latest user
Tools = 根据 PermissionMode 选择
Model.Generate
-> reasoning delta 0..n
-> answer delta 0..n
-> ChatResult(no ToolCalls)
-> Commit assistant + usage
```

如果模型先调用 `read_file`，则 AgentLoop 在工具结果追加后重新构建 Snapshot，再进行第二次 Model.Generate。

## 32. 推荐断点

| 文件 | 函数 | 观察内容 |
|---|---|---|
| `core/service/chat.go` | `generateAssistantForSession` | GenerationTask 输入 |
| `task_service.go` | `Generate` | 内部任务 ID、Recorder |
| `message/command_service.go` | `AppendUserMessage` | runtime instruction、Synthetic/User 顺序 |
| `assistant_generation_service.go` | `Generate` | model、generation settings、compact |
| `runtime_instruction_builder.go` | `Build` | Plan/Skill 合并结果 |
| `context.go` | `BuildSnapshot` | Prefix、选择消息、Token |
| `agent_loop_service.go` | `Run` | 每轮 ToolCalls、Usage |
| `core/llm/model.go` | `ChatWithStreamToolsHandlerCtx` | provider options 和 chunk |
| `response_commit_service.go` | `Commit` | 内存 Assistant、Usage、Plan |

## 33. 测试

重点测试：

```text
TestAssistantGenerationServiceGenerateOrchestratesUseCase
TestAssistantGenerationServiceCompactErrorIsNonFatal
TestAssistantGenerationServiceReturnsModelNotFound
TestGenerationTaskServiceWrapsGenerationWithRecorder
TestAgentLoopServiceReturnsWhenModelDoesNotRequestTools
TestAgentLoopServiceExecutesToolsAndContinuesGeneration
TestAgentLoopServiceFinalGenerationOmitsToolsAfterMaxRounds
TestContextSnapshotServiceRuntimePromptDoesNotChangeCacheablePrefix
TestMessageMapperPreservesToolParts
TestResponseCommitServiceWritesAssistantMessageAndUsage
```

运行：

```powershell
go test ./core/application/chat ./core/application/model/service
go test ./core/adapter/llm/langchaingo ./core/llm
go test ./...
```

## 34. 源码索引

```text
core/application/chat/generation/service/
core/application/chat/context/service/
core/application/chat/compaction/service/
core/application/runtime/service/
core/application/model/service/
core/port/model/
core/llm/
core/adapter/llm/langchaingo/
core/adapter/model/langchaingo/
core/adapter/chat/generation/
```

## 35. 阅读检查

阅读后应能回答：模型如何从 Registry 取得、动态提示词为什么不写历史、PrefixHash 包含什么、何时压缩、为什么模型可能调用多轮、Token 如何汇总、最终回答何时进入内存、哪些失败不会返回给手机。
