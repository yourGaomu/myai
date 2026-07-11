# myai 架构重构理解草案

本文档只讨论 `myai` 主项目。

`UrlShortener` 暂时不纳入本次架构判断。

## 1. 当前判断

我重新看了 `myai` 主项目后，结论是：

当前混乱的主要来源不是短链接项目，而是 `myai` 主项目内部的边界没有隔离好。

现在项目已经有了这些能力：

- CLI
- PC agent
- relay server
- mobile app
- LLM 调用
- tool calling
- skill 管理
- session 管理
- plan 模式
- 文件预览
- 变更预览
- history / checkpoint
- mongo / redis
- asset 上传

功能已经比较多，但代码结构还停留在“按功能随手分包”的阶段，没有形成稳定的架构层。

所以现在的问题不是“缺少目录”，而是：

```text
transport、application、domain、infrastructure 之间没有严格边界。
```

## 2. 当前主项目结构观察

当前 Go module 包大致是：

```text
myai
├─ core/App.go
├─ core/cmd
├─ core/service
├─ core/session
├─ core/plan
├─ core/contextmgr
├─ core/llm
├─ core/tool
├─ core/skill
├─ core/mcp
├─ core/remote
│  ├─ agent
│  ├─ relay
│  ├─ protocol
│  ├─ files
│  └─ changes
├─ core/store
│  ├─ data
│  └─ cache
├─ core/infra
├─ core/asset
├─ core/history
├─ core/hook
├─ core/sandbox
├─ mobile
├─ skills
└─ resource
```

这些包名看起来已经分了模块，但依赖方向还不清楚。

例如：

- `core/service/chat.go` 同时知道 session、plan、llm、tool、store、cache、history、hook、skill。
- `core/session` 直接使用 `langchaingo/llms.MessageContent`。
- `core/llm` 直接暴露 `langchaingo/llms.ToolCall`。
- `core/store/data` 里直接引用 `core/plan.Plan`。
- `core/remote/agent` 既做 websocket，又做 payload decode，又调用业务，又组装返回。
- `mobile/src/screens/MobileAppScreen.tsx` 聚合了大量状态和动作。
- `mobile/src/hooks/useRemoteMessageHandler.ts` 是一个大型协议分发器。

## 3. 当前最明显的职责混合点

### 3.1 ChatService 过重

`core/service/chat.go` 是当前后端最明显的“上帝类”。

它现在承担：

- 新建 / 加载 / 删除 / 恢复 session
- 用户消息写入
- assistant 消息写入
- LLM 调用
- tool calling loop
- tool permission
- plan mode prompt
- plan capture
- plan execution
- context snapshot
- auto compact
- compact summary
- token usage
- Mongo 持久化
- Redis 当前 session 缓存
- history recorder
- asset record 保存
- hook 触发

这在 Java 里类似于：

```text
ChatServiceImpl
  = Controller helper
  + UseCase
  + DomainService
  + Repository caller
  + PromptBuilder
  + LLM adapter
  + Tool executor
  + Event publisher
```

这个类必须拆。

### 3.2 Agent 过重

`core/remote/agent/agent.go` 现在也承担了太多东西：

- websocket 连接
- 心跳
- relay 消息读取
- protocol decode
- request routing
- session runtime cancel
- permission ask
- chat service 调用
- file service 调用
- change service 调用
- protocol response 组装

它本质上应该只是 transport adapter。

业务编排不应该放在这里。

### 3.3 Domain 不纯

目前很多领域对象被外部 SDK 或持久化结构污染。

典型例子：

```go
type Session struct {
    Messages []llms.MessageContent
}
```

这意味着 domain/session 直接依赖 LLM SDK。

再比如：

```go
type ChatResult struct {
    ToolCalls []llms.ToolCall
}
```

这意味着 application 层如果使用 ChatResult，也会被 langchaingo 类型污染。

领域对象应该是项目自己的对象，不应该直接暴露第三方 SDK 类型。

### 3.4 DTO / PO / Domain 混用

现在项目里至少有四类对象：

1. transport DTO  
   例如 `core/remote/protocol.Message`、`SessionSummary`、`AssistantDonePayload`

2. persistence PO  
   例如 `core/store/data.SessionRecord`、`MessageRecord`

3. domain model  
   例如 `core/session.Session`、`core/plan.Plan`

4. application result  
   例如 `service.ChatResponse`

但这些对象之间没有明确边界。

当前很多地方是手写拼装，甚至跨层直接引用。

例如：

```go
type SessionRecord struct {
    CurrentPlan *agentplan.Plan
}
```

这让持久化对象直接依赖 domain plan。

长期看应该有 mapper：

```text
domain.Plan <-> persistence.PlanRecord
domain.Plan <-> protocol.PlanDTO
```

## 4. 目标架构原则

我理解你想要的是更接近 Java 项目的清晰分层：

```text
接口就是接口
实现就是实现
DTO 就是 DTO
PO 就是 PO
Domain Model 就是 Domain Model
UseCase 就是 UseCase
```

不要让一个对象同时承担多个身份。

## 5. 目标分层

### 5.1 domain

纯业务对象和业务规则。

建议包含：

```text
domain/session
domain/message
domain/plan
domain/model
domain/tool
domain/usage
```

这一层不能依赖：

- websocket
- mongo
- redis
- mobile protocol
- langchaingo
- viper
- cobra
- React Native

### 5.2 application

业务用例和业务编排。

建议包含：

```text
application/chat
application/plan
application/session
application/model
application/context
application/runtime
```

这里负责“做什么”。

例如：

- SendMessage
- RegenerateMessage
- ExecutePlan
- SetSessionMode
- CompactSession
- SwitchModel

### 5.3 port

接口层。

建议包含：

```text
port/repository
port/model
port/tool
port/skill
port/event
port/asset
```

示例：

```go
type ChatModelPort interface {
    Generate(ctx context.Context, request GenerateRequest, stream StreamHandler) (GenerateResult, error)
}

type SessionRepository interface {
    Get(ctx context.Context, id string) (domain.Session, error)
    Save(ctx context.Context, session domain.Session) error
}

type ToolExecutor interface {
    Execute(ctx context.Context, call domain.ToolCall) (domain.ToolResult, error)
}
```

这一层只放接口和 command/result 类型。

### 5.4 adapter

具体实现。

建议包含：

```text
adapter/llm/langchaingo
adapter/persistence/mongo
adapter/cache/redis
adapter/tool/local
adapter/skill/filesystem
adapter/asset/http
adapter/transport/remote
```

这里负责“怎么做”。

### 5.5 bootstrap

应用启动和依赖组装。

当前 `core/App.go` 应该逐渐变成 bootstrap 层。

它可以负责：

- 读取配置
- 初始化 mongo / redis
- 初始化 model adapter
- 初始化 repository impl
- 初始化 application service
- 初始化 transport adapter

但不应该承载业务逻辑。

## 6. 建议依赖方向

目标依赖方向：

```text
transport adapter -> application -> domain
                         |
                         v
                       port

infrastructure adapter -> port

bootstrap -> all
```

也就是说：

- domain 不依赖任何外部实现
- application 依赖 domain 和 port
- adapter 实现 port
- transport 调 application
- bootstrap 负责把它们装起来

禁止出现：

```text
domain -> protocol
domain -> mongo
domain -> langchaingo
application -> websocket
application -> mongo concrete implementation
```

## 7. myai 当前包到目标层的映射

### 7.1 core/session

当前职责：

- session domain
- session memory manager
- message storage
- current session pointer
- model pointer
- permission mode

目标拆分：

```text
domain/session
  Session
  SessionID
  AgentMode
  PermissionMode

domain/message
  Message
  MessageRole

application/session
  SessionUseCase

adapter/memory
  InMemorySessionStore
```

重点：

`Session` 不能继续直接存 `llms.MessageContent`。

### 7.2 core/plan

当前职责相对清楚：

- Plan
- Step
- 状态
- markdown step extraction

目标：

```text
domain/plan
  Plan
  PlanStep
  PlanStatus

application/plan
  PlanExtractor
  PlanExecutionService
```

注意：

`ExtractSteps` 可以留在 plan 相关 domain service 或 application service 中。

### 7.3 core/contextmgr

当前职责：

- token estimate
- context window selection
- summary prefix
- cacheable prefix

问题：

当前直接依赖 `llms.MessageContent`。

目标：

```text
application/context
  ContextBuilder
  ContextWindowPolicy
  ContextSnapshot

adapter/llm/langchaingo
  MessageMapper
```

重点：

先用项目自己的 `domain.Message` 做上下文裁剪，最后再映射成 LLM SDK message。

### 7.4 core/llm

当前职责：

- model registry
- langchaingo 调用
- streaming
- tool calls
- usage extraction

问题：

这是 adapter，但现在被 application 直接当业务模型使用。

目标：

```text
port/model
  ChatModelPort
  GenerateRequest
  GenerateResult
  StreamHandler

adapter/llm/langchaingo
  LangChainGoChatModel
  LangChainGoMessageMapper
  LangChainGoToolMapper

application/model
  ModelRegistry
```

### 7.5 core/tool

当前职责：

- tool interface
- tool registry
- local tools
- permission
- LLM tool schema mapper

问题：

tool registry 里直接有 LLM schema 转换。

目标：

```text
domain/tool
  ToolDefinition
  ToolCall
  ToolResult
  ToolPermission

port/tool
  ToolRegistry
  ToolExecutor

adapter/tool/local
  LocalToolExecutor

adapter/llm/langchaingo
  ToolSchemaMapper
```

### 7.6 core/store/data

当前职责：

- persistence PO
- Store interface
- model config PO
- asset PO
- message PO
- session PO

问题：

PO 和 repository interface 混在一个包。

目标：

```text
port/repository
  SessionRepository
  MessageRepository
  ModelConfigRepository
  AssetRepository

adapter/persistence/mongo
  MongoSessionRepository
  MongoMessageRepository
  MongoModelConfigRepository
  MongoAssetRepository
  records.go
```

PO 放 adapter 内部，不暴露给 application。

### 7.7 core/remote/protocol

当前职责：

- mobile / agent websocket protocol DTO

目标：

```text
adapter/transport/remote/protocol
```

它只应该放 DTO。

不能把 protocol DTO 传进 application 深处。

### 7.8 core/remote/agent

当前职责：

- websocket client
- protocol route
- permission bridge
- session runtime
- usecase 调用
- response mapping

目标：

```text
adapter/transport/remote/agent
  AgentClient
  RemoteMessageRouter
  PermissionBridge
  RuntimeController
  ProtocolMapper
```

Agent 不应该直接组装复杂业务结果。

### 7.9 core/remote/relay

当前职责：

- websocket server
- pair
- auth
- agent/client registry
- message forwarding
- embedded web

目标：

relay 可以作为独立 transport adapter。

它不应该依赖 chat domain。

目前 relay 的职责相对独立，但 message whitelist 需要更系统化，不能散落在 switch 里。

### 7.10 mobile

当前职责：

- mobile UI
- relay connection
- protocol DTO
- remote state
- chat/session/files/changes/settings 状态

问题：

- `MobileAppScreen.tsx` 聚合太多状态。
- `useRemoteMessageHandler.ts` 是一个巨大的 message reducer。
- 前端 protocol 类型和后端 protocol 类型手写同步，容易漂移。

目标：

```text
mobile/src/domain
mobile/src/application
mobile/src/protocol
mobile/src/state
mobile/src/features/chat
mobile/src/features/session
mobile/src/features/plan
mobile/src/features/files
mobile/src/features/changes
```

前端也应该按 feature 分边界。

## 8. Plan 模式在目标架构中的位置

Plan 模式应该拆成几个对象：

```text
domain/plan
  Plan
  PlanStep

application/runtime
  RuntimeInstructionBuilder
  ModePolicy

application/plan
  PlanExtractor
  PlanExecutionService
```

职责：

- `ModePolicy` 判断当前 agent mode 允许什么。
- `RuntimeInstructionBuilder` 生成本轮 runtime instruction。
- `PlanExtractor` 从模型回复里提取 plan。
- `PlanExecutionService` 执行已有 plan。
- `ChatUseCase` 只负责调用这些组件，不自己写 plan 细节。

## 9. Prompt / Runtime Instruction 的目标位置

Prompt 不应该散在 `ChatService` 里。

目标：

```text
application/runtime
  RuntimeInstructionBuilder

application/skill
  SkillSelector

application/context
  ContextBuilder
```

职责：

- SkillSelector 选择本轮技能。
- ModePolicy 提供模式规则。
- RuntimeInstructionBuilder 合成动态 runtime instruction。
- ContextBuilder 保证稳定前缀和缓存友好。

## 10. 重构顺序建议

我建议不要一上来全项目大搬家。

更稳的顺序是：

### 第一步：先立边界

新增目标包，但不立刻删除旧包。

例如：

```text
core/domain
core/application
core/port
core/adapter
```

### 第二步：先抽 domain.Message

这是最关键的一步。

目标是让 session/context/chat 先不直接依赖 `llms.MessageContent`。

### 第三步：抽 RuntimeInstructionBuilder

把 Plan/Chat 模式和 skill prompt 从 `ChatService` 中拿出来。

### 第四步：抽 ChatModelPort

让 application 不直接依赖 langchaingo。

### 第五步：拆 ChatUseCase 和 PlanUseCase

把 `ChatService` 拆成清晰用例。

### 第六步：拆 Repository

把 `core/store/data.Store` 拆成多个 repository interface。

### 第七步：整理 transport adapter

把 `agent.go` 中的 protocol mapping 和 usecase 调用拆开。

### 第八步：前端按 feature 分治

先拆 `useRemoteMessageHandler`，再拆 `MobileAppScreen` 的状态聚合。

## 11. 不建议现在做的事情

暂时不建议：

- 一次性移动所有目录
- 一次性重命名所有包
- 一次性重写 mobile
- 一次性改 protocol
- 一次性重写 store

原因：

这会产生巨大 diff，而且很容易把功能搞坏。

更合理的方式是：

```text
先抽边界 -> 加 adapter -> 迁移调用点 -> 删除旧代码
```

## 12. 当前最小可执行切入点

如果要开始动手，我建议第一批只做这几件：

1. 新增 `core/domain/message`
2. 新增 `core/domain/session`
3. 新增 `core/application/runtime`
4. 把 runtime prompt 构造从 `ChatService` 抽出去
5. 增加 mapper：domain message <-> langchaingo message
6. 保持外部行为不变

这一批改完以后，后面拆 ChatUseCase 会自然很多。

## 13. 最终目标

最终希望 `ChatService` 这种大类消失，变成：

```text
ChatUseCase
  - 处理发送消息

PlanUseCase
  - 处理计划生成 / 捕获 / 执行

SessionUseCase
  - 处理会话生命周期

ContextBuilder
  - 处理上下文窗口和缓存前缀

RuntimeInstructionBuilder
  - 处理本轮动态提示

ChatModelPort
  - 隔离 LLM SDK

Repository
  - 隔离 Mongo

Transport Adapter
  - 隔离 websocket / protocol
```

这样项目才会更像一个可维护的 Java 风格后端，而不是所有能力都往一个 service 里堆。

## 14. 按 Spring Boot 风格理解目标架构

你熟悉 Spring Boot，所以后续可以用 Spring Boot 的分层方式来约束 Go 项目。

虽然 Go 没有 class、annotation、bean container，但可以在包结构和依赖方向上模拟类似架构。

Spring Boot 常见结构：

```text
controller
service
repository
entity / domain
dto
mapper
config
client
event
```

对应到 `myai` 可以理解成：

```text
transport/controller   -> 接收外部请求，例如 websocket / CLI / mobile protocol
application/service    -> 业务用例编排
domain/entity          -> 纯业务对象
port/repository        -> repository 接口
adapter/repository     -> mongo / redis 实现
dto                    -> protocol request / response
mapper                 -> dto/entity/po 转换
config/bootstrap       -> 配置读取和依赖组装
client/adapter         -> LLM、asset、skillhub、MCP 等外部客户端
event                  -> hook / domain event / application event
```

## 15. Spring Boot 类比下的 myai 分层

### 15.1 Controller 层

在 Spring Boot 中：

```java
@RestController
class ChatController {
    private final ChatService chatService;
}
```

在 `myai` 中对应：

```text
core/remote/agent
core/remote/relay
core/cmd
mobile protocol handler
```

它们只应该做：

- 接收请求
- 参数校验
- DTO 转 Command
- 调用 UseCase / Service
- Result 转 Response DTO

它们不应该做：

- 拼 prompt
- 执行业务规则
- 直接操作 repository
- 直接操作 domain 内部状态

### 15.2 Service 层

在 Spring Boot 中：

```java
@Service
class ChatServiceImpl implements ChatService {
    private final SessionRepository sessionRepository;
    private final ChatModelClient chatModelClient;
}
```

在 `myai` 中对应：

```text
core/application/chat
core/application/plan
core/application/session
core/application/context
core/application/runtime
```

这一层负责业务编排。

例如：

```text
ChatApplicationService.SendMessage
  1. 加载 Session
  2. 写入 User Message
  3. 构建 Runtime Instruction
  4. 构建 Context
  5. 调用 ChatModelPort
  6. 执行 Tool Loop
  7. 捕获 Plan
  8. 保存 Message / Session
  9. 发布事件
```

注意：

Service 层可以依赖接口，但不应该依赖具体实现。

### 15.3 Repository 层

在 Spring Boot 中：

```java
interface SessionRepository extends MongoRepository<SessionEntity, String> {
}
```

在 `myai` 中应该拆成：

```text
core/port/repository
  SessionRepository
  MessageRepository
  ModelConfigRepository
  AssetRepository

core/adapter/persistence/mongo
  MongoSessionRepository
  MongoMessageRepository
  MongoModelConfigRepository
  MongoAssetRepository
```

当前 `core/store/data.Store` 太大，类似一个巨型 DAO：

```go
type Store interface {
    GetSession(...)
    SaveSession(...)
    SaveModelConfig(...)
    SaveMessage(...)
    SaveAsset(...)
    ListSessions(...)
    ListModelConfigs(...)
    ListMessages(...)
    ListAssets(...)
}
```

这应该按聚合拆开。

### 15.4 Entity / Domain 层

在 Spring Boot 中：

```java
class Session {
    private SessionId id;
    private List<Message> messages;
}
```

在 `myai` 中应该是：

```text
core/domain/session
core/domain/message
core/domain/plan
core/domain/tool
core/domain/model
```

Domain 不应该出现：

```go
llms.MessageContent
mongo bson tag
protocol json dto
websocket
viper
```

也就是说，Domain 是业务核心，不是数据库结构，也不是接口返回结构。

### 15.5 DTO 层

在 Spring Boot 中：

```java
class SendMessageRequest {}
class SendMessageResponse {}
class SessionDTO {}
```

在 `myai` 中对应：

```text
core/remote/protocol
mobile/src/protocol.ts
```

DTO 应该只在 transport 层使用。

不要让 application service 直接返回 protocol DTO。

应该是：

```text
protocol DTO -> application command -> domain
domain/application result -> protocol DTO
```

### 15.6 Mapper 层

Spring Boot 项目里常见：

```java
SessionMapper.toDTO(session)
SessionMapper.toEntity(session)
```

`myai` 现在缺少明确 mapper，导致到处手动组装。

后续需要：

```text
SessionMapper
MessageMapper
PlanMapper
TokenUsageMapper
ToolMapper
ModelMapper
```

分三类：

```text
domain <-> protocol dto
domain <-> persistence po
domain <-> llm sdk model
```

### 15.7 Config / Bootstrap 层

Spring Boot 中：

```java
@Configuration
class AppConfig {
    @Bean ChatModel chatModel() {}
}
```

`myai` 当前的 `core/App.go` 就是类似 bootstrap/config，但它现在是全局单例。

后续目标：

```text
core/bootstrap
  Config
  Container
  Wiring
```

`Application` 可以保留，但职责应该变成：

- 加载配置
- 初始化 adapter
- 初始化 usecase
- 启动 transport

它不应该做业务判断。

## 16. Spring Boot 风格的目标目录草案

可以先按这个方向思考，不一定一步到位：

```text
core
├─ bootstrap
│  ├─ config.go
│  └─ container.go
├─ domain
│  ├─ session
│  ├─ message
│  ├─ plan
│  ├─ tool
│  └─ model
├─ application
│  ├─ chat
│  ├─ plan
│  ├─ session
│  ├─ model
│  ├─ context
│  └─ runtime
├─ port
│  ├─ repository
│  ├─ model
│  ├─ tool
│  ├─ skill
│  ├─ asset
│  └─ event
├─ adapter
│  ├─ transport
│  │  ├─ remote
│  │  └─ cli
│  ├─ persistence
│  │  └─ mongo
│  ├─ cache
│  │  └─ redis
│  ├─ llm
│  │  └─ langchaingo
│  ├─ tool
│  │  └─ local
│  ├─ skill
│  │  └─ filesystem
│  └─ asset
│     └─ http
└─ mapper
   ├─ protocol
   ├─ persistence
   └─ llm
```

如果觉得 `mapper` 单独一层太散，也可以放在各 adapter 内部。

例如：

```text
adapter/transport/remote/mapper.go
adapter/persistence/mongo/mapper.go
adapter/llm/langchaingo/mapper.go
```

我更倾向后者，因为 mapper 通常服务于某个 adapter。

## 17. Spring Boot 风格命名约定

后续建议统一命名，不要继续随便叫 `Manager`、`Client`、`Service`。

建议：

