# 工具执行功能实现说明

> 本文解释模型返回 ToolCalls 后，MyAI 如何查找工具、执行 Hook 和权限判断、调用本地或 MCP 工具、记录结果，再让模型继续生成。模型请求本身见 [模型生成功能实现](FEATURE_MODEL_GENERATION_IMPLEMENTATION.md)，远程消息见 [普通聊天消息功能实现](FEATURE_CHAT_MESSAGE_IMPLEMENTATION.md)。

## 1. 功能目标

```text
模型返回 ToolCalls
-> 查找统一 Tool Registry
-> PreToolUse Hook
-> 权限判断/手机审批
-> Tool.Call
-> PostToolUse Hook
-> 流式发送 ToolResult
-> 保存执行记录和 Asset
-> ToolCall/ToolResult 写入 Session
-> 下一轮模型读取结果
```

工具执行不是模型 SDK 自动完成的。模型只返回“想调用什么”，真正副作用由应用代码控制。

## 2. 核心对象

| 对象 | 类型 | 作用 |
|---|---|---|
| `tool.Tool` | interface | 所有本地和 MCP 工具统一契约 |
| `RegisterTools` | Registry struct | 按 source 注册、去重和查找工具 |
| `Catalog` | Adapter | 生成模型可见的 Tool schema |
| `SelectionService` | Application Service | 按 Chat/Plan 模式过滤工具 |
| `adapter/tool/executor.Executor` | Adapter | generation 命令映射为 tool 命令 |
| `ExecutionService` | Application Service | Hook、权限、调用、结果编排 |
| `PermissionService` | Application Service | 权限决策 |
| `HookBridge` | Adapter | tool 事件映射到 hook.Manager |
| `TaskRecorder` | Context 能力 | 记录文件修改前后快照 |
| `ToolRecords.Recorder` | Persistence Adapter | 保存 ToolCall、ToolResult 和 Asset |

## 3. Tool 接口

```go
type Tool interface {
	Name() string
	Description() string
	Schema() any
	Permission() Permission
	Call(ctx context.Context, args json.RawMessage) (ToolOutput, error)
}
```

对应 Java：接口类定义能力，实现类提供具体行为。Go 使用隐式实现，不写 `implements Tool`。

| 方法 | 用途 |
|---|---|
| `Name` | Registry key 和模型函数名 |
| `Description` | 告诉模型何时使用 |
| `Schema` | JSON Schema 参数定义 |
| `Permission` | read/write/execute 分类 |
| `Call` | 实际执行 |

## 4. 权限分类

```go
const (
	PermissionRead    Permission = "read"
	PermissionWrite   Permission = "write"
	PermissionExecute Permission = "execute"
)
```

未知或空权限会被规范为 `read`。因此新增有副作用工具时必须显式返回 write 或 execute。

示例：

```text
list_files/read_file/search_files/read_asset -> read
write_file/edit_file/share_file -> write
shell/install_skill -> execute
```

## 5. 本地工具如何注册

`Application.InitRegister` 创建 Registry 并注册：

```text
list_files
read_file
search_files
write_file
edit_file
shell
install_skill
read_asset（Asset 已配置）
share_file（Asset 已配置）
```

```go
tools.RegisterSource("local", localTools)
```

这些对象通常持有 workspace、Sandbox、Asset Client、Skill Manager 或 Hook Manager。

## 6. MCP 工具如何注册

```text
Application.InitMCP
-> Manager.RegisterAll
-> Client.Start
-> Client.ListTools
-> NewToolWithName
-> registry.RegisterSource("mcp:<server>", tools)
```

MCP 工具被包装成同一个 `Tool` 接口，因此后续 Catalog、权限、Hook、流式事件和记录流程完全复用。

暴露名称包含 server 前缀，并限制冲突和长度。required MCP 启动失败会阻止应用启动；可选 MCP 只记录 warning。

## 7. Registry 内部结构

```go
type RegisterTools struct {
	sources   map[string]map[string]Tool
	flatTools []Tool
	flatMap   map[string]Tool
}
```

`RegisterSource` 会替换整个 source 后重新构建扁平视图。优先顺序：

