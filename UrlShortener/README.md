# MyAI URL Shortener

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

```powershell
cd D:\Go_All\myai\UrlShortener
go run .\cmd\url-shortener
```

默认监听：

```text
http://localhost:18081
```

## Config

```env
URL_SHORTENER_ADDR=:18081
URL_SHORTENER_BASE_URL=http://localhost:18081
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
URL_SHORTENER_MINIO_ACCESS_KEY=myaiadmin
URL_SHORTENER_MINIO_SECRET_KEY=myaiadmin123456
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
  -ContentType application/json `
  -Body '{"url":"https://example.com/image.png","ttl_seconds":3600,"max_visits":10}'
```

## Create File Link

```powershell
curl.exe -X POST http://localhost:18081/api/assets `
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