```text
UseCase / ApplicationService  业务编排
Repository                    持久化接口
RepositoryImpl                持久化实现
Adapter                       外部系统适配
Mapper                        对象转换
Policy                        判断规则
Builder                       构造对象
Executor                      执行动作
Registry                      注册表
Provider                      提供只读资源
Publisher                     发布事件
Handler                       处理 transport message
Controller                    transport 入口
```

Go 里不一定要真的写 `Impl` 后缀，但语义要清楚。

例如：

```go
type SessionRepository interface {}
type MongoSessionRepository struct {}
```

比下面这种更清楚：

```go
type Store interface {}
type MongoDb struct {}
```

## 18. Spring Boot 风格下的禁止规则

后续应该明确这些规则：

```text
domain 禁止 import adapter
domain 禁止 import protocol
domain 禁止 import mongo / redis
domain 禁止 import langchaingo

application 禁止 import concrete mongo repository
application 禁止 import websocket
application 禁止 import mobile protocol DTO
application 禁止 import viper / cobra

transport 禁止直接操作 mongo
transport 禁止直接拼 prompt
transport 禁止直接改 domain 内部状态

adapter 可以依赖外部 SDK
adapter 不应该承载核心业务规则
bootstrap 可以依赖所有层，但只负责组装
```

这些规则比目录更重要。

目录只是表现，依赖方向才是架构。

## 19. 和 Spring Boot 最大的不同

虽然按 Spring Boot 思路设计，但 Go 项目和 Java 仍然有几个差异：

### 19.1 Go 的接口通常由使用方定义

Java 里经常先定义接口再写实现。

Go 里更常见的是：

```go
application/chat 定义自己需要的 interface
adapter/mongo 去实现它
```

这依然符合你的“接口和实现分离”的目标。

### 19.2 Go 没有 DI 容器

Spring Boot 自动装配 Bean。

Go 需要手动 wiring。

所以 `bootstrap/container.go` 会承担 Spring 容器的部分职责。

### 19.3 Go 不适合过度抽象

接口要按业务边界抽，不要每个 struct 都配一个 interface。

我们要追求的是：

```text
边界清晰
依赖稳定
测试容易
```

不是机械照搬 Java。

## 20. 我建议的第一批 Spring Boot 式重构对象

第一批不要动太大，建议只动这些：

```text
RuntimeInstructionBuilder
ModePolicy
Message domain model
LangChainGoMessageMapper
ChatModelPort
```

原因：

这些正好对应当前最痛的地方：

- Plan/Chat prompt 动态拼接
- LLM SDK 类型污染
- 上下文缓存前缀
- ChatService 过重

第一批做完后，结构会变成：

```text
ChatService
  -> RuntimeInstructionBuilder
  -> ContextBuilder
  -> ChatModelPort
```

然后再逐步拆：

```text
ChatService -> ChatApplicationService
Plan 逻辑 -> PlanApplicationService
Session 逻辑 -> SessionApplicationService
```

## 21. 本轮补充检查发现的遗漏

重新检查主项目后，我认为文档还需要补充这些边界。

### 21.1 asset 模块

当前：

```text
core/asset
core/asset/parser
```

它承担：

- asset 上传
- asset 下载
- uploaded file 解析
- PDF / Python parser 调用

目标归属：

```text
domain/asset
  Asset
  UploadedFile

port/asset
  AssetStorage
  AssetParser

adapter/asset/http
  HttpAssetClient

adapter/asset/parser
  PythonAssetParser
```

注意：

asset 既不是 chat 业务，也不是 protocol DTO。它应该是一个独立能力，由 application 通过 port 使用。

### 21.2 history / checkpoint 模块

当前：

```text
core/history
core/remote/changes
```

它承担：

- SQLite workspace baseline
- checkpoint
- file diff
- revert
- task recorder

目标归属：

```text
domain/history
  Checkpoint
  FileSnapshot
  FileChange

port/history
  WorkspaceHistoryRepository
  FileChangeRecorder

application/history
  HistoryUseCase
  ChangePreviewUseCase

adapter/persistence/sqlite
  SQLiteWorkspaceHistoryRepository

adapter/transport/remote/changes
  ChangesController
```

注意：

`changes` 不是 remote 层的业务，它只是现在被 mobile 通过 remote 调用。真正的业务应该沉到 application/history。

### 21.3 hook / event 模块

当前：

```text
core/hook
```

它承担：

- pre tool hook
- post tool hook
- command hook
- skill reload hook
- session changed hook

目标归属：

```text
domain/event
  DomainEvent

port/event
  EventPublisher
  EventHandler

application/event
  ApplicationEventPublisher

adapter/event/command
  CommandHookHandler
```

注意：

hook 不应该被 `ChatService` 到处直接调用。更合理的是 application 发布事件，由 event adapter 处理。

### 21.4 sandbox 模块

当前：

```text
core/sandbox
```

它承担：

- shell command 执行
- workspace 限制
- stdout/stderr 截断
- destructive command 基础防护

目标归属：

```text
port/sandbox
  Sandbox

adapter/sandbox/local
  LocalSandbox

domain/sandbox
  RunRequest
  RunResult
```

注意：

sandbox 是安全边界，不能只是工具实现细节。后续所有 shell / command 能力都应该经过 sandbox port。

### 21.5 MCP 模块

当前：

```text
core/mcp
```

它承担：

- MCP config
- MCP client
- MCP manager
- MCP tool adapter

目标归属：

```text
adapter/tool/mcp
  MCPClient
  MCPToolAdapter
  MCPRuntimeManager

port/tool
  ToolProvider
```

注意：

MCP 是外部工具来源，不应该让 application 直接知道 MCP 协议细节。application 只应该看到统一 ToolRegistry / ToolExecutor。

### 21.6 skill / skillhub 模块

当前：

```text
core/skill
core/skillhub
skills/
```

它承担：

- 本地 skill 扫描
- skill prompt 选择
- skillhub 安装
- skill reload

目标归属：

```text
domain/skill
  Skill
  SkillTrigger

port/skill
  SkillRepository
  SkillSelector
  SkillInstaller

application/skill
  SkillUseCase
  SkillSelectionService

adapter/skill/filesystem
  FileSystemSkillRepository

adapter/skill/skillhub
  SkillHubInstaller
```

注意：

skill prompt 选择不应该直接塞进 ChatService。它应该由 RuntimeInstructionBuilder 调用 SkillSelector。

### 21.7 files 模块

当前：

```text
core/remote/files
```

它承担：

- workspace 文件列表
- 文件读取
- path resolve
- preview payload

目标归属：

```text
domain/file
  FileEntry
  FilePreview

port/file
  WorkspaceFileReader

application/file
  FileBrowserUseCase

adapter/file/local
  LocalWorkspaceFileReader

adapter/transport/remote/files
  FilesController
```

注意：

文件浏览是业务能力，不应该因为它只给 mobile 用，就放在 remote 包里。

### 21.8 remote/client 模块

当前：

```text
core/remote/client
```

它更像测试/CLI remote client 或未来 SDK。

目标归属：

```text
adapter/transport/remote/client
```

如果它只是调试工具，可以归到：

```text
core/cmd/client
```

后面需要确认它的真实用途。

## 22. 错误模型需要补充

现在很多地方直接返回 `error` 字符串，transport 层再包装成：

```text
protocol.TypeError
```

这会导致前端很难判断：

- 是用户输入错误
- 是权限错误
- 是会话不存在
- 是模型错误
- 是工具错误
- 是系统错误

后续建议引入应用错误模型：

```text
domain/error 或 application/error
  AppError
  ErrorCode
  ErrorKind
```

例如：

```text
SESSION_NOT_FOUND
MODEL_NOT_FOUND
PERMISSION_DENIED
TOOL_EXECUTION_FAILED
VALIDATION_FAILED
REMOTE_AGENT_OFFLINE
INTERNAL_ERROR
```

Transport adapter 再映射成 protocol error payload。

## 23. 协议版本和兼容性需要补充

现在后端 `core/remote/protocol/message.go` 和前端 `mobile/src/protocol.ts` 是手写同步。

风险：

- 后端新增字段，前端不知道
- 前端新增 message type，relay whitelist 漏掉
- DTO 名字一样但字段含义变了

建议：

```text
protocol version
schema source of truth
code generation 或 schema 校验
compatibility policy
```

至少应该先明确：

- message type 变更必须同时更新 Go 和 TS
- relay whitelist 必须有测试覆盖
- protocol DTO 不进入 application 层

## 24. 并发和运行时状态需要补充

现在有多处 runtime state：

- session runtime cancel
- permission channel
- active request id
- request session map
- pending action
- current session cache
- websocket client registry

这些属于运行时控制，不是 domain。

目标：

```text
application/runtime
  RequestRuntime
  SessionRuntime
  PermissionRequestManager

adapter/transport/remote
  WebSocketConnectionRegistry
```

并发规则也要写清楚：

- 同一 session 是否允许并发
- pause/cancel 如何传播
- permission ask 超时如何处理
- websocket 断开后 pending request 如何清理

## 25. 测试分层需要补充

后续重构需要测试策略，否则拆架构会很危险。

建议：

```text
domain test
  不依赖外部系统
  测纯规则

application test
  使用 fake repository / fake model / fake tool
  测业务编排

adapter test
  测 mongo / redis / langchaingo mapper / local tool

transport test
  测 protocol decode / encode / routing / relay forwarding

mobile test
  测 reducer / hook / protocol handler
```

当前已经有一些测试，但更多是在 adapter 或工具层。

最缺的是 application 层测试。

## 26. 配置和启动隔离需要补充

当前 `core/App.go` 直接使用：

- viper
- mongo init
- redis init
- model config load
- tool register
- MCP init
- skill manager
- hook manager
- ChatService bootstrap

后续应该拆成：

```text
bootstrap/config
  AppConfig
  ModelConfig
  MongoConfig
  RedisConfig
  SkillConfig
  MCPConfig

bootstrap/container
  Container
  BuildApplication
```

UseCase 不应该知道 viper。

Adapter 也不应该自己到处读配置。

## 27. 安全边界需要补充

这个项目会执行本地工具、shell、文件读写，所以安全边界很重要。

需要明确：

- workspace path boundary
- shell command policy
- destructive command policy
- file write policy
- asset download policy
- secret redaction
- permission mode
- plan mode readonly policy

这些规则现在散在：

- prompt
- tool permission
- sandbox
- local tool
- agent

后续应该有统一的：

```text
application/security
  PermissionPolicy
  WorkspacePolicy
  ToolExecutionPolicy
```

## 28. 可观测性需要补充

现在系统里有：

- history recorder
- hook
- logs
- token usage
- context info

但还没有统一观测模型。

建议后续定义：

```text
application/telemetry
  TaskTrace
  TokenUsageReporter
  ToolCallTrace
  ModelCallTrace
```

至少需要统一记录：

- request id
- session id
- model id
- selected context tokens
- cacheable prefix hash
- tool calls
- errors
- duration

## 29. 文档当前还没有覆盖的前端重构细节

mobile 目前只粗略说了 feature 分层，但还缺更具体规则。

建议后续补：

```text
mobile/src/features/chat
  components
  hooks
  state
  mapper

mobile/src/features/session
mobile/src/features/plan
mobile/src/features/files
mobile/src/features/changes
mobile/src/features/settings
```

`useRemoteMessageHandler` 应该拆成：

```text
chatMessageHandler
sessionMessageHandler
modelMessageHandler
skillMessageHandler
assetMessageHandler
fileMessageHandler
changesMessageHandler
historyMessageHandler
errorMessageHandler
```

`MobileAppScreen` 应该逐步变成组合容器，而不是所有状态的总控室。

## 30. 目前文档覆盖结论

当前文档已经覆盖：

- Chat / Plan 主业务
- Session
- Context
- LLM
- Tool
- Store
- Remote protocol
- Agent / Relay
- Mobile 大方向
- Spring Boot 类比

本轮补充后，文档也覆盖：

- Asset
- History / Changes
- Hook / Event
- Sandbox
- MCP
- SkillHub
- Files
- Remote Client
- Error Model
- Protocol Versioning
- Runtime Concurrency
- Test Strategy
- Config / Bootstrap
- Security Policy
- Observability
- Mobile feature split

这样才算比较完整地覆盖了 `myai` 主项目的架构面。

## 31. RedisTemplate / Infrastructure Template 思路

可以封装一些常用基础设施操作对象，类似 Spring Boot 里的：

```text
RedisTemplate
StringRedisTemplate
MongoTemplate
RestTemplate / WebClient
JdbcTemplate
```

在 `myai` 中，这类对象可以存在，但它们应该属于 adapter / infrastructure 层，不应该变成业务层到处调用的万能工具类。

### 31.1 Redis 操作对象

当前 Redis 只用于：

```text
current session cache
```

代码在：

```text
core/store/cache
core/store/cache/redisCache
```

后续可以抽成两层：

```text
port/cache
  CacheStore
  CurrentSessionCache

adapter/cache/redis
  RedisTemplate
  RedisCurrentSessionCache
```

其中：

```text
RedisTemplate
  封装 Redis 通用操作

RedisCurrentSessionCache
  封装 myai 当前 session 缓存业务
```

不要让业务直接写：

```go
redisClient.Set(...)
redisClient.Get(...)
```

也不要让业务层到处拼 Redis key。

### 31.2 RedisTemplate 应该提供什么

可以先提供这些常用能力：

```text
SetString(ctx, key, value, ttl)
GetString(ctx, key)
Delete(ctx, key)
Exists(ctx, key)
SetJSON(ctx, key, value, ttl)
GetJSON(ctx, key, out)
SetNX(ctx, key, value, ttl)
Expire(ctx, key, ttl)
TTL(ctx, key)
```

但要注意：

`RedisTemplate` 只封装技术操作，不表达业务语义。

业务语义应该放在更具体的 cache adapter 里。

例如：

```go
type CurrentSessionCache interface {
    SetCurrentSession(ctx context.Context, userID string, sessionID string, ttl time.Duration) error
    GetCurrentSession(ctx context.Context, userID string) (string, error)
    DeleteCurrentSession(ctx context.Context, userID string) error
}
```

它的实现可以使用 `RedisTemplate`。

### 31.3 Key 管理

Redis key 不应该散落在业务代码里。

建议统一：

```text
adapter/cache/redis/keys.go
```

例如：

```go
func CurrentSessionKey(userID string) string {
    return "myai:current_session:" + userID
}
```

更进一步可以定义：

```text
KeyBuilder
KeyNamespace
```

保证所有 key 有统一前缀、统一命名规则、统一 TTL 策略。

### 31.4 不限于 Redis

这个思路不仅适用于 Redis。

其他基础设施也可以有 template / helper：

```text
MongoTemplate
  封装 collection、upsert、pagination、soft delete 等通用操作

HTTPClientTemplate
  封装 timeout、retry、json decode、error handling

FileSystemTemplate
  封装 workspace path resolve、safe read、safe write

JSONTemplate
  封装 marshal / unmarshal / strict decode

CommandTemplate
  封装 command run、timeout、stdout/stderr limit
```

但原则一样：

```text
Template 是基础设施复用工具
Repository / Adapter 才表达业务语义
Application 不直接依赖 Template
```

### 31.5 防止 utils 膨胀

不能建一个类似：

```text
core/utils
```

然后什么都往里面丢。

更好的方式：

```text
adapter/cache/redis/RedisTemplate
adapter/persistence/mongo/MongoTemplate
adapter/http/HTTPClient
adapter/file/FileSystemTemplate
adapter/command/CommandRunner
```

也就是说：

工具对象要靠近它服务的基础设施，而不是放在全局 utils 里。

### 31.6 Spring Boot 类比

Spring Boot 里通常不会在业务 service 里直接使用底层连接：

```java
RedisConnection
MongoClient
HttpURLConnection
```

而是通过：

```java
RedisTemplate
MongoTemplate
WebClient
Repository
```

`myai` 后续也应该类似：

```text
application service
  -> port interface
  -> adapter implementation
  -> template/client
  -> concrete SDK
```

例如：

```text
ChatApplicationService
  -> CurrentSessionCache
  -> RedisCurrentSessionCache
  -> RedisTemplate
  -> go-redis Client
```

这样既能复用 Redis 常用操作，又不会让 Redis SDK 污染业务层。

## 32. 第一阶段已落地内容

本轮先落地 `application/runtime`，不做全项目大搬迁。
目标是把 Plan / Chat 模式相关的运行时规则从 `ChatService` 中拆出来，先建立清晰边界。

新增包：
```text
core/application/runtime
```

当前包含：
```text
SkillPromptProvider
  只定义 skill prompt 提供接口

ModePolicy
  判断 Plan / Chat 模式
  判断 Plan 模式下工具权限

RuntimeInstructionBuilder
  根据 AgentMode、ForceChatMode、SkillPrompt 构建本轮 runtime instruction

InsertRuntimeInstructions
  将 runtime instruction 插入到最新用户消息前
  保持稳定 system prompt 前缀不被打乱
```

`ChatService` 当前只保留薄入口：
```text
agentPrompt(...)
messagesWithRuntimePrompt(...)
llmToolsForSession(...)
```

这些方法后续还可以继续移动到更明确的 usecase / context builder 中。
但第一阶段先保证：
- 外部行为不变
- Plan 模式提示词不再散落在 `ChatService`
- Plan 模式工具权限规则不再写死在 `ChatService`
- runtime prompt 继续放在最新用户消息前，避免破坏缓存前缀

本轮验证：
```text
go test ./core/application/runtime ./core/service ./core/plan ./core/remote/relay
```

后续第二阶段建议：
```text
core/domain/message
adapter/llm/langchaingo/MessageMapper
application/context/ContextBuilder
```

也就是先处理 `llms.MessageContent` 对 session/context/application 的污染。

## 33. 第二阶段已落地内容

本轮落地 `domain/message` 和 LLM message mapper。
目标是让 session/context 不再直接依赖 `langchaingo/llms.MessageContent`。

新增包：
```text
core/domain/message
core/adapter/llm/langchaingo
```

当前边界：
```text
domain/message
  Message
  Role
  Part
  ToolCall
  ToolResult

adapter/llm/langchaingo
  domain.Message <-> llms.MessageContent
```

已经完成：
- `core/session.Session.Messages` 改为 `[]domainmessage.Message`
- `core/contextmgr` 改为基于 domain message 做窗口裁剪、摘要前缀和 cache hash
- `core/application/runtime.InsertRuntimeInstructions` 改为接收 domain message
- `ChatService` 只在调用 LLM 前使用 mapper 转成 `llms.MessageContent`
- store 里的 `MessageRecord` 仍然作为 persistence PO 保持独立

当前 `llms.MessageContent` 允许出现的位置：
```text
core/llm
core/adapter/llm/langchaingo
ChatService 调用模型的临时边界
```

本轮验证：
```text
go test ./core/...
```

## 98. 第二阶段：包级目录隔离

本阶段不再只依靠文件名区分接口、命令、结果和实现，而是使用 Go package 目录直接表达职责。

统一模板：

```text
core/application/<module>/
  api/       # 入站用例接口，类似 Java Service interface
  command/   # 输入 DTO / Command，只允许数据对象
  result/    # 输出 VO / Result，只允许数据对象
  port/      # 出站接口，类似 Repository/Gateway interface
  service/   # 用例实现，类似 Service/impl
```

约束：
- `api` 和 `port` 不声明 struct。
- `command` 和 `result` 不声明 interface。
- `service` 不声明 interface，只实现 `api` 或 `port` 中的契约。
- composition 负责构造实现对象，外层业务 facade 依赖 `api` 接口。
- Go 目录就是 package，不能照搬 Java 的包关系制造循环依赖。

## 99. model 包级模板已完成

```text
core/application/model/
  api/
  command/
  result/
  port/
  service/
```

已完成：
- `ConfigService`、`BootstrapService`、`QueryService` 实现迁入 `service`。
- 调用方依赖 `api`，组合根创建具体 `service`。
- model 根目录不再放生产声明文件。

## 100. session 包级重构已完成

```text
core/application/session/
  bootstrap/{api,command,port,result,service}
  current/{api,port,result,service}
  lifecycle/{api,command,port,result,service}
  load/{api,command,port,service}
  message/{api,command,port,result,service}
  persistence/{api,command,port,service}
  query/{api,command,port,service}
  settings/{api,command,port,service}
  command/
  result/
  port/
```

已完成：
- session 生产根包只保留 `doc.go`。
- `ChatDependencies` 对生命周期、加载、消息、查询、设置、当前状态和启动服务均依赖接口。
- adapter 和 remote 直接依赖明确的 command/result/api package，不再依赖聚合根包。
- 旧根包测试使用 `_test.go` 兼容声明，兼容代码不会进入生产构建。

## 101. tool 包级重构已完成

```text
core/application/tool/
  api/
  command/
  result/
  port/
  service/
```

已完成：
- execution、permission、selection 实现迁入 `service`。
- registry、hook、asset extractor、catalog、mode policy 迁入 `port`。
- executor/catalog adapter 直接依赖明确子包。
- tool 生产根包只保留 `doc.go`。

## 102. chat 包级重构已完成

已完成：

```text
core/application/chat/
  context/{api,port,service}
  compaction/{api,command,port,result,service}
  generation/{api,command,port,result,service}
  plan/{api,command,port,result,service}
  port/
```

已完成：
- Agent Loop、Assistant Generation、Generation Task 和 Response Commit 迁入 `generation/service`。
- Plan Execution 迁入 `plan/service`。
- 工具执行、用户消息持久化、任务记录、生成结果迁入明确的 command/result/port。
- `ChatDependencies` 依赖 application `api` 接口，不再持有具体应用服务。
- chat 生产根包只保留 `doc.go`。

