# Turn 生命周期：Codex 怎么做，我们怎么落地

> 对照基线：`the_baseLIne/codex-main/codex-main/codex-rs/core/src/session/turn.rs`  
> 我们的入口：`core/application/chat/generation/service/task_service.go` + `agent_loop_service.go`  
> 工具执行已单独对齐，见 [FEATURE_TOOL_EXECUTION_IMPLEMENTATION.md](FEATURE_TOOL_EXECUTION_IMPLEMENTATION.md)

本文不发明新协议。Codex 对外事件和内部函数是两套名字：表里「生命周期」多数是 **内部步骤**，只有少数会发到 UI。

## 1. Codex 真正的控制流

用户发一条消息后，Codex 不是「一次 HTTP 然后结束」，而是 **一个 Task 里套一层 Turn，Turn 里再套多次 Sampling**：

```text
RegularTask.run                          tasks/regular.rs
  ├─ emit_turn_started                   对外：EventMsg::TurnStarted
  ├─ emit_turn_start_lifecycle           扩展贡献者（MCP 预热等）
  └─ loop（有 pending input 就再开一轮 run_turn）
       run_turn                          session/turn.rs
         ├─ drain 上一轮异步 Hook 结果
         ├─ run_pre_sampling_compact     采样前压缩
         ├─ 解析本轮需要的 MCP / plugin
         ├─ capture_step_context         冻结本轮模型/工具/cwd
         ├─ record_context_updates       world_state / 环境快照
         ├─ build_skills_and_plugins     技能说明写入 history
         ├─ SessionStart / UserPrompt Hook
         ├─ record 用户消息
         └─ loop（有 follow-up 就再采）
              ├─ drain 中途插话 (input_queue)
              ├─ clone_history → stream 模型
              │    try_run_sampling_request
              │      OutputItemDone → 文本 / FunctionCall
              │      FunctionCall 立刻丢进 ToolCallRuntime（可并行）
              ├─ 超窗口 → 中途 compact，continue
              ├─ 无 tool → Stop Hook；Hook 可要求再采一轮
              └─ 有 tool 结果 → 下一轮 sampling
         对外结束：TurnComplete / Error / TurnAborted
```

Codex 自己写的语义（`turn.rs` 注释）：

- 每次 sampling，模型要么要 function call，要么给 assistant 文本。
- 有 function call：执行后把 output 放进 history，再采一次。
- 只有 assistant 文本：Turn 结束。
- **没有「最多 6 轮」硬编码**；停靠「没有 follow-up + Stop Hook 不续跑 + 用户取消」。

### 1.1 对外事件 vs 内部步骤

| 用户能看到（EventMsg） | 只是内部函数 | 作用 |
|---|---|---|
| `TurnStarted` | `emit_turn_started` | Turn 对 UI 可见、可中断 |
| （无单独事件） | `emit_turn_start_lifecycle` | 扩展：MCP 预热、catalog |
| （无单独事件） | `run_pre_sampling_compact` | 历史太长先压 |
| （无单独事件） | `TurnEnvironmentSnapshot` + `record_context_updates` | cwd / sandbox / 时间进 prompt |
| （无单独事件） | `build_skills_and_plugins` | skill 文本注入 history |
| `ItemStarted` / `ItemCompleted` | `handle_output_item_done` | 一条 reasoning / 文本 / tool |
| 工具审批事件 | `ToolCallRuntime` | 边流边跑工具 |
| `TurnComplete` | 采样结束且 Stop Hook 不续跑 | 成功结束 |
| `Error` | `emit_turn_error_lifecycle` | 失败 |
| `TurnAborted` | cancellation_token | 用户取消 |

不要把内部函数名当成我们要新增的 Relay 消息。落地时：**先把步骤做对，事件能复用现有 `agent_run_*`。**

### 1.2 一次 Sampling 里 Codex 干什么

`try_run_sampling_request`：

1. `client_session.stream(prompt)` 打开模型流。
2. 每个 `OutputItemDone`：
   - 普通文本 → 记 history，更新 `last_agent_message`。
   - FunctionCall → **立刻** `tool_runtime.handle_tool_call`，不必等整次 response 结束。
3. 能并行的 tool 用读锁，写/执行用写锁（我们已在 `ExecutionService` 对齐）。
4. 返回 `needs_follow_up`：还有未完成的 tool 或模型还要看结果 → 外层 `run_turn` 再采。

