# 第二轮优先问题修复与验收（2026-11-10）

基线提交为 `dbcf61a`。本轮于 2026-11-10 开始，2026-11-17 完成中断后的接受门禁收尾与最终复验。范围是 Plan 正确性、子代理结果交付、AI Memory 候选池与持久化写入恢复。首页、移动端设计草稿和动画示例等原有工作区内容未纳入修改；本轮未提交、推送或执行线上部署。

本文记录实际实现与验证边界，最终统一回归已完成。第 3 节列出实际检查结果，第 4 节将本轮目标对应到实现和验收证据；未覆盖的功能与平台限制见第 5 节。

## 1. 功能修复

| 问题 | 修改后的行为 | 主要实现与验证位置 |
| --- | --- | --- |
| PL3：计划解析误判完成 | 只解析明确 Plan 的顶层列表或完整/围栏 JSON；生成步骤不能自报已执行。独立非空 Result 可以作为纯回答结束，Plan 加 Result 仍为待执行草稿。 | `core/plan/plan.go`、`core/application/plan/service/capture_service.go`、对应解析/捕获测试 |
| PL7：非法依赖执行前未校验 | 执行前完整检查唯一 ID/order、依赖存在性、自依赖、环及 ID/order 引用歧义；错误时不创建 Run、不保存执行状态、不调用模型或工具。 | `core/application/chat/plan/service/execution_service.go`、零副作用校验测试 |
| PL6：恢复步骤漏做或过早满足依赖 | 真正替换失败步骤；替代步骤继承上游依赖，下游改接全部替代终点。空替代、重复标题、非法图与超限明确失败。 | `mergeRecoveryPlan` 与菱形/多终点/去重失败测试 |
| S20：长子代理结果永久 pending | 完整投递封装使用共享 8000 rune 上限，包含 JSON 转义、标题、状态和文件列表；Task/Run 全文不截断，摘要注明省略与全文读取入口。旧超大通知按自身已保存内容确定性缩减。 | `core/domain/subagent/completion.go`、`parent_completion_test.go`、真实 pendinginput 队列用例 |
| S21：消费确认与未读状态脱节 | memory 锁或 Mongo 事务内同时提交 delivered 与匹配 run/完成指纹的 Unread 更新；旧 ACK 不清新结果。Resume 成功走同一确认路径，失败不回写旧任务快照。 | memory/Mongo 子代理仓库、Mongo 驱动 mock 事务命令测试 |
| M1：候选上限与使用反馈 | Mongo、内存、catalog 共用默认 50/最大 200 的归一规则；101～200 不再掉回 50。使用仅更新 use_count/last_used_at，不改内容时间；候选在 limit 前按当前修订内容筛选。 | `core/port/memory/filters.go`、memory 仓库及 retrieval/catalog 测试 |
| M4：弱子串召回 | 英文按完整词、中文保留双字匹配；标题/标签加分要求边界，空或单字标签不获加分。排序使用修订内容时间，避免旧的 updated_at 使用污染继续自强化。 | `core/domain/memory/memory.go`、205 条候选/当前版本与词边界回归 |
| D2 后续：仅进程内队列 | 新增 SQLite typed outbox。消息、工具和附件记录在接受前映射稳定 ID，日志事务提交后才算接受；后端成功且 ACK 后清理正文。重启按会话顺序恢复，无 closure 序列化或工具重执行。 | `core/adapter/persistence/outbox`、`core/adapter/chat/generation/durable_queue.go`、真实临时 SQLite 与子进程退出测试 |
| 清空、删除和重新生成 | 清空/删除前取消并等待活动生成，提交持久屏障再修改后端；清空成功显式解封，删除保持阻止旧内容恢复。同步 transcript replacement 排在旧 append ACK 后，永不纳入自动重放。 | ChatService 生命周期、`SubmitAndWait`、真实装配清空/删除重开测试 |
| 日志拒写传播 | user、assistant、tool 使用可返回错误的 Accept 接口。日志关闭/满/拒写时停止后续主生成；工具已执行时保留内存证据并返回错误，不重放副作用；回答已生成时保正文与错误。 | `AcceptUserMessage/AcceptAssistant/AcceptToolExecution`、generation 与 caller 专用回归 |
| 拒写后的重试缓存 | 待接受批次按会话保留，总槽位上限 128。新会话在追加输入前预留容量，原会话重试不新增槽位；成功、显式清空/删除后释放。缓存、调用者与写入端使用独立深拷贝。 | `core/service/user_acceptance_gate.go`、真实追加前容量拒绝和写入端篡改隔离测试 |

