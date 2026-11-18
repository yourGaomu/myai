# 部署安全迁移说明

适用于从旧的 `compose.prod.yaml` 升级到显式环境变量、持久 history/trust 卷和外部短链 MCP 的 Linux Docker 部署。以下命令是供部署维护者审阅后执行的示例；编写本文时未连接线上服务或执行部署。

## 1. 先备份，再重建 Agent

旧镜像的 `HOME=/home/myai`。未设置 `XDG_CONFIG_HOME` 时，SQLite history 的旧位置为：

```text
/home/myai/.config/myai/workspaces/<workspace-hash>/history.db
```

新部署分别挂载两个 named volume：

| Compose 卷键 | 容器目录 | 用途 |
| --- | --- | --- |
| `agent-history` | `/var/lib/myai/history` | `workspaces/<workspace-hash>/history.db` 及 SQLite WAL/SHM |
| `agent-user-config` | `/home/myai/.config/myai` | 用户批准记录 `trust.json` 及其锁文件 |

镜像预先创建两个目录，属主为容器用户 `myai`（UID/GID `10001:10001`），用户配置目录权限为 `0700`。Docker 首次初始化空卷时继承镜像目录属主；已有卷的权限需单独核对，更新镜像不会重写已有卷的属主。工作区仍是宿主机 bind mount，宿主目录也必须允许 UID 10001 写入。

下面使用 `/opt/myai` 作为示例部署目录；按现场路径调整。备份包含工作区快照，应放在工作区之外的私有目录。不要启用 shell `set -x`，不要打印 `.env`、配置文件或数据库内容。

```sh
project_dir=/opt/myai
backup_dir=$(mktemp -d /var/tmp/myai-migration.XXXXXX)
chmod 700 "$backup_dir"

compose_prod() {
  docker compose --project-directory "$project_dir" \
    --env-file "$project_dir/.env" \
    --file "$project_dir/compose.prod.yaml" --profile agent "$@"
}

# 必须在 update-myai.sh / force-recreate / 删除旧容器之前执行。
compose_prod stop --timeout 45 agent

docker cp myai-agent:/home/myai/.config/myai/workspaces "$backup_dir/history-workspaces"
# 仅在旧版本已经存在用户批准文件时执行；没有此文件则在新容器中重新审阅批准。
# docker cp myai-agent:/home/myai/.config/myai/trust.json "$backup_dir/trust.json"
```

停止 Agent 后复制整个 `workspaces`，同时保留可能仍存在的 `history.db-wal` 和 `history.db-shm`；不要从运行中的进程仅复制 `history.db`。如果旧环境改过 `HOME`、`XDG_CONFIG_HOME` 或 history 目录，备份源必须使用旧环境的实际目录；不要用新镜像的目录推断旧容器。

本轮的敏感路径过滤与拒绝回传不会清洗旧数据库。旧 `file_changes` 的敏感正文 BLOB、WAL 和 SQLite 空闲页可能仍物理保留；baseline Reset 会过滤后重写记录，也不属于安全擦除，没有自动执行清洗或 `VACUUM`。将旧库、迁移备份和恢复备份都按秘密数据保护，优先轮换可能暴露过的凭据。是否另行清库或销毁备份应由数据所有者决定，本次迁移不自动删除历史。多文件回滚采用进程内补偿，不保证断电时原子恢复；失败后可能保留需要人工处理的恢复备份，仍须保密。

同样在更新 checkout 或移除旧容器前，通过私有文件复制备份部署配置及旧服务本地运行配置和数据，不要把内容放入终端日志。本次只按已核对清单清理旧代码、测试、模块、文档、示例和 Dockerfile，未删除整个目录。唯一已跟踪的私有运行 YAML 保持本地文件、Git 跟踪及索引状态不变，不读取、不删除、不执行 unstage；仅作为不参与当前构建、配置加载或运行的独立历史存档保留。忽略的运行配置与数据也保持原状。

## 2. 还原 history 与用户批准卷

先完成第 5 节的共享网络准备，并确认使用已包含新目录属主设置的 Agent 镜像。下面只创建新容器，尚不启动 Agent：

```sh
compose_prod config --quiet
compose_prod pull agent
compose_prod up --no-start --force-recreate --no-deps agent

# 仅查询 mount 名称和镜像 ID，不输出容器环境变量。
history_volume=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/myai/history"}}{{.Name}}{{end}}{{end}}' myai-agent)
user_config_volume=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/home/myai/.config/myai"}}{{.Name}}{{end}}{{end}}' myai-agent)
agent_image=$(docker inspect --format '{{.Image}}' myai-agent)
test -n "$history_volume"
test -n "$user_config_volume"

# 空目标检查防止覆盖已有 history；辅助容器断网、不加载服务环境变量。
docker run --rm --network none --user 0:0 --entrypoint /bin/sh \
  --mount "type=volume,source=$history_volume,target=/target" \
  --mount "type=bind,source=$backup_dir,target=/backup,readonly" \
  "$agent_image" -eu -c '
    test -z "$(find /target -mindepth 1 -maxdepth 1 -print -quit)"
    cp -a /backup/history-workspaces /target/workspaces
    chown -R 10001:10001 /target
  '
```

