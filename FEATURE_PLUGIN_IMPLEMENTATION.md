# 本地插件系统实现说明

> 当前版本实现的是第一阶段本地工具插件：Agent 扫描工作区的 plugins 目录，读取每个插件的 plugin.json，启动 MCP 兼容的外部进程，并把插件工具注册到现有 Tool Registry。
>
> 当前插件运行在 PC Agent/Go 进程侧。Android Mobile 通过 Relay 使用插件，不在 APK 内加载第三方原生代码。

## 1. 当前能力

已实现：

- 插件目录扫描。
- plugin.json 解析、默认值填充和基础校验。
- 插件启用/禁用。
- 相对 entrypoint 和 working_dir 解析。
- MCP stdio JSON-RPC 进程启动。
- 插件工具注册到 RegisterTools。
- 插件失败状态记录。
- required 插件失败时阻止 Agent 启动。
- Agent 关闭时停止插件进程并注销工具。
- Reload 接口，可重新扫描和加载插件。

暂未实现：

- 插件市场和在线下载安装。
- 插件签名验证和自动更新。
- Skill、子智能体 Definition、Hook 的统一插件打包。
- Mobile/桌面端插件管理页面。
- 跨进程任务队列和插件级沙箱。

## 2. 目录结构

默认插件根目录是当前工作区下的 plugins：

~~~text
D:\Go_All\myai\plugins\
└── github\
    ├── plugin.json
    └── github-plugin.exe
~~~

可以通过配置修改：

~~~yaml
plugin:
  enabled: true
  root: plugins
~~~

相对路径会根据 Agent 的 workspace 解析。配置结构定义在 core/config/properties.go，加载逻辑在 core/config/loader.go。

## 3. plugin.json

当前第一版只支持 protocol=mcp：

~~~json
{
  "id": "github",
  "name": "GitHub Plugin",
  "version": "1.0.0",
  "protocol": "mcp",
  "entrypoint": "github-plugin.exe",
  "args": [],
  "env": {
    "GITHUB_TOKEN": "replace-me"
  },
  "permission": "read",
  "timeout_seconds": 30,
  "enabled": true,
  "required": false
}
~~~

字段：

| 字段 | 作用 |
|---|---|
| id | 唯一 ID；只允许字母、数字、点、下划线和短横线 |
| name | 用户可见名称；省略时使用 id |
| version | 插件版本；省略时为 0.1.0 |
| protocol/type | 当前必须是 mcp；type 可作为 protocol 的兼容别名 |
| entrypoint | 可执行文件或 PATH 命令；相对路径相对插件目录 |
| args | 启动参数 |
| env | 传递给子进程的环境变量 |
| working_dir | 进程工作目录；省略时使用插件目录 |
| permission | 插件工具默认权限：read、write、execute |
| timeout_seconds | MCP 请求超时，范围 1-3600 秒 |
| enabled | 是否加载；省略时默认为 true |
| required | 启动失败时是否阻止 Agent 启动；默认为 false |

## 4. 启动加载流程

初始化顺序位于 core/App.go：

~~~text
InitRegister
  -> 注册本地工具
InitMCP
  -> 加载 application.yaml 中的 MCP 服务
InitPlugins
  -> 扫描 plugin.root
  -> 读取 plugin.json
  -> 启动 enabled=true 的插件
  -> 通过 MCP initialize 握手
  -> tools/list 获取工具
  -> 注册到 toolRegister
InitChatService
  -> 模型看到本地工具、MCP 工具和插件工具
~~~

核心实现：

- core/plugin/manifest.go：清单读取、校验和 MCP ServerConfig 转换。
- core/plugin/manager.go：发现、加载、重载、状态和生命周期。
- core/App.go：初始化、关闭和 GetPluginManager。

## 5. 工具名称与调用

插件工具复用现有 MCP 适配器。对外名称格式为：

~~~text
mcp_<plugin_id>_<tool_name>
~~~

例如插件暴露 echo，模型看到的工具名是：

~~~text
mcp_github_create_issue
~~~

调用链：

~~~text
模型发起工具调用
  -> Tool Registry 查找 mcp_github_create_issue
  -> mcp.Tool.Call
  -> MCP Client 发送 tools/call
  -> 插件进程执行
  -> JSON-RPC 结果返回
  -> Hook、权限和工具记录沿用现有链路