## 2. 磁盘待写日志的运行规则

配置在 `persistence.outbox`，环境变量为 `PERSISTENCE_OUTBOX_PATH`、`PERSISTENCE_OUTBOX_MAX_PENDING`、`PERSISTENCE_OUTBOX_MAX_ENTRIES`、`PERSISTENCE_OUTBOX_MAX_BYTES`、`PERSISTENCE_OUTBOX_RETAIN_DONE`。

默认待写上限 10000，总条目上限 20000，SQLite 主数据库页上限 64 MiB，完成回执保留 10000 条。完成回执仅保存身份哈希，窗口外的重复仍由后端 immutable ID 防线处理；tombstone 不自动过期，容量不足会明确拒绝，不通过丢弃旧 pending 腾空间。SQLite 回滚日志和一次事务临时空间不包含在主数据库页上限内。

未配 Mongo 的纯内存模式不启用磁盘 outbox。配置 Mongo 后默认启用，初始化、权限或 scope 校验失败会阻止启动，不静默降级。默认位置为：

```text
<os.UserConfigDir()>/myai/outbox/<canonical-workspace-hash>.db
```

日志必须在工具工作区之外的私有目录。容器默认使用已有 `agent-user-config` 卷，避免另建易遗忘的数据路径；显式绝对路径必须自行挂持久卷。不能把真实日志或其备份提交到仓库。

scope 为数据库端点、库名和规范工作区身份的 SHA256，不保存 URI、用户名、密码。修改目标数据库、工作区或损坏 scope/version 后不会自动认领旧 pending。需要改目标时，应先恢复原配置并完成旧日志排空，再按维护流程备份和切换；不要通过删除元数据绕过保护。

日志单属主，第二个进程不能同时打开同文件。读取 pending 时遇到损坏封装，会隔离该记录并阻止同会话恢复，不影响无关会话；`Stats` 和 `Flush` 报告异常，不能把损坏记录当成功。当前没有自动修复或销毁隔离记录的产品入口，需保留私有备份后维护。

受限权限、非 nil 能力上限、Plan、子会话、工作区或系统约束等状态需要已有后端会话。恢复不会用消息内容重建缺失的安全设置；后端缺失或已删除时停止该会话写入。新普通会话仅允许保守的 chat/ask 默认值初始化。

## 3. 验证记录

已完成的定向证据：

- Plan 解析、捕获、图校验、重新规划与 root chat 包测试通过；独立只读复核未确认新增 P0/P1。
- 子代理完整封装预算、旧通知迁移、旧 ACK 隔离、Resume 失败重试测试通过；Mongo 使用无网络 mock 观察真实事务命令。
- Memory retrieval、catalog、Mongo/内存仓库、domain/port 包测试通过，包含 205 条记录挤占、旧使用时间污染和当前修订不在数组末尾。
- 真实临时 SQLite 的三个装配测试、六个子场景通过：首次/部分写入失败重开恢复、done 不重放、清空/删除屏障与重开不复活、关闭/满日志拒写时主生成及工具零调用。
- 上一轮的五个 Plan 真实装配重载测试已改为真实 outbox，继续保留数据库写入延迟与计划状态不被覆盖的断言。
- 独立日志审查指出的启动 map 竞态、受限会话错误重建、旧日志 scope 元数据缺失时重新认领问题均已定点修复；Windows 文件 URI 初始化问题也已通过真实重开验证。

第二轮最终冻结回归已完成：

| 检查 | 结果 |
| --- | --- |
| `go test -mod=readonly -p 2 -json ./... -count=1 -timeout=180s` | 通过：115 个包 PASS，0 个失败，173 个包无测试文件；1776 个通过测试项，16 个平台/集成测试跳过。 |
| Windows `go build -mod=readonly ./...` | 通过。 |
| Windows `go vet -mod=readonly ./...` | 通过。 |
| `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -mod=readonly ./...` | 通过交叉编译；未执行 Linux 运行时测试。 |
| 新增 outbox / 接受门禁 / 生命周期定向测试 | 通过，覆盖 Windows SQLite URI、重开恢复、scope 缺失拒绝认领、容量拒写、关闭/删除屏障、拒写零主生成/工具、128 会话容量与缓存深拷贝。 |
| 计划 / 子代理 / Memory 定向测试 | 通过，覆盖非法计划图零副作用、重规划依赖、长结果 8000 rune、run/revision ACK 隔离、Memory 50/200 上限与内容时间排序。 |

