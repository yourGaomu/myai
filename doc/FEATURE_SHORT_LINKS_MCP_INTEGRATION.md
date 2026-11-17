# 外部短链接 MCP 接入

## 1. 职责与调用链

1. 新短链后端由独立项目维护：`D:/Go_All/short_links/back`。它负责账号鉴权、链接规则、访问统计和 MinIO 素材存储。
2. MyAI 通过该项目的 `POST /api/mcp` 端点发现、调用工具，不引入其 Go 包，不重复维护短链业务逻辑。
3. 统一调用链如下：

```text
模型返回 ToolCall
  -> 原有 ToolExecutor（Hook / 权限审批 / 执行记录）
  -> mcp.Tool
  -> mcp.Client（HTTP JSON-RPC）
  -> 独立短链后端 /api/mcp
  -> 工具结果回到 AgentLoop，模型继续回答
```

## 2. 配置

1. 在 Agent 进程环境或生产 `.env` 中配置：

```dotenv
SHORT_LINKS_MCP_URL=https://你的短链域名/api/mcp
SHORT_LINKS_API_KEY=在短链后台创建的机器调用Key
```

1.1 URL 留空时不自动注册短链服务。配置 URL 后，自动注册为可选服务：缺少 Key、鉴权失败、握手失败或离线时记录警告并跳过该服务，Agent 仍可启动。恢复后可使用 MCP 重载入口重新接入。
1.2 Key 通过 `X-API-Key` 请求头发送，只保存在部署密钥中，不放进聊天消息、工具参数或仓库。
1.3 自动注册名称为 `short_links`，默认权限为 `execute`，单次请求超时 60 秒。查询类工具同样继承该服务权限；尚未按远端单个工具细分权限。
1.4 地址必须能被 Agent 访问。容器 DNS 只有加入对应网络才可解析；新项目当前的代理网络默认是 `web_net`，MyAI 默认是 `myai-shared`，二者不会自动互通。可使用可达的 HTTPS 域名，或由部署方显式配置共同网络。

2. 也可以显式写进 `application.yaml`：

```yaml
mcp:
  servers:
    - name: short_links
      url: "${SHORT_LINKS_MCP_URL}"
      headers:
        X-API-Key: "${SHORT_LINKS_API_KEY}"
      permission: execute
      timeout_seconds: 60
      required: false
```

2.1 `url` 与 `command` 互斥；配置 `command` 时继续走原有 stdio MCP。
2.2 显式配置同名服务器时优先使用该配置，不重复注册；`disabled: true` 也不会被自动配置覆盖。
2.3 `${NAME}` 在 HTTP 客户端初始化时从进程环境读取。未设置或空白的变量会报错。
2.4 运行中可使用现有 MCP 重载入口发布新配置；若任何候选服务失败，保留旧工具与旧客户端。旧轮次持有的快照在释放前仍能正常调用旧客户端。
2.5 修改容器环境变量需要重建 Agent 容器，单纯重载 MCP 无法读取外部修改的 `.env` 文件。
2.6 显式设置 `required: true` 可要求该服务启动成功；此时缺少 Key 或离线会阻止 Agent 启动。显式同名配置优先于自动配置，旧文件里的 `required: true` 需要由部署方调整。
2.7 生产 Compose 只透传明确列出的变量，已包含 `SHORT_LINKS_MCP_URL` 与 `SHORT_LINKS_API_KEY`；其他 HTTP MCP 请求头和 stdio MCP `env` 中引用的 `${NAME}` 需要在 Agent 专用 Compose overlay 中逐项加入 `environment`。`.env` 的任意键不会自动进入容器，操作示例见[部署安全迁移说明](DEPLOYMENT_SECURITY_MIGRATION.md)。

## 3. 新服务当前提供的工具