## 103. 新增架构守卫

`core/architecture/dependency_rules_test.go` 新增：
- package role 声明类型检查。
- chat/model/session/tool 模块根目录容器检查。
- Mongo BSON 标签只能位于 `po` 包。
- Mongo 根目录不得重新出现 `document.go`、`mapper.go`、`store.go` 混合文件。
- 已有 inward dependency、序列化 tag、legacy package 规则继续保留。

## 104. Mongo 持久化目录隔离已完成

主存储：

```text
core/adapter/persistence/mongo/
  template/    # 通用 MongoTemplate 与 Operations
  po/          # BSON persistence object
  mapper/      # PO 与 domain/repository record 转换
  repository/  # Store 仓储实现
```

授权存储：

```text
core/adapter/persistence/mongo/authorization/
  po/
  mapper/
  repository/
```

已完成：
- BSON tag 只存在于 `po` package。
- Mapper 不再和 Store、PO 位于同一 package。
- Repository 通过 `mongo/template.Operations` 访问数据库。
- Mongo 与 authorization 根包只保留兼容工厂，现有调用方 API 不变。

## 105. 第三阶段：剩余模块统一整理完成

应用模块：

```text
core/application/plan/
  command/
  service/

core/application/runtime/
  command/
  port/
  service/

core/application/skill/
  api/
  port/
  query/
  result/
  service/
```

已完成：
- plan 的状态命令和无状态应用服务完成物理隔离。
- runtime 的指令命令、skill prompt 端口和运行时实现完成物理隔离。
- skill 的入站 API、Catalog 端口、Query、Result 和实现完成物理隔离。
- plan/runtime/skill 生产根包只保留 `doc.go`。

小型持久化适配器：

```text
core/adapter/persistence/chatmessage/
  mapper/
  port/
  repository/

core/adapter/persistence/toolrecords/
  mapper/
  port/
  repository/

core/adapter/persistence/sqlite/history/
  repository/
```

已完成：
- chatmessage Writer 与 Mapper、Port 分离。
- toolrecords Recorder 与 Mapper、Port 分离。
- SQLite history Store/Factory 迁入 repository，根包保留兼容入口。
- 拆包后测试不再跨 package 共享私有 fake/stub。
- 架构守卫覆盖全部 application 模块根目录和结构化 persistence adapter 根目录。

本阶段验证：

```text
go test ./...
go test ./core/application/tool/... ./core/adapter/tool/...
go test ./core/application/chat/... ./core/composition/chat ./core/service
git diff --check
```

## 66. 第三十五阶段已落地内容
本轮继续减少 `ChatService` 对持久化 store 的直接操作，把 clear current 时的消息清空并入 session 生命周期应用服务。

已经完成：
- `LifecycleService.Messages` 从单纯 `MessageRecordLister` 扩展为 `LifecycleMessageRepository`。
- `LifecycleMessageRepository` 明确包含 `ListMessages` 与 `ClearMessages` 两个消息仓库能力。
- `LifecycleService.ClearCurrent` 现在负责：
  - 获取当前内存 session。
  - 清空该 session 的持久化 messages。
  - 清空内存 session。
  - 返回清空后的 session。
- `ChatService.ClearCurrent` 不再直接调用 `store.ClearMessages`。
- `ChatService.ClearCurrent` 继续保留 facade side effect：保存 session 记录和发布 hook。

当前效果：
```text
service.ChatService
  -> ClearCurrent facade
  -> save session + hook

application/session.LifecycleService
  -> clear current use case
  -> message repository port 清空持久化消息
  -> memory port 清空内存会话
```

新增测试覆盖：
- `LifecycleService.ClearCurrent` 会调用 message repository 清空当前 session 的持久化消息。
- 清空后内存 session 会重置 summary 和 messages。

本轮验证：
```text
go test ./core/application/session ./core/service
go test ./core/...
```

## 68. 第三十七阶段已落地内容

本阶段把“追加用户消息”和“准备重新生成”从 `ChatService` 迁移到 `application/session`，由会话应用服务统一维护内存会话消息状态。

新增契约和实现：
```text
core/application/session
  message_command_contracts.go
    MessageSessionLoader
    MessageCommandMemory
    AppendUserMessageCommand
    PrepareRegenerationCommand
    MessageCommandResult

  message_command_service.go
    MessageCommandService
```

已经完成：
- `AppendUserMessage` 负责输入校验、session id 归一化、会话加载、用户消息追加和更新后 session 返回。
- `PrepareRegeneration` 负责加载会话、裁剪最后一条用户消息之后的内容、返回重新生成输入和更新后 session。
- `ChatService.SendMessageStreamForSession` 不再直接调用 `SessionManage.AddUserMessageTo`。
- `ChatService.RegenerateLastMessageStreamForSession` 不再直接调用 `TrimAfterLastUserMessage` 和 `GetSession`。
- plan step 执行时的用户消息追加复用同一个 message command service。

## 69. 第三十八阶段已落地内容

本阶段把当前会话的只读状态查询从 `ChatService` 收拢到独立 query service，统一默认值、规范化和 plan clone 规则。

新增契约、DTO 和实现：
```text
core/application/session
  current_state_contracts.go
    CurrentStateMemory
    CurrentState

  current_state_query_service.go
    CurrentStateQueryService
```

已经完成：
- `CurrentState` 只作为 query result，不承担持久化对象或 domain entity 身份。
- `CurrentStateQueryService.State` 统一返回 session/model/mode/context/usage/plan 快照。
- plan 查询结果使用 clone，避免协议层修改内存聚合。
- `ChatService.CurrentSessionID`、`CurrentModelID`、`CurrentPermissionMode`、`CurrentAgentMode`、`CurrentPlan`、`CurrentContextWindowK`、`CurrentUsage`、`CurrentLastUsage` 全部改为委托 query service。
- 对 nil session manager 做显式装配保护，避免 Go interface 持有 nil pointer 后出现伪非空问题。

## 70. 第三十九阶段已落地内容

本阶段建立 repository 级“未找到”语义，并把 session record 的组装、已有记录合并和保存流程迁移到 application service。

新增端口错误：
```text
core/port/repository/errors.go
  ErrNotFound
```

新增契约和实现：
```text
core/application/session
  persistence_service_contracts.go
    SessionSnapshotMemory
    SessionPersistenceRepository
    SaveSessionCommand

  persistence_service.go
    SessionPersistenceService
```

已经完成：
- Mongo adapter 将 `mongo.ErrNoDocuments` 翻译为 `repository.ErrNotFound`。
- application/service 层不再依赖 Mongo driver 的具体异常类型。
- `SessionPersistenceService.Save` 负责读取内存快照或已有记录并构造 session record。
- `SessionPersistenceService.SaveRecord` 负责保留创建时间和已有业务标题、补齐默认值并保存。
- `ChatService.saveSession` 和 `saveSessionRecord` 只保留 facade 委托。
- `ChatService` 不再直接调用 `store.GetSession` 或 `store.SaveSession`。

## 71. 第四十阶段已落地内容

本阶段把手动压缩会话的完整用例迁移到 `application/chat`，统一编排 session 加载、model 解析、压缩执行和 context result 返回。

新增契约、Command 和实现：
```text
core/application/chat
  session_compaction_contracts.go
    SessionLoader
    SessionCompactor
    ContextInfoQuery
    CompactSessionCommand

  session_compaction_service.go
    SessionCompactionService
```

已经完成：
- `LoadService.Load` 提供明确的非 current session 加载入口，并满足 chat application contract。
- `SessionCompactionService.Compact` 负责 session id 校验、session 加载、model lookup、压缩执行和 context query。
- 历史不足时保持原有行为：返回当前 context info 和 `ErrNotEnoughHistoryToCompact`。
- `ChatService.CompactSession` 只负责构造 Command 并委托 application service。
- `ChatService` 不再直接调用 model registry 的 `GetModel`。

本轮新增测试覆盖：
- 追加用户消息并保持原始输入内容。
- 拒绝空用户输入。
- 重新生成时裁剪 assistant 消息并重置 last usage。
- 当前状态默认值、规范化和 plan clone。
- session 持久化使用内存状态、保留已有标题和创建时间、传播 repository 错误。
- 手动压缩的正常编排、历史不足和模型不存在分支。

当前 `ChatService` 直接基础依赖残留：
```text
Bootstrap
  -> 启动时检查 SessionManage 当前状态

ListModels
  -> model registry 只读列表
```

下一阶段建议：
```text
application/session
  提取 BootstrapSessionService，统一 cached/current/new/load 决策

application/model
  增加 ModelQueryService，收拢 model list 查询

service
  继续审计 skill facade 和异步任务装配职责
```

本轮验证：
```text
go test ./core/application/chat
go test ./core/application/session
go test ./core/adapter/persistence/mongo
go test ./core/service
go test ./core/...
```

## 72. 第四十一阶段已落地内容

本阶段把应用启动时的 session 决策流迁移到 application/session，不再由 ChatService 直接读取 session manager 和 cache 状态。

新增契约、Command、Result 和实现：
~~~text
core/application/session
  bootstrap_contracts.go
    BootstrapSessionCache
    BootstrapSessionLifecycle
    BootstrapSessionState
    BootstrapSessionPersistence
    BootstrapSessionCommand
    BootstrapSessionResult
    BootstrapSessionAction

  bootstrap_service.go
    BootstrapSessionService
~~~

已经完成：
- cached session 存在时加载并刷新 current session cache。
- cached session 已失效时使用统一的 repository.ErrNotFound 判断并继续 fallback。
- 无可复用 session 时创建、持久化并缓存新 session。
- 已有内存 session 时保持原有行为，只补持久化。
- ChatService.Bootstrap 只根据 Result 发布 load/new hook。
- ChatService 不再直接调用 SessionManage.CurrentSessionId 或 Current。

## 73. 第四十二阶段已落地内容

本阶段补齐 model 和 skill 的 query application service，避免 facade 直接读取具体 manager。

新增 model query：
~~~text
core/application/model
  query_contracts.go
    ModelCatalog
    ListModelsResult

  query_service.go
    QueryService
~~~

新增 skill query：
~~~text
core/application/skill
  catalog_contracts.go
    Catalog
    ListSkillsQuery
    ListSkillsResult

  catalog_service.go
    CatalogService
~~~

已经完成：
- model list result 持有独立 slice，不暴露 registry 内部切片。
- skill list 的 refresh 语义由 ListSkillsQuery 明确表达。
- skill root 查询和列表刷新从 ChatService 迁移到 application service。
- ChatService.ListModels、ListSkills、SkillRoot 只保留 facade 委托。
- ReloadSkills 继续由 facade 发布 reload hook，application skill service 不依赖 hook adapter。

## 74. 第四十三阶段已落地内容

本阶段隔离异步任务端口，ChatService 不再依赖具体 utills.ThreadPool。

新增端口：
~~~text
core/port/async
  Executor
~~~

新增 application service：
~~~text
core/application/runtime
  AsyncTaskService
~~~

新增 adapter：
~~~text
core/adapter/async/threadpool
  Executor
~~~

已经完成：
- AsyncTaskService.Submit 统一处理 nil task、executor 提交和失败 fallback。
- 默认 fallback 保持原行为：使用 goroutine 执行任务。
- thread pool adapter 负责把 func() 转换为 utills.Task，端口层不依赖工具包。
- NewChatService 构造参数改为 async.Executor。
- Application 在 composition root 中注入 thread pool adapter。
- ChatService.runAsync 只委托 AsyncTaskService。

本轮新增测试覆盖：
- cached session 加载、stale cache 后创建、内存 session 复用。
- model query result slice 隔离。
- skill refresh、result slice 隔离和错误传播。
- async executor 正常提交、失败 fallback、nil task。
- thread pool adapter 正常执行和 nil pool 错误。

当前 ChatService 基础依赖访问状态：
~~~text
直接调用 SessionManage 方法：0
直接调用 repository Store 方法：0
直接调用 model registry 方法：0
直接调用 skill Manager 方法：0
直接调用 concrete ThreadPool 方法：0
~~~

下一阶段审计方向：
~~~text
core/Application
  检查 composition root 是否仍混入配置转换和基础设施创建逻辑

core/session
  评估 SessionManage 是否需要拆分为 repository-style memory adapter 与 domain aggregate

core/remote/agent
  检查协议映射、command dispatch 和业务调用是否仍集中在单一对象
~~~

本轮验证：
~~~text
go test ./core/application/session
go test ./core/application/model
go test ./core/application/skill
go test ./core/application/runtime
go test ./core/adapter/async/threadpool
go test ./core/service ./core
go test ./core/...
~~~

## 75. 第四十四阶段已落地内容

本阶段开始重构 remote agent。审计确认 core/remote/agent/agent.go 是当前最大的高耦合对象，同时承担连接、路由、协议映射、会话运行时、权限等待和业务调用。

新增配置 DTO：
~~~text
core/remote/agent/config.go
  Config
~~~

新增按能力分组的 facade 接口：
~~~text
core/remote/agent/chat_facade.go
  ChatGenerationFacade
  SessionLifecycleFacade
  SessionQueryFacade
  SessionSettingsFacade
  CurrentSessionFacade
  CatalogFacade
  ChatFacade
~~~

新增运行时实现：
~~~text
core/remote/agent/session_runtime.go
  sessionRuntime
  sessionRuntimeManager
~~~

新增权限等待实现：
~~~text
core/remote/agent/permission_waiters.go
  permissionWaiterRegistry
~~~

已经完成：
- Agent 构造器不再依赖具体的 service.ChatService 指针，而是依赖 ChatFacade 组合接口。
- 大接口按 generation、lifecycle、query、settings、current state、catalog 六类能力拆分，组合接口只用于 composition。
- Config 与 Agent 实现分文件，配置 DTO 不再混在实现文件中。
- per-session 运行锁、取消和暂停状态迁移到独立 runtime 对象。
- permission waiter 的注册、解析和移除迁移到独立 registry。
- Agent 本身不再直接持有 permission map 和对应互斥锁。
- agent.go 从约 1598 行降到 1483 行；下一阶段继续拆 handler 与 mapper。

新增测试覆盖：
- 同 session runtime 复用、不同 session 隔离、默认 session 稳定映射。
- 同 session 并发运行拒绝、pause 取消、finish 后可再次启动。
- permission waiter resolve/unregister。
- 同 request id replacement 不会被旧 waiter 的 unregister 删除。

下一阶段：
~~~text
core/remote/agent
  protocol mapper 独立文件
  session/catalog handler 独立文件
  chat streaming handler 独立文件
  file/change service 改为接口依赖
~~~

本轮验证：
~~~text
go test ./core/remote/agent
go test ./core/cmd ./core
go test ./core/...
~~~

## 76. 第四十五阶段已落地内容

本阶段继续拆分 remote agent，把协议映射和不同业务域的 handlers 从 websocket transport 主文件中迁出。

新增 workspace facade 接口：
~~~text
core/remote/agent/workspace_facade.go
  WorkspaceFileFacade
  WorkspaceChangeFacade
~~~

新增协议 mapper：
~~~text
core/remote/agent/payload_mapper.go
  context / compact / session / history / usage / plan
  model / skill / asset protocol mapping
~~~

新增 handler 文件：
~~~text
catalog_handlers.go
workspace_handlers.go
session_query_handlers.go
session_settings_handlers.go
chat_handlers.go
permission_handlers.go
~~~

已经完成：
- Agent 的 fileService 和 changeService 字段改为接口依赖。
- repository record、domain plan、token usage 到 protocol DTO 的转换集中到 mapper 文件。
- workspace、catalog、session query、session settings、chat streaming、permission handler 按职责分文件。
- agent.go 只保留 websocket transport、message router 和 per-session 异步运行调度。
- agent.go 从上一阶段的 1483 行进一步下降到 359 行。
- 没有引入额外 router framework，原 switch dispatch 行为保持不变。

新增测试覆盖：
- session id 解析优先级。
- session record 到 protocol summary 映射。
- plan 和 usage 映射。
- history count、last id 和 version 对比。

## 77. 第四十六阶段已落地内容

本阶段修正 session 领域包中混入具体内存实现的问题，把 SessionManage 迁移到 adapter 层。

领域层新增构造值对象：
~~~text
core/session/initial_state.go
  InitialState
  NewFromState
~~~

新增内存 adapter：
~~~text
core/adapter/session/memory/store.go
  Store
  NewStore
~~~

已经完成：
- core/session 只保留 Session 聚合、模式值对象、领域行为和构造状态。
- 线程锁、session map、current session 指针和内存 CRUD 操作迁移到 memory adapter。
- 具体实现由 SessionManage 重命名为 memory.Store。
- Application composition root 字段改为 sessionMemory。
- ChatService、plan state adapter 和测试改为引用 memory.Store。
- application/session 继续通过已有的小接口使用内存存储，不依赖具体实现。
- InitSessionManage/GetSessionManage 改为 InitSessionMemory/GetSessionMemory。

新增 adapter 测试覆盖：
- 创建并更新当前会话。
- hydrate 非当前会话时不改变 current session。
- plan 写入时 clone，避免外部修改内存状态。

当前分层：
~~~text
core/session
  Domain aggregate and value objects

core/application/session
  Use cases, commands, results and contracts

core/adapter/session/memory
  In-memory implementation
~~~

下一阶段：
~~~text
core/Application
  提取配置 DTO 和配置 mapper

core/adapter/persistence/mongo
  审计通用 collection/update/query helper

core/adapter/cache/redis
  审计 hash/zset/string 等可复用操作边界
~~~

本轮验证：
~~~text
go test ./core/remote/agent
go test ./core/adapter/session/memory
go test ./core/application/session
go test ./core/service ./core
go test ./core/...
~~~

## 78. 第四十七阶段已落地内容

本阶段把 Application 中散落的 Viper 配置读取和对象转换迁移到独立 config 层。

新增配置 DTO：
~~~text
core/config/properties.go
  Properties
  ModelProperties
  MongoProperties
  RedisProperties
  ThreadProperties
  AssetProperties
  SkillProperties
  HookProperties
  CommandHookProperties
  MCPProperties
  MCPServerProperties
~~~

新增 loader 和 mapper：
~~~text
core/config/loader.go
  ViperLoader

core/config/mapper.go
  Mapper
~~~

职责边界：
- Properties 只表示外部配置 DTO，不承担 runtime Config 或 repository PO 身份。
- ViperLoader 只负责文件、环境变量、默认值和 workspace 路径归一化。
- Mapper 负责把 Properties 转换为 ModelConfig PO、asset Config、hook Config 和 MCP Config。
- Application 只保存一次加载后的 Properties，并在 composition root 中装配组件。

已经完成：
- Application 不再散落调用 viper.GetString/GetInt/GetInt64/UnmarshalKey。
- 删除 GetViper 和 modelConfigFromViper。
- relay 命令改用 ViperLoader.LoadOptional。
- Viper 依赖只存在于 core/config adapter。
- MCP runtime Config 删除 mapstructure 标签和 Viper loader，改为独立 NormalizeConfig。
- 支持 MYAI_MODEL、MYAI_API_KEY、MONGO_URI、REDIS_ADDR 等环境变量覆盖。

安全审计：
- 当前 resource/application.yaml 存在明文 API key、Mongo 和 Redis 凭据。
- 本阶段没有擅自替换部署凭据，但已经提供环境变量覆盖能力。
- 后续必须把真实凭据移出版本库并执行轮换，历史提交中的凭据也应视为已泄露。

## 79. 第四十八阶段已落地内容

本阶段扩展 Redis Template，按能力拆分常用操作接口，避免业务代码直接依赖 go-redis 命令对象。

新增接口：
~~~text
StringOperations
HashOperations
SortedSetOperations
KeyOperations
Operations
~~~

新增 DTO：
~~~text
SortedSetMember
ScoreRange
~~~

新增实现：
~~~text
hash_operations.go
  HashSet / HashGet / HashGetAll / HashDelete
  HashSetJSON / HashGetJSON

sorted_set_operations.go
  SortedSetAdd / SortedSetRange
  SortedSetRangeByScore / SortedSetRemove / SortedSetScore
~~~

设计约束：
- 不向 application 层泄漏 redis.Nil。
- HashGet/SortedSetScore 使用 found bool 明确表达数据不存在。
- 空批量操作直接返回，不发送无意义命令。
- Template 继续保留 String/JSON/NX/TTL 等已有通用操作。
- 没有引入强制链式 DSL，保持调用简单。

## 80. 第四十九阶段已落地内容

本阶段为 Mongo adapter 增加通用 Operations/Template，同时保留 repository Store 的业务语义方法。

新增接口与实现：
~~~text
core/adapter/persistence/mongo
  Operations
  Template

Template operations
  FindOne
  FindAll
  UpdateOne
  InsertOne
  DeleteMany
  Count
~~~

已经完成：
- Mongo database/collection 校验集中到 Template。
- cursor close 和 decode 集中到 FindAll。
- mongo.ErrNoDocuments 集中翻译为 repository.ErrNotFound。
- Store 支持 NewWithTemplate 注入，便于隔离测试。
- Store 不再直接调用 db.Collection 或维护 verifyDB。
- repository 方法仍保留 Session/Message/Asset/Model 明确语义，不向 application 暴露 BSON filter。
- Mongo Store 从 382 行降到约 315 行。

清理旧架构：
~~~text
删除 core/store/cache alias
删除 core/store/cache/redisCache alias
删除 core/store/data alias
删除 core/store/data/mongoDb alias
~~~

