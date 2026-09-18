/**
 * MyAI Mobile UI Prototype - Master Application Controller
 * 原型总控制器：负责将状态渲染到真机视图与演示台交互绑定
 */

import { store } from "./store/store.js";
import { renderStatusBar } from "./components/StatusBar.js";
import { renderAppHeader } from "./components/AppHeader.js";
import { renderBottomDock } from "./components/BottomDock.js";
import { renderPermissionModal } from "./components/PermissionModal.js";
import { showToast } from "./components/Toast.js";

// 业务屏幕
import { renderChatScreen } from "./screens/ChatScreen.js";
import { renderFilesScreen } from "./screens/FilesScreen.js";
import { renderChangesScreen } from "./screens/ChangesScreen.js";
import { renderChangeDetailScreen } from "./screens/ChangeDetailScreen.js";
import { renderKnowledgeScreen } from "./screens/KnowledgeScreen.js";
import { renderPlanScreen } from "./screens/PlanScreen.js";
import { renderSessionsScreen } from "./screens/SessionsScreen.js";
import { renderSettingsScreen } from "./screens/SettingsScreen.js";

class App {
  constructor() {
    this.phoneContainer = document.getElementById("phone-viewport");
    this.init();
  }

  init() {
    // 监听状态变化并全量局部渲染
    store.subscribe((state) => this.render(state));
    this.bindWorkbenchEvents();
    this.render(store.state);
  }

  render(state) {
    if (!this.phoneContainer) return;

    // 根据当前路由渲染主屏幕
    let screenHtml = "";
    switch (state.currentView) {
      case "chat":
        screenHtml = renderChatScreen(state);
        break;
      case "files":
        screenHtml = renderFilesScreen(state);
        break;
      case "changes":
        screenHtml = renderChangesScreen(state);
        break;
      case "changeDetail":
        screenHtml = renderChangeDetailScreen(state);
        break;
      case "knowledge":
        screenHtml = renderKnowledgeScreen(state);
        break;
      case "plan":
        screenHtml = renderPlanScreen(state);
        break;
      case "sessions":
        screenHtml = renderSessionsScreen(state);
        break;
      case "settings":
        screenHtml = renderSettingsScreen(state);
        break;
      default:
        screenHtml = renderChatScreen(state);
    }

    // 组合真机屏幕结构
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
    // 1. 底部 5Tab 切换
    document.querySelectorAll(".bottom-tab-item").forEach((tabEl) => {
      tabEl.addEventListener("click", () => {
        const tabKey = tabEl.getAttribute("data-tab");
        if (tabKey) store.setView(tabKey);
      });
    });

    // 2. 顶部设置按钮
    const btnSettings = document.getElementById("btn-toggle-settings");
    if (btnSettings) {
      btnSettings.addEventListener("click", () => store.toggleSettings());
    }

    // 3. 对话模式切换胶囊
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

    // 4. 输入框发送与暂停
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

    // 5. 变更列表点击条目跳转至 Diff 详情
    document.querySelectorAll(".change-item").forEach((item) => {
      item.addEventListener("click", () => {
        store.setView("changeDetail");
      });
    });

    // 6. Diff 详情返回变更
    const btnBackDiff = document.getElementById("btn-back-to-changes");
    if (btnBackDiff) {
      btnBackDiff.addEventListener("click", () => store.setView("changes"));
    }

    // 7. 还原变更按钮
    const btnRevertChange = document.getElementById("btn-revert-change");
    if (btnRevertChange) {
      btnRevertChange.addEventListener("click", () => {
        showToast("已成功回滚该文件至基准版本！");
        setTimeout(() => store.setView("changes"), 800);
      });
    }

    // 8. 知识库子 Tab 切换
    document.querySelectorAll(".knowledge-tab-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        const ktab = btn.getAttribute("data-ktab");
        if (ktab) store.setKnowledgeTab(ktab);
      });
    });

    // 9. 设置分栏药丸切换
    document.querySelectorAll(".settings-nav-pill").forEach((btn) => {
      btn.addEventListener("click", () => {
        const sec = btn.getAttribute("data-ssec");
        if (sec) store.setSettingsSection(sec);
      });
    });

    // 10. 权限弹窗事件
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

    // 11. 规划面板按钮
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

    // 12. 文件附件上传模拟
    const btnUpload = document.getElementById("btn-upload-file");
    if (btnUpload) {
      btnUpload.addEventListener("click", () => {
        showToast("已模拟选择本地文件: architecture-spec.md");
      });
    }
  }

  bindWorkbenchEvents() {
    // 左侧快速切屏导航
    document.querySelectorAll(".wb-nav-item").forEach((item) => {
      item.addEventListener("click", () => {
        const targetView = item.getAttribute("data-view");
        if (targetView) store.setView(targetView);
      });
    });

    // 右侧场景模拟触发器
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

    // 切换真机机身 / 平铺画布模式
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

// 启动入口
window.addEventListener("DOMContentLoaded", () => {
  new App();
});
