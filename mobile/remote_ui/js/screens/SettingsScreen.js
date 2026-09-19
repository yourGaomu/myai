/**
 * MyAI Mobile UI Prototype - SettingsScreen
 * 全局设置与控制台：会话管理、模型中心、Relay连接配对、权限策略、子智能体、技能与插件
 */

export function renderSettingsScreen(state) {
  const { settingsSection = "session", relaySettings, models, skills, plugins, subagents, sessions, deletedSessions } = state;

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

  return `
    <div class="settings-container">
      <!-- 顶部标题栏 (去除冗余完成按钮，符合主流Tab规范) -->
      <div class="ui-panel-header" style="background: none; padding: 0 0 2px 0;">
        <div class="ui-panel-title-group">
          <span class="ui-panel-title" style="font-size: 18px; font-weight: 900; color: var(--ink);">设置中心</span>
          <span class="ui-panel-meta">连接、模型、会话与工具权限集中管理</span>
        </div>
      </div>

      <!-- 横向二级导航药丸 (去除生硬代码代号，使用直观图标与文字) -->
      <div class="settings-pill-nav hide-scrollbar">
        ${sections.map(s => `
          <button class="settings-nav-pill ${settingsSection === s.key ? 'active' : ''}" data-ssec="${s.key}">
            ${s.label}
          </button>
        `).join("")}
      </div>

      <!-- 分栏内容渲染 -->
      ${renderSettingsContent(settingsSection, { relaySettings, models, skills, plugins, subagents, sessions, deletedSessions })}
    </div>
  `;
}

function renderSettingsContent(section, data) {
  const { relaySettings, models, skills, plugins, subagents, sessions = [], deletedSessions = [] } = data;

  // 1. 会话管理中心 (核心改造: 废弃生硬两列方块，采用 Neo-Brutalism 单列流式卡片与当前激活高亮)
  if (section === "session") {
    const activeSession = sessions[0] || {
      id: "30de8b48",
      title: "你好",
      model: "grok-4.6",
      permission_mode: "ask",
      context_window_k: 16,
      tokens: 187435,
      mode: "chat"
    };

    return `
      <div style="display: flex; flex-direction: column; gap: 10px;">
        <!-- 会话列表顶栏 -->
        <div style="display: flex; justify-content: space-between; align-items: center; padding: 2px 0;">
          <div>
            <span style="font-size: 15px; font-weight: 900; color: var(--ink);">会话管理</span>
            <span style="font-size: 11px; color: var(--text-muted); font-weight: 700; margin-left: 6px;">${sessions.length} 个活跃记录</span>
          </div>
          <button class="ui-btn-action ui-btn-primary" style="font-size: 12px; font-weight: 900; padding: 4px 12px;" onclick="alert('已模拟创建新对话！')">
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
  }

  // 2. Relay 中继连接
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

  // 3. 模型管理中心
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

  // 4. 权限模式策略 (新集成)
  if (section === "permission") {
    return `
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
  }

  // 5. 子智能体
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

  // 6. 本地技能
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

  // 7. 扩展插件
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

  // 8. 默认 常规 General
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