这些包已无人引用，只会造成新旧架构并存的误解。

本轮新增测试覆盖：
- Properties 默认值、路径归一化和环境变量覆盖。
- Properties 到 model/asset/hook/MCP 对象映射及嵌套集合隔离。
- Redis Operations 接口实现和 nil client 行为。
- Hash/ZSet DTO 映射。
- Mongo Template nil database 和 not-found 翻译。
- Mongo Store collection 选择、session read/list 和 message insert 委托。

本轮验证：
~~~text
go test ./core/config
go test ./core/adapter/cache/redis
go test ./core/adapter/persistence/mongo
go test ./core/cmd ./core
go test ./core/...
~~~

下一阶段：
~~~text
core/service/chat.go
  继续将 application service 装配移到 composition root，减少每次调用时临时构造对象

core/port/repository
  审计 Record/PO 命名和 DTO 边界

core/history
  审计 recorder/store 大对象与 SQLite adapter 边界
~~~

## 81. 第五十阶段已落地内容

本阶段完成 History 模块的领域对象、存储端口和 SQLite 适配器分层，并进一步拆分原本过大的 recorder 实现。

分层结构：
```text
core/domain/history
  FileSnapshot
  Checkpoint
  CheckpointSummary
  FileChange
  StoredFileChange

core/port/history
  Store
  StoreFactory

core/history
  Recorder
  TaskRecorder
  RecordCommand
  snapshot service
  path policy

core/adapter/persistence/sqlite/history
  Store
  Factory

core/adapter/history/taskrecorder
  Factory
```

已经完成：
- History 领域数据对象从 SQLite store 文件迁入 `core/domain/history`。
- `core/port/history.Store` 只声明存储能力，`StoreFactory` 只声明实例创建和默认路径能力。
- SQLite 的表结构、SQL、序列化和连接管理全部收敛到 persistence adapter。
- `Recorder` 和 `TaskRecorder` 只依赖 history port，不再依赖 SQLite 具体类型。
- local tools、remote changes 和 task recorder adapter 通过 Factory 或 Store 注入具体实现。
- 原 `RecordOptions` 改名为 `RecordCommand`，明确其应用命令语义。
- 原 700 余行 `recorder.go` 按职责拆成 `recorder.go`、`task_recorder.go`、`snapshot_service.go`、`path_policy.go` 和 `record_command.go`。
- `recorder.go` 只负责单次文件变化记录，`task_recorder.go` 负责任务级聚合和 context 传播。
- 文件读取、hash、二进制识别和工作区扫描集中在 snapshot service。
- workspace 归一化、越界检查、忽略目录和敏感文件规则集中在 path policy。

依赖方向：
```text
tool / remote / application adapter
  -> core/history
  -> core/port/history

core/adapter/persistence/sqlite/history
  -> core/port/history
  -> core/domain/history
```

测试覆盖：
- Recorder 通过 fake Store 保存 checkpoint，不依赖 SQLite。
- TaskRecorder 通过 fake StoreFactory 创建和聚合 workspace 变更。
- SQLite adapter 的 checkpoint、baseline 和文件变化持久化行为。
- local write/edit/shell 与 history recorder 的集成行为。
- remote changes 对 SQLite adapter 的注入和查询行为。

本轮验证：
```text
go test ./core/history ./core/adapter/persistence/sqlite/history ./core/adapter/history/taskrecorder ./core/tool/local ./core/remote/changes
go test ./core/...
```

下一阶段：
```text
core/remote/changes
  拆分约 900 行的 service，区分查询、恢复、diff 和 workspace 文件操作

core/port/repository
  继续审计 Record/PO 命名和 DTO 边界

core/service
  将 application service 装配逐步迁到 composition root
```

## 82. 第五十一阶段已落地内容

本阶段拆分 `core/remote/changes` 的大对象，并补齐 History Store 的构造注入边界。对外协议方法和行为保持不变。

拆分后的职责：
```text
service.go
  Service 状态
  构造和关闭
  baseline 加载与重置

change_query.go
  List
  Diff

history_query.go
  History
  HistoryDiff
  checkpoint diff 映射

revert_service.go
  Revert
  RevertCheckpoint
  workspace 文件恢复

snapshot_service.go
  workspace 扫描
  文件 snapshot
  domain snapshot 映射

diff_service.go
  文本 diff
  大文件截断策略

path_policy.go
  workspace 路径约束
  忽略目录和敏感文件规则
```

新增构造边界：
```text
NewWithStoreFactory(root, historyPath, StoreFactory)
NewWithStore(root, Store)
```

已经完成：
- 原 `service.go` 从约 900 行降到约 140 行，只保留装配和 baseline 生命周期。
- query、history、revert、snapshot、diff 和 path policy 分开维护。
- 默认构造器仍使用 SQLite Factory，保持现有调用方兼容。
- 测试和上层 composition root 可以注入 `historyport.Store` 或 `historyport.StoreFactory`。
- `remote/changes` 的业务实现不再只能自行创建 SQLite store。
- 新增 fake Store/StoreFactory 测试，验证 baseline 加载、factory 委托和资源关闭。

本轮验证：
```text
go test ./core/remote/changes
go test ./core/...
```

下一阶段：
```text
core/port/repository
  审计 Record、PO 和查询结果对象的命名及文件边界

core/remote/changes
  后续可将 protocol DTO 映射迁出核心用例，避免业务逻辑直接依赖 transport 对象

core/service
  将 application service 装配迁到 composition root
```

## 83. 第五十二阶段已落地内容

本阶段隔离 repository port 对象与 Mongo BSON Document，避免端口 Record 和领域 Plan 携带具体数据库元数据。

新增 Mongo PO：
```text
core/adapter/persistence/mongo/document.go
  sessionDocument
  messageDocument
  assetDocument
  modelConfigDocument
  tokenUsageDocument
  planDocument
  planStepDocument
```

新增 Mapper：
```text
core/adapter/persistence/mongo/mapper.go
  repository Record <-> Mongo Document
  plan domain <-> Mongo plan Document
```

已经完成：
- `SessionRecord`、`MessageRecord`、`AssetRecord`、`ModelConfig`、`TokenUsageRecord` 移除全部 `bson` 标签。
- `plan.Plan` 和 `plan.Step` 移除全部 `bson` 标签。
- Mongo Store 查询先解码为 adapter Document，再映射为 repository Record。
- Mongo Store 插入先把 repository Record 映射为 adapter Document。
- Session 保存时的嵌套 usage、last usage 和 current plan 全部使用 Mongo Document。
- `_id`、snake_case 字段名和 `omitempty` 策略只存在于 Mongo adapter。
- repository port 不再依赖 Mongo 字段命名约定。

对象边界：
```text
application
  -> repository Record
  -> repository interface

mongo Store
  -> mapper
  -> Mongo Document
  -> Template / driver
```

新增测试覆盖：
- Session Record 与 Document 往返，包括嵌套 Plan、Step 和 TokenUsage。
- Message、Asset、ModelConfig Record 与 Document 往返。
- Store fake template 测试改为验证 adapter Document，不再假设 repository Record 可直接 BSON 持久化。

本轮验证：
```text
go test ./core/adapter/persistence/mongo ./core/plan ./core/port/repository
go test ./core/...
```

下一阶段：
```text
core/remote/relay
  隔离 auth 业务对象与 Mongo/JSON 双重序列化对象

core/port/repository
  评估 JSON 标签是否仍有真实调用方；无调用方时继续迁出 transport 元数据

core/service
  将 application service 装配迁到 composition root
```

## 84. 第五十三阶段已落地内容

本阶段拆分 Relay Auth 职责，并隔离认证业务对象、HTTP DTO 与 Mongo Document。

拆分结构：
```text
client_authorization.go
  ClientAuthorization 业务对象
  active 判断和排序规则

auth_store.go
  AuthStore 接口
  not-found 错误

memory_auth_store.go
  MemoryAuthStore 实现

auth_service.go
  token 创建和 hash
  authorize / validate / list / revoke 用例

authorization.go
  HTTP request / response DTO
  domain -> response mapper

mongo_auth.go
  authorizationDocument
  domain <-> Mongo Document mapper
  MongoAuthStore
```

已经完成：
- 原 `auth.go` 不再同时承载接口、业务对象、内存实现、HTTP DTO 和 token 用例。
- `ClientAuthorization` 移除 BSON 和 JSON 双重标签，成为纯业务对象。
- `AuthStore` 接口单独放在接口文件中。
- `AuthorizationInfo` 只属于 HTTP transport，并通过 mapper 从业务对象生成。
- Mongo 解码和持久化只使用 `authorizationDocument`。
- MemoryAuthStore 保持原有并发、过期和撤销过滤语义。
- Server 现有公开构造器、配对、token 校验、授权列表和撤销调用保持兼容。

新增测试覆盖：
- ClientAuthorization 与 Mongo Document 往返。
- MemoryAuthStore 只返回匹配用户和设备的有效授权。
- 已过期和已撤销授权不会进入列表。
- 原 Relay HTTP/WebSocket 流程测试继续通过。

本轮验证：
```text
go test ./core/remote/relay
go test ./core/...
```

下一阶段：
```text
core/port/repository
  评估并迁出 JSON transport 标签

core/service
  将 application service 装配迁到 composition root

core/remote/relay
  后续可把 MongoAuthStore 迁到 adapter/persistence 下，进一步明确包边界
```

## 85. 第五十四阶段已落地内容

本阶段完成 repository Record、domain Plan 与 JSON transport 元数据的隔离。

调用审计结果：
- repository Record 没有被 `encoding/json` 直接编码或解码。
- remote agent 已通过 `payload_mapper.go` 映射为 `core/remote/protocol` DTO。
- Plan 已通过 `planPayload` 映射为 `protocol.Plan`。
- CLI 只读取 Record 字段用于显示，不依赖 JSON 标签。
- Mongo 已在上一阶段通过专用 Document/Mapper 持久化。

已经完成：
- `SessionRecord`、`MessageRecord`、`MessageHistoryMeta` 移除 JSON 标签。
- `AssetRecord`、`ModelConfig`、`TokenUsageRecord` 移除 JSON 标签。
- `plan.Plan` 和 `plan.Step` 移除 JSON 标签。
- `core/port/repository` 和 `core/plan` 当前不包含任何 BSON/JSON transport 标签。
- 网络字段命名、`omitempty` 和 snake_case 约定只由 `core/remote/protocol` DTO 维护。
- Mongo 字段命名只由 Mongo Document 维护。

隔离后的对象关系：
```text
repository Record / domain Plan
  -> application 与 port 之间传递

remote agent mapper
  -> protocol DTO
  -> JSON

mongo mapper
  -> Mongo Document
  -> BSON
```

本轮验证：
```text
go test ./core/port/repository ./core/plan ./core/adapter/persistence/mongo ./core/remote/agent ./core/cmd
go test ./core/...
```

下一阶段：
```text
core/service
  将 application service 装配迁到 composition root

core/remote/relay
  将 MongoAuthStore 移入 adapter/persistence

core/application/model
  评估 ModelConfig 是否应从 repository Record 中进一步拆为 command/domain/persistence 对象
```

## 86. 第五十五阶段已落地内容

本阶段将 Chat application service 对象图从 `ChatService` 迁入独立 composition root，消除 facade 内部随用随组装依赖的模式。

新增 composition root：
```text
core/composition/chat/configuration.go
  Configuration
  BuildDependencies
  NewService
```

一次性装配的对象图包括：
```text
session
  LoadService
  LifecycleService
  SettingsService
  SessionQueryService
  MessageQueryService
  MessageCommandService
  CurrentStateQueryService
  SessionPersistenceService
  MessagePersistenceService
  BootstrapSessionService
  CurrentSessionService

chat
  ContextQueryService
  CompactService
  SessionCompactionService
  AgentLoopService
  AssistantGenerationService
  GenerationTaskService
  ResponseCommitService

model / skill / runtime / plan
  ConfigService
  QueryService
  CatalogService
  AsyncTaskService
  StatePersistenceService
```

新增 generation adapter：
```text
core/adapter/chat/generation
  Persistence
  SummaryStore
```

已经完成：
- `ChatService` 构造器改为只接收 `ChatDependencies`。
- `Application.InitChatService` 通过 `core/composition/chat` 创建完整对象图。
- 删除 `ChatService` 内约二十个 application service builder 方法。
- `ChatService` 不再保存仅用于装配的 modelFactory、cache、pool、userID 和 planStates 字段。
- `ChatService` 不再 import 任何 `core/adapter/*` 或 `core/composition/*` 包。
- UUID、history task recorder、tool catalog/executor、tool record persistence、hook publisher 等具体 adapter 全部由 composition root 注入。
- Assistant/current-session 异步持久化从 facade callback 迁入 generation persistence adapter。
- Summary memory update 和 session persistence 从 facade callback 迁入 SummaryStore adapter。
- `MessagePersistenceService` 删除两个函数回调，改为依赖明确的 `SessionPersistence` 接口。
- `core/service/chat.go` 缩减到约 640 行，保留 API facade、流程协调和结果映射职责。

依赖方向：
```text
Application
  -> composition/chat
  -> adapters + application services
  -> service.ChatDependencies
  -> ChatService facade

ChatService
  -> application services / ports
  x adapter concrete types
```

新增测试覆盖：
- composition 对象图可以完成 session 创建、用户消息追加和 current state 查询。
- composition 创建的 ChatService 使用注入后的默认 model state。
- generation Persistence 委托 assistant record 和 current-session cache。
- SummaryStore 同时更新 memory summary 和 session persistence。

本轮验证：
```text
go test ./core/application/session ./core/adapter/chat/generation ./core/composition/chat ./core/service ./core
go test ./core/...
```

下一阶段：
```text
core/remote/relay
  将 MongoAuthStore 从 transport 包迁入 adapter/persistence

core/application/model
  拆分 ModelConfig command/domain/persistence 多重身份

core/service/chat.go
  继续评估 ExecutePlan 和 session facade 是否应拆成独立 facade 对象
```

## 87. 第五十六阶段已落地内容

本阶段将 Relay Authorization 从 transport 包迁移为完整的 Domain/Port/Adapter 分层，并让 Mongo 实现复用通用 Operations/Template。

新增领域对象：
```text
core/domain/authorization
  ClientAuthorization
  ActiveAt
```

新增端口：
```text
core/port/authorization
  Store
  ErrNotFound
```

新增适配器：
```text
core/adapter/authorization/memory
  Store

core/adapter/persistence/mongo/authorization
  Store
  document
  domain <-> document mapper
```

已经完成：
- 删除 Relay transport 包内的 ClientAuthorization、AuthStore、MemoryAuthStore 和 MongoAuthStore。
- Relay Server 只依赖 `authorizationport.Store`。
- Relay auth service 只使用 authorization domain 和 port。
- `NewServer` 改为显式接收 Store，不再在 transport 构造器中选择具体实现。
- CLI composition root 默认注入 memory adapter，配置 Mongo 时切换为 Mongo adapter。
- Mongo Authorization Store 依赖通用 `mongo.Operations`，不再自行管理 collection、cursor 和 decode。
- Mongo Document 与 domain ClientAuthorization 分离。
- Memory adapter 使用 domain 的 `ActiveAt` 规则，并负责筛选和排序。

通用 Mongo 边界修正：
```text
mongo.Template
  mongo.ErrNoDocuments -> mongo adapter ErrNotFound

repository Store
  mongo adapter ErrNotFound -> repository.ErrNotFound

authorization Store
  mongo adapter ErrNotFound -> authorization.ErrNotFound
```

此前通用 Template 直接返回 repository 错误，导致其他业务 Store 无法干净复用；当前 Template 已不再依赖 repository port。

新增测试覆盖：
- Memory Store 过滤过期、撤销和不同用户设备的授权。
- Mongo Authorization Store 实现 port 接口。
- Mongo Store 通过 fake Operations 完成 Get/List/Save/Touch 委托。
- Mongo not-found 正确翻译为 authorization port error。
- Relay 原有配对、token 校验、授权列表和撤销流程继续通过。

本轮验证：
```text
go test ./core/domain/authorization ./core/port/authorization
go test ./core/adapter/authorization/memory
go test ./core/adapter/persistence/mongo ./core/adapter/persistence/mongo/authorization
go test ./core/remote/relay ./core/cmd
go test ./core/...
```

下一阶段：
```text
core/application/model
  拆分 ModelConfig command/domain/persistence 多重身份

core/service/chat.go
  拆分 ExecutePlan 和 session facade 流程

core/composition
  为 Relay 增加独立 configuration 对象，进一步减少 cmd 装配细节
```

## 88. 第五十七阶段已落地内容

本阶段拆分原 `repository.ModelConfig` 的 Command、Result、Domain、Port 和 Mongo Document 多重身份。

新增 Domain：
```text
core/domain/model/config.go
  Config
```

新增 Application 对象：
```text
core/application/model
  add_config_command.go
    AddConfigCommand
  add_config_result.go
    AddConfigResult
  bootstrap_command.go
    BootstrapCommand
  bootstrap_result.go
    BootstrapResult
```

新增 Port：
```text
core/port/model/config_repository.go
  ConfigWriter
  ConfigReader
  ConfigRepository

core/port/persistence/store.go
  Store
```

已经完成：
- 删除 `repository.ModelConfig` 和 `repository.ModelConfigRepository`。
- AddConfigCommand 只包含外部输入字段，不再携带 Enabled、CreatedAt、UpdatedAt 等领域/持久化状态。
- ConfigService 从 Command 构造和校验 Domain Config。
- ConfigService 返回独立 AddConfigResult。
- Bootstrap Command/Result 从 Service 文件迁到独立对象文件。
- BootstrapService 和 ConfigService 依赖 `model` port，不再依赖 repository Record。
- Mongo Store 改为实现 SaveConfig/ListConfigs。
- Mongo 使用 Domain Config 与 modelConfigDocument 显式双向映射。
- Config Mapper 负责把外部 ModelProperties 转换为 Domain Config seed。
- CLI 交互式新增模型直接构造 AddConfigCommand。
- Application/Chat composition 使用新的跨端口 persistence.Store 聚合接口。

对象流向：
```text
CLI input
  -> AddConfigCommand
  -> ConfigService
  -> domain/model.Config
  -> model.ConfigWriter
  -> Mongo mapper
  -> modelConfigDocument

Config Properties
  -> config.Mapper
  -> domain/model.Config seed
  -> BootstrapService
```

边界检查：
- model Domain、Command、Result、Port 均无 BSON/JSON 标签。
- 已无 repository.ModelConfig、SaveModelConfig 或 ListModelConfigs 引用。
- Mongo 字段名只存在于 modelConfigDocument。

本轮验证：
```text
go test ./core/domain/model ./core/port/model ./core/port/persistence
go test ./core/application/model ./core/adapter/persistence/mongo
go test ./core/config ./core/composition/chat ./core/service ./core/cmd ./core
go test ./core/...
```

下一阶段：
```text
core/port/model
  拆分接口、Command/Request、Result 和值对象文件

core/service/chat.go
  拆分 ExecutePlan 和 session facade 流程
```

## 89. 第五十八阶段已落地内容

本阶段整理 `core/port/model` 的文件职责，接口文件不再混放 Request、Result 和值对象。

拆分前：
```text
model.go
  ChatModelPort
  Registry
  MutableRegistry
  GenerateRequest
  ModelInfo
  TokenUsage
  ChatResult
  ChatStreamHandler
  ToolCall
  ToolPermissionRequest
  Tool
  FunctionDefinition
```

拆分后：
```text
chat_model.go
  ChatModelPort

registry.go
  Registry
  MutableRegistry

generate_request.go
  GenerateRequest

chat_result.go
  ChatResult

model_info.go
  ModelInfo

token_usage.go
  TokenUsage

chat_stream_handler.go
  ChatStreamHandler

tool_call.go
  ToolCall
  ToolPermissionRequest

tool.go
  Tool
  FunctionDefinition
```

已经完成：
- 删除混合职责的 `core/port/model/model.go`。
- 接口只保留在接口文件中。
- Request、Result、DTO/值对象分别独立维护。
- TokenUsage 的领域值运算行为与对象保留在同一文件。
- 类型名、公开 API 和运行时行为保持不变。

本轮验证：
```text
go test ./core/port/model
go test ./core/adapter/model/langchaingo ./core/adapter/llm/langchaingo
go test ./core/application/model ./core/application/chat ./core/llm
go test ./core/...
```

下一阶段：
```text
core/service/chat.go
  将 ExecutePlan 流程迁入独立 application service

core/port
  继续审计其他接口文件是否混放 DTO
```

## 90. 第五十九阶段已落地内容

本阶段将 ExecutePlan 的多步骤执行流程从 `ChatService` 迁入独立 application use case。

新增 Application 对象：
```text
core/application/chat
  plan_execution_command.go
    PlanExecutionCommand
    PersistUserMessageCommand

  plan_execution_result.go
    PlanExecutionResult

  plan_execution_ports.go
    PlanSessionLoader
    PlanMessageAppender
    PlanGenerationTask
    PlanStateStore
    UserMessagePersistence
    PlanSessionEventPublisher
    PlanUpdateSink

  plan_execution_service.go
    PlanExecutionService
```

新增 Adapter：
```text
core/adapter/chat/generation
  UserMessagePersistence
```