跳过项主要来自 Windows 无普通 symlink 创建权限、POSIX 专属行为和未启用的 Python 外部服务集成；Windows junction、Windows Job Object 和本地 SQLite 恢复已实跑。Windows race 运行时此前以 `0xc0000139` 无法启动，未声称 race 通过。

## 4. 目标到交付的完成审计

| 本轮目标 | 具体交付 | 覆盖该目标的证据 |
| --- | --- | --- |
| Plan 解析与执行正确性 | `core/plan/plan.go`、计划捕获与 `mergeRecoveryPlan` | 生成状态重置；非法依赖零 Run/消息/模型副作用；恢复继承上游并重接全部终点；空或重复替代拒绝；既有串行/CAS/中断保护测试保持通过 |
| 子代理长结果与确认 | `completion.go`、父完成服务、memory/Mongo ACK | 真实父队列验证完整封装不超 8000 rune；Task/Run 全文保留；旧通知使用保存内容迁移；旧 run/修订 ACK 不清新 Unread；Mongo 同事务提交 delivered 与 Unread |
| Memory 召回候选池 | 共用 limit、内容时间、词边界 | 101～200 保留；RecordUse 不写 updated_at；205 条候选用例找回旧相关内容；当前修订不在末尾；英文子串误命中拒绝与中文正常匹配 |
| 已接受写入可重启恢复 | typed SQLite outbox、稳定映射、后台恢复 | 真实数据库关闭重开和无 Close 的子进程退出；后端部分成功/ACK 前失败复投相同 ID；完成回执不重放；容量拒绝不丢已接受项 |
| 恢复不改变安全边界 | Scope、RequiresExisting、路径与单属主检查 | 跨目标、缺 scope/version/元数据表拒绝；非 Ask、空能力上限、Plan/子会话缺少既有会话时不重建；真实多会话并发恢复测试；独立审查闭环 |
| 清空删除与拒写 | 生命周期屏障、可返回错误 Accept、有限重试缓存 | 真实装配在后端修改前观察 tombstone；重开旧 pending 不复活；拒写主生成/工具为零；failed clear 重试；缓存 128 槽位满时追加前拒绝，重试成功释放，写入端修改不污染缓存 |
| 可部署与可审阅 | App/config/Compose 接线、迁移说明和本记录 | 默认私有日志位于现有持久卷；配置上限和目标绑定测试；统一回归、Windows/Linux 构建和静态检查；未执行线上部署或自动提交 |

## 5. 仍需区分的边界

1. 日志提交保证的是已经返回接受成功的数据库写入记录。工具副作用发生到其结果进入日志之间仍有窗口；本轮没有为任意外部操作提供 exactly-once。
2. 同步重新生成的 transcript replacement 不自动重放；日志只恢复已经映射的 append 和窄元数据写入。计划状态仍由独立 CAS 管理。
3. 分类器或前置检索可能先于 user Accept 调用；拒写后保证不进入后续主生成与工具副作用，不宣称此前零外部只读请求。
4. 日志不是加密保险库；私有权限、错误脱敏和路径保护不等同于磁盘加密。原聊天、工具结果及附件链接可能含敏感内容，备份按秘密数据保护。
5. S21 不回填历史上已经 delivered 但 Unread=true 的异常记录，也未新增实时 UI 未读事件；新的 ACK 持久值立即正确，界面通过刷新任务列表或详情读取。
6. S41 无界事件日志、RAG 索引生命周期、后台 shell、独立并行 Plan、runtime/RAG 完全脱离历史存储、微压缩等未在本轮实现。
7. Memory 的词项 regex/当前修订表达式没有全文词项索引，不能承诺 Mongo 查询成本固定；本轮未连接真实数据库执行 explain。
8. 未执行线上配置变更、凭据轮换、镜像构建或部署。普通 symlink、POSIX 专属及需要外部服务的测试仍按环境能力或原开关处理；本机 Windows race runtime 不可用，不能声称 race 通过。