```text
local
-> 其他 source 按名称排序
```

同名时先加入的工具生效，因此本地工具优先于 MCP 同名工具。

Registry 使用 RWMutex，支持运行中刷新 source。

## 8. 模型看到的工具 DTO

`LLMToolsByPermission` 把 Tool 转成：

```go
modelport.Tool{
	Type: "function",
	Function: &FunctionDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Schema(),
	},
}
```

模型只看到定义，不持有实际 `Call` 方法。执行时再按名称回 Registry 取实现对象。

## 9. 模式如何筛选工具

调用：

```text
AgentLoopService.toolsForSession
-> Catalog.ToolsForSession
-> SelectionService.ToolsForSession
-> ModePolicy
```

行为：

| 场景 | 可见工具 |
|---|---|
| Chat 模式 | 根据 Session PermissionMode 选择 |
| Plan 生成阶段 | 只读工具 |
| Plan 执行 `ForceChatMode=true` | 恢复 Chat 工具选择 |

Plan 的详细语义见 [Plan 模式功能实现](FEATURE_PLAN_MODE_IMPLEMENTATION.md)。

## 10. PermissionMode 与工具可见性

Session 支持：

```text
readonly
ask
full
```

readonly 通常只把 read 工具暴露给模型；ask/full 可暴露有副作用工具。即便工具出现在模型请求中，执行阶段仍会再次做权限判断，形成双层保护。

## 11. ToolCall DTO

模型返回：

```go
type ToolCall struct {
	ID        string
	Type      string
	Name      string
	Arguments string
}
```

示例：

```json
{
  "id": "call_1",
  "type": "function",
  "name": "read_file",
  "arguments": "{\"path\":\"main.go\"}"
}
```

`Arguments` 保留 JSON 字符串，直到 Tool.Call 前再作为 `json.RawMessage` 解码。

## 12. 从 AgentLoop 进入工具层

```go
toolResult, err := s.executeTools(ctx, generationcommand.ToolExecution{
	Session: command.Session,
	Calls: result.ToolCalls,
	Stream: command.Stream,
	RequestID: command.RequestID,
})
```

这里的 RequestID 是 TaskService 内部任务 ID，用于 Tool 记录和 Asset，不是手机 request ID。

## 13. Executor Adapter

`adapter/tool/executor.Executor` 把 generation 命令转换为 tool 应用命令：

```go
toolcommand.Execution{
	SessionID:      command.Session.ID,
	PermissionMode: NormalizePermissionMode(...),
	RequestID:      command.RequestID,
	WorkspaceRoot:  command.Session.WorkspaceRoot,
	Isolated:       command.Session.WorkspaceSandboxID != "",
	Calls:          command.Calls,
	Callbacks:      callbacksFromStream(command.Stream),
}
```

它还把 `ChatStreamHandler` 的 Tool 回调适配为应用层 callback，避免 tool 模块依赖 remote 或 llm 包。

## 14. ExecutionService 顺序

对每个 ToolCall：

```text
Registry.GetTool(name)
-> NormalizePermission
-> BeforeToolUse Hook
-> Hook 可重写 arguments
-> OnToolCall
-> 记录 ToolCall Entry
-> Hook deny? 生成 tool error
-> Plan 模式硬门禁
-> PermissionService.Allow
-> Allowed? Tool.Call
-> 标准化工具错误
-> OnToolResult
-> AfterToolUse Hook
-> 可选提取 SharedAsset
-> 构造 ToolResult Message
-> 记录 ToolResult Entry
```

同一批 Calls 按 Codex 读写锁并行：

- `read` 工具共享读锁，可以同时执行。
- `write` / `execute` 拿独占锁，彼此互斥，也要等当前读工具结束。
- 结果仍按原始 Calls 顺序写回 Session，模型看到的顺序不变。
- 单个工具查找失败或 Hook 预检失败只记录该 call 的 tool error，不再中止整批。

## 15. PreToolUse Hook

Hook 输入包含：

```text
SessionID
ToolName
ToolArguments
Permission
Reason = tool execution
```

结果可以：

