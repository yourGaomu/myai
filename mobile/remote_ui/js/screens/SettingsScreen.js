/**
 * MyAI Mobile UI Prototype - SettingsScreen
 * 全局设置与控制台：Relay连接配对、模型管理、技能Skills、插件Plugins与子智能体Subagents
 */

export function renderSettingsScreen(state) {
  const { settingsSection, relaySettings, models, skills, plugins, subagents } = state;

  const sections = [
    { key: "general", label: "常规" },
    { key: "connection", label: "连接" },
    { key: "model", label: "模型" },
    { key: "subagent", label: "子智能体" },
    { key: "skill", label: "技能" },
    { key: "plugin", label: "插件" }
  ];

  return `
    <div class="settings-container">
      <!-- 横向二级导航药丸 -->
      <div class="settings-pill-nav hide-scrollbar">
        ${sections.map(s => `
          <button class="settings-nav-pill ${settingsSection === s.key ? 'active' : ''}" data-ssec="${s.key}">
            ${s.label}
          </button>
        `).join("")}
      </div>

      <!-- 分栏内容渲染 -->
      ${renderSettingsContent(settingsSection, { relaySettings, models, skills, plugins, subagents })}
    </div>
  `;
}

function renderSettingsContent(section, data) {
  const { relaySettings, models, skills, plugins, subagents } = data;

  if (section === "connection") {
    return `
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
  }

  if (section === "model") {
    return `
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
  }

  if (section === "subagent") {
    return `
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
  }

  if (section === "skill") {
    return `
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
  }

  if (section === "plugin") {
    return `
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
  }

  // 默认 General
  return `
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