如果第 1 节备份了 `trust.json`，可在保持原 workspace 和声明路径的条件下恢复，随后用第 3 节的 `list` 复核：

```sh
docker run --rm --network none --user 0:0 --entrypoint /bin/sh \
  --mount "type=volume,source=$user_config_volume,target=/target" \
  --mount "type=bind,source=$backup_dir,target=/backup,readonly" \
  "$agent_image" -eu -c '
    test ! -e /target/trust.json
    cp /backup/trust.json /target/trust.json
    chown -R 10001:10001 /target
    chmod 700 /target
    chmod 600 /target/trust.json
  '
```

如果空目标检查失败，应停止并保留两个版本，选择经确认的备份或恢复到新的卷；不要通过清空已有卷来绕过检查。不要执行 `docker compose down -v`，这会删除持久卷。

History 路径中的 hash 是 `hex(SHA256(strings.ToLower(filepath.Clean(absoluteWorkspace)))[:16])`。旧、新容器都使用同一个 `/workspace` 时，目录 hash 不变，迁移整个 `workspaces` 即可；修改宿主 bind mount 的来源不会改变容器路径，但必须确保仍然是同一个工作区的数据。SQLite 表中也保存 workspace 绝对路径；如果从宿主原生 CLI 路径、另一个容器路径或不同 symlink 解析结果迁入，不能只重命名 hash 目录。当前没有自动重写数据库 workspace 字段的迁移命令：保留旧备份与旧路径，或由维护者设计并验证数据库迁移后再使用新路径。

## 3. 审阅并批准 hook、plugin 与 stdio MCP

用户批准记录位于工作区之外的 `/home/myai/.config/myai/trust.json`，由 `agent-user-config` 卷保留。首次部署没有批准记录；声明内容、路径或工作区改变后仍需重新审阅。HTTP MCP 不执行本地命令，不在 stdio MCP 的执行批准列表中。

下面的 `trust list` 仅列出类型、状态、完整 SHA256 和声明路径，不执行声明，也不显示参数或环境变量的值。维护者仍需在本地安全审阅实际声明文件。

```sh
compose_prod run --rm --no-deps agent trust hook list \
  --workspace /workspace --config /app/resource/application.yaml
compose_prod run --rm --no-deps agent trust plugin list \
  --workspace /workspace --config /app/resource/application.yaml --root /workspace/plugins
compose_prod run --rm --no-deps agent trust mcp list \
  --workspace /workspace --config /app/resource/application.yaml
```

若 `PLUGIN_ROOT` 改过，`--root` 必须与 Agent 实际使用的目录一致。批准只针对已人工审阅的完整 ID；不要自动批准 `list` 返回的全部声明。

```sh
reviewed_digest=replace-with-the-reviewed-full-sha256
compose_prod run --rm --no-deps agent trust mcp approve \
  --workspace /workspace --config /app/resource/application.yaml --id "$reviewed_digest"
```

将 `mcp` 换成 `hook` 或 `plugin` 执行相应批准，plugin 继续带实际 `--root`。撤销使用相同命令形式的 `revoke --id`。完成必要批准、凭据配置和网络准备后，再执行：

```sh
compose_prod up -d relay agent
compose_prod ps
```

保留迁移备份直至确认历史、权限和重建后批准状态正常。审批被持久化不表示工作区自身可以授予执行权限。

## 4. 显式环境变量与自定义 MCP overlay

生产 Compose 的 `.env` 仅用于插值，Relay 和 Agent 分别接收 `environment` 中列出的键。内置列表覆盖模型 provider/protocol/auth、生成容量、RAG Milvus/local/document processor/MinIO、插件开关、子 Agent、Skill 与 sandbox 配置。新增的可选覆盖默认空值，保持挂载 YAML 的设置；`false`、`0` 等非空值仍会明确覆盖。`rag.embedding.models`、`mcp.servers`、`hooks.commands` 等列表继续写在受保护的部署 YAML 中。

自定义 HTTP MCP 请求头和 stdio MCP `env` 的 `${NAME}` 引用不会自动获得 `.env` 任意变量。示例文件 `compose.mcp.yaml` 仅给 Agent 增加所需变量：

```yaml
services:
  agent:
    environment:
      EXTERNAL_MCP_API_KEY: ${EXTERNAL_MCP_API_KEY:?set EXTERNAL_MCP_API_KEY in deployment environment}
```