~~~

因此插件不需要重新实现模型工具格式、权限回调和工具结果适配。

## 6. 插件状态和错误处理

Manager 对每个发现到的插件记录：

~~~go
type Info struct {
    Manifest  Manifest
    Directory string
    Status    Status
    Error     string
}
~~~

状态：

~~~text
loaded     已启动并注册工具
disabled   发现了清单，但 enabled=false
failed     清单无效或启动/握手失败
~~~

普通插件失败只记录 warning，不影响 Agent 启动。required=true 的插件失败会停止初始化，并回滚已经加载的插件运行时。

关闭时：

~~~text
Application.Close
  -> PluginManager.Close
  -> MCP Manager.Close
  -> 注销插件工具
  -> 关闭插件 stdin
  -> Kill 插件进程
~~~

## 7. 与现有 MCP、Skill 和子智能体的关系

当前插件系统复用了 MCP 的进程和工具协议：

- MCP：外部进程提供工具。
- Skill：本地 SKILL.md 提供模型指令。
- 子智能体：DefinitionRegistry 提供角色和能力模板。
- Plugin：负责把外部能力作为可安装、可启停的单元组织起来。

第一版只完成 Plugin -> MCP Tool 的桥接。后续可以在同一个 plugin.json 中增加：

~~~json
{
  "capabilities": {
    "tools": [],
    "skills": [],
    "subagents": [],
    "hooks": []
  }
}
~~~

这部分目前只是扩展位，不会被当前加载器执行。

## 8. Android 和 Relay

插件进程不在 Android 中启动：

~~~text
Android Mobile
  -> Relay WebSocket
  -> PC Agent
  -> Plugin Manager
  -> 本地插件进程
  -> 外部 API / 数据库 / 浏览器
~~~

所以普通工具插件：

- 不需要修改 Android 原生代码。
- 不需要重新构建 APK。
- 不需要在手机上保存第三方可执行文件。

如果未来插件需要提供 Android 原生页面或本地原生能力，则属于另一类 APK 扩展，需要单独的安全审核和重新构建流程。

## 9. 验证

插件包测试覆盖：

- MCP 插件启动和工具注册。
- 工具调用结果返回。
- Manager.Close 注销工具和停止运行时。
- disabled 插件不启动进程。
- 非法清单不会阻止其他普通插件继续加载。

建议执行：

~~~powershell
go test ./core/plugin ./core/mcp ./core/config ./core
go test ./...
git diff --check
~~~

## 10. 第二阶段：Agent/Relay/Mobile 管理

第二阶段已经接入远程管理协议，Android 不直接启动插件进程，而是通过 Relay 操作 PC Agent 上的 Plugin Manager。

支持的消息：

| 请求 | 响应 | 作用 |
|---|---|---|
| `plugin_list` | `plugin_list_result` | 查询插件目录、版本、协议和状态 |
| `plugin_reload` | `plugin_reload_result` | 重新扫描清单并重建 MCP 工具 |
| `plugin_enable` | `plugin_mutation_result` | 写回 `plugin.json.enabled=true` 并立即重载 |
| `plugin_disable` | `plugin_mutation_result` | 写回 `plugin.json.enabled=false` 并立即注销工具 |

消息链路：

~~~text
Mobile Settings / PluginPanel
  -> Relay client whitelist
  -> Agent handler
  -> PluginManager
  -> plugin.json + MCP process lifecycle
  -> mutation/list result
  -> Mobile plugin state
~~~

移动端设置新增“插件”页面，可执行刷新、重载和启用/禁用。插件失败时仍会展示错误，不会让普通插件阻塞 Agent 启动；`required=true` 的插件继续遵循启动失败即终止的规则。

## 11. 下一阶段建议

1. 增加插件删除、安装、版本更新和失败回滚。
2. 增加插件目录的安全边界、签名和 hash 校验。
3. 增加 Skill、Subagent Definition 和 Hook 的插件清单加载。
4. 增加插件市场或受控的远程仓库同步。
5. 对高风险插件接入更严格的 Workspace、网络和 Shell 权限。
