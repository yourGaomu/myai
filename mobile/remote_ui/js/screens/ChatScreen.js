/**
 * MyAI Mobile UI Prototype - ChatScreen
 * 对话主屏幕：模式切换胶囊、Agent 运行时间线、Thinking 思考折叠卡、消息气泡与工具调用组
 */

export function renderChatScreen(state) {
  const { chatMessages, agentRuns, agentMode } = state;
  const currentRun = agentRuns[0];

  return `
    <div class="chat-container">
      <!-- 极简模式切换胶囊 (节省垂直高度) -->
      <div class="mode-switch-wrapper">
        <div class="mode-segmented">
          <button class="mode-segment-btn ${agentMode === 'chat' ? 'active' : ''}" data-mode="chat">
            💬 对话模式
          </button>
          <button class="mode-segment-btn ${agentMode === 'plan' ? 'active' : ''}" data-mode="plan">
            🎯 规划模式
          </button>
        </div>
      </div>

      <!-- 紧凑型 Agent 运行指示条 -->
      ${currentRun ? `
        <div class="agent-run-timeline">
          <div class="timeline-left">
            <span>⚡ ${currentRun.title}</span>
          </div>
          <div class="timeline-steps">
            ${currentRun.steps.map(s => `
              <span class="step-indicator ${s.status === 'completed' ? 'done' : s.status === 'in_progress' ? 'active' : ''}" title="${s.title}"></span>
            `).join("")}
          </div>
        </div>
      ` : ''}

      <!-- 聊天消息流 -->
      <div class="chat-messages custom-scrollbar" id="chat-messages-container">
        ${chatMessages.map(msg => renderChatMessage(msg)).join("")}
      </div>
    </div>
  `;
}

function renderChatMessage(msg) {
  if (msg.role === "user") {
    return `
      <div class="message-bubble-user">
        ${escapeHtml(msg.text)}
      </div>
    `;
  }

  if (msg.role === "assistant") {
    return `
      <div class="message-bubble-assistant">
        ${msg.reasoning ? `
          <div class="thinking-box">
            <div class="thinking-header" onclick="const c = this.parentElement.querySelector('.thinking-content'); const isHidden = c.style.display === 'none' || !c.style.display; c.style.display = isHidden ? 'block' : 'none'; this.querySelector('.thinking-arrow').textContent = isHidden ? '▲ 收起' : '▼ 展开';">
              <div class="thinking-badge">
                <span>💡 思考推导</span>
              </div>
              <span class="thinking-arrow" style="font-size: 10px; opacity: 0.8;">▼ 展开</span>
            </div>
            <div class="thinking-content" style="display: none;">
              ${escapeHtml(msg.reasoning)}
            </div>
          </div>
        ` : ''}

        <div class="markdown-body">
          ${formatSimpleMarkdown(msg.text)}
        </div>

        ${msg.tokens ? `
          <div style="display: flex; justify-content: flex-end; font-size: 9.5px; color: var(--text-muted); font-weight: 700; margin-top: 2px;">
            消耗 ${msg.tokens.total} tokens (In: ${msg.tokens.input} / Out: ${msg.tokens.output})
          </div>
        ` : ''}
      </div>
    `;
  }

  if (msg.role === "tool_call" || msg.role === "tool") {
    const isCall = msg.role === "tool_call";
    return `
      <div class="tool-group-card">
        <div class="tool-card-header" onclick="const b = this.parentElement.querySelector('.tool-card-body'); const isHidden = b.style.display === 'none' || !b.style.display; b.style.display = isHidden ? 'block' : 'none'; this.querySelector('.tool-toggle').textContent = isHidden ? '▲ 收起' : '▼ 详情';">
          <span class="${isCall ? 'tool-badge-call' : 'tool-badge-done'}">
            ${isCall ? '调用' : '完成'}
          </span>
          <span class="tool-name">${msg.toolName || 'tool'}</span>
          <span class="tool-toggle" style="font-size: 10px; color: var(--text-muted); font-weight: 700;">▼ 详情</span>
        </div>
        <div class="tool-card-body" style="display: none;">
          <pre class="tool-code-preview">${escapeHtml(msg.toolArguments || msg.text || '')}</pre>
        </div>
      </div>
    `;
  }

  return "";
}

function escapeHtml(text) {
  if (!text) return "";
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function formatSimpleMarkdown(text) {
  if (!text) return "";
  let html = escapeHtml(text);

  // 代码块 ```code```
  html = html.replace(/```([\s\S]*?)```/g, '<pre><code>$1</code></pre>');
  // 行内代码 `code`
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
  // 粗体 **bold**
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  // 标题 ###
  html = html.replace(/^### (.*$)/gim, '<h4 style="margin: 6px 0 4px; font-weight: 900;">$1</h4>');
  // 列表 - item
  html = html.replace(/^\- (.*$)/gim, '<li style="margin-left: 14px;">$1</li>');
  // 换行
  html = html.replace(/\n\n/g, '<p style="margin-top: 6px;"></p>');
  html = html.replace(/\n/g, '<br/>');

  return html;
}
