/**
 * MyAI Mobile UI Prototype - StatusBar Component
 * 动态时钟与 Android 状态图标
 */

export function renderStatusBar() {
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
        <!-- 信号图标 -->
        <svg class="status-icon-svg" viewBox="0 0 24 24"><path d="M12 3c-4.97 0-9 4.03-9 9 0 2.12.74 4.07 1.97 5.61L12 22l7.03-4.39C20.26 16.07 21 14.12 21 12c0-4.97-4.03-9-9-9zm0 14c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5z"/></svg>
        <!-- Wi-Fi 图标 -->
        <svg class="status-icon-svg" viewBox="0 0 24 24"><path d="M12 4C7.31 4 3.07 5.9 0 8.98L12 21 24 8.98A16.88 16.88 0 0 0 12 4zm0 4.5c3.08 0 5.92 1.05 8.16 2.83L12 19.34 3.84 11.33A12.44 12.44 0 0 1 12 8.5z"/></svg>
        <!-- 电池图标 -->
        <svg class="status-icon-svg" viewBox="0 0 24 24"><path d="M17 5v14c0 .55-.45 1-1 1H8c-.55 0-1-.45-1-1V5c0-.55.45-1 1-1h2V2h4v2h2c.55 0 1 .45 1 1zm-2 1H9v12h6V6z"/></svg>
      </div>
    </div>
  `;
}
