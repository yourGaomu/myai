# 外部文件与短链接 MCP 接入

## 1. 部署边界

MyAI 的文件上传、元数据查询、下载、知识库文件导入和公开分享统一通过主机侧 `files.Service` 调用名为 `short_links` 的 HTTP MCP。独立后端负责账号鉴权、素材存储、上传会话、链接规则和访问统计；MyAI 保留文件解析、会话资源记录、权限审批和 UI 协议。

必须另行部署支持本页文件契约的新版短链后端。旧服务地址不能直接当成新 MCP 地址。本仓库按已核对清单删除旧 UrlShortener 的代码、测试、模块、文档、示例和专用 Dockerfile，未删除整个目录。唯一已跟踪的私有运行 YAML 保持文件、Git 跟踪及索引状态不变，不读取、不删除、不执行 unstage；仅作为不参与当前构建、配置加载或运行的独立历史存档保留。其他私有运行配置与数据也不在删除范围。旧 HTTP 附件接口没有回退路径。MinIO 基础设施与知识库 `RAG_MINIO_*` 配置继续保留。

```text
手机选择文件 -> 已配对认证的 Relay WebSocket -> Agent 上传接收器
                                                   -> 主机 files.Service
工作区 share_file / read_asset / 知识库导入 ---------> 主机 files.Service
                                                   -> short_links MCP /api/mcp
                                                   -> 独立后端的素材与短链服务
下载字节 ------------------------------------------> MyAI 文档解析器
结构化资源元数据 ----------------------------------> 会话记录 / 手机资源卡片
```

## 2. 配置与停用行为

实际 URL 和机器 key 尚未确定时保持为空，模板默认停用：

```dotenv
# Agent 可访问、包含 /api/mcp 的真实端点；未知时留空。
SHORT_LINKS_MCP_URL=
# 在兼容新后端创建的机器 key；未知时留空，仅由 Agent 持有。
SHORT_LINKS_API_KEY=
```

URL 为空且 YAML 的 `short_links.mcp_url` 也为空时不自动注册。非空时自动配置名称为 `short_links`、`permission: execute`、`required: false`、超时 60 秒的服务，key 使用 `X-API-Key` 请求头。缺 key、鉴权失败、离线或初始化失败会跳过可选服务，普通聊天仍可启动；上传、读取、导入和分享在缺少服务或必需工具时明确失败。不能将跳过服务解释为文件能力可用。

生产 Compose 已显式传入以上两个变量。`.env` 的其他变量不会自动进入容器；自定义 MCP 凭据需使用 Agent 专用 overlay，见[部署安全迁移说明](DEPLOYMENT_SECURITY_MIGRATION.md)。空的环境覆盖通常沿用挂载 YAML，停用时还要清空 YAML 的对应 URL，或显式禁用同名 MCP。

也可显式声明服务；同名声明优先于自动配置，不重复注册，`disabled: true` 同样优先：

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

仅在 URL/key 已提供时启用该显式声明；未设置或空的 `${NAME}` 引用会报错。设置 `required: true` 会将初始化失败变成启动失败。`url` 与 stdio 的 `command` 互斥。容器与后端需有实际可达的网络路径，不推测部署域名或跨 Compose 网络连通性。

修改环境变量后需重建 Agent 容器，MCP 重载不能重新读取宿主 `.env`。重载候选失败时保留旧工具与客户端；在途上传持有同一 MCP 客户端快照，避免中途切换后端会话。

## 3. 后端工具契约

后端必须通过 `tools/list` 提供以下现有 10 个工具，并增加最后 3 个素材工具。表格是部署兼容性要求，不表示已在真实后端完成部署或验收。模型可见工具与主机内部文件传输工具分开管理；不要让模型生成、转述或搬运 Base64 文件内容。

| 远端工具 | 用途 |
| --- | --- |
| `create_url_shortlink` | 创建普通 URL 短链 |
| `create_burn_link` | 创建一次访问短链 |
| `upload_file_and_share` | 后端已有的上传并分享工具；主机正常上传不使用它隐式发布 |
| `get_link_info` | 查询短链 |
| `list_my_links` | 列出当前凭据的短链 |
| `begin_asset_upload` | 开始素材上传会话 |
| `upload_asset_part` | 上传有序素材分片 |
| `get_asset_upload_status` | 查询上传状态与进度 |
| `complete_asset_upload` | 完成上传，仅产生素材 |
| `cancel_asset_upload` | 取消上传 |
| `get_asset_info`（新增要求） | 按 `asset_id` 查询文件元数据，不要求先创建短链 |
| `get_asset_download_url`（新增要求） | 为主机下载素材字节提供临时下载地址 |
| `create_asset_shortlink`（新增要求） | 对已上传的 `asset_id` 单独创建分享短链 |

