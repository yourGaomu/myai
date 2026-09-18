/**
 * MyAI Mobile UI Prototype - BottomDock Component
 * 底部 Dock 停靠栏：仅在对话模式显示 Composer，所有模式常驻 5Tab 导航栏
 */

export function renderBottomDock(state) {
  const { currentView, isBusy } = state;
  const isChatView = currentView === "chat";

  // 5 个主 Tab 定义
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
        <!-- 输入框区域 (Composer) -->
        <div class="composer-box">
          <button id="btn-upload-file" class="composer-btn-upload" title="附加文件">
            📎
          </button>
          <textarea id="composer-text-input" class="composer-input hide-scrollbar" rows="1" placeholder="输入需求，AI 将控制电脑执行..."></textarea>
          <button id="btn-send-message" class="composer-btn-send ${isBusy ? 'btn-pause' : ''}" title="${isBusy ? '暂停' : '发送'}">
            ${isBusy ? '■' : '▶'}
          </button>
        </div>
      ` : ''}

      <!-- 底部 5Tab 导航 -->
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
