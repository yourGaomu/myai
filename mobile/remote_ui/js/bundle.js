/**
 * MyAI Mobile UI Prototype - Standalone Bundle
 * 兼容 file:/// 本地双击协议与 http:// 协议的自包含运行包
 */

(function () {
  "use strict";

  // ==========================================
  // 1. 数据层 (Mock Data)
  // ==========================================
  const mockChatMessages = [
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

  const mockAgentRuns = [
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

  const mockAssets = [
    {
      id: "asset-01",
      file_name: "android-ui-prototype.html",
      path: "/mobile/android-ui-prototype.html",
      size: 23393,
      content_type: "text/html",
      short_url: "https://assets.mikasa.wiki/s/proto-html",
      expires_at: 1774500000000
    },
    {
      id: "asset-02",
      file_name: "app-preview-screenshot.png",
      path: "/mobile/assets/preview.png",
      size: 142050,
      content_type: "image/png",
      short_url: "https://assets.mikasa.wiki/s/shot-png",
      expires_at: 1774500000000
    }
  ];

  const mockFileEntries = [
    { name: "..", is_dir: true, size: 0, mod_time: "" },
    { name: "components", is_dir: true, size: 0, mod_time: "2026-09-18 22:45" },
    { name: "constants", is_dir: true, size: 0, mod_time: "2026-09-18 22:40" },
    { name: "hooks", is_dir: true, size: 0, mod_time: "2026-09-18 22:42" },
    { name: "screens", is_dir: true, size: 0, mod_time: "2026-09-18 22:44" },
    { name: "types", is_dir: true, size: 0, mod_time: "2026-09-18 22:40" },
    { name: "protocol.ts", is_dir: false, size: 30872, mod_time: "2026-09-18 22:30" },
    { name: "App.tsx", is_dir: false, size: 697, mod_time: "2026-09-18 22:15" },
    { name: "package.json", is_dir: false, size: 1204, mod_time: "2026-09-18 22:00" },
    { name: "app.json", is_dir: false, size: 1536, mod_time: "2026-09-18 21:50" }
  ];

  const mockFilePreview = {
    path: "d:/Go_All/myai/mobile/src/protocol.ts",
    content: `export type PermissionState = {
  requestID: string;
  sessionID: string;
  name: string;
  permission: string;
  arguments: string;
};

export type SessionPermissionMode = "readonly" | "ask" | "full";
export type SessionAgentMode = "chat" | "plan";

export type ViewMode = "chat" | "knowledge" | "files" | "changes" | "changeDetail" | "sessions" | "settings" | "plan";`,
    size: 30872,
    is_binary: false
  };

  const mockChanges = [
    {
      path: "mobile/src/screens/MobileAppScreen.tsx",
      index_status: "M",
      worktree_status: "M",
      kind: "modified"
    },
    {
      path: "mobile/src/components/chat/Composer.tsx",
      index_status: "M",
      worktree_status: " ",
      kind: "modified"
    },
    {
      path: "mobile/remote_ui/index.html",
      index_status: "?",
      worktree_status: "?",
      kind: "added"
    },
    {
      path: "mobile/legacy-preview.html",
      index_status: "D",
      worktree_status: " ",
      kind: "deleted"
    }
  ];

  const mockHistoryCheckpoints = [
    {
      checkpoint_id: "chk-89b3f1",
      message: "完成移动端底部停靠栏 5Tab 架构重构与状态绑定",
      created_at: 1774000000000,
      files_count: 5
    },
    {
      checkpoint_id: "chk-71a2e4",
      message: "新增 AI 记忆中心抽取与知识库 RAG 增强",
      created_at: 1773950000000,
      files_count: 12
    }
  ];

  const mockDiffDetail = {
    path: "mobile/src/screens/MobileAppScreen.tsx",
    additions: 14,
    deletions: 3,
    diff_text: `@@ -865,9 +865,14 @@ export function MobileAppScreen() {
           onSend={sendUserMessage}
           onKnowledgePress={openKnowledge}
+          onPlanPress={openPlan}
           onSettingsPress={toggleSettings}
           onUploadFile={uploadLocalFile}
-          pendingPause={currentPauseBusy}
+          pendingPause={currentPauseBusy || isProcessing}
+          showVoiceInput={true}
           pendingSend={Boolean(currentChat.pendingRequestID)}
@@ -881,3 +886,9 @@ export function MobileAppScreen() {
         activity={headerActivity}
         buttonFeedback={buttonFeedback}
+        enableHaptics={true}
+        themeMode="neo-brutalism"`
  };

  const mockKnowledgeBases = [
    {
      id: "kb-01",
      name: "MyAI 技术架构与规范",
      description: "涵盖服务端中继协议、PC Agent 通信与移动端状态流",
      document_count: 18,
      chunk_count: 342,
      created_at: 1773000000000
    },
    {
      id: "kb-02",
      name: "Android 原生与 EAS 打包指南",
      description: "Gradle 构建、EAS CLI 打包 APK/AAB 与 OTA 差分更新",
      document_count: 6,
      chunk_count: 94,
      created_at: 1773500000000
    }
  ];

  const mockCategories = [
    { id: "cat-01", name: "架构设计", count: 8 },
    { id: "cat-02", name: "API 接口协议", count: 5 },
    { id: "cat-03", name: "移动端适配", count: 5 }
  ];

  const mockDocuments = [
    {
      id: "doc-01",
      title: "DEVELOPER_FLOW_GUIDE.md",
      size: 41699,
      status: "indexed",
      category: "架构设计",
      updated_at: "2026-09-18"
    },
    {
      id: "doc-02",
      title: "PROJECT_ARCHITECTURE_GUIDE.md",
      size: 32157,
      status: "indexed",
      category: "架构设计",
      updated_at: "2026-09-17"
    },
    {
      id: "doc-03",
      title: "FEATURE_AI_MEMORY_IMPLEMENTATION.md",
      size: 22607,
      status: "indexed",
      category: "API 接口协议",
      updated_at: "2026-09-16"
    }
  ];

  const mockAIMemories = [
    {
      id: "mem-01",
      category: "用户习惯",
      importance: "High",
      content: "用户偏好简洁直观的粗线条设计（Neo-Brutalism），且要求移动端支持免构建纯静态原型预览。",
      source: "Chat #42",
      created_at: "2026-09-18 22:30"
    },
    {
      id: "mem-02",
      category: "工程约定",
      importance: "Medium",
      content: "移动端工程所有静态原型图统一存放在 mobile/remote_ui 目录下。",
      source: "User Explicit",
      created_at: "2026-09-18 22:50"
    }
  ];

  const mockPlan = {
    id: "plan-101",
    goal: "为 MyAI Mobile 移动端构建高保真产品 UI 原型系统 (remote_ui)",
    status: "running",
    steps: [
      {
        order: 1,
        id: "step-1",
        title: "分析安卓客户端工程架构与视觉设计系统",
        description: "提取设计色调令牌 (--ink, --yellow, --cyan)、边框粗细与圆角规范。",
        status: "completed"
      },
      {
        order: 2,
        id: "step-2",
        title: "创建模块化样式系统与 Android 真机物理外壳",
        description: "实现包含状态栏、摄像头打孔、底栏手势条的精准机身框架。",
        status: "completed"
      },
      {
        order: 3,
        id: "step-3",
        title: "实现 8 大核心屏幕组件与响应式数据绑定",
        description: "复原 Chat、Files、Changes、Diff、Knowledge、Plan、Sessions 与 Settings 面板。",
        status: "running"
      },
      {
        order: 4,
        id: "step-4",
        title: "集成工作台演示外壳与交互模拟控制器",
        description: "支持一键模拟流式打字、工具调用、权限审批弹窗与模式切换。",
        status: "pending"
      }
    ]
  };

  const mockSessions = [
    {
      id: "30de8b48",
      title: "你好 (当前会话)",
      model: "grok-4.6",
      permission_mode: "ask",
      agent_mode: "chat",
      context_window_k: 16,
      tokens: 187435,
      updated_at: 1774000000000,
      created_at: 1773990000000,
      message_count: 5
    },
    {
      id: "7b12e094",
      title: "帮我查一下为什么启动动画会卡顿",
      model: "grok-4.6",
      permission_mode: "ask",
      agent_mode: "chat",
      context_window_k: 16,
      tokens: 42100,
      updated_at: 1773950000000,
      created_at: 1773940000000,
      message_count: 14
    },
    {
      id: "9c8821fa",
      title: "检查本地 SQLite 知识库与向量记忆索引",
      model: "deepseek-r1",
      permission_mode: "readonly",
      agent_mode: "plan",
      context_window_k: 64,
      tokens: 95400,
      updated_at: 1773800000000,
      created_at: 1773750000000,
      message_count: 22
    }
  ];

  const mockDeletedSessions = [
    {
      id: "del-01",
      title: "临时测试指令会话",
      model: "gemini-2.5-flash",
      permission_mode: "ask",
      deleted_at: 1773700000000
    }
  ];

  const mockRelaySettings = {
    relayURL: "https://relay.mikasa.wiki",
    assetBaseURL: "https://assets.mikasa.wiki",
    bindCode: "682914",
    clientToken: "tok_mobile_8a7d9f2c1b",
    deviceID: "pixel-9-pro-dev",
    userID: "gaomu",
    connected: true,
    status: "Connected"
  };

  const mockModels = [
    {
      id: "gemini-2.5-pro",
      name: "Gemini 2.5 Pro (Google)",
      protocol: "google-generative-ai",
      enabled: true,
      is_default: true,
      context_window: 128000
    },
    {
      id: "claude-3-7-sonnet",
      name: "Claude 3.7 Sonnet (Anthropic)",
      protocol: "anthropic-messages",
      enabled: true,
      is_default: false,
      context_window: 200000
    },
    {
      id: "deepseek-r1",
      name: "DeepSeek R1 (OpenAI 兼容)",
      protocol: "openai-chat-completions",
      enabled: true,
      is_default: false,
      context_window: 64000
    }
  ];

  const mockSkills = [
    { id: "agy-customizations", name: "Antigravity Customization", desc: "技能与插件开发规范" },
    { id: "generative-ui", name: "Generative UI Widget", desc: "富交互 HTML 渲染" }
  ];

  const mockPlugins = [
    { id: "plugin-bash", name: "Local Bash Terminal", enabled: true, desc: "终端命令执行器" },
    { id: "plugin-fs", name: "Workspace File Explorer", enabled: true, desc: "本地工作区文件操作" }
  ];

  const mockSubagents = [
    {
      id: "sub-01",
      name: "Code Reviewer",
      role: "代码审计子智能体",
      status: "idle",
      active_tasks: 0
    },
    {
      id: "sub-02",
      name: "UI Refactor Bot",
      role: "前端原型与样式构建",
      status: "running",
      active_tasks: 1
    }
  ];

  // ==========================================
  // 2. 状态管理 (Store)
  // ==========================================
  class Store {
    constructor() {
      this.state = {
        currentView: "chat",
        previousView: "chat",
        agentMode: "chat",
        settingsVisible: false,
        connected: true,
        activity: "idle",
        statusText: "在线",
        isBusy: false,
        permissionPrompt: null,

        chatMessages: [...mockChatMessages],
        agentRuns: [...mockAgentRuns],
        files: [...mockFileEntries],
        assets: [...mockAssets],
        selectedFilePreview: mockFilePreview,
        changes: [...mockChanges],
        historyCheckpoints: [...mockHistoryCheckpoints],
        currentDiff: mockDiffDetail,
        knowledgeBases: [...mockKnowledgeBases],
        categories: [...mockCategories],
        documents: [...mockDocuments],
        aiMemories: [...mockAIMemories],
        plan: { ...mockPlan },
        sessions: [...mockSessions],
        deletedSessions: [...mockDeletedSessions],
        relaySettings: { ...mockRelaySettings },
        models: [...mockModels],
        skills: [...mockSkills],
        plugins: [...mockPlugins],
        subagents: [...mockSubagents],

        knowledgeTab: "documents",
        settingsSection: "session"
      };

      this.listeners = new Set();
    }

    subscribe(listener) {
      this.listeners.add(listener);
      return () => this.listeners.delete(listener);
    }

    notify() {
      this.listeners.forEach((listener) => listener(this.state));
    }

    setView(viewName) {
      if (this.state.currentView !== viewName) {
        this.state.previousView = this.state.currentView;
        this.state.currentView = viewName;
        if (viewName !== "settings") {
          this.state.settingsVisible = false;
        }
        this.notify();
      }
    }

    setAgentMode(mode) {
      this.state.agentMode = mode;
      this.notify();
    }

    toggleSettings() {
      if (this.state.currentView === "settings") {
        this.setView(this.state.previousView || "chat");
      } else {
        this.setView("settings");
      }
    }

    setKnowledgeTab(tab) {
      this.state.knowledgeTab = tab;
      this.notify();
    }

    setSettingsSection(section) {
      this.state.settingsSection = section;
      this.notify();
    }

    simulateStreamingResponse(onProgress, onDone) {
      if (this.state.isBusy) return;

      this.state.chatMessages.push({
        id: `msg-user-${Date.now()}`,
        role: "user",
        text: "请帮我分析对话界面重构为 Agent Turn 后的优势。",
        createdAt: Date.now(),
        status: "completed"
      });

      this.state.isBusy = true;
      this.state.activity = "thinking";
      this.state.statusText = "正在思考...";
      this.notify();

      setTimeout(() => {
        this.state.activity = "idle";
        this.state.statusText = "生成中...";

        const newMsgId = `msg-stream-${Date.now()}`;
        const fullText = "重构为统一 **Agent Turn（智能体回合）** 后具有以下三大飞跃式提升：\n\n1. **杜绝孤儿卡片**：不再有漂浮在底部的系统运行框，所有思考、工具与回答收拢在同一 AI 容器内；\n2. **垂直屏效倍增**：思考推导与多步工具轨迹默认收进顶部紧凑折叠托盘（28px），不抢占正文空间；\n3. **Token 透明化**：清晰呈现 `输入 21.2k · 输出 86`，消除几句对话两万 Token 的误解。";

        const streamMsg = {
          id: newMsgId,
          role: "assistant",
          reasoning: "正在推演 Agent Turn 架构收益：\n1. 消息模型从平铺流升级为聚合回合模型；\n2. 思考推导与工具链紧凑折叠收纳；\n3. 输入/输出 Token 明细化拆解呈现；\n4. 消除底部孤儿卡片堆积现象。",
          text: "",
          createdAt: Date.now(),
          status: "streaming",
          tokens: { input: 21240, output: 86, total: 21326 }
        };

        this.state.chatMessages.push(streamMsg);
        this.notify();

        let charIndex = 0;
        const timer = setInterval(() => {
          if (charIndex < fullText.length) {
            charIndex += 3;
            streamMsg.text = fullText.slice(0, charIndex);
            if (onProgress) onProgress();
            this.notify();
          } else {
            clearInterval(timer);
            streamMsg.status = "completed";
            this.state.isBusy = false;
            this.state.statusText = "在线";
            if (onDone) onDone();
            this.notify();
          }
        }, 30);
      }, 1200);
    }

    simulateToolExecution() {
      if (this.state.isBusy) return;

      this.state.chatMessages.push({
        id: `msg-user-tool-${Date.now()}`,
        role: "user",
        text: "请运行类型检查，确认 TypeScript 编译是否通过。",
        createdAt: Date.now(),
        status: "completed"
      });

      this.state.isBusy = true;
      this.state.activity = "tool";
      this.state.statusText = "正在调用工具...";

      const toolCallId = `tool-call-${Date.now()}`;
      const toolCallMsg = {
        id: toolCallId,
        role: "tool_call",
        toolName: "run_command",
        toolArguments: JSON.stringify({ CommandLine: "npm run typecheck", Cwd: "mobile" }, null, 2),
        status: "running"
      };

      this.state.chatMessages.push(toolCallMsg);
      this.notify();

      setTimeout(() => {
        toolCallMsg.status = "completed";
        const toolResMsg = {
          id: `tool-res-${Date.now()}`,
          role: "tool",
          toolName: "run_command",
          text: "Exit Code: 0\nOutput: > tsc --noEmit\nDone in 1.48s. 0 errors found.",
          status: "completed"
        };
        this.state.chatMessages.push(toolResMsg);

        const finalReplyMsg = {
          id: `tool-reply-${Date.now()}`,
          role: "assistant",
          text: "TypeScript 类型检查已顺利执行完成！全工程 **0 编译错误**，类型系统完好就绪。",
          createdAt: Date.now(),
          status: "completed",
          tokens: { input: 12400, output: 42, total: 12442 }
        };
        this.state.chatMessages.push(finalReplyMsg);

        this.state.isBusy = false;
        this.state.activity = "idle";
        this.state.statusText = "在线";
        this.notify();
      }, 1600);
    }

    triggerPermissionPrompt() {
      this.state.activity = "permission";
      this.state.statusText = "等待权限确认...";
      this.state.permissionPrompt = {
        name: "bash",
        permission: "运行本地 Shell 命令",
        arguments: "npm run build:android:apk --profile preview"
      };
      this.notify();
    }

    resolvePermission(allowed) {
      this.state.permissionPrompt = null;
      this.state.activity = "idle";
      this.state.statusText = "在线";

      this.state.chatMessages.push({
        id: `msg-perm-res-${Date.now()}`,
        role: "assistant",
        text: allowed ? "✅ 您已允许执行构建命令，任务已在后台排队运行。" : "❌ 操作已被用户拒绝，取消执行命令。",
        createdAt: Date.now(),
        status: "completed"
      });

      this.notify();
    }

    toggleConnection() {
      this.state.connected = !this.state.connected;
      this.state.statusText = this.state.connected ? "在线" : "离线";
      this.notify();
    }
  }

  const store = new Store();

  // ==========================================
  // 3. 通用工具与组件 (Components)
  // ==========================================
  function escapeHtml(text) {
    if (!text) return "";
    return text
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  }

  function showToast(message, duration = 2000) {
    let toast = document.getElementById("ui-toast");
    if (!toast) {
      toast = document.createElement("div");
      toast.id = "ui-toast";
      toast.style.cssText = `
        position: absolute;
        top: 70px;
        left: 50%;
        transform: translateX(-50%);
        background: #12100e;
        color: #fffdf7;
        padding: 8px 16px;
        border-radius: 999px;
        font-size: 12px;
        font-weight: 800;
        z-index: 1000;
        box-shadow: 0 4px 12px rgba(0,0,0,0.2);
        pointer-events: none;
        transition: opacity 0.2s ease, transform 0.2s ease;
        opacity: 0;
      `;
      document.querySelector(".screen-viewport")?.appendChild(toast);
    }

    toast.textContent = message;
    toast.style.opacity = "1";
    toast.style.transform = "translateX(-50%) translateY(0)";

    setTimeout(() => {
      toast.style.opacity = "0";
      toast.style.transform = "translateX(-50%) translateY(-10px)";
    }, duration);
  }

  function renderStatusBar() {
    const now = new Date();
    const hours = String(now.getHours()).padStart(2, "0");
    const minutes = String(now.getMinutes()).padStart(2, "0");
    const timeStr = `${hours}:${minutes}`;

    return `
      <div class="android-status-bar">
        <div class="status-left">
          <span class="status-time">${timeStr}</span>
        </div>
        <div class="camera-punchhole"></div>
        <div class="status-right">
          <svg class="status-icon-svg" viewBox="0 0 24 24"><path d="M12 3c-4.97 0-9 4.03-9 9 0 2.12.74 4.07 1.97 5.61L12 22l7.03-4.39C20.26 16.07 21 14.12 21 12c0-4.97-4.03-9-9-9zm0 14c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5z"/></svg>
          <svg class="status-icon-svg" viewBox="0 0 24 24"><path d="M12 4C7.31 4 3.07 5.9 0 8.98L12 21 24 8.98A16.88 16.88 0 0 0 12 4zm0 4.5c3.08 0 5.92 1.05 8.16 2.83L12 19.34 3.84 11.33A12.44 12.44 0 0 1 12 8.5z"/></svg>
          <svg class="status-icon-svg" viewBox="0 0 24 24"><path d="M17 5v14c0 .55-.45 1-1 1H8c-.55 0-1-.45-1-1V5c0-.55.45-1 1-1h2V2h4v2h2c.55 0 1 .45 1 1zm-2 1H9v12h6V6z"/></svg>
        </div>
      </div>
    `;
  }

  function renderAppHeader(state) {
    const { connected, activity, statusText, relaySettings, currentView } = state;
    const isOnline = connected;
    const isBusy = activity !== "idle";

    let subtitleText = isOnline
      ? `手机控制电脑 / ${relaySettings.userID || "本地用户"} / ${relaySettings.deviceID || "本地设备"}`
      : "未连接 Relay 中继服务";

    if (activity === "thinking") subtitleText = "正在思考中...";
    if (activity === "tool") subtitleText = "正在调用系统工具...";
    if (activity === "permission") subtitleText = "等待用户确认权限...";

    let pillClass = isOnline ? "online" : "offline";
    if (isBusy) pillClass = "busy";

    const avatarClass = isBusy ? "brand-avatar animating" : "brand-avatar";

    return `
      <header class="app-header">
        <div class="brand-section">
          <div class="brand-mark">
            <img class="${avatarClass}" src="./assets/icon.png" alt="MYAI 图标" onerror="this.src='./assets/favicon.png'" />
          </div>
          <div class="brand-info">
            <span class="brand-title">MYAI</span>
            <span class="brand-subtitle" title="${subtitleText}">${subtitleText}</span>
          </div>
        </div>
        <div class="header-actions">
          <button id="btn-toggle-settings" class="btn-icon-ghost ${currentView === 'settings' ? 'active' : ''}" title="设置">
            ⚙
          </button>
          <div class="status-pill ${pillClass}">
            <span class="status-dot ${isBusy ? 'pulsing' : ''}"></span>
            <span>${statusText}</span>
          </div>
        </div>
      </header>
    `;
  }

  function renderBottomDock(state) {
    const { currentView, isBusy } = state;
    const isChatView = currentView === "chat";

    const tabs = [
      { key: "chat", label: "对话" },
      { key: "files", label: "文件" },
      { key: "changes", label: "变更" },
      { key: "knowledge", label: "知识" },
      { key: "settings", label: "设置" }
    ];

    return `
      <div class="bottom-dock">
        ${isChatView ? `
          <div class="composer-box">
            <button id="btn-upload-file" class="composer-btn-upload" title="附加文件">📎</button>
            <textarea id="composer-text-input" class="composer-input hide-scrollbar" rows="1" placeholder="输入需求，AI 将控制电脑执行..."></textarea>
            <button id="btn-send-message" class="composer-btn-send ${isBusy ? 'btn-pause' : ''}" title="${isBusy ? '暂停' : '发送'}">
              ${isBusy ? '■' : '▶'}
            </button>
          </div>
        ` : ''}

        <nav class="bottom-tabs">
          ${tabs.map(tab => `
            <div class="bottom-tab-item ${currentView === tab.key ? 'active' : ''}" data-tab="${tab.key}">
              ${tab.label}
            </div>
          `).join("")}
        </nav>
      </div>
    `;
  }

  function renderPermissionModal(permission) {
    if (!permission) return "";

    return `
      <div class="permission-modal-overlay">
        <div class="permission-card">
          <div class="permission-title">
            <span class="permission-badge-warn">危险操作授权</span>
            <span>${permission.name} 申请权限</span>
          </div>
          <div style="font-size: 13px; font-weight: 800; color: var(--ink);">
            权限项：${permission.permission}
          </div>
          <div class="permission-content-box">
            ${permission.arguments}
          </div>
          <div class="permission-actions">
            <button id="btn-perm-deny" class="permission-btn btn-deny">
              拒绝执行
            </button>
            <button id="btn-perm-allow" class="permission-btn btn-allow">
              确认允许
            </button>
          </div>
        </div>
      </div>
    `;
  }

  // ==========================================
  // 4. 屏幕渲染器 (Screens)
  // ==========================================
  function formatSimpleMarkdown(text) {
    if (!text) return "";
    let html = escapeHtml(text);
    html = html.replace(/```([\s\S]*?)```/g, '<pre><code>$1</code></pre>');
    html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/^### (.*$)/gim, '<h4 style="margin: 6px 0 4px; font-weight: 900;">$1</h4>');
    html = html.replace(/^\- (.*$)/gim, '<li style="margin-left: 14px;">$1</li>');
    html = html.replace(/\n\n/g, '<p style="margin-top: 6px;"></p>');
    html = html.replace(/\n/g, '<br/>');
    return html;
  }

  function formatTokenCount(num) {
    if (typeof num !== "number") return "n/a";
    if (num >= 1000) {
      return (num / 1000).toFixed(1).replace(/\.0$/, "") + "k";
    }
    return num.toString();
  }

  function buildChatTurns(messages) {
    const turns = [];
    let currentAgentTurn = null;

    messages.forEach((msg) => {
      if (msg.role === "user") {
        currentAgentTurn = null;
        turns.push({
          type: "user",
          id: msg.id,
          text: msg.text,
          createdAt: msg.createdAt || Date.now(),
          attachments: msg.attachments || []
        });
        return;
      }

      if (!currentAgentTurn) {
        currentAgentTurn = {
          type: "agent",
          id: `agent-turn-${msg.id}`,
          model: "Claude 3.7 Sonnet",
          status: "completed",
          elapsed: "3.2s",
          reasoning: "",
          tools: [],
          text: "",
          tokens: null,
          createdAt: msg.createdAt || Date.now()
        };
        turns.push(currentAgentTurn);
      }

      if (msg.role === "assistant") {
        if (msg.reasoning) {
          currentAgentTurn.reasoning = currentAgentTurn.reasoning
            ? currentAgentTurn.reasoning + "\n\n" + msg.reasoning
            : msg.reasoning;
        }
        if (msg.text) {
          currentAgentTurn.text = currentAgentTurn.text
            ? currentAgentTurn.text + "\n\n" + msg.text
            : msg.text;
        }
        if (msg.tokens) {
          currentAgentTurn.tokens = msg.tokens;
        }
        if (msg.status === "streaming" || msg.status === "tool_running") {
          currentAgentTurn.status = "running";
        }
      } else if (msg.role === "tool_call") {
        currentAgentTurn.tools.push({
          id: msg.id,
          name: msg.toolName || "tool",
          arguments: msg.toolArguments || "",
          status: msg.status || "completed",
          duration: "0.8s"
        });
        if (msg.status === "running") {
          currentAgentTurn.status = "running";
        }
      } else if (msg.role === "tool") {
        const lastTool = currentAgentTurn.tools[currentAgentTurn.tools.length - 1];
        if (lastTool && lastTool.name === msg.toolName) {
          lastTool.result = msg.text || "";
          lastTool.status = "completed";
        } else {
          currentAgentTurn.tools.push({
            id: msg.id,
            name: msg.toolName || "tool",
            result: msg.text || "",
            status: "completed",
            duration: "0.6s"
          });
        }
      }
    });

    return turns;
  }

  function renderTurn(turn) {
    if (turn.type === "user") {
      return `
        <div class="turn-wrapper user-turn-wrapper">
          <div class="user-turn-bubble">
            ${escapeHtml(turn.text)}
          </div>
          <div class="user-turn-meta">已发送</div>
        </div>
      `;
    }

    const isRunning = turn.status === "running";
    const hasProcess = Boolean(turn.reasoning || (turn.tools && turn.tools.length > 0));
    const toolCount = turn.tools ? turn.tools.length : 0;
    const processSummary = hasProcess
      ? `⚡ 已执行 ${toolCount} 个工具调用 · 耗时 ${turn.elapsed || '2.8s'}`
      : "⚡ 执行活动";

    return `
      <div class="turn-wrapper agent-turn-wrapper">
        <div class="agent-turn-header">
          <div class="agent-identity">
            <div class="agent-avatar-icon">🤖</div>
            <span class="agent-name">MyAI Agent</span>
            <span class="agent-model-pill">${turn.model || 'v2.4 Auto'}</span>
          </div>
          <div class="agent-status-badge ${isRunning ? 'running' : 'completed'}">
            ${isRunning
              ? `<span class="agent-pulse-dot"></span> 正在执行...`
              : `✓ 完成 · ${turn.elapsed || '3.2s'}`
            }
          </div>
        </div>

        <div class="agent-turn-box">
          ${hasProcess ? `
            <div class="agent-process-tray">
              <div class="process-tray-header" onclick="window.__toggleProcessTray(this)">
                <div class="tray-summary-left">
                  <span class="tray-summary-text">${processSummary}</span>
                </div>
                <span class="tray-toggle-btn">展开详情 ▼</span>
              </div>

              <div class="process-tray-body" style="display: none;">
                ${turn.reasoning ? `
                  <div class="tray-sub-section">
                    <div class="tray-sub-title">🧠 思考推导 (Reasoning)</div>
                    <div class="tray-reasoning-box">${escapeHtml(turn.reasoning)}</div>
                  </div>
                ` : ''}

                ${toolCount > 0 ? `
                  <div class="tray-sub-section">
                    <div class="tray-sub-title">🛠️ 工具调用轨迹 (${toolCount} 步)</div>
                    <div class="tool-chain-list">
                      ${turn.tools.map((tool) => `
                        <div class="tool-chain-item">
                          <div class="tool-chain-header" onclick="window.__toggleToolDetail(this)">
                            <span class="tool-status-dot">✓</span>
                            <span class="tool-fn-name">${tool.name}</span>
                            <span class="tool-duration-tag">${tool.duration || '0.7s'}</span>
                            <span class="tool-step-arrow">▼</span>
                          </div>
                          <div class="tool-chain-detail" style="display: none;">
                            ${tool.arguments ? `
                              <div class="tool-detail-label">输入参数:</div>
                              <pre class="tool-detail-pre">${escapeHtml(tool.arguments)}</pre>
                            ` : ''}
                            ${tool.result ? `
                              <div class="tool-detail-label">执行结果:</div>
                              <pre class="tool-detail-pre">${escapeHtml(tool.result)}</pre>
                            ` : ''}
                          </div>
                        </div>
                      `).join("")}
                    </div>
                  </div>
                ` : ''}
              </div>
            </div>
          ` : ''}

          <div class="agent-turn-body markdown-body">
            ${formatSimpleMarkdown(turn.text)}
          </div>

          <div class="agent-turn-footer">
            <div class="agent-actions">
              <button class="agent-btn-mini" onclick="window.__copyAgentResponse(this)">📋 复制</button>
              <button class="agent-btn-mini" onclick="window.__regenerateTurn(this)">🔄 重新生成</button>
            </div>
            ${turn.tokens ? `
              <div class="agent-token-stat">
                <span class="token-highlight">${formatTokenCount(turn.tokens.total)} tokens</span>
                <span>(入 ${formatTokenCount(turn.tokens.input)} · 出 ${formatTokenCount(turn.tokens.output)})</span>
              </div>
            ` : ''}
          </div>
        </div>
      </div>
    `;
  }

  function renderChatScreen(state) {
    const { chatMessages, agentMode } = state;
    const turns = buildChatTurns(chatMessages);

    return `
      <div class="chat-container">
        <div class="mode-switch-wrapper">
          <div class="mode-segmented">
            <button class="mode-segment-btn ${agentMode === 'chat' ? 'active' : ''}" data-mode="chat">
              💬 对话模式
            </button>
            <button class="mode-segment-btn ${agentMode === 'plan' ? 'active' : ''}" data-mode="plan">
              🎯 规划模式
            </button>
          </div>
        </div>

        <div class="chat-messages custom-scrollbar" id="chat-messages-container">
          ${turns.map((turn, index) => renderTurn(turn, index)).join("")}
        </div>
      </div>
    `;
  }

  function renderFilesScreen(state) {
    const { assets, files, selectedFilePreview } = state;

    return `
      <div class="files-container">
        <div class="asset-box">
          <div class="ui-panel-header" style="padding: 0 0 8px 0;">
            <div class="ui-panel-title-group">
              <span class="ui-panel-title">会话资源</span>
              <span class="ui-panel-meta">${assets.length} 个共享资产</span>
            </div>
            <button class="ui-btn-action ui-btn-primary" id="btn-refresh-assets">刷新</button>
          </div>
          ${assets.map(asset => `
            <div class="asset-item">
              <div>
                <div class="asset-title">📦 ${asset.file_name}</div>
                <div class="asset-meta">${asset.path} (${(asset.size / 1024).toFixed(1)} KB)</div>
              </div>
              <a href="${asset.short_url}" target="_blank" class="ui-btn-action" style="padding: 4px 8px; font-size: 11px;">打开</a>
            </div>
          `).join("")}
        </div>

        <div class="file-browser-box">
          <div class="ui-panel-header" style="padding: 0 0 8px 0;">
            <div class="ui-panel-title-group">
              <span class="ui-panel-title">工作区文件</span>
              <span class="ui-panel-meta">PC Agent Workspace</span>
            </div>
            <button class="ui-btn-action" id="btn-refresh-files">刷新目录</button>
          </div>

          <div class="file-breadcrumb">
            <span>📂 d:/Go_All/myai/mobile/src</span>
            <button class="ui-btn-action" style="padding: 2px 6px; font-size: 10px;">上一级</button>
          </div>

          <div style="display: flex; flex-direction: column; max-height: 240px; overflow-y: auto;">
            ${files.map(f => `
              <div class="file-row" data-filename="${f.name}">
                <div class="file-row-left">
                  <span class="file-icon">${f.is_dir ? '📁' : '📄'}</span>
                  <span class="file-name">${f.name}</span>
                </div>
                <span class="file-size">${f.is_dir ? '目录' : (f.size / 1024).toFixed(1) + ' KB'}</span>
              </div>
            `).join("")}
          </div>
        </div>

        ${selectedFilePreview ? `
          <div class="file-browser-box">
            <div class="ui-panel-header" style="padding: 0 0 8px 0;">
              <div class="ui-panel-title-group">
                <span class="ui-panel-title" style="font-size: 14px;">📄 ${selectedFilePreview.path.split('/').pop()}</span>
                <span class="ui-panel-meta">${selectedFilePreview.path}</span>
              </div>
              <button class="ui-btn-action ui-btn-primary" id="btn-attach-preview" style="font-size: 11px;">附加到对话</button>
            </div>
            <pre style="background: #25231f; color: #f7f5f0; padding: 10px; border-radius: 8px; font-family: var(--font-mono); font-size: 11px; overflow-x: auto; max-height: 180px;">${escapeHtml(selectedFilePreview.content)}</pre>
          </div>
        ` : ''}
      </div>
    `;
  }

  function renderChangesScreen(state) {
    const { changes, historyCheckpoints } = state;

    return `
      <div class="changes-container">
        <div class="ui-panel" style="padding: 12px; gap: 10px;">
          <div class="ui-panel-header" style="padding: 0 0 8px 0;">
            <div class="ui-panel-title-group">
              <span class="ui-panel-title">工作区文件变更</span>
              <span class="ui-panel-meta">${changes.length} 个文件变动</span>
            </div>
            <button class="ui-btn-action ui-btn-primary" id="btn-refresh-changes">刷新</button>
          </div>

          <div style="display: flex; flex-direction: column; gap: 8px;">
            ${changes.map(ch => `
              <div class="change-item" data-path="${ch.path}">
                <span class="change-badge ${ch.kind === 'modified' ? 'mod' : ch.kind === 'added' ? 'add' : 'del'}">
                  ${ch.kind === 'modified' ? 'M' : ch.kind === 'added' ? 'A' : 'D'}
                </span>
                <span class="change-path">${ch.path}</span>
                <span style="font-size: 11px; color: var(--text-muted);">查看 Diff ❯</span>
              </div>
            `).join("")}
          </div>
        </div>

        <div class="ui-panel" style="padding: 12px; gap: 10px;">
          <div class="ui-panel-header" style="padding: 0 0 8px 0;">
            <div class="ui-panel-title-group">
              <span class="ui-panel-title">历史版本快照</span>
              <span class="ui-panel-meta">SQLite Checkpoints 检查点</span>
            </div>
            <button class="ui-btn-action" id="btn-refresh-history">检查点</button>
          </div>

          <div style="display: flex; flex-direction: column; gap: 8px;">
            ${historyCheckpoints.map(chk => `
              <div class="checkpoint-item">
                <div class="checkpoint-header">
                  <span>🏷️ ${chk.checkpoint_id}</span>
                  <span style="color: var(--text-muted); font-size: 11px;">${chk.files_count} 文件</span>
                </div>
                <div style="font-size: 12px; font-weight: 700; color: var(--text-primary);">
                  ${chk.message}
                </div>
                <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 4px;">
                  <button class="ui-btn-action" style="padding: 3px 8px; font-size: 11px;" onclick="alert('已模拟对比检查点差异')">对比</button>
                  <button class="ui-btn-action ui-btn-danger" style="padding: 3px 8px; font-size: 11px;" onclick="alert('已模拟回滚至检查点')">一键回滚</button>
                </div>
              </div>
            `).join("")}
          </div>
        </div>
      </div>
    `;
  }

  function renderChangeDetailScreen(state) {
    const { currentDiff } = state;
    const lines = (currentDiff.diff_text || "").split("\n");

    return `
      <div class="diff-container">
        <div class="diff-header-bar">
          <button class="ui-btn-action" id="btn-back-to-changes">
            ❮ 返回变更
          </button>
          <div style="font-size: 13px; font-weight: 900; font-family: var(--font-mono);">
            ${currentDiff.path.split('/').pop()}
          </div>
          <button class="ui-btn-action ui-btn-danger" id="btn-revert-change" style="font-size: 11px;">
            还原文件
          </button>
        </div>

        <div style="display: flex; justify-content: space-between; align-items: center; font-size: 12px; font-weight: 800; padding: 0 4px;">
          <span style="color: var(--text-secondary);">${currentDiff.path}</span>
          <div>
            <span style="color: #7ee787;">+${currentDiff.additions}</span>
            <span style="color: #ffa198; margin-left: 6px;">-${currentDiff.deletions}</span>
          </div>
        </div>

        <div class="diff-view-box custom-scrollbar">
          ${lines.map((line, idx) => {
            let type = "normal";
            if (line.startsWith("+")) type = "add";
            else if (line.startsWith("-")) type = "del";
            else if (line.startsWith("@")) type = "meta";
            return `
              <div class="diff-line ${type}">
                <span class="diff-line-num">${idx + 1}</span>
                <span>${escapeHtml(line)}</span>
              </div>
            `;
          }).join("")}
        </div>
      </div>
    `;
  }

  function renderKnowledgeScreen(state) {
    const { knowledgeTab, knowledgeBases, documents, aiMemories } = state;

    return `
      <div class="knowledge-container">
        <div class="knowledge-subtabs">
          <button class="knowledge-tab-btn ${knowledgeTab === 'documents' ? 'active' : ''}" data-ktab="documents">
            资料知识库
          </button>
          <button class="knowledge-tab-btn ${knowledgeTab === 'memory' ? 'active' : ''}" data-ktab="memory">
            AI 记忆
          </button>
        </div>

        ${knowledgeTab === 'documents' ? `
          <div style="display: flex; flex-direction: column; gap: 10px;">
            <div class="ui-panel-header" style="background: none; padding: 0;">
              <span class="ui-panel-title">向量知识库</span>
              <button class="ui-btn-action ui-btn-primary" style="font-size: 11px;">+ 新建知识库</button>
            </div>

            ${knowledgeBases.map(kb => `
              <div class="kb-card">
                <div style="display: flex; justify-content: space-between; align-items: center;">
                  <span style="font-size: 14px; font-weight: 900; color: var(--text-primary);">📚 ${kb.name}</span>
                  <span class="status-pill online" style="font-size: 10px; min-height: 22px; padding: 0 6px;">已连接</span>
                </div>
                <div style="font-size: 11px; color: var(--text-secondary);">${kb.description}</div>
                <div style="display: flex; gap: 12px; font-size: 11px; color: var(--text-muted); font-weight: 700; margin-top: 4px;">
                  <span>${kb.document_count} 篇文档</span>
                  <span>${kb.chunk_count} 个向量分块</span>
                </div>
              </div>
            `).join("")}

            <div class="ui-panel" style="padding: 12px; gap: 8px;">
              <div class="ui-panel-header" style="padding: 0 0 6px 0;">
                <span class="ui-panel-title" style="font-size: 14px;">已索引文档</span>
                <button class="ui-btn-action" style="font-size: 11px;">上传文档</button>
              </div>
              ${documents.map(doc => `
                <div style="display: flex; justify-content: space-between; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--line-subtle); font-size: 12px;">
                  <div>
                    <div style="font-weight: 800;">📄 ${doc.title}</div>
                    <div style="font-size: 10px; color: var(--text-muted);">${doc.category} / ${(doc.size / 1024).toFixed(1)} KB</div>
                  </div>
                  <span class="status-pill online" style="font-size: 9px; min-height: 20px; padding: 0 6px;">已解析</span>
                </div>
              `).join("")}
            </div>
          </div>
        ` : `
          <div style="display: flex; flex-direction: column; gap: 10px;">
            <div class="ui-panel-header" style="background: none; padding: 0;">
              <span class="ui-panel-title">已沉淀用户记忆</span>
              <button class="ui-btn-action ui-btn-primary" style="font-size: 11px;">+ 添加记忆</button>
            </div>

            ${aiMemories.map(mem => `
              <div class="memory-card">
                <div style="display: flex; justify-content: space-between; align-items: center;">
                  <span style="font-size: 11px; font-weight: 900; background: var(--surface-alt); padding: 2px 6px; border-radius: 4px; border: 1px solid var(--line);">
                    ${mem.category}
                  </span>
                  <span style="font-size: 10px; color: var(--text-muted);">${mem.created_at}</span>
                </div>
                <div style="font-size: 12.5px; font-weight: 700; color: var(--ink); line-height: 1.4; margin-top: 4px;">
                  ${mem.content}
                </div>
                <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 6px; font-size: 10px; color: var(--text-muted);">
                  <span>来源: ${mem.source}</span>
                  <button class="ui-btn-action ui-btn-danger" style="padding: 2px 6px; font-size: 10px;" onclick="alert('已模拟删除记忆')">删除</button>
                </div>
              </div>
            `).join("")}

            <div style="background: var(--paper); border: 2px dashed var(--line); border-radius: 12px; padding: 12px; text-align: center;">
              <div style="font-size: 13px; font-weight: 900;">✨ 记忆自动化固化 (Dream)</div>
              <div style="font-size: 11px; color: var(--text-secondary); margin-top: 4px;">
                Agent 每天凌晨会自动提取高频对话事实并合并至主记忆库
              </div>
              <button class="ui-btn-action ui-btn-primary" style="margin-top: 8px;" onclick="alert('已模拟触发即时 Dream 整理！')">立即运行 Dream 整合</button>
            </div>
          </div>
        `}
      </div>
    `;
  }

  function renderPlanScreen(state) {
    const { plan } = state;
    const completedCount = plan.steps.filter(s => s.status === 'completed').length;
    const totalCount = plan.steps.length;
    const progressPercent = Math.round((completedCount / totalCount) * 100);

    return `
      <div class="plan-container">
        <div class="plan-goal-box">
          <div style="font-size: 11px; font-weight: 900; color: var(--text-muted); text-transform: uppercase;">
            🎯 计划执行总目标
          </div>
          <div style="font-size: 14px; font-weight: 900; color: var(--ink); line-height: 1.35; margin-top: 2px;">
            ${plan.goal}
          </div>
        </div>

        <div class="ui-panel" style="padding: 12px; gap: 8px;">
          <div style="display: flex; justify-content: space-between; font-size: 12px; font-weight: 900;">
            <span>执行进度 (${completedCount}/${totalCount})</span>
            <span style="color: var(--blue-accent);">${progressPercent}%</span>
          </div>
          <div class="plan-progress-track">
            <div class="plan-progress-fill" style="width: ${progressPercent}%;"></div>
          </div>
        </div>

        <div style="display: flex; flex-direction: column; gap: 8px;">
          ${plan.steps.map(step => `
            <div class="plan-step-card">
              <span class="step-num-badge">${step.order}</span>
              <div style="flex: 1; display: flex; flex-direction: column; gap: 2px;">
                <div style="font-size: 13px; font-weight: 900; color: var(--text-primary);">
                  ${step.title}
                </div>
                <div style="font-size: 11px; color: var(--text-secondary); line-height: 1.4;">
                  ${step.description}
                </div>
              </div>
              <span class="status-pill ${step.status === 'completed' ? 'online' : step.status === 'running' ? 'busy' : ''}" style="font-size: 9px; min-height: 22px; padding: 0 6px;">
                ${step.status === 'completed' ? '已完成' : step.status === 'running' ? '运行中' : '等待'}
              </span>
            </div>
          `).join("")}
        </div>

        <div style="display: flex; gap: 10px; margin-top: 6px;">
          <button class="ui-btn-action" id="btn-plan-to-chat" style="flex: 1; height: 42px; justify-content: center; font-size: 13px;">
            返回对话
          </button>
          <button class="ui-btn-action ui-btn-primary" id="btn-execute-plan" style="flex: 1; height: 42px; justify-content: center; font-size: 13px;">
            ⚡ 执行计划
          </button>
        </div>
      </div>
    `;
  }

  function renderSessionsScreen(state) {
    return renderSettingsScreen({ ...state, settingsSection: "session" });
  }

  function renderSettingsScreen(state) {
    const { settingsSection = "session", relaySettings, models, skills, plugins, subagents, sessions = [], deletedSessions = [] } = state;

    const sections = [
      { key: "session", label: "💬 会话" },
      { key: "model", label: "🧠 模型" },
      { key: "connection", label: "⚡ 连接" },
      { key: "general", label: "⚙️ 常规" },
      { key: "permission", label: "🛡️ 权限" },
      { key: "subagent", label: "🤖 智能体" },
      { key: "skill", label: "🛠️ 技能" },
      { key: "plugin", label: "🔌 插件" }
    ];

    let contentHtml = "";

    // 1. 会话管理中心 (核心改造: 废弃生硬两列方块，采用 Neo-Brutalism 单列流式卡片与当前激活高亮)
    if (settingsSection === "session") {
      const activeSession = sessions[0] || {
        id: "30de8b48",
        title: "你好",
        model: "grok-4.6",
        permission_mode: "ask",
        context_window_k: 16,
        tokens: 187435,
        mode: "chat"
      };

      contentHtml = `
        <div style="display: flex; flex-direction: column; gap: 10px;">
          <!-- 会话列表顶栏 -->
          <div style="display: flex; justify-content: space-between; align-items: center; padding: 2px 0;">
            <div>
              <span style="font-size: 15px; font-weight: 900; color: var(--ink);">会话管理</span>
              <span style="font-size: 11px; color: var(--text-muted); font-weight: 700; margin-left: 6px;">${sessions.length} 个活跃记录</span>
            </div>
            <button class="ui-btn-action ui-btn-primary" style="font-size: 12px; font-weight: 900; padding: 4px 12px;" onclick="alert('已模拟创建新会话！')">
              + 新建会话
            </button>
          </div>

          <!-- 当前激活会话卡片 (黄色侧边条指示 + 模式药丸选择器) -->
          <div class="active-session-card" style="background: var(--surface); border: var(--border-main); border-left: 6px solid var(--yellow); border-radius: var(--radius-md); padding: 12px; box-shadow: var(--shadow-sm); display: flex; flex-direction: column; gap: 8px;">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <div style="display: flex; align-items: center; gap: 6px;">
                <span style="font-size: 10px; font-weight: 900; background: var(--yellow); border: 1px solid var(--ink); padding: 1px 6px; border-radius: 4px;">当前会话</span>
                <span style="font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); font-weight: 700;">#${activeSession.id || '30de8b48'}</span>
              </div>
              <button class="ui-btn-action ui-btn-danger" style="font-size: 10.5px; padding: 2px 8px;" onclick="alert('已模拟归档/删除当前会话')">删除</button>
            </div>

            <div style="font-size: 14.5px; font-weight: 900; color: var(--ink);">
              ${activeSession.title}
            </div>

            <div style="font-size: 11px; color: var(--text-muted); font-weight: 700;">
              ${activeSession.model || 'grok-4.6'} · 询问授权 · 16K 窗口 · 187k 令牌
            </div>

            <!-- 会话执行模式轻量分段选择 (替代原本占据大半屏幕的双粗按钮) -->
            <div style="display: flex; gap: 6px; margin-top: 4px;">
              <button style="flex: 1; padding: 6px 10px; background: var(--yellow); border: 1.5px solid var(--ink); border-radius: 6px; font-size: 11.5px; font-weight: 900; color: var(--ink); display: flex; align-items: center; justify-content: center; gap: 4px; box-shadow: 0 1px 0 var(--ink);">
                💬 对话执行 (激活)
              </button>
              <button style="flex: 1; padding: 6px 10px; background: var(--surface-alt); border: 1px solid var(--line-light); border-radius: 6px; font-size: 11.5px; font-weight: 700; color: var(--text-secondary); display: flex; align-items: center; justify-content: center; gap: 4px;" onclick="alert('已切换为只读计划规划模式！')">
                📋 计划规划
              </button>
            </div>
          </div>

          <!-- 历史会话流式卡片 (单列优雅排版，告别两列压迫方块) -->
          <div style="display: flex; flex-direction: column; gap: 8px;">
            ${sessions.map((sess, idx) => `
              <div class="session-card ${idx === 0 ? 'active' : ''}" style="display: flex; justify-content: space-between; align-items: center; background: var(--surface); border: var(--border-main); border-radius: var(--radius-md); padding: 10px 12px; box-shadow: var(--shadow-sm); cursor: pointer;" onclick="alert('已切换到会话：${sess.title}')">
                <div style="display: flex; align-items: center; gap: 10px; flex: 1; min-width: 0;">
                  <div style="width: 32px; height: 32px; border-radius: 7px; background: ${idx === 0 ? 'var(--yellow)' : 'var(--cyan)'}; border: 1.5px solid var(--ink); display: flex; align-items: center; justify-content: center; font-size: 14px; flex-shrink: 0;">
                    💬
                  </div>
                  <div style="flex: 1; min-width: 0;">
                    <div style="font-size: 13.5px; font-weight: 900; color: var(--ink); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">
                      ${sess.title}
                    </div>
                    <div style="font-size: 11px; color: var(--text-muted); font-weight: 700; margin-top: 2px;">
                      ${sess.model || 'grok-4.6'} · 对话模式 · 16K
                    </div>
                  </div>
                </div>
                <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 4px; flex-shrink: 0;">
                  <span style="font-size: 10px; color: var(--text-muted); font-weight: 700;">${idx === 0 ? '刚刚' : idx === 1 ? '10分钟前' : '昨天'}</span>
                  <button class="ui-btn-action ui-btn-danger" style="padding: 2px 7px; font-size: 10.5px;" onclick="event.stopPropagation(); alert('已删除会话至回收站');">删除</button>
                </div>
              </div>
            `).join("")}
          </div>

          <!-- 会话回收站 (虚线底衬) -->
          <div style="background: var(--paper); border: 2px dashed var(--line); border-radius: 12px; padding: 12px; margin-top: 4px;">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span style="font-size: 12.5px; font-weight: 900; color: var(--ink);">🗑️ 回收站 (${deletedSessions.length || 2})</span>
              <span style="font-size: 10.5px; color: var(--text-muted); font-weight: 700;">保留 30 天</span>
            </div>
            <div style="display: flex; flex-direction: column; gap: 6px; margin-top: 8px;">
              ${(deletedSessions.length > 0 ? deletedSessions : [
                { id: "del-1", title: "测试旧代码检查会话" },
                { id: "del-2", title: "临时排查端口冲突" }
              ]).map(ds => `
                <div style="display: flex; justify-content: space-between; align-items: center; font-size: 12px; padding: 4px 0; border-bottom: 1px solid var(--line-subtle);">
                  <span style="color: var(--text-secondary); text-decoration: line-through;">${ds.title}</span>
                  <button class="ui-btn-action ui-btn-secondary" style="padding: 2px 8px; font-size: 10px;" onclick="alert('已模拟恢复该会话！')">恢复</button>
                </div>
              `).join("")}
            </div>
          </div>
        </div>
      `;
    } else if (settingsSection === "permission") {
      // 2. 权限模式策略
      contentHtml = `
        <div class="settings-section-card">
          <span class="ui-panel-title">会话工具调用权限策略</span>
          <div style="display: flex; flex-direction: column; gap: 8px; margin-top: 4px;">
            <div style="background: var(--surface-alt); border: var(--border-main); border-radius: var(--radius-sm); padding: 10px; display: flex; justify-content: space-between; align-items: center;">
              <div>
                <div style="font-size: 13px; font-weight: 900;">🔒 只读模式 (Readonly)</div>
                <div style="font-size: 11px; color: var(--text-muted); margin-top: 2px;">禁止所有写入与外部终端命令执行</div>
              </div>
              <button class="ui-btn-action ui-btn-secondary" style="font-size: 11px;" onclick="alert('已切换为只读模式')">选择</button>
            </div>

            <div style="background: var(--surface); border: 2px solid var(--ink); border-left: 6px solid var(--yellow); border-radius: var(--radius-sm); padding: 10px; display: flex; justify-content: space-between; align-items: center; box-shadow: var(--shadow-sm);">
              <div>
                <div style="font-size: 13px; font-weight: 900; color: var(--ink);">🛡️ 询问确认 (Ask · 当前推荐)</div>
                <div style="font-size: 11px; color: var(--text-muted); margin-top: 2px;">每个工具调用、写文件或跑命令需弹窗授权</div>
              </div>
              <span style="font-size: 11px; font-weight: 900; color: var(--ink); background: var(--yellow); padding: 2px 8px; border-radius: 4px; border: 1px solid var(--ink);">已生效</span>
            </div>

            <div style="background: var(--surface-alt); border: var(--border-main); border-radius: var(--radius-sm); padding: 10px; display: flex; justify-content: space-between; align-items: center;">
              <div>
                <div style="font-size: 13px; font-weight: 900;">⚡ 完全开放 (Full)</div>
                <div style="font-size: 11px; color: var(--text-muted); margin-top: 2px;">自动放行所有授权范围内的工具指令</div>
              </div>
              <button class="ui-btn-action ui-btn-secondary" style="font-size: 11px;" onclick="alert('已切换为完全开放模式')">选择</button>
            </div>
          </div>
        </div>
      `;
    } else if (settingsSection === "connection") {
      contentHtml = `
        <div class="settings-section-card">
          <span class="ui-panel-title">Relay 中继连接</span>
          <div class="input-field-group">
            <label class="input-field-label">中继服务器 URL</label>
            <input class="ui-text-input" value="${relaySettings.relayURL}" />
          </div>
          <div class="input-field-group">
            <label class="input-field-label">Agent 6位配对码 (Bind Code)</label>
            <input class="ui-text-input" value="${relaySettings.bindCode}" maxlength="6" />
          </div>
          <div style="display: flex; gap: 10px; margin-top: 6px;">
            <button class="ui-btn-action ui-btn-primary" style="flex: 1; height: 38px; justify-content: center;" onclick="alert('配对成功，已交换 Client Token！')">
              配对 (Pair)
            </button>
            <button class="ui-btn-action ui-btn-secondary" style="flex: 1; height: 38px; justify-content: center;" onclick="alert('WebSocket 连接建立成功！')">
              连接 (Connect)
            </button>
          </div>
        </div>
      `;
    } else if (settingsSection === "model") {
      contentHtml = `
        <div class="settings-section-card">
          <div style="display: flex; justify-content: space-between; align-items: center;">
            <span class="ui-panel-title">模型管理中心</span>
            <button class="ui-btn-action ui-btn-primary" style="font-size: 11px;">+ 添加配置</button>
          </div>
          <div style="display: flex; flex-direction: column; gap: 8px;">
            ${models.map(m => `
              <div style="background: var(--surface-alt); border: var(--border-main); border-radius: var(--radius-sm); padding: 10px; display: flex; justify-content: space-between; align-items: center;">
                <div>
                  <div style="font-size: 13px; font-weight: 900;">${m.name} ${m.is_default ? '⭐' : ''}</div>
                  <div style="font-size: 10.5px; color: var(--text-muted); font-family: var(--font-mono);">${m.protocol} · ${m.context_window / 1000}k 窗口</div>
                </div>
                <button class="ui-btn-action" style="padding: 3px 8px; font-size: 10.5px;" onclick="alert('测试模型连通性：延迟 420ms，状态正常！')">测试</button>
              </div>
            `).join("")}
          </div>
        </div>
      `;
    } else if (settingsSection === "subagent") {
      contentHtml = `
        <div class="settings-section-card">
          <div style="display: flex; justify-content: space-between; align-items: center;">
            <span class="ui-panel-title">子智能体 (Subagents)</span>
            <button class="ui-btn-action ui-btn-primary" style="font-size: 11px;">+ 新建定义</button>
          </div>
          <div style="display: flex; flex-direction: column; gap: 8px;">
            ${subagents.map(sa => `
              <div style="background: var(--surface-alt); border: var(--border-main); border-radius: var(--radius-sm); padding: 10px; display: flex; justify-content: space-between; align-items: center;">
                <div>
                  <div style="font-size: 13px; font-weight: 900;">🤖 ${sa.name}</div>
                  <div style="font-size: 11px; color: var(--text-secondary);">${sa.role}</div>
                </div>
                <span class="status-pill ${sa.status === 'running' ? 'busy' : 'online'}" style="font-size: 9px; min-height: 20px; padding: 0 6px;">
                  ${sa.status === 'running' ? '运行中' : '就绪'}
                </span>
              </div>
            `).join("")}
          </div>
        </div>
      `;
    } else if (settingsSection === "skill") {
      contentHtml = `
        <div class="settings-section-card">
          <span class="ui-panel-title">本地技能中心 (Skills)</span>
          <div style="display: flex; flex-direction: column; gap: 8px;">
            ${skills.map(sk => `
              <div style="background: var(--surface-alt); border: var(--border-main); border-radius: var(--radius-sm); padding: 10px;">
                <div style="font-size: 13px; font-weight: 900;">🛠️ ${sk.name}</div>
                <div style="font-size: 11px; color: var(--text-secondary); margin-top: 2px;">${sk.desc}</div>
              </div>
            `).join("")}
          </div>
        </div>
      `;
    } else if (settingsSection === "plugin") {
      contentHtml = `
        <div class="settings-section-card">
          <span class="ui-panel-title">扩展插件 (Plugins)</span>
          <div style="display: flex; flex-direction: column; gap: 8px;">
            ${plugins.map(pl => `
              <div style="background: var(--surface-alt); border: var(--border-main); border-radius: var(--radius-sm); padding: 10px; display: flex; justify-content: space-between; align-items: center;">
                <div>
                  <div style="font-size: 13px; font-weight: 900;">🔌 ${pl.name}</div>
                  <div style="font-size: 11px; color: var(--text-secondary);">${pl.desc}</div>
                </div>
                <span class="status-pill online" style="font-size: 9px; min-height: 20px; padding: 0 6px;">已启用</span>
              </div>
            `).join("")}
          </div>
        </div>
      `;
    } else {
      contentHtml = `
        <div class="settings-section-card">
          <span class="ui-panel-title">系统状态总览</span>
          <div style="display: flex; flex-direction: column; gap: 8px; font-size: 13px;">
            <div style="display: flex; justify-content: space-between; border-bottom: 1px solid var(--line-subtle); padding-bottom: 6px;">
              <span style="color: var(--text-secondary);">客户端版本</span>
              <span style="font-weight: 800;">v1.0.4 (Build 8)</span>
            </div>
            <div style="display: flex; justify-content: space-between; border-bottom: 1px solid var(--line-subtle); padding-bottom: 6px;">
              <span style="color: var(--text-secondary);">设备识别号</span>
              <span style="font-weight: 800; font-family: var(--font-mono);">${relaySettings.deviceID}</span>
            </div>
            <div style="display: flex; justify-content: space-between; border-bottom: 1px solid var(--line-subtle); padding-bottom: 6px;">
              <span style="color: var(--text-secondary);">OTA 热更新</span>
              <span style="font-weight: 800; color: var(--green-dark);">已是最新版本</span>
            </div>
          </div>
          <button class="ui-btn-action ui-btn-primary" style="margin-top: 10px; height: 38px; justify-content: center;" onclick="alert('正在检查 Expo OTA 热更新...')">
            检查最新更新
          </button>
        </div>
      `;
    }

    return `
      <div class="settings-container">
        <!-- 顶部标题栏 (去除冗余完成按钮，符合主流Tab规范) -->
        <div class="ui-panel-header" style="background: none; padding: 0 0 2px 0;">
          <div class="ui-panel-title-group">
            <span class="ui-panel-title" style="font-size: 18px; font-weight: 900; color: var(--ink);">设置中心</span>
            <span class="ui-panel-meta">连接、模型、会话与工具权限集中管理</span>
          </div>
        </div>

        <!-- 横向二级导航药丸 (直观文字与图标) -->
        <div class="settings-pill-nav hide-scrollbar">
          ${sections.map(s => `
            <button class="settings-nav-pill ${settingsSection === s.key ? 'active' : ''}" data-ssec="${s.key}">
              ${s.label}
            </button>
          `).join("")}
        </div>
        ${contentHtml}
      </div>
    `;
  }

  // ==========================================
  // 5. 原型控制器驱动 (Master App Controller)
  // ==========================================
  class App {
    constructor() {
      this.phoneContainer = document.getElementById("phone-viewport");
      this.init();
    }

    init() {
      store.subscribe((state) => this.render(state));
      this.bindWorkbenchEvents();
      this.render(store.state);
    }

    render(state) {
      if (!this.phoneContainer) return;

      let screenHtml = "";
      switch (state.currentView) {
        case "chat": screenHtml = renderChatScreen(state); break;
        case "files": screenHtml = renderFilesScreen(state); break;
        case "changes": screenHtml = renderChangesScreen(state); break;
        case "changeDetail": screenHtml = renderChangeDetailScreen(state); break;
        case "knowledge": screenHtml = renderKnowledgeScreen(state); break;
        case "plan": screenHtml = renderPlanScreen(state); break;
        case "sessions": screenHtml = renderSessionsScreen(state); break;
        case "settings": screenHtml = renderSettingsScreen(state); break;
        default: screenHtml = renderChatScreen(state);
      }

      this.phoneContainer.innerHTML = `
        <div class="screen-viewport">
          ${renderStatusBar()}
          <div class="app-container">
            ${renderAppHeader(state)}
            <main class="app-screen-body custom-scrollbar" id="screen-body">
              ${screenHtml}
            </main>
            ${renderBottomDock(state)}
          </div>
          <div class="android-nav-bar">
            <div class="gesture-bar"></div>
          </div>
          ${renderPermissionModal(state.permissionPrompt)}
        </div>
      `;

      this.bindScreenEvents(state);
      this.syncWorkbenchActive(state.currentView);
    }

    bindScreenEvents(state) {
      document.querySelectorAll(".bottom-tab-item").forEach((tabEl) => {
        tabEl.addEventListener("click", () => {
          const tabKey = tabEl.getAttribute("data-tab");
          if (tabKey) store.setView(tabKey);
        });
      });

      const btnSettings = document.getElementById("btn-toggle-settings");
      if (btnSettings) {
        btnSettings.addEventListener("click", () => store.toggleSettings());
      }

      document.querySelectorAll(".mode-segment-btn").forEach((btn) => {
        btn.addEventListener("click", () => {
          const mode = btn.getAttribute("data-mode");
          if (mode === "plan") {
            store.setView("plan");
          } else {
            store.setAgentMode("chat");
            store.setView("chat");
          }
        });
      });

      const btnSend = document.getElementById("btn-send-message");
      const inputMsg = document.getElementById("composer-text-input");
      if (btnSend && inputMsg) {
        btnSend.addEventListener("click", () => {
          if (state.isBusy) {
            showToast("已请求暂停当前任务");
            return;
          }
          const text = inputMsg.value.trim();
          if (!text) {
            showToast("请输入有效指令");
            return;
          }
          store.state.chatMessages.push({
            id: `msg-user-${Date.now()}`,
            role: "user",
            text,
            createdAt: Date.now(),
            status: "completed"
          });
          inputMsg.value = "";
          store.simulateStreamingResponse(() => {
            this.scrollChatToBottom();
          });
        });
      }

      document.querySelectorAll(".change-item").forEach((item) => {
        item.addEventListener("click", () => {
          store.setView("changeDetail");
        });
      });

      const btnBackDiff = document.getElementById("btn-back-to-changes");
      if (btnBackDiff) {
        btnBackDiff.addEventListener("click", () => store.setView("changes"));
      }

      const btnRevertChange = document.getElementById("btn-revert-change");
      if (btnRevertChange) {
        btnRevertChange.addEventListener("click", () => {
          showToast("已成功回滚该文件至基准版本！");
          setTimeout(() => store.setView("changes"), 800);
        });
      }

      document.querySelectorAll(".knowledge-tab-btn").forEach((btn) => {
        btn.addEventListener("click", () => {
          const ktab = btn.getAttribute("data-ktab");
          if (ktab) store.setKnowledgeTab(ktab);
        });
      });

      document.querySelectorAll(".settings-nav-pill").forEach((btn) => {
        btn.addEventListener("click", () => {
          const sec = btn.getAttribute("data-ssec");
          if (sec) store.setSettingsSection(sec);
        });
      });

      const btnAllow = document.getElementById("btn-perm-allow");
      const btnDeny = document.getElementById("btn-perm-deny");
      if (btnAllow) {
        btnAllow.addEventListener("click", () => {
          store.resolvePermission(true);
          showToast("已授予执行权限");
        });
      }
      if (btnDeny) {
        btnDeny.addEventListener("click", () => {
          store.resolvePermission(false);
          showToast("已拒绝权限申请");
        });
      }

      const btnPlanToChat = document.getElementById("btn-plan-to-chat");
      if (btnPlanToChat) {
        btnPlanToChat.addEventListener("click", () => store.setView("chat"));
      }
      const btnExecutePlan = document.getElementById("btn-execute-plan");
      if (btnExecutePlan) {
        btnExecutePlan.addEventListener("click", () => {
          showToast("🚀 正在调度执行计划，任务已分配给子智能体...");
          store.state.plan.steps[2].status = "completed";
          store.state.plan.steps[3].status = "running";
          setTimeout(() => store.setView("chat"), 1000);
        });
      }

      const btnUpload = document.getElementById("btn-upload-file");
      if (btnUpload) {
        btnUpload.addEventListener("click", () => {
          showToast("已模拟选择本地文件: architecture-spec.md");
        });
      }
    }

    bindWorkbenchEvents() {
      document.querySelectorAll(".wb-nav-item").forEach((item) => {
        item.addEventListener("click", () => {
          const targetView = item.getAttribute("data-view");
          if (targetView) store.setView(targetView);
        });
      });

      document.getElementById("trig-stream-chat")?.addEventListener("click", () => {
        store.setView("chat");
        store.simulateStreamingResponse(() => this.scrollChatToBottom());
      });

      document.getElementById("trig-tool-call")?.addEventListener("click", () => {
        store.setView("chat");
        store.simulateToolExecution();
      });

      document.getElementById("trig-permission-prompt")?.addEventListener("click", () => {
        store.triggerPermissionPrompt();
      });

      document.getElementById("trig-toggle-online")?.addEventListener("click", () => {
        store.toggleConnection();
      });

      document.getElementById("trig-switch-mode")?.addEventListener("click", () => {
        const nextMode = store.state.agentMode === "chat" ? "plan" : "chat";
        store.setAgentMode(nextMode);
        store.setView(nextMode === "plan" ? "plan" : "chat");
        showToast(`已切换至: ${nextMode === "plan" ? "规划模式" : "对话模式"}`);
      });

      const btnFrameMode = document.getElementById("btn-toggle-device-frame");
      btnFrameMode?.addEventListener("click", () => {
        const isFlat = document.body.classList.toggle("phone-mode-flat");
        btnFrameMode.textContent = isFlat ? "切为真机外壳" : "切为平铺画布";
        btnFrameMode.classList.toggle("active", isFlat);
      });
    }

    syncWorkbenchActive(currentView) {
      document.querySelectorAll(".wb-nav-item").forEach((item) => {
        const view = item.getAttribute("data-view");
        item.classList.toggle("active", view === currentView);
      });
    }

    scrollChatToBottom() {
      const container = document.getElementById("chat-messages-container");
      if (container) {
        container.scrollTop = container.scrollHeight;
      }
    }
  }

  // 挂载原型工作台全局交互函数
  window.__toggleProcessTray = function(el) {
    const tray = el.closest(".agent-process-tray");
    if (!tray) return;
    const body = tray.querySelector(".process-tray-body");
    const toggleBtn = tray.querySelector(".tray-toggle-btn");
    if (!body || !toggleBtn) return;
    const isHidden = body.style.display === "none";
    body.style.display = isHidden ? "flex" : "none";
    toggleBtn.textContent = isHidden ? "收起详情 ▲" : "展开详情 ▼";
  };

  window.__toggleToolDetail = function(el) {
    const item = el.closest(".tool-chain-item");
    if (!item) return;
    const detail = item.querySelector(".tool-chain-detail");
    const arrow = item.querySelector(".tool-step-arrow");
    if (!detail) return;
    const isHidden = detail.style.display === "none";
    detail.style.display = isHidden ? "block" : "none";
    if (arrow) arrow.textContent = isHidden ? "▲" : "▼";
  };

  window.__copyAgentResponse = function(btn) {
    const card = btn.closest(".agent-turn-box");
    if (!card) return;
    const textEl = card.querySelector(".agent-turn-body");
    if (textEl && navigator.clipboard) {
      navigator.clipboard.writeText(textEl.innerText);
    }
    const orig = btn.innerText;
    btn.innerText = "✓ 已复制";
    setTimeout(() => { btn.innerText = orig; }, 1200);
  };

  window.__regenerateTurn = function(btn) {
    const orig = btn.innerText;
    btn.innerText = "⚡ 生成中...";
    setTimeout(() => { btn.innerText = orig; }, 1000);
  };

  // 启动挂载
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => new App());
  } else {
    new App();
  }
})();