`tools/call` 的素材结果使用 `structuredContent` 传递机器可读元数据。至少保留稳定的 `asset_id` / AssetID、文件名、内容类型、字节数与校验信息；`business_id` 以及 `short_url`、`code`、有效期、访问上限按实际结果提供。普通上传没有公开分享字段也应能保存、回显并用于读取或导入。资源卡片不能通过扫描普通说明文本猜测素材身份。

## 4. 上传、读取与分享生命周期

1. **普通上传**：手机或主机传入文件，主机依次调用上传会话工具，完成后得到 AssetID 与结构化元数据。上传本身不创建公开短链，不消耗短链访问次数。后续消息和知识库导入引用 `asset_id`。
2. **手机传输**：手机只使用配对后的 Relay 认证通道，不保存后端机器 key，也不直连管理上传接口。协议使用 `asset_upload_begin/chunk/status/complete/cancel` 和 `asset_upload_result`；上传绑定经验证的用户、设备、客户端身份及会话，单凭上传 ID 或会话 ID 不能获得权限。
3. **大小范围与传输上限**：手机每块原始数据最多 **256 KiB**（262144 字节），整个上传 JSON 信封最多 **512 KiB**（524288 字节，含 Base64、元数据与协议字段）。手机与主机文件上传的单文件大小范围为 **1 字节至 50 MiB**（1–52428800 字节，含边界），明确拒绝空文件（0 字节）。这些不是远端 MCP 分片大小。
4. **主机到后端**：主机暂存、校验并重组小块，远端非末片为 **32 MiB** 原始数据，末片可以更小；后端 MCP 请求体上限为 **48 MiB**，包含 Base64 与 JSON 开销。不能把 32 MiB 原始分片当作 32 MiB 请求体，也不能把 50 MiB 文件塞进一次 MCP 请求。状态查询、offset、幂等键和完成回执用于恢复进度；未经状态核对不盲目重复有副作用的操作。
5. **读取与导入**：主机通过 `get_asset_info` / `get_asset_download_url` 获得元数据及临时地址，在大小、下载和路径策略约束内取回字节，再交给 **MyAI** 的解析器或文档处理器。短链后端不承担 PDF/文档解析，知识库自己的 RAG MinIO 存储继续独立管理。
6. **公开分享**：`share_file` 在明确用户批准后上传并调用 `create_asset_shortlink`；对已有素材可在批准后单独创建短链。分享结果才包含公开 `short_url` / `code`。普通手机上传和读取不是公开发布动作。
7. **失败**：无 MCP、工具缺失、鉴权失败、无效结构化结果、传输失败或超过上限均返回明确错误，不回退到旧服务，也不把未完成的文件标记成上传成功。

## 5. HTTP MCP 与权限

初始化执行 `initialize`、`notifications/initialized` 与 `tools/list`，调用使用 `tools/call`。HTTP 支持 JSON / SSE 响应，校验 JSON-RPC 版本、响应 ID 与响应大小；若后端返回会话 ID，后续请求携带会话和协议版本。客户端不跟随重定向，不对创建或上传进行无条件自动重试；取消、超时和关闭结束在途请求。错误日志不回显响应正文或凭据。

模型工具调用仍经过现有执行器的权限判断、Hook、执行记录与资源保存。上传分片、Base64 和临时下载地址属于主机文件传输职责；公开创建链接需要明确批准。当前接口不实现 OAuth、独立 GET SSE 监听或服务端主动调用。

## 6. 升级与验证边界

同时升级 Agent、Relay、手机协议及独立后端，确认后端发现到完整工具集后再启用文件能力。旧容器不会因仓库删除源码或修改 Compose 自动停止，现场容器和数据由部署维护者按其维护流程处理。新版后端尚无确认的 URL/key 时，保持停用并接受文件能力明确报错，不填写猜测值。

本页描述迁移契约与预期行为，不宣称本轮 Go、手机、Docker 或真实后端连通性测试已经通过。最终验证由主协调方记录在 [MCP-only 文件迁移记录](MCP_ONLY_FILE_MIGRATION_20261118.md)。历史评估和修复文档保留其原时点含义。
