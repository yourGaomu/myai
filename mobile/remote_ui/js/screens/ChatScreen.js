/**
 * MyAI Mobile UI Prototype - ChatScreen
 * 遵循现代 AI Agent (Cursor / Claude Code / Devin) 标准对话范式：
 * 1. 统一 Agent 回合 (Agent Turn) 容器，告别孤儿卡片堆叠；
 * 2. 思考推导 (Thinking) 与工具链 (Tool Chain) 紧凑折叠收纳；
 * 3. Token 透明细分化 (Prompt vs Completion) 与时长可视化。
 */

export function renderChatScreen(state) {
  const { chatMessages, agentMode } = state;
  const turns = buildChatTurns(chatMessages);

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

      <!-- 聊天消息流：以 Agent Turn (回合) 为一级结构展示 -->
      <div class="chat-messages custom-scrollbar" id="chat-messages-container">
        ${turns.map((turn, index) => renderTurn(turn, index)).join("")}
      </div>
    </div>
  `;
}

/**
 * 将平铺的消息/工具流归纳为结构化的 Agent 回合模型
 */
function buildChatTurns(messages) {
  const turns = [];
  let currentAgentTurn = null;

  messages.forEach((msg) => {
    if (msg.role === "user") {
      currentAgentTurn = null;
      turns.push({
        type: "user",
        id: msg.id,
        text: msg.text,
        createdAt: msg.createdAt || Date.now(),
        attachments: msg.attachments || []
      });
      return;
    }

    // AI 侧消息聚合归属于当前 Agent 回合
    if (!currentAgentTurn) {
      currentAgentTurn = {
        type: "agent",
        id: `agent-turn-${msg.id}`,
        model: "Claude 3.7 Sonnet",
        status: "completed",
        elapsed: "3.2s",
        reasoning: "",
        tools: [],
        text: "",
        tokens: null,
        createdAt: msg.createdAt || Date.now()
      };
      turns.push(currentAgentTurn);
    }

    if (msg.role === "assistant") {
      if (msg.reasoning) {
        currentAgentTurn.reasoning = currentAgentTurn.reasoning
          ? currentAgentTurn.reasoning + "\n\n" + msg.reasoning
          : msg.reasoning;
      }
      if (msg.text) {
        currentAgentTurn.text = currentAgentTurn.text
          ? currentAgentTurn.text + "\n\n" + msg.text
          : msg.text;
      }
      if (msg.tokens) {
        currentAgentTurn.tokens = msg.tokens;
      }
      if (msg.status === "streaming" || msg.status === "tool_running") {
        currentAgentTurn.status = "running";
      }
    } else if (msg.role === "tool_call") {
      currentAgentTurn.tools.push({
        id: msg.id,
        name: msg.toolName || "tool",
        arguments: msg.toolArguments || "",
        status: msg.status || "completed",
        duration: "0.8s"
      });
      if (msg.status === "running") {
        currentAgentTurn.status = "running";
      }
    } else if (msg.role === "tool") {
      const lastTool = currentAgentTurn.tools[currentAgentTurn.tools.length - 1];
      if (lastTool && lastTool.name === msg.toolName) {
        lastTool.result = msg.text || "";
        lastTool.status = "completed";
      } else {
        currentAgentTurn.tools.push({
          id: msg.id,
          name: msg.toolName || "tool",
          result: msg.text || "",
          status: "completed",
          duration: "0.6s"
        });
      }
    }
  });

  return turns;
}

function renderTurn(turn) {
  if (turn.type === "user") {
    return `
      <div class="turn-wrapper user-turn-wrapper">
        <div class="user-turn-bubble">
          ${escapeHtml(turn.text)}
        </div>
        <div class="user-turn-meta">已发送</div>
      </div>
    `;
  }

  // Agent 统一复合卡片
  const isRunning = turn.status === "running";
  const hasProcess = Boolean(turn.reasoning || (turn.tools && turn.tools.length > 0));
  const toolCount = turn.tools ? turn.tools.length : 0;
  const processSummary = hasProcess
    ? `⚡ 已执行 ${toolCount} 个工具调用 · 耗时 ${turn.elapsed || '2.8s'}`
    : "⚡ 执行活动";

  return `
    <div class="turn-wrapper agent-turn-wrapper">
      <!-- 头部：身份标签与执行状态胶囊 -->
      <div class="agent-turn-header">
        <div class="agent-identity">
          <div class="agent-avatar-icon">🤖</div>
          <span class="agent-name">MyAI Agent</span>
          <span class="agent-model-pill">${turn.model || 'v2.4 Auto'}</span>
        </div>
        <div class="agent-status-badge ${isRunning ? 'running' : 'completed'}">
          ${isRunning
            ? `<span class="agent-pulse-dot"></span> 正在执行...`
            : `✓ 完成 · ${turn.elapsed || '3.2s'}`
          }
        </div>
      </div>

      <!-- Agent 复合气泡大卡片 -->
      <div class="agent-turn-box">
        <!-- 过程折叠托盘 (Process Tray)：思考推导 + 工具链条 (默认折叠节省高度) -->
        ${hasProcess ? `
          <div class="agent-process-tray">
            <div class="process-tray-header" onclick="window.__toggleProcessTray(this)">
              <div class="tray-summary-left">
                <span class="tray-summary-text">${processSummary}</span>
              </div>
              <span class="tray-toggle-btn">展开详情 ▼</span>
            </div>

            <div class="process-tray-body" style="display: none;">
              ${turn.reasoning ? `
                <div class="tray-sub-section">
                  <div class="tray-sub-title">🧠 思考推导 (Reasoning)</div>
                  <div class="tray-reasoning-box">${escapeHtml(turn.reasoning)}</div>
                </div>
              ` : ''}

              ${toolCount > 0 ? `
                <div class="tray-sub-section">
                  <div class="tray-sub-title">🛠️ 工具调用轨迹 (${toolCount} 步)</div>
                  <div class="tool-chain-list">
                    ${turn.tools.map((tool) => `
                      <div class="tool-chain-item">
                        <div class="tool-chain-header" onclick="window.__toggleToolDetail(this)">
                          <span class="tool-status-dot">✓</span>
                          <span class="tool-fn-name">${tool.name}</span>
                          <span class="tool-duration-tag">${tool.duration || '0.7s'}</span>
                          <span class="tool-step-arrow">▼</span>
                        </div>
                        <div class="tool-chain-detail" style="display: none;">
                          ${tool.arguments ? `
                            <div class="tool-detail-label">输入参数:</div>
                            <pre class="tool-detail-pre">${escapeHtml(tool.arguments)}</pre>
                          ` : ''}
                          ${tool.result ? `
                            <div class="tool-detail-label">执行结果:</div>
                            <pre class="tool-detail-pre">${escapeHtml(tool.result)}</pre>
                          ` : ''}
                        </div>
                      </div>
                    `).join("")}
                  </div>
                </div>
              ` : ''}
            </div>
          </div>
        ` : ''}

        <!-- 最终回答正文 (Markdown 渲染) -->
        <div class="agent-turn-body markdown-body">
          ${formatSimpleMarkdown(turn.text)}
        </div>

        <!-- 底部元信息与操作栏 -->
        <div class="agent-turn-footer">
          <div class="agent-actions">
            <button class="agent-btn-mini" onclick="window.__copyAgentResponse(this)">📋 复制</button>
            <button class="agent-btn-mini" onclick="window.__regenerateTurn(this)">🔄 重新生成</button>
          </div>
          ${turn.tokens ? `
            <div class="agent-token-stat">
              <span class="token-highlight">${formatTokenCount(turn.tokens.total)} tokens</span>
              <span>(入 ${formatTokenCount(turn.tokens.input)} · 出 ${formatTokenCount(turn.tokens.output)})</span>
            </div>
          ` : ''}
        </div>
      </div>
    </div>
  `;
}

function formatTokenCount(num) {
  if (typeof num !== "number") return "n/a";
  if (num >= 1000) {
    return (num / 1000).toFixed(1).replace(/\.0$/, "") + "k";
  }
  return num.toString();
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

// 挂载原型工作台全局交互函数
if (typeof window !== "undefined") {
  window.__toggleProcessTray = function(el) {
    const tray = el.closest(".agent-process-tray");
    if (!tray) return;
    const body = tray.querySelector(".process-tray-body");
    const toggleBtn = tray.querySelector(".tray-toggle-btn");
    if (!body || !toggleBtn) return;
    const isHidden = body.style.display === "none";
    body.style.display = isHidden ? "flex" : "none";
    toggleBtn.textContent = isHidden ? "收起详情 ▲" : "展开详情 ▼";
  };

  window.__toggleToolDetail = function(el) {
    const item = el.closest(".tool-chain-item");
    if (!item) return;
    const detail = item.querySelector(".tool-chain-detail");
    const arrow = item.querySelector(".tool-step-arrow");
    if (!detail) return;
    const isHidden = detail.style.display === "none";
    detail.style.display = isHidden ? "block" : "none";
    if (arrow) arrow.textContent = isHidden ? "▲" : "▼";
  };

  window.__copyAgentResponse = function(btn) {
    const card = btn.closest(".agent-turn-box");
    if (!card) return;
    const textEl = card.querySelector(".agent-turn-body");
    if (textEl && navigator.clipboard) {
      navigator.clipboard.writeText(textEl.innerText);
    }
    const orig = btn.innerText;
    btn.innerText = "✓ 已复制";
    setTimeout(() => { btn.innerText = orig; }, 1200);
  };

  window.__regenerateTurn = function(btn) {
    const orig = btn.innerText;
    btn.innerText = "⚡ 生成中...";
    setTimeout(() => { btn.innerText = orig; }, 1000);
  };
}

