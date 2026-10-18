# 子代理实时查看：后端实现与前端接入

## 1. 本次实现范围

1.1 后端继续采用现有分层：领域对象保存身份，应用服务校验关系并编排查询，port 声明依赖，adapter 实现 Mongo/内存仓储，remote 定义 WebSocket DTO 与 Relay 路由。接口、DTO、实现分别放置；内部对象不添加 JSON/BSON 标签。

1.2 子任务运行期间即可接收 `subagent_task_event`。文字、推理、工具结果和文件资源属于子会话；父会话显示子代理摘要和入口。查看详情不调用 `LoadSession`，不改变当前主会话。

1.3 本次落地后端协议及测试，前端需按下述协议实现入口、只读详情和回放合并。没有新增 HTTP GET 接口。

## 2. 身份与关联

2.1 所有任务摘要新增 `parent_tool_call_id`、`child_session_id`、`current_run_id`、`reasoning`，包括任务列表、查询结果、实时事件中的 `task`。已有 `task.id`、`parent_session_id`、`parent_task_id`、`parent_run_id` 保留。

2.2 主聊天的 `tool_call`、`tool_result`，以及 `agent_run_event` 中的工具事件携带 `tool_call_id`。子任务的 `parent_tool_call_id` 对应创建它的 `spawn_agent` 或 `start_async_task` 调用。并发任务不能用到达顺序或工具名称配对。

2.3 `subagent_task_event` 的 `run_id` 是子任务 Run ID；子会话中的 AgentRun 有自己的 ID。两者不得当成同一命名空间。按 `task.id + run_id` 聚合子任务输出，按 `tool_call_id` 配对工具开始和结束。

2.4 任务初次创建时保存父工具调用身份；follow-up 复用同一个任务和 child session，通过不同 Run 区分后续执行。

## 3. 实时事件

3.1 沿用 `subagent_task_event`，消息信封的 `session_id` 是直接父会话 ID，payload 示例：

```json
{
  "task": {
    "id": "task-1",
    "parent_session_id": "parent-1",
    "parent_tool_call_id": "spawn-call-1",
    "child_session_id": "child-1",
    "current_run_id": "task-run-1"
  },
  "sequence": 123,
  "kind": "task.tool_result",
  "run_id": "task-run-1",
  "tool_call_id": "read-call-1",
  "tool_name": "read_file",
  "content": "工具输出",
  "status": "success",
  "delta": false
}
```

3.2 `task.answer`、`task.reasoning` 在 `delta=true` 时追加文本。`task.tool_call` 标记工具运行，`task.tool_result` 更新该调用的结果。`task.waiting` 表示等待子任务等状态变化，具体状态以 `task.status` 为准。`task.updated` 可恢复 running 等状态。

3.3 `task.completed/failed/canceled` 更新任务终态。任务的 `result` 是最终结果快照，不要把它再次追加到已收到的 answer delta 后面。`task.snapshot` 只更新状态，不属于可以推进历史游标的增量。

3.4 模型适配器没有发送 reasoning 时，不会凭空产生推理文本。工具内部如果没有进度回调，只能显示“开始”和“结束”，不能显示其内部每一步或真实百分比。

3.5 当前后台子代理没有交互审批回复通道。需要确认的调用会发送 `task.permission`，其 `status=denied`、`error_code=approval_unavailable`，随后得到拒绝的工具结果；前端不应显示一个无法提交的批准按钮。完整异步审批不在本次只读查看范围。

## 4. 子任务详情

4.1 请求类型 `subagent_task_detail`，信封必须指定直接父会话 `session_id`：

```json
{
  "task_id": "task-1",
  "after_message_id": "",
  "message_limit": 100
}
```

4.2 响应类型 `subagent_task_detail_result`，包括：

| 字段 | 含义 |
| --- | --- |
| task | 当前任务摘要及 child session、父工具调用身份 |
| read_only | 固定 true，查询不切换或修改会话 |
| session_ready | 子会话记录是否已创建 |
| event_sequence | 查询开始时的持久化事件水位，不代表 messages 已包含全部该水位之前的增量 |
| messages | 本页会话消息，复用 SessionHistoryMessage |
| next_message_id | 本页最后一条消息 ID，用于下一页 |
| has_more_messages | 是否还有历史消息页 |
| full_sync_required | 消息游标无效，应清除该历史游标从头读取 |
| runs | 子任务 Run 历史，含状态和最终结果 |
| agent_runs | 子会话最近 20 个 AgentRun 快照 |
| assets | 子会话最近最多 100 个已落库资源，复用 AssetSummary |

4.3 `message_limit` 默认 100、最大 200。用 `next_message_id` 继续请求。排队/启动失败时可能 `session_ready=false`，仍可显示 task、runs 与事件，不应将其当成详情接口失败。

4.4 服务先检查 task 的父会话，再检查 child session 的 `Kind`、`ParentSessionID`、`ParentTaskID` 和 ID。Relay 沿用用户/设备配对令牌校验。`read_only` 描述此查询和页面的行为，不是对其他接口的全局权限声明。

## 5. 历史回放与断线补取

5.1 请求 `subagent_task_events`，信封 `session_id` 为直接父会话：

