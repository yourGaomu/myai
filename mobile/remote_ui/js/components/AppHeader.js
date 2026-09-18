/**
 * MyAI Mobile UI Prototype - AppHeader Component
 * 呼吸动画小人头像、状态药丸、标题与设置入口
 */

export function renderAppHeader(state) {
  const { connected, activity, statusText, relaySettings, currentView } = state;
  const isOnline = connected;
  const isBusy = activity !== "idle";

  // 状态小文本
  let subtitleText = isOnline
    ? `手机控制电脑 / ${relaySettings.userID || "本地用户"} / ${relaySettings.deviceID || "本地设备"}`
    : "未连接 Relay 中继服务";

  if (activity === "thinking") subtitleText = "正在思考中...";
  if (activity === "tool") subtitleText = "正在调用系统工具...";
  if (activity === "permission") subtitleText = "等待用户确认权限...";

  // 药丸样式类名
  let pillClass = isOnline ? "online" : "offline";
  if (isBusy) pillClass = "busy";

  // 头像动画
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
