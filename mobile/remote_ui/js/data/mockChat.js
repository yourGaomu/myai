/**
 * MyAI Mobile UI Prototype - Mock Chat Data
 * 遵循 Agent Turn 统一交互体系规范与 protocol.ts 真实协议
 */

export const mockChatMessages = [
  // Turn 1
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
    text: "",
    createdAt: 1774000002000,
    status: "completed"
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
    text: "我已经完成对移动端工程代码的深入扫描！\n\n### 发现的核心模块：\n- **UI 风格**：高饱和粗黑线条设计（Neo-Brutalism）；\n- **核心屏幕**：包含 `Chat`、`Files`、`Changes`、`Knowledge`、`Plan`、`Settings` 等 8 大面板；\n- **交互支持**：支持实时 WebSocket 连通、Thinking 思考推导与工具调用二次确认。\n\n原型脚手架已在 `mobile/remote_ui` 目录下构建就绪！您可以通过底部的标签页无缝切换体验。",
    createdAt: 1774000007000,
    completedAt: 1774000010000,
    status: "completed",
    tokens: { input: 1240, output: 412, total: 1652 }
  },

  // Turn 2
  {
    id: "msg-user-2",
    role: "user",
    text: "帮我查一下为什么启动动画会卡顿，还有为什么切到后台老是断开重连？",
    createdAt: 1774000020000,
    status: "completed"
  },
  {
    id: "msg-asst-3",
    role: "assistant",
    reasoning: "正在定位两个性能与网络缺陷：\n1. 启动动画：AppHeader.tsx 帧动画每 100ms 重复请求 250KB PNG，导致网络洪峰与 JS 线程掉帧；\n2. 断线重连：useRelayConnection 在 AppState 焦点变化时无条件关闭已有连接，DevTools 切换被误判为掉线。",
    text: "",
    createdAt: 1774000022000,
    status: "completed"
  },
  {
    id: "msg-tool-2",
    role: "tool_call",
    toolName: "grep_search",
    toolArguments: JSON.stringify({ Query: "setInterval", Includes: ["AppHeader.tsx"] }, null, 2),
    status: "completed"
  },
  {
    id: "msg-tool-res-2",
    role: "tool",
    toolName: "grep_search",
    text: "AppHeader.tsx:64: const timer = setInterval(nextFrame, 100); // 10fps 请求远程PNG",
    status: "completed"
  },
  {
    id: "msg-tool-3",
    role: "tool_call",
    toolName: "view_file",
    toolArguments: JSON.stringify({ FilePath: "mobile/src/hooks/useRelayConnection.ts", Lines: "120-135" }, null, 2),
    status: "completed"
  },
  {
    id: "msg-tool-res-3",
    role: "tool",
    toolName: "view_file",
    text: 'if (state === "active") {\n  previousSocket.close(); // 无条件关闭连接\n  connect();\n}',
    status: "completed"
  },
  {
    id: "msg-asst-4",
    role: "assistant",
    text: "问题根因已精准定位完成：\n\n1. **动画卡顿原因**：\n   - 顶部帧动画使用 `setInterval` 频繁向 Metro 拉取大图，数秒内产生 300+ 次 HTTP 请求与 80MB 流量洪峰，阻塞了 JS 线程渲染。\n   - **已优化**：已在 React Native 端替换为预加载静态帧切片。\n\n2. **频繁重连原因**：\n   - 窗口失去焦点（如点击 Chrome 控制台）触发了 `AppState` 的 blur 事件，被误当做网络断开并强制执行 `socket.close()`。\n   - **已优化**：已加入 `WebSocket.OPEN` 状态校验与防抖，避免重复重连。",
    createdAt: 1774000026000,
    completedAt: 1774000030000,
    status: "completed",
    tokens: { input: 18420, output: 530, total: 18950 }
  }
];

export const mockAgentRuns = [
  {
    run_id: "run-001",
    title: "智能体环境就绪",
    state: "completed",
    steps: [
      { id: "s1", title: "扫描项目源码与协议", status: "completed" },
      { id: "s2", title: "搭建 CSS 设计令牌体系", status: "completed" },
      { id: "s3", title: "复原 8 大核心业务屏幕", status: "completed" },
      { id: "s4", title: "集成真机工作台并验证", status: "completed" }
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