由部署维护者把这个键的实际值保存在私有 `.env` 或密钥注入环境，HTTP MCP 声明可引用：

```yaml
mcp:
  servers:
    - name: external_service
      url: https://mcp.example.test/api/mcp
      headers:
        X-API-Key: "${EXTERNAL_MCP_API_KEY}"
      permission: execute
      required: false
```

stdio MCP 应在该服务器自己的 `env` 映射里显式引用，如 `SERVICE_API_KEY: "${EXTERNAL_MCP_API_KEY}"`；子进程默认仅继承受限系统变量。hook、plugin 和其他子进程也不再自动继承整套服务凭据，所需环境必须使用各自支持的显式声明；不要恢复全量继承来修复个别工具。

创建、重建与更新时均需带同一个 overlay：

```sh
docker compose --project-directory "$project_dir" --env-file "$project_dir/.env" \
  --file "$project_dir/compose.prod.yaml" --file "$project_dir/compose.mcp.yaml" \
  --profile agent config --quiet
# 在同样的文件参数后执行 up -d relay agent；不要去掉 overlay 再运行更新。
```

`deploy/update-myai.sh` 当前只读取一个 `MYAI_COMPOSE_FILE`，不会自动加上上述第二个文件。使用 overlay 的部署应维护带相同 `-f` 参数的更新入口，或提供经过审阅的单文件 Compose；不要将带真实插值值的 `docker compose config` 输出提交回仓库。Compose 校验使用 `config --quiet`，不要向共享日志输出完整环境。

## 5. MinIO 网络、凭据与桶

根应用与 `deploy/minio/docker-compose.yml` 都连接 `${INFRA_NETWORK:-myai-shared}` 外部网络。两个部署目录的私有 `.env` 需设置相同 `INFRA_NETWORK`，并预先创建该网络；MinIO 和 `minio-init` 都加入它。默认内网地址 `myai-minio:9000` 因此可从 Agent 解析。示例网络准备与 MinIO 启动命令：

```sh
infra_network=myai-shared
# 已存在时跳过 create；这里只查询网络元数据。
docker network inspect "$infra_network" >/dev/null 2>&1 || docker network create "$infra_network" >/dev/null

docker compose --project-directory "$project_dir/deploy/minio" \
  --env-file "$project_dir/deploy/minio/.env" \
  --file "$project_dir/deploy/minio/docker-compose.yml" config --quiet
docker compose --project-directory "$project_dir/deploy/minio" \
  --env-file "$project_dir/deploy/minio/.env" \
  --file "$project_dir/deploy/minio/docker-compose.yml" up -d
```

MinIO API/Console 仍默认只发布到宿主 `127.0.0.1`；容器应走共享网络，容器自己的 `localhost` 不是宿主机。面向客户端的公网域名通过私有部署配置及反向代理设置。

`MINIO_BUCKET` 默认 `myai-assets`，RAG 默认桶是 `myai-knowledge`，两者用途可以不同。若 `RAG_MINIO_AUTO_CREATE_BUCKET=true`，RAG 凭据要有对应创建权限；若关闭自动创建，先由维护者创建 RAG 桶并授予应用需要的访问权限。

## 6. 轮换凭据并核对服务端与客户端

模板占位值必须替换为部署维护者生成的凭据；不使用示例默认密码。启用服务的占位值会被拒绝，停用服务无需为了通过校验而填写凭据。对已经暴露的旧凭据执行服务端撤销/轮换；停止跟踪文件不会清除旧提交或旧镜像中的值。

| 服务端设置 | MyAI 客户端设置 | 核对项 |
| --- | --- | --- |
| Relay 主身份 token | `MYAI_RELAY_AGENT_TOKEN` | 根 Compose 将同一私有值传给 Relay/Agent；`MYAI_AGENT_USER`、`MYAI_AGENT_DEVICE` 与该身份一致 |
| Relay 额外身份 `MYAI_RELAY_AGENT_CREDENTIALS` | 对应外部 Agent 的 relay token 与 user/device | 每个 `user/device=token` 必须与该 Agent 一一匹配；仅传给 Relay |
| MinIO 账户或应用访问凭据 | `RAG_MINIO_ACCESS_KEY`、`RAG_MINIO_SECRET_KEY`，需要时加 `RAG_MINIO_SESSION_TOKEN` | root 部署变量为 `MINIO_ROOT_USER`/`MINIO_ROOT_PASSWORD`；应用可使用受限账户，必须具有目标桶所需权限 |
| OpenSandbox `OPENSANDBOX_API_KEY` | `SANDBOX_OPENSANDBOX_API_KEY` | 服务端和客户端使用同一有效 key，停用时设置 `SANDBOX_PROVIDER=local` |
| 外部短链后台创建的机器调用 key | `SHORT_LINKS_API_KEY` | 文件与短链共用 `short_links` MCP；`SHORT_LINKS_MCP_URL` 包含 `/api/mcp`，key 使用 `X-API-Key` 且仅由 Agent 持有 |