- `allow`：允许 Hook 链继续，但不提升 Session 权限。
- `deny`：不执行工具，产生 tool error。
- `continue`：继续 Session 权限策略。
- 返回新的 `arguments`：覆盖模型原参数。

Hook 运行错误只让当前 call 变成 tool error，其它工具继续执行。

## 16. 权限决策矩阵

`PermissionService.Allow` 的优先级：

```text
Read tool   -> allow
Readonly    -> deny write/execute
Full        -> allow
Ask         -> OnRequest：安全操作自动过，危险操作才 OnToolAsk
```

Ask 模式对齐 Codex `OnRequest`：

- 工作区内 `write`（path 落在 workspace 内）自动允许。
- 只读安全命令自动允许，例如 `Get-Date`、`date`、`git status`。
- 隔离沙箱（`WorkspaceSandboxID` 非空）里的非危险命令自动允许。
- 工作区外写入、`rm`/`curl|iex` 等危险命令仍询问手机。

| Tool Permission | readonly | ask | full |
|---|---|---|---|
| read | allow | allow | allow |
| write | deny | 工作区内自动过，否则手机审批 | allow |
| execute | deny | 安全查询/隔离沙箱自动过，否则手机审批 | allow |

Hook 显式 allow 不可覆盖 Session 权限模式。Plan 生成阶段还会在该服务之前执行硬门禁：只有 read 工具可继续；执行已批准计划时 `ForceChatMode=true` 只解除 Plan 门禁，仍需通过 Session 权限。

## 17. 手机权限审批

调用链：

```text
PermissionService.Allow
-> stream.OnToolAsk
-> Agent.askToolPermission
-> permissionWaiters.register(remote request_id)
-> permission_ask
-> Mobile useRemoteMessageHandler
-> 用户点击 Allow/Deny
-> useChatActions.sendPermissionResult
-> permission_result（复用原 request_id）
-> Agent.handlePermissionResult
-> waiter.resolve
-> PermissionService 继续
```

等待最长 60 秒。超时、Context 取消、拒绝都会返回 false，工具不执行。

## 18. 为什么权限使用手机 request ID

权限弹窗属于正在运行的远程聊天请求。Agent 的 waiter map 使用原协议 `request_id`，让手机的 `permission_result` 唤醒正确生成 goroutine。

Relay 转发 `permission_result` 时不能覆盖原 `clients[request_id].RequestType=user_message`，否则最终 `assistant_done` 无法按聊天终态释放路由。

## 19. Tool.Call

权限允许后：

```go
toolOutput, toolErr = registeredTool.Call(ctx, []byte(call.Arguments))
```

具体 Tool 负责：

- JSON 参数解码和业务校验。
- workspace 边界校验。
- Context 取消。
- 实际文件、Shell、Asset 或 MCP 调用。
- 返回 `ToolOutput` 结构化结果。

统一对象：

```go
type ToolOutput struct {
	Content      string
	Status       ResultStatus
	ErrorCode    string
	ErrorMessage string
	Truncated    bool
}
```

执行层将 Go error、Hook 拒绝、Plan 门禁、权限拒绝、超时和取消映射为稳定状态及错误码，并作为 ToolResult 交给模型，而不是总让生成流程中止。

## 20. PostToolUse Hook

无论执行成功、工具报错或 Hook deny，都会尝试发送 PostToolUse 事件，包含 result 和 error。

Post Hook 错误只交给 `OnPostError` 记录日志，不修改工具结果，也不中止模型循环。

## 21. 流式事件

执行前：

```text
OnToolCall(name, arguments)
-> Agent TypeToolCall
-> 手机显示 tool_running
```

执行后：

```text
OnToolResult(ToolResultEvent{Name, Arguments, Output})
-> Agent TypeToolResult
-> 手机收到 status/error_code/error_message/truncated，并兼容 payload.error
```

手机端错误状态由结构化 `status` 决定，不依赖结果文本匹配；SQLite 历史缓存也保存这些字段。

## 22. ToolResult 如何回到模型

ExecutionService 为每个调用创建：

```go
domainmessage.ToolResultMessage(call, toolOutput)
```

AgentLoop 再追加：

