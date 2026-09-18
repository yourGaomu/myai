/**
 * MyAI Mobile UI Prototype - KnowledgeScreen
 * 知识与记忆中心：资料知识库（RAG检索/分类/文档）与 AI 记忆中心（长期记忆/提取候选词/Dream运行）
 */

export function renderKnowledgeScreen(state) {
  const { knowledgeTab, knowledgeBases, documents, aiMemories } = state;

  return `
    <div class="knowledge-container">
      <!-- 顶部二级 Tab 切换 -->
      <div class="knowledge-subtabs">
        <button class="knowledge-tab-btn ${knowledgeTab === 'documents' ? 'active' : ''}" data-ktab="documents">
          资料知识库
        </button>
        <button class="knowledge-tab-btn ${knowledgeTab === 'memory' ? 'active' : ''}" data-ktab="memory">
          AI 记忆
        </button>
      </div>

      ${knowledgeTab === 'documents' ? `
        <!-- 资料知识库列表 -->
        <div style="display: flex; flex-direction: column; gap: 10px;">
          <div class="ui-panel-header" style="background: none; padding: 0;">
            <span class="ui-panel-title">向量知识库</span>
            <button class="ui-btn-action ui-btn-primary" style="font-size: 11px;">+ 新建知识库</button>
          </div>

          ${knowledgeBases.map(kb => `
            <div class="kb-card">
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <span style="font-size: 14px; font-weight: 900; color: var(--text-primary);">📚 ${kb.name}</span>
                <span class="status-pill online" style="font-size: 10px; min-height: 22px; padding: 0 6px;">已连接</span>
              </div>
              <div style="font-size: 11px; color: var(--text-secondary);">${kb.description}</div>
              <div style="display: flex; gap: 12px; font-size: 11px; color: var(--text-muted); font-weight: 700; margin-top: 4px;">
                <span>${kb.document_count} 篇文档</span>
                <span>${kb.chunk_count} 个向量分块</span>
              </div>
            </div>
          `).join("")}

          <!-- 文档列表 -->
          <div class="ui-panel" style="padding: 12px; gap: 8px;">
            <div class="ui-panel-header" style="padding: 0 0 6px 0;">
              <span class="ui-panel-title" style="font-size: 14px;">已索引文档</span>
              <button class="ui-btn-action" style="font-size: 11px;">上传文档</button>
            </div>
            ${documents.map(doc => `
              <div style="display: flex; justify-content: space-between; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--line-subtle); font-size: 12px;">
                <div>
                  <div style="font-weight: 800;">📄 ${doc.title}</div>
                  <div style="font-size: 10px; color: var(--text-muted);">${doc.category} / ${(doc.size / 1024).toFixed(1)} KB</div>
                </div>
                <span class="status-pill online" style="font-size: 9px; min-height: 20px; padding: 0 6px;">已解析</span>
              </div>
            `).join("")}
          </div>
        </div>
      ` : `
        <!-- AI 长期记忆列表 -->
        <div style="display: flex; flex-direction: column; gap: 10px;">
          <div class="ui-panel-header" style="background: none; padding: 0;">
            <span class="ui-panel-title">已沉淀用户记忆</span>
            <button class="ui-btn-action ui-btn-primary" style="font-size: 11px;">+ 添加记忆</button>
          </div>

          ${aiMemories.map(mem => `
            <div class="memory-card">
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <span style="font-size: 11px; font-weight: 900; background: var(--surface-alt); padding: 2px 6px; border-radius: 4px; border: 1px solid var(--line);">
                  ${mem.category}
                </span>
                <span style="font-size: 10px; color: var(--text-muted);">${mem.created_at}</span>
              </div>
              <div style="font-size: 12.5px; font-weight: 700; color: var(--ink); line-height: 1.4; margin-top: 4px;">
                ${mem.content}
              </div>
              <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 6px; font-size: 10px; color: var(--text-muted);">
                <span>来源: ${mem.source}</span>
                <button class="ui-btn-action ui-btn-danger" style="padding: 2px 6px; font-size: 10px;" onclick="alert('已模拟删除记忆')">删除</button>
              </div>
            </div>
          `).join("")}

          <!-- Dream 记忆整理提示 -->
          <div style="background: var(--paper); border: 2px dashed var(--line); border-radius: 12px; padding: 12px; text-align: center;">
            <div style="font-size: 13px; font-weight: 900;">✨ 记忆自动化固化 (Dream)</div>
            <div style="font-size: 11px; color: var(--text-secondary); margin-top: 4px;">
              Agent 每天凌晨会自动提取高频对话事实并合并至主记忆库
            </div>
            <button class="ui-btn-action ui-btn-primary" style="margin-top: 8px;" onclick="alert('已模拟触发即时 Dream 整理！')">立即运行 Dream 整合</button>
          </div>
        </div>
      `}
    </div>
  `;
}
