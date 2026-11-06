# MyAI URL Shortener

> 归档说明：MyAI 已改为集成独立短链后端的 MCP 接口，接入说明见 `../doc/FEATURE_SHORT_LINKS_MCP_INTEGRATION.md`。
> 本目录仅保留旧实现供参考，不再由 MyAI 的镜像工作流、生产 Compose 和更新脚本构建或部署。以下内容描述旧服务。

独立短链接服务，用于普通 URL、MinIO 文件、图片和后续 Artifact 的访问网关。

## Features

- `POST /api/links` 创建普通 URL 短链接
- `POST /api/assets` 上传文件到 MinIO 并创建文件短链接
- `GET /s/{code}` 访问短链接
- `GET /api/links/{code}` 查看短链接详情
- `GET /api/links` 查看短链接列表
- `DELETE /api/links/{code}` 逻辑删除短链接
- 支持 Memory/Mongo 存储切换
- 支持 TTL 和最大访问次数

## Run

启动前必须配置非空 `URL_SHORTENER_API_KEY`（或 YAML 的 `server.api_key`）。请使用独立生成的私有随机密钥，不要提交真实值；空值、纯空白或包含空格、制表符、换行的配置会在连接存储和监听端口前令启动失败，错误信息不包含密钥。显式设为空的 API key 环境变量会覆盖 YAML 值并拒绝启动。

```powershell
cd D:\Go_All\myai\UrlShortener
go run .\cmd\url-shortener
```

默认监听：

```text
http://localhost:18081
```

## Config

配置键可通过环境变量或 YAML 提供，环境变量优先。命令行 `--config`、`URL_SHORTENER_CONFIG` 可指定 YAML；未指定时会查找工作目录的 `resource/application.yaml` 或 `.yml`。`.env.example` 是模板，服务不会自动加载 `.env`，需由运行环境注入变量。

新增安全配置：

| 环境变量 | YAML 键 | 要求 / 默认值 |
| --- | --- | --- |
| `URL_SHORTENER_API_KEY` | `server.api_key` | 必填，无默认密钥 |
| `URL_SHORTENER_MAX_UPLOAD_BYTES` | `server.max_upload_bytes` | 正整数字节数，默认 `20971520`（20 MiB） |

上传上限针对整个 `POST /api/assets` 请求体，包括 multipart 边界、表单字段、所有文件和尾部内容，不是单个文件大小。超过上限返回 `413`，不写入 MinIO 或短链存储；无 Content-Length / 分块上传同样受限。设置为零、负数或无效数值会启动失败，不能用于关闭限制。合法范围内的 multipart 请求字段和响应格式保持兼容。

所有 `/api/links` 创建、列表、详情、删除接口与 `/api/assets` 上传都要求 `Authorization: Bearer <API key>`。缺失、错误、空 token 或重复 Authorization 头返回 `401` 和通用错误；API key 使用固定长度摘要做恒时比较，不写入响应或请求日志。`GET /health`、`GET /s/{code}` 继续公开，短链访问仍受 TTL 和访问次数限制；CORS OPTIONS 预检不执行管理操作，也无需密钥。

客户端迁移：手机直传 `mobile/src/utils/assetUpload.ts` 当前未发送 Authorization，启用本次修复后会收到 `401`，必须在手机端单独增加认证流程或迁移到受认证后端转发上传，不能通过放开上传鉴权恢复兼容。旧 `core/asset` 客户端的上传和详情查询也需要携带相同 Bearer 认证。本次仅修复旧服务，未修改这些客户端；公开短链打开方式不变。凭据经 HTTPS 传输，不应将共享管理密钥硬编码到手机应用。

以下仅为配置模板，API key 和对象存储凭据需由部署环境设置：