```go
Session.Messages += ToolCallMessage(result.ToolCalls)
Session.Messages += toolResult.Messages
```

下一轮 Snapshot 经过 LangChainGo mapper，分别变成 AI ToolCall 和 ToolCallResponse。ToolCallResponse 内容是包含 `status/content/error_code/error_message/truncated` 的 JSON；`ToolCallID` 必须匹配，provider 才能关联。

## 23. 工具循环

```text
Round 1: 模型 -> read_file call
Tool: read_file -> 文件正文
Round 2: 模型读取正文 -> 可能 search_files call
Tool: search_files -> 搜索结果
Round 3: 模型输出最终回答
```

默认最多 6 个有工具轮次，之后执行一次无工具生成。

## 24. TaskRecorder

TaskService 在生成开始前把 Recorder 附加到 Context。文件型工具从 Context 获取 Recorder：

```text
执行前 SnapshotPath / SnapshotWorkspace
-> Tool 修改文件或执行 Shell
-> 执行后 RecordFileChange / RecordWorkspaceChanges
```

同一任务多次修改同一文件会合并为：

```text
第一次 before
+
最后一次 after
```

中间状态不单独保存。

## 25. 哪些工具记录文件快照

- `write_file`、`edit_file`：按目标路径记录。
- `shell`：通常记录整个 workspace 执行前后差异。
- 纯读取工具不创建文件变更。

Recorder 在任务结束时保存 SQLite 检查点，支持 History Diff 和 Revert。

## 26. Tool Execution Record

ExecutionService 生成两类 `ExecutionEntry`：

```text
ToolCallEntry
ToolResultEntry
```

每个 call 和 result 的 CreatedAt 相差纳秒，保证持久层排序稳定。Recorder 异步保存为 MessageRecord。

## 27. SharedAsset

当前只有成功的 `share_file` JSON 结果会被 `SharedAssetExtractor` 提取：

```text
session_id
internal request_id
tool_call_id
local path / file name / content type / size
short_url / code / expires_at
```

之后作为 AssetRecord 保存，手机可刷新 Session Asset 列表。

## 28. 本地工具示例：read_file

```text
Name = read_file
Permission = read
Schema requires path
Call -> cleanWorkspacePath -> os.ReadFile
```

因为是 read，无论 Session 为 readonly、ask 或 full，都不询问用户。

## 29. 本地工具示例：edit_file

```text
Name = edit_file
Permission = write
Call:
  decode path/old_text/new_text
  workspace path check
  snapshot before
  exact replacement
  write file
  record after
```

ask 模式下，工作区内的 `edit_file` 会自动执行；写到 workspace 外仍会暂停等待手机批准。

## 30. 本地工具示例：shell

```text
Name = shell
Permission = execute
Call -> Sandbox.Run
```

Sandbox 限制工作目录、超时、输出大小，并阻止部分明显危险命令。Shell 前后可扫描 workspace，记录非 Git 检查点。

ask 模式下 `Get-Date` / `date` / `git status` 等只读查询自动执行，不弹权限；删除、下载执行、安装类命令仍询问。隔离沙箱中的普通命令也可自动执行。

## 31. MCP Tool 执行

MCP Tool 的 `Call` 最终通过 MCP Client 发送 tools/call 请求。其余流程与本地工具一致：

```text
Registry lookup
-> Hook
-> Permission
-> MCP Tool.Call
-> ToolResult Message
-> Record
-> 下一轮模型
```

MCP server 自己的 Permission 配置映射为统一 read/write/execute。

## 32. 错误分类

会中止当前生成：

- Registry 不存在。
- 工具名称未注册。
- PreToolUse Hook 执行错误。
- Tool Executor 基础依赖错误。

作为 ToolResult 返回模型：

- Hook deny。
- Session 权限拒绝。
- 手机拒绝或超时。
- Tool.Call 返回 error。

异步记录失败：只写日志，不中止模型。

## 33. 取消

生成 Context 取消后：

- 权限 waiter 立即返回 false。
- 支持 Context 的 Tool 应停止。
- Shell Sandbox 使用 `exec.CommandContext` 终止子进程。
- AgentLoop 将后续模型/工具错误向上返回。

