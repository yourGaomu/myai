## Context

问题出在**图片上传后的读取和展示**。安卓把文件分块上传，聊天消息用 `asset_id` 引用该文件。`read_asset` 能下载 JPEG 字节，但会把它交给文本解析器。模型最终收到的是文件信息和解析说明，图片像素没有进入模型请求。安卓聊天缩略图依赖选图时的临时 URI，历史只保存资产引用。助手正文也缺少 Markdown 图片渲染。 [附件发送](D:/Go_All/myai/mobile/src/utils/attachments.ts:17) · [文件读取](D:/Go_All/myai/core/tool/local/read_asset.go:81) · [图片卡片](D:/Go_All/myai/mobile/src/components/chat/SharedAssetCard.tsx:16) · [Markdown 渲染](D:/Go_All/myai/mobile/src/components/chat/MarkdownText.tsx:255)

## 实施

1. **让模型收到图片像素。** 在 Go 的 `generateSample` 边界，从这次请求实际选中的用户消息中识别标准附件块。凭 `session_id` 和 `asset_id` 精确校验资产归属，兼顾上传已经确认、资产记录仍在持久化队列中的情况。复用 `files.Service.Download`，校验图片格式、尺寸和完整性，并限制单张大小、总量和上下文开销。通过 `GenerateRequest` 的请求级图片字段传给适配器。优先打通截图所用的 OpenAI Chat Completions 协议，将 JPEG、PNG 的图片内容写入同一条用户消息的 `image_url`。普通文档沿用 `read_asset` 的文本分支。用户主动让工具读取图片时，保留完整的工具调用和结果，在工具批次结束后补上对应的图像输入。历史、运行记录和日志只保存资产引用及状态。失效附件、超限、格式不支持或上游拒绝时，给出可操作的错误。`gpt-6.1-sol` 第三方端点的视觉能力需要用样图验证。 [模型请求边界](D:/Go_All/myai/core/application/chat/generation/service/request_budget.go:145) · [下载服务](D:/Go_All/myai/core/adapter/files/shortlinks/download.go:21) · [Chat Completions 适配](D:/Go_All/myai/core/adapter/model/langchaingo/openai_chat_completions_adapter.go:32)
2. **让手机发送后和重开历史都能看到图。** 增加一个经过配对认证的 Relay 图片预览请求。Agent 按会话和真实资产 ID 查归属，读取私有文件并返回有上限的缩略图。安卓优先用有效的本机 URI，缺失或加载失败时按需取预览。复用 `SharedAssetCard` 的缩略图和全屏界面，提供失败重试。响应绑定请求、会话和连接，图片缓存在当前配对作用域。MCP 签名地址留在 Agent 端，预览走私有读取通道。 [Relay 请求路由](D:/Go_All/myai/core/remote/relay/server.go:452) · [历史附件恢复](D:/Go_All/myai/mobile/src/utils/chatHistory.ts:6) · [附件卡片](D:/Go_All/myai/mobile/src/components/chat/ChatAttachmentCard.tsx:14)
3. **让助手返回的图片显示在回答区。** 复用 `OnAsset` 和会话资产列表，按会话及 `tool_call_id` 把助手产物关联到对应回复，按 `asset_id` 去重。有效的 Markdown 图片资产引用和工具产物共用图片卡片，放在折叠工具详情之外。助手需要回传工作区图片时，走有路径校验和权限检查的私有附件入口。普通外部图片地址显示为点击查看的链接。已有历史图片可在原资产仍可访问时恢复。 [资产事件](D:/Go_All/myai/core/application/tool/service/execution_service.go:313) · [会话资产查询](D:/Go_All/myai/core/remote/agent/session_query_handlers.go:199) · [助手回答区](D:/Go_All/myai/mobile/src/components/chat/AgentTurnCard.tsx:155)

## 验证

- 用本地假 MCP 和假模型测试完整链路。上传一张合成 JPEG，断言模型 HTTP 请求含可解码的图像内容。覆盖马上发送、排队、追问、重生成、跨会话引用、过期资产，以及普通文本文件读取。检查历史和日志只留下引用。
- 用移动端测试覆盖发送后回显、缓存 URI 失效、重连及冷启动后按 ID 恢复、助手图片去重、失败重试和旧连接迟到响应。运行 Go 定向测试、移动端类型检查与 Node 回归。有可用 Android 设备时验证缩略图和全屏交互。
- 联调使用更新后的 Agent、Relay 和安卓端。手机部分使用现有原生模块，完成后核对当前安装包的 OTA 兼容性。