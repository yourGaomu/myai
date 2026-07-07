# MyAI Kafka

单节点 Kafka 部署，用于 MyAI 后续业务解耦。

当前配置使用 KRaft 模式，不需要 ZooKeeper。

## Files

- `docker-compose.yml`: Kafka + topic 初始化 + 可选 Kafka UI
- `.env.example`: 环境变量示例
- `data/`: Kafka 本地数据目录，已加入 `.gitignore`

## Start

```bash
cd /docker/kafka
cp .env.example .env
```

如果服务部署在服务器上，需要把 `.env` 里的 `KAFKA_EXTERNAL_HOST` 改成服务器 IP 或域名：

```env
KAFKA_EXTERNAL_HOST=你的服务器IP
KAFKA_EXTERNAL_PORT=9092
```

启动 Kafka：

```bash
docker compose --env-file .env up -d
```

查看状态：

```bash
docker ps -a
docker logs -f myai-kafka
```

## Default Topics

`kafka-init` 会自动创建这些 topic：

```text
myai.events
myai.assets
myai.tasks
myai.audit
```

你可以在 `.env` 中修改：

```env
KAFKA_INIT_TOPICS=myai.events,myai.assets,myai.tasks,myai.audit
```

## Kafka UI

Kafka UI 默认不会启动。

需要时执行：

```bash
docker compose --env-file .env --profile ui up -d
```

访问：

```text
http://服务器IP:8088
```

## Client Address

如果程序和 Kafka 在同一个 Docker Compose 网络里：

```text
kafka:29092
```

如果程序在 Docker 外部或者其他机器上：

```text
服务器IP:9092
```

当前配置是 `PLAINTEXT`，适合内网或开发环境。公网服务器请至少用防火墙或安全组限制 `9092` 和 `8088` 的访问来源。
