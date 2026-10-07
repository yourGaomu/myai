# 子代理等待与父会话续跑

## 执行语义

`spawn_agent` 异步启动任务，返回任务身份，不代表任务已完成。主模型可以继续独立工作；当下一步或最终回答依赖子代理结果时，应调用 `wait_agent` 并整合结果。当前没有“所有子代理结束前禁止主代理结束”的全局屏障。

`wait_agent` 等待 targets 中任意任务进入终态，默认 60 秒，最多 600 秒。超时只结束此次等待，不取消子任务；取消等待也不会自动取消子任务。需要取消子任务时调用 `cancel_async_task`。

等待工具在 Agent Run 中记录 `tool_call` 事件，`tool_name=wait_agent`、`status=waiting`，Run 保持 `running`。等待结束后会产生对应 `tool_result`。仅包含等待工具的批次不消耗普通工具轮数，另有每次 AgentLoop 最多 32 个等待批次的保护；达到限制返回错误，不生成假装任务已完成的最终回答。

成功或失败的子任务使用 `TaskID + CurrentRunID` 对应的稳定完成消息 ID。等待返回前先确保该完成消息进入父会话邮箱，AgentLoop 在等待工具批次完成后、下一次模型采样前读取邮箱。正常工具调用仍保留原有邮箱边界。

生产配置下，等待工具返回 `results_in_mailbox=true` 和任务的 `completion_message_id`，结果正文由邮箱提供，不在工具输出中重复附加。仅用于测试或独立集成、未配置 ParentNotifier 时，工具仍直接返回结果正文。

## 自动续跑与停止

完成通知保持 `trigger_turn`。父会话正在运行时，后台 worker 等待同一把会话操作锁；获取锁后重新检查暂停/删除状态和队列。若当前轮次已消费结果，不创建新的生成。并发通知合并到同一 worker；生成过程中到达的新通知会让 worker 再检查一次队列。

父会话空闲且允许自动续跑时，结果触发新的父轮次，使用新的 turn_id/request_id 和 Agent Run，不复用旧请求路由。该轮次强制普通聊天模式，不重新自动规划。

`session_pause` 同时取消前台运行、取消后台续跑，并禁止后续完成通知自动重启父会话。子任务不会因此自动取消，结果保留供查询或之后处理。删除会话也先停止续跑；自动续跑会检查持久化会话是否仍存在且未删除。

Mongo 使用独立的 `session_continuation_controls` 集合记录 `{_id: sessionID, paused: bool}`，不包含密钥，不需要手工迁移或清理旧数据。普通异步会话快照不会覆盖暂停标记。进程重启后仍尊重暂停状态。用户发送新消息、重新生成、执行计划或显式恢复父会话结果时重新允许续跑；单纯打开或恢复已删除会话不会解除暂停。

后台续跑没有交互式权限审批连接，沿用未提供 `OnToolAsk` 时的拒绝规则，不自动批准需要用户确认的工具。

## 前端接入契约

新增 WebSocket 类型：`background_turn_event`。Relay 将它推送给同 user_id/device_id 下仍有效的已授权连接，不要求存在对应的前台请求。客户端需要建立正常的认证连接/心跳注册。

外层 `request_id` 是新后台轮次的 `turn_id`，`session_id` 是父会话。payload：

| 字段 | 说明 |
| --- | --- |
| turn_id | 后台轮次标识；按此分组，不追加到旧前台请求 |
| session_id | 父会话 ID |
| run_id | Agent Run ID，创建成功后提供 |
| sequence | 该 turn_id 内递增的实时事件序号 |
| kind | started、delta、run_event、completed、resync_required |
| status | running 或终态 succeeded、failed、paused |
| content / reasoning | delta 时为增量；completed 时为最终结果 |
| error | 失败或暂停原因 |
| run | 与已有 AgentRun 协议结构一致 |
| event | 与已有 AgentRunEvent 协议结构一致 |

客户端处理规则：

1. `started`：为目标会话创建新的运行条目，记录 turn_id/run_id。不要要求它匹配本地已发出的请求。
2. `delta`：追加正文/推理到该后台轮次。未看到 started 的 delta 也需要按身份建立临时条目，随后查询快照。
3. `run_event`：复用运行事件渲染。`wait_agent` 的 waiting 状态显示“等待子代理”；不要将它或单个子代理完成当作父轮次完成。
4. `completed`：根据 status 结束该后台轮次；成功时用完整 content/reasoning 校准增量内容，不再重复追加。失败时保留已显示进度，显示 error。
5. 同一 turn_id 按 sequence 去重。事件里的 `event.sequence` 是持久化 Run 的序号，与外层实时 sequence 是两套独立游标。
6. 新增持久化 Run 事件 `type=answer`，其 content 是该次生成的最终正文快照，不是正文增量。前台聊天也可能收到该事件；已有 assistant_delta/assistant_done 展示不应再把 answer 事件重复添加成聊天消息。
7. 客户端重连、发现实时序号缺口、收到 `kind=resync_required` 时，调用现有 `agent_run_list` 和会话历史查询进行校准。resync_required 是设备级提示，session_id/turn_id 可为空；刷新关注的会话。
8. 用户点击停止仍调用 `session_pause`。停止结果成功后应显示已暂停，不自动发送恢复请求。

Agent 每次连接 Relay 后发送 resync_required，覆盖客户端一直在线、但 Agent 曾短暂掉线的情况。实时订阅有界，消费者过慢会关闭订阅并促使 Agent 重连，生成不等待网络写入。

实时事件不提供逐条断线重放。持久化的 Run 状态、事件和聊天记录是恢复来源；answer 事件正文最多 1 MiB，超过时 truncated=true，应以聊天历史为准。运行记录写入失败会记录错误，不能把实时事件当作持久化成功证明。

## 代码阅读顺序

1. `core/tool/local/subagent_tools.go`：模型看到的工具说明、等待状态与完成消息 ID。
2. `core/application/subagent/service/task_wait.go`：先订阅再读状态、事件等待、完成结果入邮箱。
3. `core/application/chat/generation/service/agent_loop_service.go`：等待边界领取邮箱、独立等待预算。
4. `core/service/chat.go`：合并唤醒、会话锁、后台续跑和结果确认。
5. `core/service/chat_continuation.go`：停止、恢复、后台取消与持久化检查。
6. `core/service/chat_background_events.go`、`core/remote/agent/background_turns.go`：后台事件发布与协议映射。
7. `core/remote/relay/server.go`：授权范围内的事件推送。

测试覆盖等待边界、等待预算、完成回调与等待竞争、取消不取消子任务、前台已消费结果不重复续跑、并发通知合并、运行中新增通知、暂停/删除、重启保留暂停、慢订阅、旧请求结束后的事件转发和最终回答的 Run 日志恢复。上述保证不等于跨进程崩溃下对外部副作用工具的 exactly-once 执行。
