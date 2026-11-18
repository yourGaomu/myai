# MCP-only 文件迁移记录（2026-11-18）

本记录描述已批准的代码与部署契约迁移，不作为线上部署或最终验收证明。历史评估、修复与验收文档保留其原时点含义；当前使用说明以[文件与短链接 MCP 接入](FEATURE_SHORT_LINKS_MCP_INTEGRATION.md)、[部署安全迁移说明](DEPLOYMENT_SECURITY_MIGRATION.md)和[架构指南](PROJECT_ARCHITECTURE_GUIDE.md)为准。

## 迁移契约

- 主机文件服务统一调用 `short_links` MCP，供上传、`share_file`、`read_asset` 与知识库素材导入使用；文件解析仍由 MyAI 完成。
- 普通上传只产生 AssetID 与 `structuredContent` 素材元数据，不隐式创建公开短链。聊天与导入引用 `asset_id`，公开分享另行明确批准。
- 手机使用配对认证的 Relay 通道：原始块最大 256 KiB，完整上传 JSON 信封最大 512 KiB，单文件大小范围为 1 字节至 50 MiB（1–52428800 字节，含边界），拒绝空文件（0 字节）；最小 1 字节依据已核对的后端 multipart 契约，默认上限保持 50 MiB。后端非末片为 32 MiB，MCP 请求体上限为 48 MiB，包含 Base64/JSON 开销；Agent 负责重组。
- 另行部署的兼容新版后端应提供现有 10 个工具及 `get_asset_info`、`get_asset_download_url`、`create_asset_shortlink`。这项要求不表示外部后端已在本次改动中部署完成。
- 实际 `SHORT_LINKS_MCP_URL` 和 `SHORT_LINKS_API_KEY` 未确认时留空并保持停用，机器 key 仅在 Agent 部署环境持有。缺少 MCP、凭据或必需工具时文件能力明确失败，无旧 HTTP 附件回退；普通聊天可继续启动。

## 本次部署与文档清理

更新 `compose.prod.yaml`、`.env.example`、`config/application.example.yaml`、安全脱敏的 `application.redacted.yaml` 与 `doc/deployment.env.annotated.example`，移除旧附件配置，说明 MCP 变量和默认停用状态；保留 RAG MinIO 配置及 MinIO 基础设施。同步更新上述三个当前指南及 `mobile/README.md`，手机端不再要求单独配置旧附件服务。

删除前核对 Git 跟踪清单并读取代表性入口、HTTP 路由、模块、测试和文档；使用逐文件删除，未递归删除目录、未暂存或提交。删除 `Dockerfile.url-shortener` 以及 `UrlShortener/` 中 26 个已跟踪的旧文件：16 个 Go 实现、6 个 Go 测试、`go.mod`、`go.sum`、`README.md`、`.env.example`。

唯一已跟踪的私有运行 YAML `UrlShortener/internal/shortener/resource/application.yml` 本次保持本地文件、Git 跟踪及索引状态不变，不读取、不删除；其不参与当前构建、配置加载或运行。`UrlShortener/` 目录未整体删除，忽略的私有配置与运行数据也不在删除范围。部署模板与文档清理未读取私有运行配置；主集成步骤另行备份并更新了已获授权的实际 `.env`，也补充了独立短链后端的 MCP 接口。没有连接线上服务或停止旧容器，部署现场容器与数据由维护者单独安排。

## 验证状态

已完成的集成行为包括：