流失败会按 `stream_max_retries` 重试；context overflow 走 **中途 compact**，不是直接把 Turn 打死。

### 1.3 Stop Hook

`run_turn_stop_hooks`（`hook_runtime.rs`）：模型已经给出最终文本后还跑一次。

- 普通 Turn：`Stop`
- 子 Agent：`SubagentStop`
- 返回 `should_block` / `should_stop` / `continuation_fragments`
- `should_block` 时把 Hook 文本写入 history，**再采一轮**（续跑）

这就是「Turn 停止钩子」：不是关进程，是 **答完了还能被 Hook 拉回去再干一轮**。

## 2. 我们现在实际怎么走

```text
user_message
  ChatService.SendMessageStreamForSession
    RAG / AutoPlan / AppendUserMessage
    TaskService.Generate                         ← 对应 RegularTask
      Runs.Start → agent_run_started             ← TurnStarted
      AssistantGenerationService.Generate
        CompactIfNeeded                          ← 仅 loop 前一次
        MemoryContext.Prepare
        AgentLoopService.Run                     ← 对应 run_turn 的 sampling 循环
          for round < 6:
            Model.Generate（整轮结束才拿到 ToolCalls）
            无 tool → 结束
            ExecutionService.Execute             ← 已按 Codex 读写锁并行
            Append tool messages
            AfterToolRound hook
          满 6 轮再无工具生成一次
      Runs.Finish → succeeded/failed/paused      ← TurnComplete / Error / Aborted
```

和 Codex 的差距，按「值不值得立刻抄」排：

| Codex 步骤 | 我们 | 差在哪 | 落地优先级 |
|---|---|---|---|
| `TurnStarted` / Complete / Error / Abort | `AgentRun` Start/Finish | 名字不同，产品层已有 | 保持 |
| 环境快照 + 当前时间 | 无 | 问「星期几」会去 shell | **P0** |
| 采样前 compact | `CompactIfNeeded` 一次 | 中途超限不会再压 | **P1** |
| SessionStart / UserPrompt / Stop Hook | 只有 Pre/PostToolUse | Hook 事件太少 | **P1** |
| 中途插话 `input_queue` | 无 | 跑着不能再塞一句 | **P2** |
| item 级流式 + 边到边跑 tool | 等 `Generate()` 整包 | langchaingo 限制 | **P3** |
| Stop Hook 续跑 | 无 | 答完不能被 Hook 拉回 | **P1** |
| 无硬编码 6 轮 | `DefaultMaxToolRounds = 32` | 安全阀，正常靠无 tool 结束 | 已落地 |
| world_state / skill 注入 | runtime instruction + 工具 schema | 有一部分，不完整 | **P2** |
| TurnDiff | 有 Changes 面板，不是 turn 产物 | 可后做 | P3 |

**不要做的：** 把 `AgentRun` 改名成 Turn、给 Relay 加一堆和 Codex 同名的空事件。生命周期要对的是 **步骤顺序和决策**，不是事件字符串。

## 3. 落地原则（学 Codex 的骨架，不搬 Rust）

1. **Turn 的边界就是现在的 `TaskService.Generate`。** 开始/结束继续用 AgentRun。
2. **采样循环继续是 `AgentLoopService.Run`。** 先改循环条件和压缩时机，再改流式。
3. **工具并行已经在 ExecutionService。** 这一层先别再拆。
4. **Hook 按 Codex 事件名往我们 `hook.EventType` 加**，不要新建一套生命周期总线。
5. **环境/时间是 prompt 注入，不是新事件。** 写进 messages 即可。
6. 分层不变：编排在 `application/chat/generation`，压缩走已有 Compactor，Hook 走 `core/hook`。

## 4. 分阶段改什么文件

### P0 — 环境快照（已落地）

Codex：`maybe_record_current_time` + `TurnEnvironmentSnapshot`（cwd、sandbox）。

精简版已注入 **当前时间 + cwd**，不写入 Session 历史（与 Memory 相同，只进本轮模型请求）：

- `core/application/chat/generation/service/environment.go`
- `AssistantGenerationService.Generate` 在 `AgentRunner.Run` 前生成文本
- `AgentLoopService` 用 `withTurnContexts` 插到最后一条真实 user 消息前
- 示例：`Current time: Friday, 2026-09-18 15:04:05 CST (UTC+8, 星期五).\nWorkspace: D:\Go_All\myai`