修改服务端与客户端私有配置后，按各服务部署流程重建相应容器，使环境变量生效；单纯重载 MCP 不会重新读取宿主 `.env`。文件能力要求另行部署兼容的新版后端，支持现有 10 个 MCP 工具以及 `get_asset_info`、`get_asset_download_url`、`create_asset_shortlink`。实际 URL/key 未确认时将 `SHORT_LINKS_MCP_URL`、`SHORT_LINKS_API_KEY` 留空并保持停用；空环境值通常沿用 YAML，因此还要清空 `short_links.mcp_url` 或显式禁用同名 MCP。不要从旧服务域名或 key 推测新配置。

自动 `short_links` MCP 是 `required: false`、`permission: execute`，离线、缺 key 或鉴权失败会告警并跳过，不阻止普通聊天启动。文件上传、`read_asset`、`share_file` 和知识库文件导入均调用主机侧 files 服务；缺少 MCP 或必需工具时明确失败，没有旧 HTTP 附件回退。显式配置的同名服务器优先；`required: true` 会让初始化失败阻止启动。运行中的重载对失败候选保留旧工具与客户端。

手机使用配对后认证的 Relay 上传通道，原始数据块最大 256 KiB、完整上传 JSON 信封最大 512 KiB；手机与主机上传的单文件大小范围为 1 字节至 50 MiB（1–52428800 字节，含边界），拒绝空文件（0 字节）。Agent 校验并重组后调用后端 MCP；远端非末片为 32 MiB，MCP 请求体上限为 48 MiB，包含 Base64 与 JSON 开销。普通上传只得到 AssetID 与 `structuredContent` 元数据，不创建公开链接；公开分享需明确批准后单独创建短链。下载字节仍由 MyAI 解析，RAG MinIO 和基础设施继续保留。详细契约见[文件与短链接 MCP 接入](FEATURE_SHORT_LINKS_MCP_INTEGRATION.md)。

## 7. 2026-11-10：磁盘待写日志升级

配置 Mongo 的 Agent 现在默认将已接受、尚未写入数据库的消息与工具记录保存到私有 SQLite outbox。默认目录是用户配置目录的 `myai/outbox/<工作区哈希>.db`，容器位于 `/home/myai/.config/myai/outbox/`，由本指南中的 `agent-user-config` 卷保留。未配置 Mongo 的纯内存模式不启用它。

- 不要将 outbox 放入工具工作区或共享可写目录。显式 `PERSISTENCE_OUTBOX_PATH` 必须是工作区外的绝对路径，并单独确认持久卷和权限。
- 日志绑定目标数据库端点、库名和工作区身份。更换目标之前，应先使用原配置让待写记录排空；scope/version 缺失或不匹配会拒绝打开。不要删改元数据来强行认领旧记录。
- 同一日志只允许一个 Agent 进程打开。多个工作区默认使用不同哈希文件；同工作区多实例不能共享同一文件并同时启动。
- 升级或迁移时，停止 Agent 后备份整个私有 outbox 目录以及 SQLite 侧文件，不从运行中的进程只复制主数据库。记录可能含完整聊天和工具内容，备份按秘密数据保护。
- 容量默认 64 MiB 主数据库、10000 pending、20000 总条目、10000 完成回执。日志满或无法提交时明确报错，不通过丢弃 pending 继续运行。临时回滚日志不计入主数据库页上限。
- 清空和删除先提交持久屏障，旧 pending 被取消，避免重启后恢复已删除内容。失败的清空保持阻止写入，应重试该会话清空；恢复只针对真实已删除会话，不绕过失败清空的屏障。

实现、测试证据和 at-least-once/非 exactly-once 限制见 [第二轮修复与验收](FOLLOWUP_FIXES_20261110.md)。本指南仅描述迁移步骤，本轮未执行线上迁移。

## 8. 验证边界

本指南不将此前轮次的 YAML 解析、旧服务测试或无密钥探针作为 2026-11-18 MCP-only 文件迁移的验收结果。历史评估与修复记录保持原时点含义；本次变更及主协调方最终验证结果见 [MCP-only 文件迁移记录](MCP_ONLY_FILE_MIGRATION_20261118.md)。

部署维护者应在具备 Docker 的环境先用无密钥临时 env 执行 `config --quiet`，再在维护窗口按本指南迁移并验证实际后端工具、身份、上传、读取与分享。不要输出完整插值配置。模板或离线检查通过不等同于真实网络、存储权限或部署凭据已验证；本次文档修改未执行 Docker 构建、卷迁移或线上部署。
