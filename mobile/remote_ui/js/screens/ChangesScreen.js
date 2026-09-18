/**
 * MyAI Mobile UI Prototype - ChangesScreen
 * 变更与历史：工作区文件改动列表、状态徽章与 SQLite 历史基准检查点
 */

export function renderChangesScreen(state) {
  const { changes, historyCheckpoints } = state;

  return `
    <div class="changes-container">
      <!-- 变更文件列表 -->
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

      <!-- 历史快照检查点 -->
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