PlanExecutionService 当前负责：
- 加载指定 session 和校验当前 plan。
- 启动 plan 状态机。
- 按顺序标记 step running/done/failed。
- 构造每一步的执行输入。
- 追加步骤用户消息。
- 异步持久化步骤输入。
- 强制 chat mode 执行每个步骤，避免重新生成 plan。
- 聚合每步 content、reasoning、usage 和最新 context/compact。
- context 取消时持久化 canceled 状态。
- generation 失败时持久化 failed 状态。
- 通过 PlanUpdateSink 推送 plan 更新。
- 发布 session changed 业务事件。

已经完成：
- `ChatService.ExecutePlanStreamForSession` 只保留 session id 归一化、application command 调用和 result 映射。
- 删除 facade 内的 plan 状态循环、step input 构造和 response combine 逻辑。
- 删除 facade 的 `savePlanState` 和 `combinePlanStepResponse`。
- 普通聊天和 plan step 共用 UserMessagePersistence adapter。
- 删除 facade 内旧的 persistUserMessageAsync/runAsync 基础设施逻辑。
- 删除不再需要的 generateAssistantOptions/ForceChatMode 包装层。
- composition root 一次性装配 PlanExecutionService 全部端口实现。

新增测试覆盖：
- 多步骤按顺序执行并聚合 content 和 token usage。
- 每步生成强制使用 chat mode 且不 capture 新 plan。
- 第一条步骤消息使用 Execute plan title。
- plan 状态更新、sink 和 event 发布次数正确。
- generation 失败时 plan/step 标记 failed。
- UserMessagePersistence 正确委托异步 session/message 持久化。

本轮验证：
```text
go test ./core/application/chat
go test ./core/adapter/chat/generation
go test ./core/composition/chat ./core/service ./core/remote/agent
go test ./core/...
```

下一阶段：
```text
core/service/chat.go
  继续拆 session lifecycle/settings/query facade

core/port
  审计其余接口与 DTO 混放
```

## 91. 第六十阶段已落地内容

本阶段将 session 生命周期与设置变更的完整业务编排从 `ChatService` 迁入 application use case，底层 Service 只保留内存状态和 repository 基础操作。

新增 Application UseCase：
```text
core/application/session
  lifecycle_use_case.go
    LifecycleUseCase

  settings_use_case.go
    SettingsUseCase
```

新增边界对象与端口：
```text
CreateSessionCommand
LoadSessionCommand
DeleteSessionCommand
RestoreSessionCommand
ClearSessionCommand
LifecycleResult

LifecyclePersistence
LifecycleCurrentSession
LifecycleSessionQuery
SessionEventPublisher
SettingsPersistence
```

LifecycleUseCase 当前负责：
- 创建 session 后持久化 session、保存 current session 并发布 `new` 事件。
- 加载 session 后保存 current session 并发布 `load` 事件。
- 删除 session 后发布 `delete` 事件。
- 删除当前 session 后自动加载其他有效会话；没有候选会话时创建替代会话。
- 恢复 session 后发布 `restore` 事件。
- 清空 session 后持久化清空后的状态并发布 `clear` 事件。

SettingsUseCase 当前负责：
- 委托 SettingsService 完成 model、permission、agent mode 和 context window 校验与内存更新。
- 设置成功后统一持久化 session。
- 仅在持久化成功后发布对应 session changed 事件。

已经完成：
- `ChatService.NewSession/LoadSession/DeleteSession/RestoreSession/ClearCurrent` 只构造 Command 并委托 LifecycleUseCase。
- `ChatService` 不再编排 session persistence、current-session cache 和删除后的候选会话选择。
- settings facade 方法只构造 Command 并委托 SettingsUseCase。
- 删除 ChatService 内不再使用的 `saveSession` 和 `saveCurrentSession`。
- `ChatDependencies` 不再暴露 SessionPersistenceService 和 CurrentSessionService 实现对象。
- composition root 显式装配底层 Service、UseCase、Persistence、Query、Cache 和 Event Publisher。
- `ChatService` 从约 532 行降到 356 行。

新增测试覆盖：
- 创建 session 时依次完成持久化、current session 保存和事件发布。
- 删除当前 session 后加载其他候选会话。
- 删除最后一个 session 后创建替代会话。
- 四类 session 设置变更均完成持久化并发布正确事件。
- 持久化失败时不发布设置变更事件。

本轮验证：
```text
go test ./core/application/session ./core/composition/chat ./core/service ./core/remote/agent
go test ./core/...
git diff --check
```

`git diff --check` 无空白错误，仅报告工作区已有的 LF/CRLF 转换提示。

下一阶段：
```text
core/application/tool
  移除带 json tag 的传输 DTO，避免 Application 对象兼任协议解析对象

core/remote
  继续审计 protocol DTO、handler command 和 application command 的映射边界
```

## 92. 第六十一阶段已落地内容

本阶段完成 tool execution 链路中的 Domain、Application、DTO 与 Persistence PO 隔离。

新增 Domain 对象：
```text
core/domain/tool
  ExecutionEntry
  ExecutionEntryKind
  SharedAsset
```

新增或拆分 Application 对象：
```text
core/application/tool
  Registry                         接口
  HookBridge                       接口
  AssetExtractor                   接口
  LLMToolCatalog                   接口
  ToolModePolicy                   接口

  ExecutionCommand                Command
  AssetExtractionCommand          Command
  ToolCallEntryCommand            Command
  ToolResultEntryCommand          Command
  PermissionCommand               Command

  ExecutionResult                 Result
  PermissionDecision              Result
  PermissionRequest               边界请求对象
  HookEvent / HookResult           hook 边界对象
```

新增 Adapter DTO 与 Mapper：
```text
core/adapter/tool/executor
  sharedAssetResultDTO
  SharedAssetExtractor

core/adapter/persistence/toolrecords
  IDGenerator
  Mapper
```

已经完成：
- `application/tool` 不再导入 `encoding/json`、`google/uuid` 或 `core/port/repository`。
- share_file 返回值的 JSON DTO 和解析逻辑迁入 tool executor adapter。
- `ExecutionService` 返回 `domain/tool.ExecutionEntry` 和 `SharedAsset`，不再返回 repository PO。
- `application/chat.ToolExecutionResult` 和记录 Command 改为携带 Domain 对象。
- toolrecords persistence adapter 在边界处生成 UUID 并映射为 `MessageRecord` / `AssetRecord`。
- composition root 显式注入 UUID generator。
- 删除 Application 层旧的 `SharedAssetToolResult`、`AssetRecordFromToolResult` 和 repository record mapper。
- Permission 与 Selection 模块中的 Interface、Command、Request、Result 已拆到独立文件。

新增测试覆盖：
- ToolCall / ToolResult Domain entry 构造。
- JSON transport result 到 SharedAsset Domain 的解析。
- 非 share_file 结果不会生成 asset。
- Domain execution entry 到 MessageRecord PO 的映射。
- SharedAsset Domain 到 AssetRecord PO 的映射。
- 未知 execution entry 类型在持久化边界被过滤。
- Recorder 在 Domain → PO 映射后异步持久化消息和资产。

本轮验证：
```text
go test ./core/application/tool
go test ./core/application/chat
go test ./core/adapter/tool/executor
go test ./core/adapter/persistence/toolrecords
go test ./core/composition/chat
go test ./core/...
git diff --check
```

生产代码边界扫描：
```text
rg "encoding/json|google/uuid|core/port/repository|json:|bson:" \
  core/application/tool core/domain/tool -g "*.go" -g "!*_test.go"
```

结果为零命中。`git diff --check` 无空白错误，仅报告工作区已有的 LF/CRLF 转换提示。

下一阶段：
```text
core/application/session/message_persistence.go
  拆分 Interface、Command、Service，并审计 repository PO 泄漏

core/application/session/mapper.go
  迁移 UUID 生成与持久化映射职责到 adapter
```

## 93. 第六十二阶段已落地内容

本阶段将消息持久化 PO 构造、UUID 生成和 session snapshot 映射从 application/session 迁入 persistence adapter。

新增 Persistence Adapter：
```text
core/adapter/persistence/chatmessage
  Writer
  Mapper
  IDGenerator
  MessageSaver
  SessionPersistence
```

新增 Generation Adapter 最小端口：
```text
core/adapter/chat/generation
  AssistantMessageWriter
  UserMessageWriter
  SummaryPersistence
```

已经完成：
- 删除 `application/session.MessagePersistenceService`。
- 删除 Application 层 `SaveUserMessageCommand` 和 `SaveAssistantMessageCommand`。
- 用户消息统一使用 `application/chat.PersistUserMessageCommand`，不再重复定义相同 Command。
- `chatmessage.Writer` 负责用户/助手消息 PO 构造、UUID、时间和错误聚合。
- `chatmessage.Mapper` 负责 session Domain snapshot 到 SessionRecord PO 的映射。
- generation persistence adapter 依赖最小化 writer 接口，不依赖 application service 实现类。
- toolrecords adapter 直接依赖 `SaveMessage` / `SaveAsset` 最小持久化接口，不再借用 MessagePersistenceService 的批量方法。
- `SummaryStore` 使用只包含 `Save` 的 SummaryPersistence，避免恢复宽接口。
- composition root 显式装配 chatmessage.Writer 和 UUID generator。
- 删除 application/session 中不再使用的 `SessionRecordFromSession` 和 `AssistantMessageRecord`。

新增测试覆盖：
- 用户消息与 session metadata 同步持久化。
- 助手消息和 session snapshot 同步持久化。
- 注入 ID generator 与固定时间正确应用于 PO。
- generation adapter 正确委托新 Writer。
- toolrecords 使用最小持久化端口保存映射后的 PO。

本轮验证：
```text
go test ./core/adapter/persistence/chatmessage
go test ./core/adapter/chat/generation
go test ./core/adapter/persistence/toolrecords
go test ./core/application/session
go test ./core/composition/chat
go test ./core/...
git diff --check
```

`git diff --check` 无空白错误，仅报告工作区已有的 LF/CRLF 转换提示。

下一阶段：
```text
core/application/session/mapper.go
  移除 google/uuid 依赖，把内存消息 Record ID 生成抽为端口

core/application/session
  继续区分 Query Result 与 repository Record，减少 PO 向 facade 泄漏
```

## 94. 第六十三阶段已落地内容

本阶段完成 session 查询出口 DTO 隔离，并移除 application/session 的 UUID 基础设施依赖。

新增 Application Query DTO / Result：
```text
core/application/session
  SessionListItem
  MessageListItem
  AssetListItem
  MessageHistoryMetaResult
  TokenUsageResult
```

新增和拆分 Query Port：
```text
MessageQueryStore
MemorySessionSource
MemoryMessageRecordMapper
```

已经完成：
- `SessionQueryService` 在内部读取 repository Record，对外返回 SessionListItem / AssetListItem。
- `MessageQueryService` 在内部读取 repository Record，对外返回 MessageListItem / MessageHistoryMetaResult。
- `ChatService` 查询 API 不再返回 `repository.SessionRecord/MessageRecord/AssetRecord`。
- Remote Agent 的 SessionQueryFacade 改为依赖 application query result。
- Remote payload mapper 负责 Application Result 到 protocol DTO 的映射，不再直接接收 persistence PO。
- CLI session list 改为依赖 SessionListItem。
- 内存 Domain Message 到 MessageRecord PO 的映射迁入 `chatmessage.Mapper`。
- UUID 和时间生成由 persistence adapter 的 Mapper 注入和管理。
- application/session 生产代码不再导入 `google/uuid`。
- Query Store、Memory Source、Mapper 接口均拆入独立接口文件。

新增测试覆盖：
- Memory Message mapper 过滤 system message，不消耗无效 ID。
- user/tool-call/tool-result 内存消息正确映射为 persistence Record。
- Session、Message、Asset Query Result 字段映射由原 service/remote 测试持续覆盖。
- Remote payload mapper 测试改为从 Application Result 映射协议 DTO。

本轮验证：
```text
go test ./core/adapter/persistence/chatmessage
go test ./core/application/session
go test ./core/remote/agent
go test ./core/cmd
go test ./core/composition/chat
go test ./core/...
git diff --check
```

边界扫描结果：
```text
service / remote agent / cmd
  无 repository SessionRecord / MessageRecord / AssetRecord / TokenUsageRecord 暴露

application/session
  无 google/uuid 依赖
```

`git diff --check` 无空白错误，仅报告工作区已有的 LF/CRLF 转换提示。

下一阶段：
```text
core/application/chat
  拆分 assistant_generation_service.go 中的 Interface / Command / Result
  拆分 response_commit_service.go 中的 Interface / Command / Result
  拆分 compact_service.go 和 session_compaction_contracts.go
```

## 95. 第六十四阶段已落地内容

本阶段完成 application 层 Service、Port、Command、Result 的文件职责统一。

application/chat 已拆分：
```text
assistant_generation_service.go   只保留 AssistantGenerationService 实现
assistant_generation_ports.go     只放 generation ports
assistant_generation_command.go   AssistantGenerationCommand
generation_response.go             GenerationResponse
compact_info.go                    CompactInfo

response_commit_service.go         只保留 ResponseCommitService 实现
response_commit_ports.go           ResponseMemoryStore / PlanCapturer
commit_command.go                  CommitCommand
commit_result.go                   CommitResult

compact_service.go                 只保留 CompactService 实现
compact_ports.go                   SummaryGenerator / CompactSummaryStore
compact_session_command.go         CompactSessionCommand

generation_task_service.go         只保留 GenerationTaskService 实现
generation_task_ports.go           recorder / handler / id ports
generation_task_command.go         GenerationTaskCommand
task_record.go                     TaskRecord
```

其他 application 包已拆分：
```text
application/plan
  SaveStateCommand 从 StatePersistenceService 分离

application/session
  MemoryStore
  SessionRecordGetter
  MessageRecordLister
  EnsureInMemoryCommand
  SessionListRepository
  SessionAssetRepository
  ListAssetsCommand
```

已经完成：
- application 下所有 `*Service` 文件只保留实现类和实现方法。
- Service 文件不再混放 Interface、Command、Result 或 Response。
- application/domain/port/service 生产代码无 JSON/BSON/YAML 标签。
- 保持 Go 的包级组织习惯：同一业务模块的纯接口可以集中在 ports 文件，不机械复制 Java 一类一文件。

本轮验证：
```text
go test ./core/application/chat
go test ./core/application/plan
go test ./core/application/session
go test ./core/adapter/chat/generation
go test ./core/composition/chat
go test ./core/remote/agent
go test ./core/cmd
go test ./core/...
git diff --check
```

结构扫描结果：
```text
no mixed Service/Port/Command/Result files in core/application
no transport or persistence tags in application/domain/port/service
```

`git diff --check` 无空白错误，仅报告工作区已有的 LF/CRLF 转换提示。

下一阶段：
```text
全项目依赖方向审计
  domain / port 不得依赖 application / adapter / service / remote
  application 不得依赖 adapter / service / remote
  service 不得依赖具体 persistence adapter

remote/protocol
  评估按业务功能分组 DTO，避免无价值的一类一文件膨胀
```

## 96. 第六十五阶段及最终验收

本阶段完成依赖方向守卫、剩余 contracts 对象拆分、remote workspace 依赖注入和旧线程池清理。

新增自动化架构守卫：
```text
core/architecture/dependency_rules_test.go
```

守卫规则：
- Domain 不得依赖 Application、Adapter、Service、Remote。
- Port 不得依赖 Application、Adapter、Service、Remote。
- Application 不得依赖 Adapter、Service、Remote。
- Service 不得依赖具体 Adapter。
- Application、Domain、Port、Service 不得出现 JSON/BSON/YAML 序列化标签。

完成剩余对象隔离：
- bootstrap Command / Result / Action 从接口 contracts 文件拆出。
- CurrentState 从接口 contracts 文件拆出。
- message Command / Result 从接口 contracts 文件拆出。
- SaveSessionCommand 从 persistence 接口文件拆出。
- model CreationConfig 从 Factory 接口文件拆出。
- model ListModelsResult 从 Catalog 接口文件拆出。
- skill Query / Result 从 Catalog 接口文件拆出。
- 当前 interface-bearing 的 port/application 生产文件不再包含 struct 对象。

完成 remote workspace 依赖注入：
- `remote/changes` 删除对 SQLite history adapter 的直接依赖。
- changes service 只接受 History Store 或 StoreFactory port。
- remote Agent 构造器只接受 WorkspaceFileFacade / WorkspaceChangeFacade。
- CLI 作为 composition root 创建 file service 与 SQLite history factory 并注入 Agent。
- concrete adapter import 只存在于 adapter、composition root、cmd 或 App 装配层。

完成线程池迁移：
- ThreadPool 实现迁入 `adapter/async/threadpool.Pool`。
- `Application` 不再依赖旧 `utills` 包。
- Application.Close 会关闭 MCP manager 并等待线程池退出。
- 删除旧 `utills/threadPool.go`。
- core 生产代码不再导入旧 store 或 utills 包。

数据库公共操作封装验收：
```text
adapter/cache/redis.Template
  String / JSON / NX / TTL
  Hash / Hash JSON
  SortedSet / score range
  Key operations

adapter/persistence/mongo.Template
  FindOne / FindAll
  InsertOne / UpdateOne / DeleteMany
  Count
  ErrNotFound 统一翻译
```

没有为了模仿 MyBatis-Plus 强制引入链式 API；当前 Template + Operations 接口更符合 Go 调用习惯，也避免额外流程复杂度。

最终验证：
```text
go test ./...
npm run typecheck  # mobile
go test ./core/architecture
git diff --check
```

全部通过。`git diff --check` 无空白错误，仅报告工作区已有的 LF/CRLF 转换提示。

最终结构证据：
```text
no forbidden dependency direction violations
no mixed Service/Port/Command/Result files in core/application
interface-bearing port/application files contain no struct objects
no transport or persistence tags in inner layers
no concrete adapter imports outside composition roots
no legacy store or utills imports in core production code
```

本次架构重构至此完成。后续新增业务应由架构守卫和本文件中的分层约束持续约束，不再继续进行无业务收益的机械拆文件。

## 97. 完成后的架构防回退加固

在最终验收基础上继续将一次性扫描规则固化为自动化测试。

新增守卫：
- Application / Port 中含 interface 的生产文件不得同时定义 struct。
- Remote 层不得直接 import concrete adapter。
- Core 生产代码不得重新 import `core/store/*` 或 `utills` 旧包。

这些规则与已有依赖方向、序列化标签守卫共同运行：
```text
go test ./core/architecture
```

验证：
```text
go test ./core/architecture ./...
```

全部通过。该阶段不改变运行时行为，只提高后续架构回归的可检测性。

## 62. 第三十一阶段已落地内容

本轮开始落实更严格的 Java 风格边界纪律：接口文件只放接口，Record/POJO 文件只放数据结构，组合接口单独放置。

本轮先处理 `core/port/repository`，保持包名不变，避免一次性迁移影响过大。

调整前：
```text
core/port/repository/store.go
  SessionRecord
  TokenUsageRecord
  ModelConfig
  MessageRecord
  MessageHistoryMeta
  AssetRecord
  Role 常量
  SessionRepository
  MessageRepository
  ModelConfigRepository
  AssetRepository
  Store
```

调整后：
```text
core/port/repository
  store.go                     只放 Store 组合接口
  session_repository.go        只放 SessionRepository
  message_repository.go        只放 MessageRepository
  model_config_repository.go   只放 ModelConfigRepository
  asset_repository.go          只放 AssetRepository
  session_record.go            只放 SessionRecord
  message_record.go            只放 MessageRecord / MessageHistoryMeta / Role 常量
  model_config.go              只放 ModelConfig
  asset_record.go              只放 AssetRecord
  token_usage_record.go        只放 TokenUsageRecord
```

当前效果：
```text
repository interface 文件
  -> 只表达“能力契约”

record 文件
  -> 只表达持久化/查询数据结构

store.go
  -> 只表达 repository 组合
```

这一步没有改变包路径和外部调用方式，因此是低风险的结构收敛；后续再逐步把带 `bson/json` tag 的 Record 从 `port` 迁到更合适的 persistence adapter 或独立 persistence model 边界。

下一步建议：
```text
core/application/chat
  将 agent_loop_service.go 中的接口、Command、Result、Service 实现拆成独立文件

core/application/session
  将 lifecycle/settings/query 等 service 文件中的接口、Command、Result 拆开

core/port/repository
  下一阶段再讨论是否把 Record/PO 彻底迁出 port
```

本轮验证：
```text
go test ./core/port/repository ./core/application/session ./core/service
go test ./core/application/chat ./core/application/tool ./core/adapter/persistence/toolrecords ./core/adapter/tool/catalog ./core/adapter/tool/executor
go test ./core/...
```

## 67. 第三十六阶段已落地内容
本轮继续整理 session settings 职责，把切换模型时的 registry 校验从 `ChatService` 移入 `application/session.SettingsService`。

已经完成：
- `SettingsService` 新增 `Models model.Registry` 依赖。
- `SettingsService.SwitchModel` 负责校验：
  - model id 不能为空。
  - session manager 不能为空。
  - model registry 不能为空。
  - 目标模型必须存在于 registry。
- `ChatService.SwitchModelForSession` 不再直接调用 `s.client.HasModel`。
- `ChatService.sessionSettings()` 负责装配 `s.client` 到 `SettingsService.Models`。

当前效果：
```text
service.ChatService
  -> SwitchModel facade
  -> save session + hook

application/session.SettingsService
  -> switch model use case
  -> model registry port 校验
  -> session memory 状态更新
```

新增测试覆盖：
- `SettingsService.SwitchModel` 成功切换指定 session model。
- `SettingsService.SwitchModel` 成功切换 current model。
- `SettingsService.SwitchModel` 会拒绝 registry 中不存在的模型。