```json
{
  "task_id": "task-1",
  "run_id": "task-run-1",
  "after_sequence": 0,
  "through_sequence": 0,
  "limit": 100
}
```

5.2 `task_id` 可省略以回放整个父会话；`run_id` 可省略以读取该任务所有 Run，指定 Run 时必须指定 task。limit 默认 100、最大 200。

5.3 返回 `subagent_task_events_result`：`events`、`next_sequence`、`through_sequence`、`has_more`，以及原查询范围。首次不指定 through 时，后端固定当前已持久化水位。后续页携带同一 through，直到 has_more=false；随后用 next 作为 after、清空 through，开始下一轮补取。游标超前会明确报错，客户端应从零重建该范围。

5.4 序号全局递增，其他父会话/任务的事件会被过滤，所以当前范围出现序号空隙是正常的。不要要求 sequence 连续加一。分页结束时 next 可以推进到 through，即使该页没有匹配事件。

5.5 前端首次打开：先缓冲实时事件，同时从零回放指定 task/run；按 `sequence` 去重并排序，再合并缓冲。已有缓存则从最后成功补取的游标开始。实时事件最大序号不能直接覆盖尚未完成的历史回放游标，否则可能跳过断线期间的消息。

5.6 `messages`、`agent_runs` 和 task events 是不同视图。实时过程页以 TaskEvent 为过程来源；历史消息按消息 ID 展示，工具按 ToolCallID 对齐，资源按 AssetID 去重。不要简单把三个来源的文本全部拼接成同一段回答。打开运行中详情时，仅查询 messages 无法还原尚未提交的 answer/reasoning，必须回放事件。

5.7 `event_sequence` 是捕获实时事件的衔接提示，详情并非跨数据库的原子快照。首次恢复过程不能只从这个水位开始，仍需读取此前未缓存的事件。任务快照更新比较 `task.updated_at`，避免旧事件覆盖新状态。

5.8 Mongo 模式可跨重启读取事件，内存模式仅在本进程生命周期内可恢复。现有事件发布器在落库失败时仍可能发送实时通知，因此当前不是消息零丢失承诺；数据库故障后的过程缺口不能仅靠状态快照重建。

## 6. 图片与附件

6.1 `share_file` 成功生成可访问资源后，执行器分配稳定 AssetID，触发 `task.asset_created`。payload 的 `assets` 使用已有 AssetSummary（含 id、session_id、tool_call_id、file_name、content_type、short_url、expires_at）。资源 ID 与后续资源落库记录一致。

6.2 图片根据 ContentType 使用 ShortURL 预览，其他文件显示下载入口。事件内只放引用与元数据，不放图片二进制。现有 SharedAssetCard 可以复用。

6.3 资源列表异步落库，事件可能先到。先显示实时资源，查询刷新时按 id 合并。当前适配器只识别成功的 share_file 结果，模型原生图片和其他工具的媒体输出需要各自资源适配器，不在本次实现中。

6.4 本次未实现过期链接自动续签。前端按 expires_at 显示过期状态，不能承诺图片永久可访问；是否长期保留由现有文件服务策略决定。

## 7. 前端交付提示词

请基于 FEATURE_SUBAGENT_LIVE_INSPECTION.md 实现子代理实时查看：

1. 主聊天中为 spawn_agent/start_async_task 工具行添加子代理摘要入口，按 parent_tool_call_id 精确匹配，显示状态与最近输出。
2. 点击打开独立只读 child session 面板，使用 subagent_task_detail，不能发送 session_load 或改变主会话当前 ID。
3. 使用 subagent_task_events 回放并订阅现有 subagent_task_event，按 task/run 分组、sequence 去重；实现固定水位分页以及断线补取，保持实时缓冲和历史游标分离。
4. 展示 answer、reasoning（如果有）、工具调用、结果、等待、成功/失败/取消。工具按 tool_call_id 配对，终态 result 不重复追加。
5. task.asset_created 使用 assets 显示图片/文件，复用 SharedAssetCard，按资产 ID 合并历史列表；处理过期与加载失败。
6. 后台权限事件为 denied/approval_unavailable 时展示拒绝原因，不显示不可用的审批按钮。
7. 主会话摘要只保留有限预览；完整过程留在子代理面板。支持多个子代理并发、嵌套父会话范围、follow-up 多 Run、排队未创建 session、断线恢复及切换详情后迟到响应隔离。
8. 所有文件使用 UTF-8，关键流程加中文 1.xxx、1.1 xxx、2.xxx 步骤注释。

## 8. 后端阅读入口

1. `core/application/subagent/service/inspection_service.go`：父子校验、固定水位分页。
2. `core/application/subagent/service/task_events.go`：子代理流转事件。
3. `core/application/tool/service/execution_service.go`：独立调用身份、资源回调。
4. `core/adapter/tool/executor/executor.go`：生成层与工具层回调桥接。
5. `core/remote/agent/subagent_inspection_handlers.go`：只读详情组装。
6. `core/remote/protocol/subagent_inspection.go`：前端协议 DTO。
7. `core/adapter/persistence/mongo/subagent/repository/inspection.go`：事件查询和索引。
