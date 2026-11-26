# 缓存命中率检查与修复

检查日期：2026-11-26

## 修复结论

已修复本地请求构造中会让可复用前缀缩短的三类问题：跨轮动态上下文不再被全部移除后重排；计划状态更新改为追加事件；子代理 mailbox 输入会带上当前 runtime/skill scope，并作为有来源标记的新输入追加。Anthropic 缓存检查点也会沿完整对话和已完成工具结果推进，不再停在首个 user 之前或被动态上下文截断。

这些改动让请求历史更接近 append-only，保持固定上下文内容时可延续更长的 wire 前缀。本次验证的是本地请求结构与适配器标记，没有连接真实模型服务，因此不能承诺线上缓存命中率或成本的具体提升。

## 具体改动

- [`core/contextmgr/request_context.go`](../core/contextmgr/request_context.go)：保留历史动态上下文的原有位置。仅当同一轮已附加的动态上下文被显式更新时，才在该位置替换；当前输入之后已经记录的上下文事件不再移到当前 user 前。增加了对子代理结果和 inter-agent mailbox 输入的 synthetic input 识别，使它们各自携带的上下文能被正确解析。
- [`core/application/chat/generation/service/request_budget.go`](../core/application/chat/generation/service/request_budget.go) 与 [`turn_context.go`](../core/application/chat/generation/service/turn_context.go)：每个采样前把计划快照变化作为新消息追加，状态不变时不重复追加。请求超窗重建也会复用已追加的同一计划快照。计划状态在工具推进后变化时，旧快照留在历史中，新状态出现在新的完整请求边界后。
- [`core/application/subagent/service/task_service.go`](../core/application/subagent/service/task_service.go)：子代理在工具轮之间收到 mailbox 时，复制当前 runtime/skill 指令到新输入前，并用 `SyntheticReasonInterAgentMessage` 保留来源类别与消息 ID，避免下一次请求把原子任务的动态指令过滤掉。
- [`core/adapter/model/langchaingo/anthropic_cache_transport.go`](../core/adapter/model/langchaingo/anthropic_cache_transport.go)：稳定 system/tools 之后的完整消息历史成为缓存检查点候选范围；检查点可落在已完成的 assistant 文本或完整 tool result 之后，不会落在未配对的 tool call 中间。仍遵守请求中最多四个缓存控制点及已有 caller 标记。

图片映射和普通工具调用不需要更改：已有代码把同一附件绑定回原消息并校验图片哈希，工具调用/结果按追加方式记录。上下文压缩、图片改变、工具权限/schema 改变仍会改变请求前缀，这是输入变化的预期行为。
- 官方 OpenAI HTTPS Chat Completions 端点额外发送稳定的 `prompt_cache_key`：由模型名、开头固定 system/developer 消息及工具 schema 计算；不包含会话 ID 或对话正文，让共享相同固定前缀的会话落入同一缓存路由组。请求目标会再次校验为 `api.openai.com`；Mistral 和其他兼容中转端点不会收到这个 OpenAI 专用字段。

## 本地验证

新增/调整的回归测试覆盖相邻轮相同 runtime/RAG、计划状态更新、子代理 mailbox、父会话子代理结果续跑，以及 Anthropic 完整工具结果后的缓存标记。重点断言包括：

- 相邻轮请求的旧完整消息前缀保持不变，当前轮动态上下文作为新的后缀进入请求。
- 计划从 running 转为 done 时，新计划快照出现在已有消息之后，而不会替换之前发出的快照。
- 子代理 mailbox 进入下一次采样时，旧前缀仍完整，runtime scope 和 mailbox 输入位于工具交换之后。
- Anthropic 缓存控制点落在稳定 system、已完成历史和最新完整消息边界；不会在 tool call/result 配对中间落点。

验证通过：`go test -count=1 ./...`（最终代码状态下全部 Go 包通过）。此外对请求与适配器改动执行了 `git diff --check`，没有空白错误。

## 限制

保留每轮有作用域的 runtime/RAG/计划上下文会增加长对话中的输入 token，可能让上下文压缩更早触发；压缩后前缀缩短属于预期变化。前缀更稳定只表示请求更有机会复用上游缓存，并不等于服务商已经命中。真实命中仍由服务商的 cached input token 统计确认；不同服务商的缓存键、最小缓存长度、图片处理和缓存保留规则可能不同。

## 2026-11-26 复核

本次复核针对后续图片回显、主动澄清、工具和子代理相关开发；OpenAI 路由键及缓存相关 Go 测试通过。没有发现这些功能会随机重排历史消息或改变已回放图片的绑定；权限、agent mode、工具 schema、system prompt、图片内容或上下文压缩发生变化时，缓存前缀会按设计改变。子代理拥有独立 session 和任务指令，首次请求通常无法复用父会话完整前缀，后续子代理工具轮仍可复用自身稳定前缀。

官方 OpenAI 路由键是辅助分流提示，不会强制产生缓存命中；第三方 OpenAI-compatible 服务的缓存行为仍由各服务决定。Anthropic 与 Gemini 的缓存分别依赖显式缓存控制点/服务端上下文缓存策略和上游用量报告。仓库测试只能验证请求序列、路由键和用量解析，无法替代线上服务返回的缓存 token 数据。
