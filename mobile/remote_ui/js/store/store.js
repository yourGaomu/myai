/**
 * MyAI Mobile UI Prototype - Reactive State Store
 * 单向数据流状态管理器，模拟真实移动端 Hook 交互
 */

import { mockChatMessages, mockAgentRuns } from "../data/mockChat.js";
import { mockChanges, mockHistoryCheckpoints, mockDiffDetail } from "../data/mockChanges.js";
import { mockFileEntries, mockAssets, mockFilePreview } from "../data/mockFiles.js";
import { mockKnowledgeBases, mockCategories, mockDocuments, mockAIMemories } from "../data/mockKnowledge.js";
import { mockPlan } from "../data/mockPlan.js";
import { mockSessions, mockDeletedSessions } from "../data/mockSessions.js";
import { mockRelaySettings, mockModels, mockSkills, mockPlugins, mockSubagents } from "../data/mockSettings.js";

class Store {
  constructor() {
    this.state = {
      // 导航与路由
      currentView: "chat", // "chat" | "files" | "changes" | "changeDetail" | "knowledge" | "plan" | "sessions" | "settings"
      previousView: "chat",
      agentMode: "chat", // "chat" | "plan"
      settingsVisible: false,

      // 连接与 Agent 实时状态
      connected: true,
      activity: "idle", // "idle" | "thinking" | "tool" | "permission" | "uploading"
      statusText: "在线",
      isBusy: false,

      // 弹窗与权限拦截
      permissionPrompt: null, // { name, permission, arguments }

      // 业务数据集
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

      // 页面内部子 Tab 状态
      knowledgeTab: "documents", // "documents" | "memory"
      settingsSection: "general" // "subagent" | "general" | "connection" | "model" | "skill" | "plugin"
    };

    this.listeners = new Set();
  }

  // 状态订阅器
  subscribe(listener) {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  notify() {
    this.listeners.forEach((listener) => listener(this.state));
  }

  // ==================== 视图切换 ====================
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

  // ==================== 场景模拟器 Actions ====================

  // 1. 模拟 Agent 思考并流式打字生成回复
  simulateStreamingResponse(onProgress, onDone) {
    if (this.state.isBusy) return;

    this.state.isBusy = true;
    this.state.activity = "thinking";
    this.state.statusText = "正在思考...";
    this.notify();

    // 1.5 秒后完成思考，开始输出流式消息
    setTimeout(() => {
      this.state.activity = "idle";
      this.state.statusText = "生成中...";

      const newMsgId = `msg-stream-${Date.now()}`;
      const fullText = "收到！我已经为您在 `mobile/remote_ui` 目录下构建好了高保真的 UI 原型系统。\n\n- **设计系统**：复刻 Neo-Brutalism 漫画粗黑线条与高明度配色；\n- **架构体系**：采用解耦的 Store 响应式模型与数据源，支持任意场景一键预览。";

      const streamMsg = {
        id: newMsgId,
        role: "assistant",
        reasoning: "用户触发了实时流式消息演示：\n1. 初始化打字机定时器；\n2. 动态追加字符至当前气泡；\n3. 触发滚动条吸底。",
        text: "",
        createdAt: Date.now(),
        status: "streaming"
      };

      this.state.chatMessages.push(streamMsg);
      this.notify();

      let charIndex = 0;
      const timer = setInterval(() => {
        if (charIndex < fullText.length) {
          charIndex += 2;
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
      }, 40);
    }, 1500);
  }

  // 2. 模拟工具调用执行过程
  simulateToolExecution() {
    this.state.isBusy = true;
    this.state.activity = "tool";
    this.state.statusText = "正在调用工具...";

    const toolCallId = `tool-call-${Date.now()}`;
    const toolCallMsg = {
      id: toolCallId,
      role: "tool_call",
      toolName: "git_diff_summary",
      toolArguments: JSON.stringify({ staged: false, targetPath: "mobile/remote_ui" }, null, 2),
      status: "running"
    };

    this.state.chatMessages.push(toolCallMsg);
    this.notify();

    setTimeout(() => {
      toolCallMsg.status = "completed";
      const toolResMsg = {
        id: `tool-res-${Date.now()}`,
        role: "tool",
        toolName: "git_diff_summary",
        text: "Summary: 8 files created, +1,240 lines added, 0 deletions.",
        status: "completed"
      };
      this.state.chatMessages.push(toolResMsg);
      this.state.isBusy = false;
      this.state.activity = "idle";
      this.state.statusText = "在线";
      this.notify();
    }, 2000);
  }

  // 3. 触发危险权限拦截弹窗
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

  // 4. 解析权限确认/拒绝
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

  // 5. 切换在线/离线
  toggleConnection() {
    this.state.connected = !this.state.connected;
    this.state.statusText = this.state.connected ? "在线" : "离线";
    this.notify();
  }
}

export const store = new Store();