- 文件工具和知识导入使用真实 `mcp.Manager`、MCP 文件适配器、文件源及知识导入服务，外部 MCP/对象存储使用本地模拟服务；验证上传、批准分享、asset ID 读取、解析、原始字节导入和请求头隔离。
- 手机上传协议使用真实 Agent WebSocket 读循环、MCP Manager、文件服务和文档源验证；Relay 鉴权及结果路由另有测试，没有宣称在同一测试中连接完整的真实 Relay 与手机设备。
- 上传成功但资源记录未被持久化接受时，状态查询不返回成功资产；record 重试复用原始身份，不再次上传。
- 上传在 begin 时捕获会话状态版本。清空、删除或恢复推进版本，旧上传完成或补写无法把清空前的资产重新附加；新的 begin 仍可正常工作。
- 默认分享次数为不限（省略或 `-1`），显式 `0` 保留后端的禁止访问语义，正数限制访问次数。
- 后端幂等 begin 的回归测试发现既有裸 UUID 传入公共 ID 查询的问题，已在 `short_links/back/internal/modules/assets/multipart.go` 修复并保留原测试。
- 普通短链及一次性短链使用后端支持的 `inline` 规则来源；`maxVisits` 保留 int64 精度，`expireDays` 拒绝小数、负数及溢出。链接列表也使用相同整数解析。
- 上传恢复表最多保留 256 个显式幂等键。已确认完成或取消/失败的回执保留 15 分钟后可回收；尚未确定结果的记录不因容量压力被静默删除。同一会话的临时完成失败可显式重试，已知会话不会重新 begin。该恢复表是进程内状态，不承诺硬崩溃后的外部副作用 exactly-once。
- 文件服务退出时取消操作，并在每条恢复记录自己的 gate 下释放 MCP 租约；并发 Close 等待同一轮清理，避免不同锁同时修改租约。应用关闭在依赖释放前调用该清理。

部署与文档范围的定向静态检查已完成：`git diff --check` 通过；`compose.prod.yaml`、`config/application.example.yaml`、`application.redacted.yaml` 三个安全模板 YAML 解析通过；核对当前模板和指南无旧附件配置、MCP 示例值为空、生命周期工具和上限齐全、RAG MinIO 内容及 MinIO 基础设施保持不变，并确认 26 个旧模块文件和专用 Dockerfile 已删除、私有 YAML 本地仍存在。检查未读取私有 YAML 或实际 `.env`。

最终验证在代码冻结后完成：

| 范围 | 命令 / 检查 | 结果 |
| --- | --- | --- |
| MyAI 全量 | `go test -mod=readonly -p 2 -json ./... -count=1 -timeout=180s` | 119 个包通过、0 失败；171 个包无测试；1834 个测试项通过、17 个测试项跳过（包含子测试） |
| MyAI 静态检查 | `go vet -mod=readonly ./...` | 通过 |
| MyAI 构建 | `go build -mod=readonly ./...`；Linux amd64 / CGO=0 交叉构建 | 通过；Linux 仅构建，未在 Linux 运行 |
| 独立短链后端 | 在 `D:/Go_All/short_links/back` 执行 `go test ./... -count=1` 和 `go build ./...` | 全部通过；包含新增文件工具、幂等 begin、普通短链和一次性短链回归 |
| 手机端 | `npm --prefix mobile run typecheck` | 通过 |
| 手机上传协议 | `node --test mobile/scripts/asset-upload.test.cjs` | 14/14 通过 |
| 格式与差异 | 84 个新增/修改 Go 文件的 `gofmt -l`；`git diff --check` | 通过 |
| 私有环境文件 | 仅核对变量名、引用及忽略状态，不输出实际值 | 40 个唯一变量，旧附件变量移除，RAG MinIO 保留；两个新 MCP 项为空 |

最终 Go JSON 记录：`C:/Users/mcymn/AppData/Local/Temp/myai-mcp-final-20261118.jsonl`。该次运行覆盖最终代码，随后再次普通全量回归也成功。

验证限制：Windows race 测试二进制在启动时退出 `0xc0000139`，未运行到测试；本机 Python 无法加载 `encodings`，真实 PDF Python 解析分支条件跳过，文本及错误回退已验证。原有需显式启用的真实文档处理器集成，以及部分 Windows 无权限创建符号链接的测试仍跳过。未进行手机真机、Docker 镜像发布或真实 MCP/数据库/对象存储的生产联调。

实际 MCP 地址与后台签发的机器 Key 尚未提供，当前 `.env` 的 `SHORT_LINKS_MCP_URL` / `SHORT_LINKS_API_KEY` 保持空值；模型地址和 API Key 也仍是用户提供的示例值，须按真实模型服务补全。新 MyAI、手机应用和短链后端需配套更新才能上线。此次未提交、推送、部署、停止旧容器或删除存储数据。