本轮验证：
```text
go test ./core/application/session ./core/service
go test ./core/...
```

后续第三阶段建议：
```text
port/model
  ChatModelPort
  GenerateRequest
  GenerateResult

adapter/llm/langchaingo
  LangChainGoChatModel
```

也就是把 `ChatService` 对 `core/llm.Model` 和 `llms.ToolCall` 的直接依赖继续往外推。

## 34. 第三阶段已落地内容

本轮继续把模型边界和工具边界抽成 port。

新增：
```text
core/port/model
```

当前包含：
```text
ChatModelPort
  应用层依赖的模型接口

MutableRegistry
  模型注册器接口

GenerateRequest
  本次调用的 messages / tools / stream

ChatResult
  模型返回结果

ModelInfo
  模型元信息

Tool / FunctionDefinition
  项目自己的工具描述对象
```

`core/llm` 当前状态：
```text
llm.Model
  实现 ChatModelPort

llm.Client
  实现 MutableRegistry

llm.GenerateRequest / ChatResult / TokenUsage / ChatStreamHandler
  暂时作为兼容别名保留
```

`core/tool/register.go` 现在返回：
```text
[]modelport.Tool
```

`adapter/llm/langchaingo` 负责把：
```text
domain.Message <-> llms.MessageContent
modelport.Tool <-> llms.Tool
```

`ChatService` 现在只依赖：
```text
modelport.ChatModelPort
modelport.MutableRegistry
```

也就是说它已经不再直接依赖：
```text
core/llm.Model
core/llm.Client
llms.Tool
llms.ToolCall
llms.MessageContent
```

本轮验证：
```text
go test ./core/...
```

下一阶段建议继续拆：
```text
application/chat
application/plan
application/session
adapter/persistence/mongo
adapter/cache/redis
```

也就是把 `ChatService` 再拆成更纯粹的 usecase，把持久化和缓存彻底下沉。

## 35. 第四阶段已落地内容

本轮先没有大拆 `ChatService` 的执行流程，而是把 Plan 相关的纯业务组装逻辑下沉到 application 层。

新增：
```text
core/application/plan
```

当前包含：
```text
ExecutionInputBuilder
  构建执行单个计划步骤时发给模型的用户输入

ResponseCombiner
  合并多步计划执行产生的模型回复、reasoning 和 usage
```

已经完成：
- `ChatService` 不再自己拼 `Execute the approved plan step ...` 这段执行输入
- `ChatService` 不再自己合并多步响应文本和 usage
- `application/plan` 不依赖 session / store / remote / mongo / redis
- `application/plan` 只依赖 plan domain 和 model port

本轮验证：
```text
go test ./core/...
```

后续第五阶段建议：
```text
application/plan
  PlanCaptureService
  PlanStateService
  PlanExecutionUseCase
```

其中：
```text
PlanCaptureService
  负责从模型回复中生成 / 标记 plan

PlanStateService
  负责 plan 状态迁移规则

PlanExecutionUseCase
  负责执行已批准计划的编排
```

到这一步时，`ChatService.ExecutePlanStreamForSession` 可以进一步缩小，逐步变成只负责兼容旧 API 的 facade。

## 36. 第五阶段已落地内容

本轮继续拆 Plan 逻辑，但仍然不把持久化和执行编排一次性搬走。

新增到：
```text
core/application/plan
```

新增组件：
```text
CaptureService
  从模型回复中生成 plan
  如果回复中包含 Result / 正文等结果 section，则把 plan 和 steps 标记为 done

StateService
  负责 plan 状态迁移
  Start
  MarkStepRunning
  MarkStepDone
  MarkStepFailed
  MarkCanceled
  MarkDone
```

已经完成：
- `ChatService.capturePlanForSession` 不再直接调用 `plan.NewDraft` 和 `plan.HasResultSection`
- `ChatService.ExecutePlanStreamForSession` 不再手写 plan / step 状态赋值
- 状态迁移规则有独立测试
- 捕获规则有独立测试

仍然留在 `ChatService` 的部分：
```text
savePlanState
ExecutePlanStreamForSession 的执行编排
session message 写入
plan update callback
session changed hook
```

这些内容涉及 session、store、hook、stream 等基础设施，下一步应该整体迁移到：
```text
application/plan/PlanExecutionUseCase
```

本轮验证：
```text
go test ./core/...
```

后续第六阶段建议：
```text
PlanExecutionUseCase
  依赖 SessionRepository / PlanRepository / ChatModelPort / EventPublisher
```

但在真正迁移前，需要先抽 session repository 和 plan repository port，否则 PlanExecutionUseCase 仍然会被当前的 `SessionManage` 和 `ChatService.saveSession` 污染。

## 37. 第六阶段已落地内容

本轮先把 Plan 状态保存从 `ChatService` 内部继续往外推一层。
这一步不是完整的 `PlanExecutionUseCase`，而是先把“保存当前 plan 状态”的边界立起来。

新增：
```text
core/port/plan
  StateRepository

core/adapter/plan/sessionstate
  Repository

core/application/plan
  StatePersistenceService
```

当前职责：
```text
StatePersistenceService
  克隆 plan
  更新时间
  调用 StateRepository 保存

StateRepository
  只表达保存当前 plan 状态这件业务能力

adapter/plan/sessionstate.Repository
  负责把 plan 写回 SessionManage
  并通过回调触发现有 session 持久化
```

已经完成：
- `ChatService.savePlanState` 不再直接操作 `SessionManage.SetCurrentPlanForSession`
- plan 状态保存走 `application -> port -> adapter`
- application 层测试覆盖 plan clone 和 `UpdatedAt` 更新
- 旧的 session 持久化回调仍然留在 facade 层，避免一次性迁移过大

仍然留在 `ChatService` 的部分：
```text
onPlanUpdate callback
session changed hook
ExecutePlanStreamForSession 执行编排
用户消息 / assistant 消息写入
```

这些下一步应该继续抽到：
```text
application/plan/PlanExecutionUseCase
port/session/SessionRepository
port/event/EventPublisher
```

本轮验证：
```text
go test ./core/port/plan ./core/adapter/plan/sessionstate ./core/application/plan ./core/service
go test ./core/...
```

## 38. 第七阶段已落地内容

本轮处理缓存边界。
原来 `ChatService` 直接依赖：
```text
core/store/cache.Cache
```

但这个接口并不是通用 cache，它真正表达的是：
```text
当前用户当前会话 ID 缓存
```

所以本轮把业务语义接口移动到 port 层，并新增 Redis adapter / template。

新增：
```text
core/port/cache
  CurrentSessionCache

core/adapter/cache/redis
  Template
  CurrentSessionCache
  keys.go
```

当前职责：
```text
port/cache.CurrentSessionCache
  表达业务需要的当前会话缓存能力

adapter/cache/redis.CurrentSessionCache
  使用 Redis 实现当前会话缓存

adapter/cache/redis.Template
  封装 Redis 常用技术操作
  SetString / GetString / Delete / Exists
  SetJSON / GetJSON / SetNX / Expire / TTL
```

已经完成：
- `ChatService` 改为依赖 `cacheport.CurrentSessionCache`
- `Application` 改为通过 `adapter/cache/redis.NewCurrentSessionCache` 装配缓存
- `core/store/cache` 和 `core/store/cache/redisCache` 目前保留为兼容别名 / wrapper
- Redis key 生成集中在 adapter 内部，不再由业务层拼 key

这一层的 Spring Boot 类比是：
```text
ChatApplicationService
  -> CurrentSessionCache
  -> RedisCurrentSessionCache
  -> RedisTemplate
  -> go-redis Client
```

也就是：
```text
application/service 不直接依赖 Redis SDK
业务 port 不暴露 Redis key
RedisTemplate 只做基础设施技术复用
具体 cache adapter 表达业务语义
```

后续可以继续做：
```text
port/repository/session
adapter/persistence/mongo/session
MongoTemplate
```

这样 `ChatService.saveSession`、`messagesFromStore`、`saveMessage`、`saveAsset` 这类持久化细节才能继续下沉。

本轮验证：
```text
go test ./core/port/cache ./core/adapter/cache/redis ./core/store/cache ./core/store/cache/redisCache ./core/service ./core
go test ./core/...
```

## 39. 第八阶段已落地内容

本轮开始处理持久化 Store 的边界。
原来业务装配和 `ChatService` 直接依赖：
```text
core/store/data.Store
core/store/data/mongoDb
```

这会让 application / service 看起来像是在直接依赖旧的 store 包。
本轮先把接口和实现入口迁到目标分层：

新增：
```text
core/port/repository
  Store
  SessionRepository
  MessageRepository
  ModelConfigRepository
  AssetRepository

core/adapter/persistence/mongo
  Store
```

当前装配变成：
```text
Application
  -> repository.Store
  -> adapter/persistence/mongo.Store
  -> MongoDB client
```

已经完成：
- `Application.store` 改为 `repository.Store`
- `Application.InitStore` 改为使用 `adapter/persistence/mongo.New`
- `ChatService` 改为从 `port/repository` 取得 Store / Record 类型
- `cmd` 和 `remote/agent` 的记录类型导入也切到 `port/repository`
- `core/store/data` 保留为兼容 alias
- `core/store/data/mongoDb` 保留为兼容 wrapper

当前仍然不是最终形态：
```text
port/repository 里暂时还保留 SessionRecord / MessageRecord / AssetRecord / ModelConfig
这些 Record 仍然带 bson tag
```

原因是这一步优先调整依赖方向，避免一次性大迁移。
最终更干净的目标应该是：
```text
port/repository
  使用 application/domain 需要的对象

adapter/persistence/mongo
  内部定义 Mongo PO / Record
  mapper: domain/application object <-> Mongo record
```

也就是说：
```text
当前是过渡层：service -> port record -> mongo adapter
最终目标：service -> domain/application model -> repository port -> mongo mapper -> mongo record
```

后续建议继续拆：
```text
application/session
  SessionPersistenceService
  MessageHistoryService

adapter/persistence/mongo
  records.go
  mapper.go
```

然后再把 `ChatService.saveSession`、`saveSessionRecord`、`messagesFromStore`、`messageContentRecord` 这些映射逻辑从 `ChatService` 移出去。

本轮验证：
```text
go test ./core/port/repository ./core/adapter/persistence/mongo ./core/store/data ./core/store/data/mongoDb ./core/service ./core
go test ./core/cmd ./core/remote/agent ./core/service ./core/adapter/persistence/mongo ./core/port/repository
go test ./core/...
```

## 40. 第九阶段已落地内容

本轮继续缩小 `ChatService`，把 session / message / usage 的记录映射逻辑下沉到 application 层。

新增：
```text
core/application/session
  mapper.go
```

当前包含：
```text
SessionRecordFromSession
  session.Session -> repository.SessionRecord

TokenUsageRecord
  llm.TokenUsage -> repository.TokenUsageRecord

TokenUsageFromRecord
  repository.TokenUsageRecord -> llm.TokenUsage

MemorySessionMessages
  内存 session messages -> repository.MessageRecord

MessageHistoryMetaFromRecords
  message records -> MessageHistoryMeta

MessagesAfterID
  内存消息分页

MessagesFromRecords
  repository.MessageRecord -> domain/message.Message

MessageContentRecord
  domain/message.Message -> repository.MessageRecord
```

已经完成：
- `ChatService` 不再保存 `sessionRecordFromSession`
- `ChatService` 不再保存 `tokenUsageRecord` / `tokenUsageFromRecord`
- `ChatService` 不再保存 message record <-> domain message 的转换逻辑
- `messagesFromStore` 改为委托 `application/session.MessagesFromRecords`
- 内存消息兜底仍由 `ChatService.memorySessionMessages` 保持兼容入口，但内部只取 session 并委托 mapper
- mapper 增加独立测试

这一步的意义是：
```text
ChatService
  少做 PO / domain / application result 的转换

application/session
  开始承接 session 相关的应用层映射规则
```

仍然留在 `ChatService` 的部分：
```text
saveSession
saveSessionRecord
ensureSessionInMemory
persistUserMessageAsync
persistAssistantMessageAsync
```

下一步建议：
```text
application/session
  SessionPersistenceService
  SessionLoader
  MessagePersistenceService
```

最终目标是把 `ChatService` 从“保存、加载、映射、编排都做”压缩成只负责兼容旧 API 和调用 usecase 的 facade。

本轮验证：
```text
go test ./core/application/session ./core/service
go test ./core/...
```

## 63. 第三十二阶段已落地内容
本轮继续整理 model config 职责，把 `ChatService.AddModelConfig` 中的校验、模型创建、配置保存、registry 注册迁移到独立应用服务。

新增 port：
```text
core/port/model
  Factory
  CreationConfig
```

新增 adapter：
```text
core/adapter/model/langchaingo
  Factory
```

新增应用服务：
```text
core/application/model
  ConfigService
```

已经完成：
- `model.Factory` 定义模型创建接口，只返回 `ChatModelPort`，不暴露 LangChainGo/OpenAI 细节。
- `adapter/model/langchaingo.Factory` 负责基于 OpenAI-compatible 参数创建运行时模型。
- `application/model.ConfigService.AddConfig` 接管 model config 的清洗、校验、重复检查、模型创建、持久化保存、registry 注册。
- `ChatService` 新增 `modelFactory` port 字段，通过构造函数注入。
- `core.Application.InitChatService` 注入 `adapter/model/langchaingo.Factory`。
- `ChatService.AddModelConfig` 变成 facade 委托，不再直接调用 `utills.CreateLLM`，不再直接拼装 `ModelInfo`。

当前效果：
```text
service.ChatService
  -> AddModelConfig facade
  -> modelConfigService() 装配应用服务

application/model.ConfigService
  -> 校验和 use case 编排
  -> repository port 保存配置
  -> model factory port 创建模型
  -> mutable registry port 注册模型

adapter/model/langchaingo.Factory
  -> OpenAI-compatible / LangChainGo 具体模型创建
```

新增测试覆盖：
- AddConfig 会归一化配置、保存配置、创建模型并注册 registry。
- AddConfig 会拒绝重复模型。
- AddConfig 会拒绝不支持的 provider。
- 模型工厂失败时不会保存配置。

下一步建议：
```text
core/Application
  继续把启动期 loadModelConfigs / seed config / registry 初始化迁移到 application/model

utills
  逐步移除 CreateLLM 这类基础设施创建逻辑，改由 adapter/model/langchaingo 承担
```

本轮验证：
```text
go test ./core/application/model ./core/adapter/model/langchaingo ./core/service ./core
go test ./core/...
```

## 65. 第三十四阶段已落地内容
本轮完成 model factory 迁移后的收尾清理，删除旧的工具函数入口。

已经完成：
- 删除 `utills/llm.go`。
- 运行时代码中不再存在 `utills.CreateLLM` 调用点。
- 模型创建统一由 `core/adapter/model/langchaingo.Factory` 承担。
- `utills` 当前只保留线程池相关能力，不再混入 LLM 基础设施创建逻辑。

当前效果：
```text
application/model
  -> 依赖 model.Factory port

adapter/model/langchaingo
  -> 唯一负责 OpenAI-compatible 模型创建

utills
  -> 不再负责 LLM 创建
```

本轮验证：
```text
go test ./core/...
```

## 64. 第三十三阶段已落地内容
本轮继续整理启动期 model 初始化职责，把 `core.Application.InitClient` 中的模型配置加载、seed 保存、默认模型选择、registry 注册迁移到 `application/model`。

新增应用服务：
```text
core/application/model
  BootstrapService
```

已经完成：
- `BootstrapService.Bootstrap` 负责从 repository 加载 model configs。
- repository 为空时，`BootstrapService` 负责写入 viper seed config。
- `BootstrapService` 负责计算 default model id。
- `BootstrapService` 负责通过 `model.Factory` 创建 enabled model，并注册到 `MutableRegistry`。
- `BootstrapService` 保留原有错误语义：无可用模型时报 `no enabled model urlConfig`，模型创建失败时报 `create model <id> failed`。
- `core.Application.InitClient` 变成装配层：创建 `llm.Client`，传入 repository/registry/factory/seed，保存 `DefaultModelID`。
- `core.Application` 删除重复的 `loadModelConfigs` 和 `defaultModelID` 逻辑。

当前效果：
```text
core.Application
  -> framework/bootstrap composition root
  -> 提供 viper seed config
  -> 装配 BootstrapService

application/model.BootstrapService
  -> model config 加载
  -> seed 持久化
  -> default model 选择
  -> runtime model 创建与 registry 注册
```

新增测试覆盖：
- 启动时会加载 repository 中的 configs，并只注册 enabled model。
- repository 为空时会保存 seed config。
- repository 为空且 seed 缺失时会返回 `model urlConfig is empty`。
- factory 创建失败时会带上 model id 包装错误。
- default model 选择顺序：显式 default > fallback > 第一个 enabled model。

下一步建议：
```text
utills
  CreateLLM 已无调用点，可以删除，模型创建统一走 adapter/model/langchaingo.Factory

core/Application
  继续整理 InitRegister / InitMCP / InitSkillManager 这类启动装配逻辑，避免 App 混入业务规则
```

本轮验证：
```text
go test ./core/application/model ./core/adapter/model/langchaingo ./core/service ./core
go test ./core/...
```

## 62. 第三十一阶段已落地内容
本轮继续收拢 session 生命周期和查询职责，减少 `ChatService` 对 repository/store 的直接操作。

新增/完善应用服务：
```text
core/application/session
  LifecycleService
  SessionQueryService
```

已经完成：
- `LifecycleService.DeleteSession` 接管持久化软删除：先通过 repository port 校验 session 存在，再执行 `MarkSessionDeleted`，最后移除内存 session。
- `LifecycleService.RestoreSession` 接管恢复逻辑：通过 repository port 执行 `MarkSessionRestored`，并返回归一化后的 session id。
- `ChatService.DeleteSession` 不再直接调用 `store.MarkSessionDeleted`，只负责调用 lifecycle use case、发布 hook、删除当前会话后切换到下一个会话。
- `ChatService.RestoreSession` 不再直接调用 `store.MarkSessionRestored`，只负责调用 lifecycle use case 和发布 hook。
- 新增 `SessionQueryService.ListSessions`，用于封装 session 列表查询。
- 新增 `SessionQueryService.ListAssets`，用于封装 session 资产查询和 session id 校验。
- `ChatService.ListSessionsWithDeleted` / `ListAssets` 改为委托 `sessionQueries()` 装配出的应用服务。

当前效果：
```text
service.ChatService
  -> API facade
  -> hook / current-session switching orchestration
  -> application service 装配

application/session.LifecycleService
  -> New / Load / Delete / Restore / Clear 生命周期 use case
  -> memory + repository port 编排

application/session.SessionQueryService
  -> session 列表查询
  -> session asset 查询
```

新增测试覆盖：
- 删除当前 session 时会同步标记持久化 session 为 deleted。
- 恢复 session 时会同步标记持久化 session 为 restored。
- session 列表查询会透传 includeDeleted。
- store 缺失时 session 列表查询返回 nil。
- asset 查询会归一化 session id 并透传 limit。
- asset 查询拒绝空 session id。

下一步建议：
```text
application/model
  抽离 model config 的创建、校验、保存和 registry 注册逻辑

service.ChatService
  继续把 AddModelConfig 这类非 chat facade 职责移出 ChatService
```

本轮验证：
```text
go test ./core/application/session ./core/service
go test ./core/...
```

## 46. 第十五阶段已落地内容

说明：本文档前面的阶段记录存在历史追加顺序不完全连续的问题，本节继续按新的阶段号追加，不重排旧内容，避免制造无关 diff。

本轮继续压缩 `ChatService`，重点拆出两块：

```text
1. agent loop 编排
2. LLM tool 暴露/选择策略
```

新增到：
```text
core/application/chat
  agent_loop_service.go

core/application/tool
  selection_service.go
```

### 46.1 application/chat.AgentLoopService

`AgentLoopService` 负责原来 `ChatService.runAgentLoop` 中的核心循环：

```text
AgentLoopService
  调用模型 Generate
  传入当前 context snapshot
  传入本轮可暴露给 LLM 的 tools
  累加 usage
  合并 reasoning
  检测 tool calls
  委托 ToolExecutor 执行工具
  把 assistant tool-call message 和 tool result messages 追加回 session
  工具调用后刷新 runtime prompt
  达到最大工具轮次后，做一次不带 tools 的最终生成
```

新增的接口对象：
```text
ContextProvider
  Snapshot(current, runtimePrompt)

ToolCatalog
  ToolsForSession(current, forceChatMode)

RuntimeInstructionProvider
  Prompt(ctx, current, input, forceChatMode)

ToolExecutor
  Execute(ctx, ToolExecutionCommand)
```

这里的思路类似 Spring Boot：
```text
ChatService
  像 Facade / Controller-facing Service

AgentLoopService
  像 Application Service / UseCase

ContextProvider / ToolCatalog / RuntimeInstructionProvider / ToolExecutor
  像 UseCase 依赖的接口
```

`ChatService` 现在只保留一个桥接对象：
```text
chatAgentLoopAdapter
  ContextProvider
  ToolCatalog
  RuntimeInstructionProvider
  ToolExecutor
```

这个 adapter 负责把现有的：
```text
contextSnapshot
llmToolsForSession
agentPrompt
callTools + persistToolRecordsAsync + persistAssetRecordsAsync
```

适配给 `AgentLoopService`。

