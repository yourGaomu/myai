/**
 * MyAI Mobile UI Prototype - SessionsScreen
 * 会话中心：全部活跃会话管理、新建会话与已删除回收站
 */

export function renderSessionsScreen(state) {
  const { sessions, deletedSessions } = state;

  return `
    <div class="sessions-container">
      <!-- 头部与新建按钮 -->
      <div class="ui-panel-header" style="background: none; padding: 0;">
        <div class="ui-panel-title-group">
          <span class="ui-panel-title">会话列表</span>
          <span class="ui-panel-meta">${sessions.length} 个活跃会话</span>
        </div>
        <button class="ui-btn-action ui-btn-primary" id="btn-create-session" style="font-size: 13px; font-weight: 900;">
          + 新建会话
        </button>
      </div>

      <!-- 会话卡片列表 -->
      <div style="display: flex; flex-direction: column; gap: 8px;">
        ${sessions.map((sess, idx) => `
          <div class="session-card ${idx === 0 ? 'active' : ''}" data-sessid="${sess.id}">
            <div style="display: flex; align-items: center; gap: 10px; flex: 1; min-width: 0;">
              <div style="width: 32px; height: 32px; border-radius: 8px; background: var(--cyan); border: 2px solid var(--ink); display: flex; align-items: center; justify-content: center; font-size: 14px;">
                💬
              </div>
              <div style="flex: 1; min-width: 0;">
                <div style="font-size: 13.5px; font-weight: 900; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">
                  ${sess.title}
                </div>
                <div style="font-size: 11px; color: var(--text-muted); font-weight: 700; margin-top: 2px;">
                  ${sess.model} · ${sess.permission_mode}
                </div>
              </div>
            </div>
            <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 4px;">
              <span style="font-size: 10px; color: var(--text-muted); font-weight: 700;">刚刚</span>
              <button class="ui-btn-action ui-btn-danger" style="padding: 2px 6px; font-size: 10px;" onclick="event.stopPropagation(); alert('已模拟删除该会话至回收站');">删除</button>
            </div>
          </div>
        `).join("")}
      </div>

      <!-- 回收站卡片 -->
      <div class="ui-panel" style="padding: 12px; gap: 8px; margin-top: 8px;">
        <div class="ui-panel-header" style="padding: 0 0 6px 0;">
          <span class="ui-panel-title" style="font-size: 13px;">🗑️ 回收站 (${deletedSessions.length})</span>
          <span style="font-size: 11px; color: var(--text-muted);">保留 30 天</span>
        </div>
        ${deletedSessions.map(ds => `
          <div style="display: flex; justify-content: space-between; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--line-subtle); font-size: 12px;">
            <span style="color: var(--text-secondary); text-decoration: line-through;">${ds.title}</span>
            <button class="ui-btn-action ui-btn-secondary" style="padding: 2px 6px; font-size: 10px;" onclick="alert('已模拟恢复该会话')">恢复</button>
          </div>
        `).join("")}
      </div>
    </div>
  `;
}