效果：闲聊问日期不必依赖 shell。

### P1 — Turn 级 Hook + 采样前/中途压缩（已落地）

**Hook**（对齐 Codex `SessionStart` / `UserPromptSubmit` / `Stop`）：

- `core/hook` 增加 `session_start`、`user_prompt_submit`、`stop`
- 应用层只依赖 `generationport.TurnLifecycleHooks`，adapter：`core/adapter/hook/lifecycle`
- `TaskService.Generate` 在真正生成前：首次无 assistant 消息时跑 `session_start`，然后 `user_prompt_submit`（deny 则整轮失败）
- `AgentLoopService` 在本轮无 ToolCalls 时跑 `stop`；Hook 返回续跑文本则写入 `hook_context` 再采一轮（最多 3 次）
- 内部任务（`Internal=true`）不跑 Turn Hook

**压缩**（对齐 `run_pre_sampling_compact` + mid-turn）：

- 仍保留 `AssistantGenerationService` 进 loop 前的 `CompactIfNeeded`
- `AgentLoopService` 每轮 `Generate` 前再调一次同一 `Compactor`；低于阈值则 no-op
- 超窗口时先 compact 再校验，而不是直接失败

### P2 — 循环停条件 + 中途插话（已落地）

**安全阀：**

- `DefaultMaxToolRounds = 32`（不再是 6）。正常停靠仍是：无 tool call + Stop Hook 不续跑 + 没有 pending 插话。
- 满上限仍做一次无工具终轮。

**中途插话：**

- `generationport.PendingTurnInput` + `adapter/chat/pendinginput.Queue`
- 会话已有 Run 时，再来的 `user_message` 进入队列并回 `user_message_queued`，不再报 `session is already running`
- `AgentLoopService` 第一轮采样不 drain（先消化本轮用户消息）；之后每轮采样前 drain
- 模型已给出最终回复但队列里还有插话时，继续采样而不是结束 Turn

### P3 — 流式 item（最后做）

Codex 能边收到 FunctionCall 边执行，是因为 Responses 流里每个 item 独立完成。

我们的 `core/llm/model.go` 用 langchaingo `GenerateContent`，**整包结束后才有 ToolCalls**。要对齐需要：

- 在 OpenAI/兼容流里解析增量 `tool_calls` delta；
- `Model.Generate` 增加「OnToolCallReady」或把 loop 改成消费 stream item；
- `AgentLoopService` 不再等整轮 `ChatResult.ToolCalls`。

工作量大，且 P0–P2 已经能覆盖「日期、压缩、Hook、长任务」。P3 单独立项。

## 5. 推荐落地顺序（一次只做一块）

```text
P0  当前时间 + cwd 注入          已落地
P1  user_prompt / stop Hook
    + loop 内 compact            已落地
P2  循环停条件 + pending input   已落地
P3  流式 function call           对得上 OutputItem + 边到边跑
```

每一阶段完成后再改 Relay 事件。P0/P1 **不必**新增 `TurnStartLifecycle` 这类手机消息。

## 6. 验证

```powershell
go test ./core/application/chat/... ./core/hook/... ./core/application/tool/...
```

手动：

- 「你好」仍一轮结束，回复里不该无故调 shell。
- 「今天星期几」应直接回答（P0），或最多自动跑 `Get-Date` 且不再弹权限（工具 OnRequest 已做）。
- 超长会话在第二轮 tool 之后仍能继续，而不是窗口错误直接失败（P1）。
- 模型说完后若配置了 Stop Hook 续跑，会再采一轮（P1）。

## 7. 和 Codex 源码的索引

| 要抄的行为 | Codex 文件 | 我们落点 |
|---|---|---|
| Task 外壳、先发 TurnStarted | `tasks/regular.rs` | `task_service.go` |
| Turn 主循环 | `session/turn.rs` `run_turn` | `agent_loop_service.go` |
| 采样流 + 并行 tool | `turn.rs` `try_run_sampling_request` | `llm/model.go` + `ExecutionService`（并行已做） |
| 采样前/中途 compact | `turn.rs` `run_pre_sampling_compact` | `assistant_generation_service.go` + loop 内 |
| 时间/环境 | `session/time_reminder.rs` | generation `environment.go` |
| Stop / UserPrompt Hook | `hook_runtime.rs` | `core/hook` + Task/Loop 调用点 |
| 中途插话 | `input_queue` in `run_turn` | session mailbox / pending user |