| 远端工具 | MyAI 工具名 | 功能 |
| --- | --- | --- |
| `create_url_shortlink` | `mcp_short_links_create_url_shortlink` | 创建普通短链 |
| `create_burn_link` | `mcp_short_links_create_burn_link` | 创建一次访问、24 小时有效的短链 |
| `upload_file_and_share` | `mcp_short_links_upload_file_and_share` | 上传 Base64 文件并创建下载短链 |
| `get_link_info` | `mcp_short_links_get_link_info` | 查询单条短链 |
| `list_my_links` | `mcp_short_links_list_my_links` | 查询当前凭据下的短链列表 |

1. 工具定义由 `tools/list` 动态发现，这张表描述接入时核对的新后端实现，MyAI 不硬编码工具列表。
2. 远端 `isError: true` 会转换成现有失败工具结果；HTTP 或 JSON-RPC 异常交给原有执行层处理。
3. 新后端 MCP 请求体上限是 10 MiB，Base64 本身有编码开销，不能沿用旧 `share_file` 的 50 MiB 上限。此工具当前接收完整 Base64，不是本地文件路径。

## 4. 传输与生命周期

1. 初始化执行 `initialize`、`notifications/initialized`，随后执行 `tools/list`；调用工具使用 `tools/call`。
2. POST 接收 JSON 或 SSE 响应，验证 JSON-RPC 版本、响应 ID 和响应大小。服务端若返回会话 ID，后续请求会携带会话 ID 与协商后的协议版本。
3. 客户端不会跟随 HTTP 重定向，也不会自动重试创建、上传等工具调用，以免重复产生副作用。
4. HTTP 错误日志仅包含状态，不回显错误响应正文；请求取消、超时、客户端关闭会结束在途请求。
5. 当前面向新后端的无状态 HTTP 工具接口，不实现 OAuth 流程、独立 GET SSE 监听、服务端主动发起调用或 SSE 断线续传。

## 5. 旧服务与文件功能边界

1. 原 `UrlShortener` 不再进入 MyAI 的镜像发布矩阵、Compose 服务列表和更新脚本。旧源码及原 HTTP 适配器保留供后续明确迁移范围时参考。
2. 默认示例的旧 `asset.shortener_base_url` 留空；生产 Compose 保留 `ASSET_SHORTENER_BASE_URL`、`ASSET_API_KEY` 的显式透传。需要 `share_file`、`read_asset` 或旧文件导入的部署应保留可达的旧服务地址，并使客户端 `ASSET_API_KEY` 与服务端 `URL_SHORTENER_API_KEY` / `server.api_key` 一致；禁用时将 URL 的环境变量和 YAML 值都留空，MCP 不会自动改写部署方文件。
3. 旧版移动端上传仍是 `POST /api/assets` 的 multipart 协议，`read_asset` 和知识库导入也仍依赖旧元数据/下载接口。它们没有在本次变成新服务的接口，不能只替换域名就认为兼容。
4. 新 MCP 暂无下载并解析文件的工具，也没有与 MyAI 资源卡片约定一致的结构化输出。当前结果按普通工具消息显示，不承诺旧资源卡片、移动端上传和知识库文件导入已迁移。
5. 若后续要统一文件能力，需要明确新 MCP 的下载/解析工具及结构化资源结果，或单独设计文件传输通道；不应让模型为了搬运大文件生成完整 Base64。
6. 修改 Compose 文件不会自动停止已经运行的旧容器。若部署现场仍有 `myai-url-shortener`，需在确认不再使用旧文件功能后手动停止；MyAI 更新脚本不会擅自删除该容器或数据。

## 6. 验证

1. `go test ./core/mcp ./core/config ./core/plugin`：验证 HTTP/stdio 工具注册、调用、配置隔离、失败重载回滚与旧轮次快照。
2. HTTP 测试覆盖 JSON/SSE、环境变量凭据、会话请求头、超时取消、关闭取消、重定向拦截与错误响应校验。
3. 本地自动化使用模拟服务，不连接生产数据库或真实短链账号。上线仍需配置实际可达 URL 和机器 Key 后进行连通性验证。