已经完成：
- `runAgentLoop` 主体逻辑迁出 `ChatService`
- `appendReasoningPart` 迁到 `application/chat`
- `assistantToolCallMessage` 被删除，直接使用 domain message 构造函数
- 最大工具轮次从 service 层常量迁到 `application/chat.DefaultMaxToolRounds`
- 增加独立测试覆盖：
  - 无 tool calls 时直接返回
  - 有 tool calls 时执行工具并继续生成
  - 达到最大工具轮次后最终生成不再暴露 tools

### 46.2 application/tool.SelectionService

`ChatService.llmToolsForSession` 原来同时承担：
```text
读取 session permission mode
读取 session agent mode
应用 Plan 模式工具限制
应用 readonly/full/ask 权限模式过滤
调用 RegisterTools.LLMToolsByPermission
```

这部分现在迁到：
```text
application/tool.SelectionService
```

新增的接口对象：
```text
LLMToolCatalog
  LLMToolsByPermission(allow)

ToolModePolicy
  AllowsToolPermission(permission, agentMode, forceChatMode)
```

现在调用方向是：
```text
ChatService.llmToolsForSession
  -> application/tool.SelectionService
       -> LLMToolCatalog
       -> ToolModePolicy
```

已经完成：
- `ChatService` 不再直接实现 tool 暴露过滤规则
- `exposeToolForPermissionMode` 从 `ChatService` 删除
- `tooldef` 依赖从 `ChatService` 删除
- selection service 增加独立测试覆盖：
  - readonly 模式只暴露 read tool
  - plan 模式通过 mode policy 隐藏 write tool
  - force chat mode 会传给 mode policy，并允许计划执行步骤使用写工具

### 46.3 当前状态

现在 `ChatService` 比之前更接近 facade：

```text
ChatService
  接收 transport/CLI/mobile/remote 调用
  管理 session 生命周期
  调用 application/chat.AgentLoopService
  调用 application/tool.ExecutionService
  调用 application/tool.SelectionService
  调用 application/session.*Service
  调用 application/plan.*Service
```

但它还没有完全变薄。

仍然留在 `ChatService` 的重要职责：
```text
generateAssistantForSessionWithOptions
  task recorder
  model lookup
  auto compact
  assistant message 写入内存 session
  usage 写入内存 session
  plan capture
  assistant message 持久化
  current session cache 持久化
  ChatResponse 组装

chatAgentLoopAdapter
  仍然把 tool execution 和 tool records persistence 绑在一起

contextSnapshot / agentPrompt
  仍然作为 adapter helper 留在 service 文件中
```

下一步建议继续拆：
```text
application/chat
  AssistantGenerationService
    负责一次 assistant 回复的完整用例编排

application/chat
  SessionMessageWriter 或 ConversationAppender
    负责 assistant message / usage 写入内存 session

application/chat 或 port/event
  ToolExecutionSink / ToolRecordPublisher
    让 agent loop 不再通过 ChatService adapter 触发持久化副作用
```

另外还有一个中期问题：
```text
application/chat.AgentLoopService 目前仍然直接使用 *session.Session
```

这是为了小步迁移、保持风险可控。
长期更理想的方向是：
```text
domain/session
  Conversation / SessionState

application/chat
  只依赖 domain session 抽象

core/session
  逐步退化为内存 repository / manager
```

本轮验证：
```text
go test ./core/application/chat ./core/service
go test ./core/application/tool ./core/service
go test ./core/...
```

## 44. 第十三阶段已落地内容

本轮开始拆 `ChatService.callTools`。
这一阶段还没有把完整 tool execution loop 移走，而是先下沉三类纯逻辑：

新增：
```text
core/application/tool
  records.go
  permission_service.go
```

新增组件：
```text
ToolCallRecord
  构造 tool_call message record

ToolResultRecord
  构造 tool result message record

ToolResultMessage
  构造 domain/tool result message

AssetRecordFromToolResult
  从 share_file 工具结果中提取 asset record

PermissionService
  处理工具权限判断
  read 权限允许
  readonly 模式拒绝 write / execute
  full 模式允许
  ask 模式委托外部确认回调
  hook allow 可以放行
```

已经完成：
- `ChatService` 不再自己构造 tool call record
- `ChatService` 不再自己构造 tool result record
- `ChatService` 不再自己解析 `share_file` 结果生成 asset record
- `ChatService` 不再自己维护工具权限判断规则
- 删除了原来 `ChatService` 中的 `assetRecordFromToolResult`
- 删除了原来 `ChatService` 中的 `sharedAssetToolResult`
- tool record / asset extraction / permission policy 都有独立测试

当前 `callTools` 仍然保留：
```text
tool registry lookup
pre tool hook
stream OnToolCall / OnToolResult / OnToolAsk bridge
tool.Call(...)
post tool hook
messages / records / assets 的循环编排
```

这一步之后的结构更接近：
```text
ChatService
  -> application/tool.PermissionService
  -> application/tool record builders
  -> application/session.MessagePersistenceService
```

下一步建议：
```text
application/tool
  ExecutionService
  HookBridge / EventPublisher port
  ToolRegistry port
```

等这些 port 建好后，`ChatService.callTools` 才适合整体迁到 `application/tool.ExecutionService`。

本轮验证：
```text
go test ./core/application/tool ./core/service
go test ./core/...
```

## 45. 第十四阶段已落地内容

本轮继续拆 `ChatService.callTools`，把完整工具执行循环迁到 application 层。

新增到：
```text
core/application/tool
  execution_service.go
```

新增组件：
```text
ExecutionService
  负责执行一批 tool calls
  查找工具
  执行 pre tool hook
  应用 hook 改写后的 arguments
  触发 OnToolCall / OnToolResult / OnToolAsk 回调
  应用 PermissionService
  调用 tool.Call
  执行 post tool hook
  构造 tool result messages
  构造 message records
  提取 asset records

Registry
  只表达 GetTool(name) 能力

HookBridge
  只表达 BeforeToolUse / AfterToolUse 能力

ExecutionCallbacks
  把 stream 回调变成 application/tool 能理解的小接口
```

`ChatService` 新增了一个很薄的桥接：
```text
chatToolHookBridge
  hook.Manager -> application/tool.HookBridge
```

已经完成：
- `ChatService.callTools` 不再手写 tool 执行循环
- `ChatService.askPreToolUseHook` 被删除
- `ChatService.emitPostToolUseHook` 被删除
- `ChatService.allowToolCall` 被删除
- `sessionIDOrEmpty` 被删除
- hook decision 映射集中在 `hookDecisionToToolDecision`
- tool execution 增加独立测试，覆盖正常执行、hook deny、ask deny

现在调用方向是：
```text
ChatService
  -> application/tool.ExecutionService
       -> Registry
       -> HookBridge
       -> PermissionService
       -> record builders / asset extractor
```

这一步之后，`ChatService.callTools` 基本已经变成 facade 方法。

仍然留在 `ChatService` 的 tool 相关部分：
```text
llmToolsForSession
exposeToolForPermissionMode
assistantToolCallMessage
toolHookBridge adapter
```

下一步可以继续做：
```text
port/tool
  Registry
  Executor

adapter/tool/local
  local registry / executor

application/chat
  AgentLoopService
```

尤其是 `runAgentLoop` 仍然在 `ChatService` 中，里面还混着模型调用、tool loop、多轮工具结果追加和 usage 合并。

本轮验证：
```text
go test ./core/application/tool ./core/service
go test ./core/...
```

## 43. 第十二阶段已落地内容

本轮继续拆 `ChatService` 的消息持久化职责。

新增到：
```text
core/application/session
  message_persistence.go
```

新增组件：
```text
MessagePersistenceService
  保存用户消息
  保存助手消息
  保存工具消息记录
  保存资产记录

MessageSaver
  只表达 SaveMessage 能力

AssetSaver
  只表达 SaveAsset 能力

SaveSessionFunc
  保存 session 的函数接口

SaveSessionRecordFunc
  保存 session record 的函数接口
```

已经完成：
- `ChatService.saveAssistantMessage` 被删除
- `ChatService.saveMessage` 被删除
- `ChatService.saveAsset` 被删除
- `persistUserMessageAsync` 只负责异步调度，并委托 `MessagePersistenceService.SaveUserMessage`
- `persistAssistantMessageAsync` 只负责异步调度，并委托 `MessagePersistenceService.SaveAssistantMessage`
- `persistToolRecordsAsync` 只负责异步调度，并委托 `MessagePersistenceService.SaveMessageRecords`
- `persistAssetRecordsAsync` 只负责异步调度，并委托 `MessagePersistenceService.SaveAssetRecords`
- 用户消息、助手消息、批量工具记录、资产记录都有独立测试

为了尽量保持原行为，本轮保留了原来的容错方式：
```text
session 保存失败后仍然尝试保存 message
assistant message 保存失败后仍然尝试保存 session record
批量 records 中某条失败后仍继续保存后续 records
最后用 errors.Join 汇总错误
```

现在的调用方向是：
```text
ChatService
  -> application/session.MessagePersistenceService
  -> repository small interfaces / function ports
  -> adapter/persistence/mongo
```

这一步之后，`ChatService` 里与 message persistence 相关的方法已经明显变薄。

仍然可以继续拆的部分：
```text
callTools
  仍然负责 tool 执行、permission、hook、tool message 构造、tool record 构造、asset record 提取

assetRecordFromToolResult
  仍然留在 ChatService 文件中

runAgentLoop
  仍然包含 tool loop 和模型重试/收敛逻辑
```

下一步建议：
```text
application/tool
  ToolExecutionService
  ToolRecordBuilder
  AssetRecordExtractor

port/event
  Hook/EventPublisher
```

本轮验证：
```text
go test ./core/application/session ./core/service
go test ./core/...
```

## 42. 第十一阶段已落地内容

本轮继续把 session 相关的基础设施策略从 `ChatService` 中移出。

新增：
```text
core/application/session
  CurrentSessionService
```

职责：
```text
CurrentSessionService
  读取当前 session id 缓存
  保存当前 session id 缓存
  删除当前 session id 缓存
  统一处理 cache nil 的 no-op 行为
  统一持有 userID 和 TTL 策略
```

已经完成：
- `ChatService.cachedCurrentSession` 不再直接判断 cache nil
- `ChatService.saveCurrentSession` 不再直接调用 cache port
- 当前 session cache 的 TTL 从调用点细节变成 `CurrentSessionService` 的配置
- 增加独立测试覆盖 cache delegate 和 nil cache no-op

现在的调用方向是：
```text
ChatService
  -> application/session.CurrentSessionService
  -> port/cache.CurrentSessionCache
  -> adapter/cache/redis.CurrentSessionCache
```

这比之前更接近：
```text
Controller/Facade
  -> Application Service
  -> Port
  -> Adapter
```

本轮验证：
```text
go test ./core/application/session ./core/service
go test ./core/...
```

## 41. 第十阶段已落地内容

本轮继续压缩 `ChatService` 的 session 持久化和加载职责。

新增到：
```text
core/application/session
  persistence.go
  loader.go
```

新增组件：
```text
BuildSessionRecord
  根据 sessionID、model、title、当前内存 session、旧持久化记录构建待保存记录

PrepareSessionRecordForSave
  补齐保存前默认值
  处理 CreatedAt / UpdatedAt
  保留已有非默认 title

LoadService
  从内存 session store 读取 session
  如内存不存在，则从 repository 读取 session record 和 message records
  把持久化记录还原到内存 session store
```

本轮有意把接口收窄：
```text
SessionRecordGetter
  只需要 GetSession

MessageRecordLister
  只需要 ListMessages

MemoryStore
  只需要 session hydration 相关方法
```

这比直接依赖一个巨大的 repository/store 接口更接近 Spring Boot 里的小接口思路：
```text
需要什么能力，就依赖什么能力
不要为了一个方法注入完整 Store
```

已经完成：
- `ChatService.saveSession` 不再自己维护 agent mode / permission mode / context window / usage / plan 的合成规则
- `ChatService.saveSessionRecord` 不再自己维护 CreatedAt / UpdatedAt / title 保留逻辑
- `ChatService.ensureSessionInMemory` 改为委托 `application/session.LoadService`
- `ChatService.messagesFromStore` 被删除
- session persistence / loader 都有独立测试

仍然留在 `ChatService` 的部分：
```text
saveSession
saveSessionRecord
ensureSessionInMemory
memorySessionMessages
persistUserMessageAsync
persistAssistantMessageAsync
```

这些现在已经变薄，但还没有完全挪走。
下一步可以继续做：
```text
application/session
  PersistenceService
  MessagePersistenceService
  CurrentSessionService

port/event
  SessionEventPublisher
```

这样 `ChatService` 会进一步变成 facade，而不是保存和加载的实际实现者。

本轮验证：
```text
go test ./core/application/session ./core/service
go test ./core/...
```

## 47. 第十六阶段已落地内容

本轮继续拆 `generateAssistantForSessionWithOptions` 中的结果提交逻辑。

新增到：
```text
core/application/chat
  response_commit_service.go
```

新增组件：
```text
ResponseCommitService
  提交一次 assistant 生成结果
  写入 assistant message
  写入 usage
  在 Plan 模式下捕获 current plan
  把 current plan 写回 session memory store

ResponseMemoryStore
  AddAssistantMessageTo
  AddUsageTo
  SetCurrentPlanForSession

PlanCapturer
  Capture(sessionID, goal, content, now)
```

现在 `ChatService.generateAssistantForSessionWithOptions` 不再直接做：
```text
s.sessions.AddAssistantMessageTo
s.sessions.AddUsageTo
s.capturePlanForSession
```

而是委托：
```text
ChatService
  -> application/chat.ResponseCommitService
       -> ResponseMemoryStore
       -> PlanCapturer
```

已经完成：
- `capturePlanForSession` 从 `ChatService` 删除
- assistant message / usage 的内存提交逻辑下沉到 application/chat
- Plan 模式下的 plan capture 下沉到 application/chat
- `ChatService` 只保留 `responseCommitService()` 适配方法
- 增加独立测试覆盖：
  - Chat 模式只写 assistant message 和 usage，不捕获 plan
  - Plan 模式捕获 plan 并写回 session memory store
  - assistant message 写入失败时短路，不继续写 usage

这一步之后，`generateAssistantForSessionWithOptions` 剩余职责变成：
```text
task recorder
model lookup
runtime prompt 构造
auto compact
agent loop 调用
response commit 调用
assistant message 异步持久化
current session 异步缓存
ChatResponse 组装
```

下一步建议：
```text
application/chat
  AssistantGenerationService
    把一次 assistant 回复的完整 use case 移出 ChatService

application/chat
  ModelResolver 或 ModelProvider
    把 model lookup 从 ChatService 中抽出

application/chat
  ResponsePersistenceSink
    把 persistAssistantMessageAsync / persistCurrentSessionAsync 变成明确的输出端口
```

本轮验证：
```text
go test ./core/application/chat ./core/service
go test ./core/...
```

## 52. 第二十一阶段已落地内容

本轮继续拆 `ChatService.generateAssistantForSessionWithOptions` 中的 history task recorder 职责。

新增到：
```text
core/application/chat
  generation_task_service.go

core/adapter/history/taskrecorder
  task_recorder.go

core/adapter/id/uuid
  generator.go
```

新增组件：
```text
GenerationTaskService
  生成 requestID
  创建 task recorder
  把 recorder attach 到 context
  调用 AssistantGenerationService
  在结束时 save / close recorder

RequestIDGenerator
  NewRequestID()

GenerationTaskRecorderFactory
  NewTaskRecorder(record)

GenerationTaskRecorder
  Attach(ctx)
  Save(ctx)
  Close()
```

现在 `ChatService` 不再直接依赖：
```text
github.com/google/uuid
myai/core/history
history.NewTaskRecorder
history.WithTaskRecorder
```

而是通过：
```text
ChatService
  -> application/chat.GenerationTaskService
       -> RequestIDGenerator
       -> GenerationTaskRecorderFactory
       -> GenerationTaskHandler
  -> adapter/id/uuid.Generator
  -> adapter/history/taskrecorder.Factory
```

已经完成：
- `generateAssistantForSessionWithOptions` 不再直接创建 requestID
- `generateAssistantForSessionWithOptions` 不再直接创建、保存、关闭 history task recorder
- `ChatService` 只保留 `generationTaskService()` 适配构造
- UUID 生成从 service 层迁到 adapter 层
- history task recorder 具体实现从 service 层迁到 adapter 层
- 增加独立测试覆盖：
  - requestID / title / reason / sessionID 被传入 task record
  - recorder attach 后再调用 generation handler
  - generation 成功时 save / close recorder
  - generation 失败时仍 save / close recorder
  - save / close 错误通过 callback 报告，保持原日志策略可接入

这一步之后，`generateAssistantForSessionWithOptions` 已经非常薄：
```text
调用 GenerationTaskService
把 GenerationResponse 映射成 ChatResponse
```

下一步建议：
```text
application/chat
  ToolExecutionPersistenceSink
    把 tool records / asset records 的持久化从 chatAgentLoopAdapter 中拆出

application/chat
  RuntimePromptProvider
    把 agentPrompt / runtimeInstructionBuilder 下沉

application/chat
  ResponseMapper
    把 GenerationResponse -> ChatResponse 映射进一步收口
```

本轮验证：
```text
go test ./core/application/chat ./core/adapter/history/taskrecorder ./core/adapter/id/uuid ./core/service
go test ./core/...
```

## 50. 第十九阶段已落地内容

本轮继续收尾上下文压缩相关职责，把摘要生成 prompt 和消息转文本规则从 `ChatService` 迁到 application 层。

新增到：
```text
core/application/chat
  summary_service.go
```

新增组件：
```text
SummaryService
  实现 SummaryGenerator
  构造压缩摘要 prompt
  把 domain messages 转为 summary text
  调用 model.Generate
  校验 compact summary 非空
```

现在 `ChatService` 不再直接包含：
```text
summarizeMessages
messagesForSummary
writeSummaryLine
messageText
truncateForSummary
```

而是通过：
```text
chatGenerationAdapter.Summarize
  -> application/chat.SummaryService
```

已经完成：
- 摘要 prompt 从 service 层迁出
- message -> summary text 转换规则从 service 层迁出
- compact summary 空结果校验从 service 层迁出
- `ChatService` 中压缩相关的纯策略代码进一步减少
- 增加独立测试覆盖：
  - prompt 包含 existing summary、user、assistant tool call、tool result
  - system message 不进入 summary text
  - 空 compact input 返回错误
  - 模型返回空 summary 返回错误
  - 模型错误向上返回

这一步之后，`ChatService` 的压缩职责只剩：
```text
CompactSession 对外入口
compactService adapter 构造
chatGenerationAdapter.SaveSummary
```

下一步建议：
```text
application/context
  ContextSnapshotService
    把 contextSnapshot / messagesWithRuntimePrompt 迁出 ChatService

application/history
  TaskRecorderService
    把 history recorder 创建和关闭迁出 ChatService

application/chat
  ToolExecutionPersistenceSink
    把 tool records / asset records 的持久化从 chatAgentLoopAdapter 中拆出
```

本轮验证：
```text
go test ./core/application/chat ./core/service
go test ./core/...
```

## 49. 第十八阶段已落地内容

本轮继续拆 `ChatService` 中的上下文压缩职责，把原来的 `autoCompactIfNeeded / compactSession` 迁到 application 层。

新增到：
```text
core/application/chat
  compact_service.go
```

新增组件：
```text
CompactService
  CompactSession
  CompactIfNeeded
  判断是否达到压缩阈值
  计算 compactable messages
  调用 SummaryGenerator
  调用 CompactSummaryStore
  生成 CompactInfo

SummaryGenerator
  Summarize(ctx, model, existingSummary, messages)

CompactSummaryStore
  SaveSummary(ctx, current, summary, compactedMessages)

ErrNotEnoughHistoryToCompact
  表示没有足够新历史可压缩
```

现在 `ChatService` 不再直接实现：
```text
autoCompactIfNeeded
compactSession
compactReason
compactKeepChunks
errNotEnoughHistoryToCompact
```

而是通过：
```text
ChatService
  -> application/chat.CompactService
       -> ContextProvider
       -> SummaryGenerator
       -> CompactSummaryStore
```

`CompactSession(ctx, sessionID)` 这个对外入口仍然保留在 `ChatService`，但内部已经改成委托：
```text
s.compactService().CompactSession(ctx, current, model)
```

`AssistantGenerationService` 的 auto compact 也通过 adapter 委托：
```text
chatGenerationAdapter.CompactIfNeeded
  -> compactService().CompactIfNeeded
```

已经完成：
- `ChatService.autoCompactIfNeeded` 删除
- `ChatService.compactSession` 删除
- `ChatService.compactReason` 删除
- service 层压缩常量删除
- 历史不足错误迁到 `application/chat.ErrNotEnoughHistoryToCompact`
- `chatGenerationAdapter` 增加：
  - `Summarize`
  - `SaveSummary`
- 增加独立测试覆盖：
  - 手动压缩会调用 summarizer 并保存 summary / cutoff
  - 未达到阈值时不压缩
  - 达到阈值时返回 CompactInfo
  - 自动压缩遇到历史不足时静默跳过
  - 手动压缩遇到历史不足时返回 `ErrNotEnoughHistoryToCompact`

这一步之后，`ChatService` 剩余的压缩相关代码只剩：
```text
CompactSession 对外入口
compactService adapter 构造
summarizeMessages 具体摘要 prompt
messagesForSummary / messageText 等摘要文本转换 helper
```

