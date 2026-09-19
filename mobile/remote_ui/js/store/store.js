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

    // 先推入用户指令
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

    // 1.2 秒后思考完毕，开始输出流式消息
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

  // 2. 模拟工具调用执行过程
  simulateToolExecution() {
    if (this.state.isBusy) return;

    // 先推入用户指令
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

      // 伴随最终助手回答
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