```env
URL_SHORTENER_ADDR=:18081
URL_SHORTENER_BASE_URL=http://localhost:18081
URL_SHORTENER_API_KEY=
URL_SHORTENER_MAX_UPLOAD_BYTES=20971520
URL_SHORTENER_DEFAULT_TTL_SECONDS=86400

URL_SHORTENER_STORE=mongo
URL_SHORTENER_MONGO_URI=mongodb://localhost:27017
URL_SHORTENER_MONGO_DATABASE=myai
URL_SHORTENER_MONGO_COLLECTION=short_links

URL_SHORTENER_REDIS_ENABLED=true
URL_SHORTENER_REDIS_ADDR=localhost:6379
URL_SHORTENER_REDIS_PASSWORD=
URL_SHORTENER_REDIS_DB=0
URL_SHORTENER_REDIS_PREFIX=myai:url-shortener
URL_SHORTENER_REDIS_CACHE_TTL_SECONDS=600

URL_SHORTENER_MINIO_ENDPOINT=localhost:9000
URL_SHORTENER_MINIO_ACCESS_KEY=
URL_SHORTENER_MINIO_SECRET_KEY=
URL_SHORTENER_MINIO_BUCKET=myai-assets
URL_SHORTENER_MINIO_USE_SSL=false
URL_SHORTENER_MINIO_ENSURE_BUCKET=true
URL_SHORTENER_OBJECT_URL_TTL_SECONDS=3600
```

## Create URL Link

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:18081/api/links `
  -Headers @{ Authorization = "Bearer $env:URL_SHORTENER_API_KEY" } `
  -ContentType application/json `
  -Body '{"url":"https://example.com/image.png","ttl_seconds":3600,"max_visits":10}'
```

## Create File Link

```powershell
curl.exe -X POST http://localhost:18081/api/assets `
  -H "Authorization: Bearer $env:URL_SHORTENER_API_KEY" `
  -F "file=@D:\tmp\demo.png" `
  -F "title=demo image" `
  -F "ttl_seconds=86400" `
  -F "max_visits=10"
```

返回：

```json
{
  "code": "abc12345",
  "short_url": "http://localhost:18081/s/abc12345",
  "bucket": "myai-assets",
  "object_key": "uploads/2026/07/05/xxx-demo.png",
  "file_name": "demo.png",
  "content_type": "image/png",
  "size": 12345,
  "expires_at": "2026-07-06T12:00:00Z"
}
```

访问文件短链接时，服务会现场生成 MinIO presigned URL，然后返回 302 跳转。

## Deployment notes

- 为仍在运行的旧服务单独注入 API key 和上传上限，协调管理客户端与手机上传的认证迁移后再发布；反向代理的请求体限制应与服务限制协调。
- 根 `.gitignore` 已忽略各子项目 `resource/application.yaml`、`.env` 和 `.env.*`，保留 `.env.example` 与 `resource/application.example.yaml`。已跟踪配置不会因新增忽略规则退出版本管理。
- `UrlShortener/resource/application.yaml` 仍是已跟踪文件；本次未读取或修改它，也未删除索引或重写历史。部署负责人需另行核对已跟踪配置中的凭据、必要时轮换，并安排配置迁移与版本管理清理。

## Redis Cache

Redis 缓存是 Store 层的装饰器：

```text
Service -> CachedStore -> MongoStore
```

Redis 使用 Hash 保存短链运行态数据，包括：

```text
code / kind / url / visits / max_visits / expires_at / is_deleted / object_bucket / object_key
```

读取单条短链时会优先查 Redis，未命中时会先尝试获取 Redis 分布式锁：

```text
myai:url-shortener:lock:link:{code}
```

拿到锁的请求负责查询 Mongo 并回填 Redis；没拿到锁的请求会短暂等待 Redis 回填，避免高并发同时打穿 Mongo。

访问短链时，`CachedStore.IncrementVisits` 会通过 Redis Lua 脚本原子完成：

```text
检查是否存在
检查是否逻辑删除
检查是否过期
检查 max_visits 是否超限
visits + 1
```

Lua 成功后，请求会把 code 放入内存队列并立即返回。后台 goroutine 会按时间窗口聚合访问次数，再批量 `$inc` 到 Mongo：

```text
abc123 +12
def456 +3
```

如果队列满了，会退回同步写 Mongo，优先保证访问次数不丢。删除短链时会自动清理对应 Redis key。