下一步建议：
```text
application/chat
  SummaryService
    把 summarizeMessages / messagesForSummary 移出 ChatService

application/context
  ContextSnapshotService
    把 contextSnapshot / messagesWithRuntimePrompt 移出 ChatService

application/history
  TaskRecorderService
    把 history task recorder 创建和关闭移出 ChatService
```

本轮验证：
```text
go test ./core/application/chat ./core/service
go test ./core/...
```

## 48. 第十七阶段已落地内容

本轮继续拆 `generateAssistantForSessionWithOptions`，把“一次 assistant 回复生成”的主 use case 移到 application 层。

新增到：
```text
core/application/chat
  assistant_generation_service.go
```

新增组件：
```text
AssistantGenerationService
  根据 session model 查找模型
  构造 runtime prompt
  执行 auto compact
  调用 AgentRunner
  调用 ResponseCommitter
  调用 GenerationPersistence
  返回 GenerationResponse

ModelProvider
  GetModel(name)

Compactor
  CompactIfNeeded(ctx, current, model, runtimePrompt)

AgentRunner
  Run(ctx, RunCommand)

ResponseCommitter
  Commit(CommitCommand)

GenerationPersistence
  PersistAssistant(current, result)
  PersistCurrentSession(sessionID)
```

现在 `ChatService.generateAssistantForSessionWithOptions` 不再直接做：
```text
model lookup
runtime prompt 构造
auto compact
runAgentLoop
response commit
assistant message 持久化
current session cache 持久化
context info 组装
```

而是变成：
```text
生成 requestID
创建 task recorder
把 ctx 注入 history recorder
委托 application/chat.AssistantGenerationService
把 GenerationResponse 映射成 ChatResponse
```

现在调用方向是：
```text
ChatService
  -> application/chat.AssistantGenerationService
       -> ModelProvider
       -> RuntimeInstructionProvider
       -> ContextProvider
       -> Compactor
       -> AgentRunner
       -> ResponseCommitter
       -> GenerationPersistence
```

`ChatService` 只保留一个适配器：
```text
chatGenerationAdapter
  RuntimeInstructionProvider
  ContextProvider
  Compactor
  GenerationPersistence
```

已经完成：
- `service.CompactInfo` 改成 `application/chat.CompactInfo` 的类型别名
- model lookup 从 `ChatService` 主流程中移出
- auto compact 非致命错误处理移入 use case，通过 `OnCompactError` 回调保留原日志行为
- assistant response 持久化和 current session cache 持久化通过 `GenerationPersistence` 输出端口触发
- `ChatService` 仍保留 task recorder，因为这是当前 history 模块的横切上下文能力，下一步再单独拆更稳
- 增加独立测试覆盖：
  - 完整主流程编排
  - compact 错误非致命并继续生成
  - model not found 直接返回错误

这一步之后，`ChatService` 中的 assistant 生成主干已经明显变薄。

仍然留在 `ChatService` 的相关职责：
```text
history task recorder 创建和保存
autoCompactIfNeeded / compactSession 的具体实现
contextSnapshot / agentPrompt 的具体实现
chatGenerationAdapter / chatAgentLoopAdapter
```

下一步建议：
```text
application/chat
  GenerationTaskRecorder 或 TaskRecordingRunner
    把 history recorder 从 ChatService 迁走

application/context
  ContextSnapshotService
    把 runtime prompt 注入 + context snapshot 合并成独立对象

application/chat
  CompactService
    把 autoCompactIfNeeded / compactSession 迁出 ChatService
```

本轮验证：
```text
go test ./core/application/chat ./core/service
go test ./core/...
```

## 51. 第二十阶段已落地内容

本轮继续拆 `ChatService` 中的上下文快照职责，把 runtime prompt 注入和 context snapshot 构建迁到 application 层。

新增到：
```text
core/application/chat
  context_snapshot_service.go
```

新增组件：
```text
ContextSnapshotService
  Snapshot(current, runtimePrompt)
  MessagesWithRuntimePrompt(messages, runtimePrompt)
```

现在 `ChatService` 不再直接包含：
```text
contextSnapshot
messagesWithRuntimePrompt
```

而是通过：
```text
application/chat.ContextSnapshotService
  -> application/runtime.InsertRuntimeInstructions
  -> contextmgr.BuildSnapshot
```

已经完成：
- `chatGenerationAdapter.Snapshot` 直接委托 `ContextSnapshotService`
- `chatAgentLoopAdapter.Snapshot` 直接委托 `ContextSnapshotService`
- `ChatService.contextMessages` 改为调用 `ContextSnapshotService`
- `ChatService.contextInfoWithRuntimePrompt` 改为调用 `ContextSnapshotService`
- 原 service 层 runtime prompt 缓存前缀测试迁到 application/chat
- 增加 nil session 空快照测试
- 删除 `core/service/chat_test.go`，避免 service 层继续绑定上下文快照私有实现

这一步之后，runtime prompt 插入位置、cacheable prefix 稳定性、context window snapshot 构建，都归到 application/chat 的明确对象里。

`ChatService` 剩余相关职责：
```text
agentPrompt
runtimeInstructionBuilder
contextInfo 对外 helper
adapter 构造
```

下一步建议：
```text
application/chat
  RuntimePromptProvider 或 InstructionService adapter
    把 agentPrompt / runtimeInstructionBuilder 再下沉

application/history
  TaskRecorderService
    把 history task recorder 创建和关闭迁出 ChatService

application/chat
  ToolExecutionPersistenceSink
    把 tool records / asset records 的持久化从 chatAgentLoopAdapter 中拆出
```

本轮验证：
```text
go test ./core/application/chat ./core/service
go test ./core/...
```

## 53. 第二十二阶段已落地内容

本轮继续整理 `ChatService` 中的工具执行链路，把“执行工具”和“保存工具执行记录”拆成显式协作对象。

新增边界：
```text
core/application/chat
  ToolExecutionRecordSink
  ToolExecutionRecordCommand
```

新增适配器：
```text
core/adapter/persistence/toolrecords
  Recorder
```

已经完成：
- `ToolExecutionResult` 现在显式携带 `MessageRecords` 和 `AssetRecords`。
- `AgentLoopService` 在工具执行完成后，通过 `ToolExecutionRecordSink` 记录工具消息和资产记录。
- `chatAgentLoopAdapter.Execute` 只负责调用工具并返回结果，不再直接做持久化 side effect。
- 新增 `toolrecords.Recorder` 适配器，复用现有批量保存能力，并保留异步保存和错误回调。
- 删除 `ChatService` 中原来的 `persistToolRecordsAsync` / `persistAssetRecordsAsync` 职责。
- 增加测试覆盖：
  - 工具执行记录会被 sink 接收。
  - 无记录结果不会触发 sink。
  - 持久化适配器会保存 message/asset records，并回调保存错误。

当前效果：
```text
application/chat.AgentLoopService
  -> ToolExecutor: 执行工具
  -> ToolExecutionRecordSink: 记录工具执行产生的持久化数据

service.ChatService
  -> 只负责装配 tool executor 与 tool record sink
```

这一步之后，工具执行链路的隐藏副作用减少了：工具执行器是工具执行器，记录持久化是记录持久化，`ChatService` 不再亲自保存工具记录。

下一步建议：
```text
application/tool
  继续降低 ExecutionResult 对 repository record 的直接依赖

application/chat
  将 agentPrompt / runtimeInstructionBuilder 再下沉为更明确的 RuntimeInstructionProvider 适配器

service
  继续瘦身 ChatService 中的外部查询、session 状态 helper 和装配方法
```

本轮验证：
```text
go test ./core/application/chat ./core/adapter/persistence/toolrecords ./core/service
```

## 54. 第二十三阶段已落地内容

本轮继续把工具执行职责从 `ChatService` 迁出，新增真正的工具执行适配器。

新增适配器：
```text
core/adapter/tool/executor
  Executor
  HookBridge
```

已经完成：
- `toolexecutor.Executor` 实现 `application/chat.ToolExecutor`。
- `Executor` 负责把 `ToolExecutionCommand` 转为 `application/tool.ExecutionCommand`。
- stream 回调映射、permission ask 映射、permission mode 归一化迁出 `ChatService`。
- 工具 pre/post hook 桥接迁入 `toolexecutor.HookBridge`。
- `ChatService.agentLoopService()` 只负责装配：
  - `ToolExecutor: toolexecutor.Executor`
  - `ToolRecords: toolrecords.Recorder`
- 删除 `ChatService.callTools` facade。
- 删除 `chatAgentLoopAdapter.Execute`。
- 删除 service 层工具 hook bridge 和 `hookDecisionToToolDecision`。

当前效果：
```text
service.ChatService
  -> 装配 agent loop

adapter/tool/executor.Executor
  -> 执行工具
  -> 映射 stream callbacks
  -> 桥接 hook.Manager
  -> 返回 ToolExecutionResult

application/chat.AgentLoopService
  -> 编排模型生成、工具执行、工具记录 sink
```

这一步之后，`chatAgentLoopAdapter` 只剩上下文快照、工具目录、runtime prompt 三类协作方法，不再包含工具执行细节。

新增测试覆盖：
- executor 会映射 `OnToolAsk` 并返回工具执行记录。
- executor 会拒绝 nil session。
- hook bridge 会映射 pre/post tool hook。
- post hook 错误会通过回调报告，保持原来的日志式非中断行为。

下一步建议：
```text
application/runtime + adapter/runtime
  把 ChatService.agentPrompt / runtimeInstructionBuilder 迁成独立 RuntimeInstructionProvider 装配对象

application/tool
  继续降低 ExecutionResult 对 repository record 的直接依赖

service
  继续把 session 查询 helper、状态变更 helper 拆到 application/session
```

本轮验证：
```text
go test ./core/adapter/tool/executor ./core/application/chat ./core/service
```

## 55. 第二十四阶段已落地内容

本轮继续整理 runtime prompt 构建职责，把 `ChatService.agentPrompt` / `runtimeInstructionBuilder` 下沉到 application/runtime。

新增对象：
```text
core/application/runtime
  SessionPromptProvider
```

已经完成：
- `SessionPromptProvider` 根据 session 的 `AgentMode` 构建 runtime prompt。
- nil session 默认按 chat mode 处理。
- `AssistantGenerationService` 和 `AgentLoopService` 都直接注入 `SessionPromptProvider`。
- 删除 `ChatService.agentPrompt`。
- 删除 `ChatService.runtimeInstructionBuilder`。
- 删除 `ChatService.runtimeInstructions` 字段。
- `ChatService.contextMessages/contextInfo` 改为通过 provider 获取 runtime prompt。

当前效果：
```text
application/runtime.SessionPromptProvider
  -> RuntimeInstructionBuilder
  -> ModePolicy
  -> SkillPromptProvider

service.ChatService
  -> 只负责装配 SessionPromptProvider
```

新增测试覆盖：
- provider 会读取 session agent mode 并插入 plan prompt。
- provider 对 nil session 默认使用 chat mode，不插入 plan prompt。

下一步建议：
```text
adapter/tool/catalog
  把 ChatService.llmToolsForSession 迁出 service

application/chat
  让 AgentLoopService 直接依赖 ContextSnapshotService + ToolCatalog adapter，删除 chatAgentLoopAdapter
```

本轮验证：
```text
go test ./core/application/runtime ./core/application/chat ./core/service
```

## 56. 第二十五阶段已落地内容

本轮继续整理 agent loop 装配，删除 `chatAgentLoopAdapter`。

新增适配器：
```text
core/adapter/tool/catalog
  Catalog
```

已经完成：
- `toolcatalog.Catalog` 实现 `application/chat.ToolCatalog`。
- catalog adapter 内部复用 `application/tool.SelectionService`。
- 默认使用 `application/runtime.ModePolicy`，保持 plan mode 只暴露 read 工具的行为。
- `AgentLoopService` 现在直接装配：
  - `Contexts: application/chat.ContextSnapshotService`
  - `Tools: adapter/tool/catalog.Catalog`
  - `RuntimeInstructions: application/runtime.SessionPromptProvider`
  - `ToolExecutor: adapter/tool/executor.Executor`
  - `ToolRecords: adapter/persistence/toolrecords.Recorder`
- 删除 `ChatService.llmToolsForSession`。
- 删除 `ChatService.modePolicy` 字段。
- 删除 `chatAgentLoopAdapter`。
- 删除 service 层对 `application/tool.SelectionService` 的直接依赖。

当前效果：
```text
service.ChatService.agentLoopService()
  -> 只做装配

application/chat.AgentLoopService
  -> 编排 use case

adapter/tool/catalog.Catalog
  -> 工具目录过滤

adapter/tool/executor.Executor
  -> 工具执行

adapter/persistence/toolrecords.Recorder
  -> 工具执行记录持久化
```

新增测试覆盖：
- catalog adapter 默认 mode policy 会在 plan mode 下隐藏 write 工具。
- forced chat mode 会保留 write 工具。

下一步建议：
```text
application/chat 或 application/session
  继续把 contextMessages/contextInfo helper 从 ChatService 中迁出

application/session
  整理 ListSessionMessages / SessionHistoryMeta / ListSessionMessagesAfter 这类查询方法
```

本轮验证：
```text
go test ./core/adapter/tool/catalog ./core/application/chat ./core/service
```

## 57. 第二十六阶段已落地内容

本轮继续整理上下文查询职责，把 context info 计算迁到 application/chat。

新增对象：
```text
core/application/chat
  ContextQueryService
```

已经完成：
- `ContextQueryService.Info` 负责根据 session + runtime prompt provider 计算 `contextmgr.Info`。
- `ContextQueryService.InfoWithRuntimePrompt` 负责复用已有 runtime prompt 计算 context info。
- `AssistantGenerationService.contextInfo` 改为委托 `ContextQueryService`。
- `ChatService.contextInfo` 改为薄 facade，内部委托 `ContextQueryService`。
- 删除未使用的 `ChatService.contextMessages`。
- 删除 `ChatService.contextInfoWithRuntimePrompt`。

当前效果：
```text
application/chat.ContextQueryService
  -> ContextProvider
  -> RuntimeInstructionProvider

service.ChatService
  -> contextQueries() 装配 ContextSnapshotService + SessionPromptProvider
  -> contextInfo() 只做委托
```

新增测试覆盖：
- context info 会携带 runtime prompt 进入 snapshot。
- nil session 返回默认 context info。
- nil context provider 返回默认 context info。

下一步建议：
```text
application/session
  整理 ListSessionMessages / SessionHistoryMeta / ListSessionMessagesAfter 查询职责

adapter/persistence
  继续把 repository 细节封到查询适配器里
```

本轮验证：
```text
go test ./core/application/chat ./core/service
```

## 58. 第二十七阶段已落地内容

本轮继续整理 session 消息查询职责，把消息列表、历史 meta、增量分页查询迁到 application/session。

新增对象：
```text
core/application/session
  MessageQueryService
```

新增接口：
```text
MessageQueryStore
MemorySessionSource
```

已经完成：
- `MessageQueryService.ListMessages` 负责 store 优先、内存兜底的消息列表查询。
- `MessageQueryService.HistoryMeta` 负责历史 meta 查询；无 store 时从内存消息记录计算。
- `MessageQueryService.ListMessagesAfter` 负责增量分页查询；无 store 时复用 `MessagesAfterID`。
- `ChatService.ListSessionMessages` 只做 sessionID 清洗和委托。
- `ChatService.SessionHistoryMeta` 只做 sessionID 清洗和委托。
- `ChatService.ListSessionMessagesAfter` 只做 sessionID/afterMessageID 清洗和委托。
- 删除 `ChatService.memorySessionMessages`。

当前效果：
```text
service.ChatService
  -> messageQueries() 装配 Store + Memory
  -> 公开查询方法只保留 API facade 职责

application/session.MessageQueryService
  -> 查询策略
  -> store / memory fallback
```

新增测试覆盖：
- 无 store 时使用内存 session 消息。
- store 有消息时优先返回 store 消息。
- store 消息为空时 fallback 到内存消息。
- 无 store 时增量分页使用内存消息。
- store session 检查错误会向上返回。

下一步建议：
```text
application/session
  继续整理 ListSessionsWithDeleted / ListAssets 等查询方法

application/session
  整理 SwitchModel / SetPermissionMode / SetAgentMode / SetContextWindowK 这些状态变更 use case
```

本轮验证：
```text
go test ./core/application/session ./core/service
```

## 59. 第二十八阶段已落地内容

本轮继续整理 session 设置变更职责，把 model/permission/agent mode/context window 的核心更新逻辑迁到 application/session。

新增对象：
```text
core/application/session
  SettingsService
```

新增命令对象：
```text
SwitchModelCommand
SetPermissionModeCommand
SetAgentModeCommand
SetContextWindowCommand
```

已经完成：
- `SettingsService.SwitchModel` 负责 model id 空值校验、session 加载、内存 model 更新。
- `SettingsService.SetPermissionMode` 负责 permission mode 校验和内存状态更新。
- `SettingsService.SetAgentMode` 负责 agent mode 校验和内存状态更新。
- `SettingsService.SetContextWindow` 负责 context window 校验和内存状态更新。
- `SettingsService` 复用 `LoadService.EnsureInMemory`，不复制 session hydrate 逻辑。
- `ChatService.SwitchModelForSession` 保留 model registry 校验、持久化、hook。
- `ChatService.SetPermissionModeForSession` / `SetAgentModeForSession` / `SetContextWindowKForSession` 变成 application service 调用 + 持久化 + hook。

当前效果：
```text
service.ChatService
  -> API facade
  -> model registry 校验
  -> 持久化和 hook side effect

application/session.SettingsService
  -> session 设置校验
  -> ensure in memory
  -> 更新内存 session 状态
```

新增测试覆盖：
- 切换指定 session model。
- 无 sessionID 时只更新 current model。
- 设置 permission mode。
- 拒绝不支持的 permission mode。
- 设置 agent mode。
- 设置 context window。

下一步建议：
```text
application/session
  继续整理 Clear/Delete/Restore/New/Load 这些 session 生命周期 use case

adapter/hook
  把 session changed / skill reloaded hook 从 ChatService 中迁出为 hook event publisher
```

本轮验证：
```text
go test ./core/application/session ./core/service
```

## 60. 第二十九阶段已落地内容

本轮继续隔离 hook 事件发布职责，把 session/skill 事件构造迁出 `ChatService`。

新增适配器：
```text
core/adapter/hook/events
  Publisher
```

已经完成：
- `Publisher.SessionChanged` 负责构造并发布 `hook.EventSessionChanged`。
- `Publisher.SkillReloaded` 负责构造并发布 `hook.EventSkillReloaded`。
- hook 发布错误通过 `OnError` 回调报告，保持原来的非中断行为。
- `ChatService.emitSessionChangedHook` 改为委托 publisher。
- `ChatService.emitSkillReloadedHook` 改为委托 publisher。
- `ChatService` 不再直接构造 session changed / skill reloaded 的 `hook.Event`。

当前效果：
```text
service.ChatService
  -> hookEvents() 装配 Publisher
  -> 只表达业务事件：SessionChanged / SkillReloaded

adapter/hook/events.Publisher
  -> hook.Event 构造
  -> hook.Manager.Emit
  -> 错误回调
```

新增测试覆盖：
- session changed 事件字段正确。
- skill reloaded 事件字段正确。
- hook emit 错误会被包装并回调。

下一步建议：
```text
application/session
  继续拆 New/Load/Delete/Restore/Clear 生命周期 use case

service
  继续减少 ChatService 对 store/cache/hook 的直接操作
```

本轮验证：
```text
go test ./core/adapter/hook/events ./core/service
```

## 61. 第三十阶段已落地内容
本轮继续整理 session 生命周期职责，把 New/Load/Delete/Clear 的核心内存操作从 `ChatService` 迁移到 `application/session`。

新增应用服务：
```text
core/application/session
  LifecycleService
```

新增命令/结果对象：
```text
DeleteSessionCommand
DeleteSessionResult
```

已经完成：
- `LifecycleService.NewSession` 负责调用内存 session manager 创建会话，并返回当前会话。
- `LifecycleService.LoadSession` 复用 `LoadService.EnsureInMemory`，统一处理从内存或持久化仓库加载 session。
- `LifecycleService.DeleteSession` 负责 session id 归一化、判断是否删除当前会话、从内存移除会话。
- `LifecycleService.ClearCurrent` 负责清空当前内存会话并返回清空后的会话对象。
- `ChatService.NewSession` / `LoadSession` / `DeleteSession` / `ClearCurrent` 改为通过 `sessionLifecycle()` 委托生命周期逻辑。
- `ChatService` 继续保留 facade 级职责：持久化 current session、软删除持久化 session、清空持久化 messages、发布 hook。

当前效果：
```text
service.ChatService
  -> 生命周期 API facade
  -> store/cache/hook side effects
  -> application service 装配

application/session.LifecycleService
  -> session 生命周期 use case
  -> memory session 操作
  -> LoadService 复用
```

新增测试覆盖：
- 创建新 session 后成为当前 session。
- 加载 session 时设置 current。
- 删除当前 session 时返回 `DeletedCurrent=true`。
- 清空当前 session 时重置消息和摘要。

下一步建议：
```text
application/session
  继续整理 RestoreSession / ListSessionsWithDeleted / ListAssets 等查询或生命周期用例

service.ChatService
  继续减少对 store/cache 的直接访问，把持久化 side effect 收拢到 application 层或 adapter 边界
```

本轮验证：
```text
go test ./core/application/session ./core/service
go test ./core/...
```
