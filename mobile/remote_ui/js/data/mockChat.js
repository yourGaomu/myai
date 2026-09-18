/**
 * MyAI Mobile UI Prototype - Mock Chat Data
 * 遵循 protocol.ts 与 chat.ts 的真实数据结构
 */

export const mockChatMessages = [
  {
    id: "msg-user-1",
    role: "user",
    text: "你好，请帮我检查当前项目的架构并复原 Android 移动端的 UI 原型图。",
    createdAt: 1774000000000,
    status: "completed"
  },
  {
    id: "msg-asst-1",
    role: "assistant",
    reasoning: "正在分析用户需求...\n1. 检测到项目为 MyAI Mobile，基于 React Native + Expo 架构；\n2. 移动端包含 8 个核心面板：对话、文件、变更、Diff、知识库、计划、会话与设置；\n3. 设计风格为高对比度 Neo-Brutalism 漫画线条风格，主色调为黄色 (#ffd84f) 与青色 (#4fd7ee)；\n4. 规划构建模块化静态原型工程至 mobile/remote_ui 目录。",
    text: "我已经完成对移动端工程代码的深入扫描！\n\n### 发现的核心模块：\n- **UI 风格**：高饱和粗黑线条设计（Neo-Brutalism）；\n- **核心屏幕**：包含 `Chat`、`Files`、`Changes`、`Knowledge`、`Plan`、`Settings` 等；\n- **交互支持**：支持实时 WebSocket 连通、Thinking 思考过程展示与工具调用二次确认。\n\n接下来我将调用工作区工具为您准备原型脚手架。",
    createdAt: 1774000002000,
    completedAt: 1774000005000,
    status: "completed",
    tokens: { input: 1240, output: 412, total: 1652 }
  },
  {
    id: "msg-tool-1",
    role: "tool_call",
    toolName: "list_dir",
    toolArguments: JSON.stringify({ DirectoryPath: "d:/Go_All/myai/mobile/src" }, null, 2),
    status: "completed"
  },
  {
    id: "msg-tool-res-1",
    role: "tool",
    toolName: "list_dir",
    text: "Found 10 directories: components, constants, hooks, native, screens, services, storage, types, utils...",
    status: "completed"
  },
  {
    id: "msg-asst-2",
    role: "assistant",
    reasoning: "已获取所有目录结构，现在开始在 mobile/remote_ui 目录生成高保真原型系统。",
    text: "原型系统脚手架已就绪！您可以通过底部的标签页无缝切换体验 **文件列表**、**Git变更对比**、**知识库** 及 **全局设置**。",
    createdAt: 1774000007000,
    completedAt: 1774000010000,
    status: "completed",
    tokens: { input: 890, output: 180, total: 1070 }
  }
];

export const mockAgentRuns = [
  {
    run_id: "run-001",
    title: "构建移动端 UI 原型系统",
    state: "running",
    steps: [
      { id: "s1", title: "扫描项目源码与协议", status: "completed" },
      { id: "s2", title: "搭建 CSS 设计令牌体系", status: "completed" },
      { id: "s3", title: "复原 8 大核心业务屏幕", status: "in_progress" },
      { id: "s4", title: "集成真机工作台并验证", status: "pending" }
    ]
  }
];

export const mockAttachedFiles = [
  {
    key: "file-spec-1",
    name: "ARCHITECTURE_NOTES.md",
    size: 24500,
    type: "text/markdown"
  }
];