工具是否能快速取消取决于具体 Tool 是否正确检查或传递 Context。

## 34. 问题清理结果与剩余风险

已解决：

1. `ToolOutput`、Session ToolResult、Mongo MessageRecord、模型 ToolCallResponse 和手机协议已经贯通结构化状态。
2. `permissionWaiterRegistry` 按聊天 request ID 保存 FIFO waiter 队列，同一请求的多个审批不会相互覆盖。
3. readonly 现在是硬权限边界，Hook allow 不能绕过 write/execute 禁止。
4. `share_file` 已声明为 write 权限，readonly 和 Plan 生成阶段不会暴露该工具，ask 模式会进行审批。

仍需注意：

1. `Content` 的业务数据格式仍由具体工具约定；结构化信封只统一执行状态和错误元数据。
2. 全 workspace 快照在大型项目中成本较高。

## 35. 完整示例

用户：

```text
把 README 的标题改成 MyAI Server
```

ask 模式：

```text
Model -> edit_file ToolCall
ExecutionService -> Registry.GetTool
PreToolUse -> continue
OnToolCall -> 手机显示参数
PermissionService -> ask
Agent -> permission_ask
Mobile -> allowed=true
EditFile.Call -> 快照、替换、写入、记录
OnToolResult -> 手机显示结果
PostToolUse
ToolCall/ToolResult -> Session
Model round 2 -> 最终说明
TaskRecorder.Save -> SQLite checkpoint
ToolRecords.Recorder -> Mongo tool records
```

## 36. 推荐断点

| 文件 | 函数 | 观察内容 |
|---|---|---|
| `core/tool/register.go` | `RegisterSource/GetTool` | source、冲突和工具实例 |
| `selection_service.go` | `ToolsForSession` | 模式过滤结果 |
| `agent_loop_service.go` | `executeTools` | ToolCalls 和内部 RequestID |
| `adapter/tool/executor/executor.go` | `Execute` | DTO 映射和 callbacks |
| `application/tool/service/execution_service.go` | `Execute` | Hook、权限、输出 |
| `permission_service.go` | `Allow` | 决策分支 |
| `permission_handlers.go` | `askToolPermission` | waiter、timeout、ctx |
| 具体 local tool | `Call` | 参数、路径、Recorder |
| `task_recorder.go` | `addChange/Save` | 文件合并和检查点 |

## 37. 测试

重点测试：

```text
TestSelectionServiceFiltersReadonlyPermissionMode
TestSelectionServiceUsesModePolicyForPlanMode
TestPermissionServiceAllowsRead
TestPermissionServiceDeniesWriteInReadonlyMode
TestPermissionServiceAsksInAskMode
TestExecutionServiceHookAllowStillAsksForPermission
TestExecutionServicePlanModeBlocksWriteEvenWithFullPermission
TestExecutionServiceExecutesTool
TestExecutionServiceHonorsHookDeny
TestExecutionServiceHonorsAskDenial
TestExecutorMapsStreamCallbacksAndReturnsExecutionRecords
TestRecorderSavesToolExecutionRecords
TestPermissionWaiterRegistryResolvesAndUnregisters
TestAgentLoopServiceExecutesToolsAndContinuesGeneration
```

运行：

```powershell
go test ./core/application/tool ./core/adapter/tool/...
go test ./core/tool/local ./core/remote/agent
go test ./core/application/chat
```

## 38. 源码索引

```text
core/tool/
core/tool/local/
core/application/tool/
core/adapter/tool/
core/application/chat/generation/service/agent_loop_service.go
core/remote/agent/permission_handlers.go
core/remote/agent/permission_waiters.go
core/history/
core/adapter/history/taskrecorder/
core/adapter/persistence/toolrecords/
core/mcp/
```

## 39. 阅读检查

阅读后应能回答：工具如何注册、模型为什么只看到 Schema、Plan 为什么只见 read 工具、权限优先级是什么、手机审批如何唤醒 goroutine、ToolResult 如何进入下一轮、文件检查点何时创建、哪些错误交给模型而不是终止请求。
