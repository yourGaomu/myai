# 本地插件目录

Agent 启动时会扫描当前工作区下的 plugins 目录。每个直接子目录如果包含 plugin.json，就会被识别为一个本地插件。

当前第一版插件协议复用 MCP，插件本体是一个支持 MCP stdio JSON-RPC 的可执行程序或脚本。

目录示例：

~~~text
plugins/
└── github/
    ├── plugin.json
    └── github-plugin.exe
~~~

plugin.json 示例：

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

字段说明：

| 字段 | 说明 |
|---|---|
| id | 插件唯一 ID，只允许字母、数字、点、下划线和短横线 |
| name | 用户可见名称 |
| version | 插件版本 |
| protocol | 当前必须为 mcp |
| entrypoint | 可执行文件或 PATH 中的命令；相对路径相对插件目录 |
| args | 启动参数 |
| env | 传给插件进程的环境变量 |
| working_dir | 工作目录，省略时使用插件目录 |
| permission | 工具默认权限：read、write 或 execute |
| timeout_seconds | MCP 请求超时时间，范围 1-3600 秒 |
| enabled | 是否加载，省略时默认为 true |
| required | 启动失败时是否阻止 Agent 启动，默认 false |

插件加载后，其工具会注册到现有工具表，工具名格式为：

~~~text
mcp_<plugin_id>_<tool_name>
~~~

插件运行在 PC Agent/Go 进程侧。Android Mobile 通过 Relay 使用插件，不需要因为普通工具插件重新构建 APK。
