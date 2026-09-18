/**
 * MyAI Mobile UI Prototype - FilesScreen
 * 文件工作区：会话共享资产、工作区目录树、路径导航与代码详情预览卡片
 */

export function renderFilesScreen(state) {
  const { assets, files, selectedFilePreview } = state;

  return `
    <div class="files-container">
      <!-- 会话共享文件卡片 -->
      <div class="asset-box">
        <div class="ui-panel-header" style="padding: 0 0 8px 0;">
          <div class="ui-panel-title-group">
            <span class="ui-panel-title">会话资源</span>
            <span class="ui-panel-meta">${assets.length} 个共享资产</span>
          </div>
          <button class="ui-btn-action ui-btn-primary" id="btn-refresh-assets">刷新</button>
        </div>
        ${assets.map(asset => `
          <div class="asset-item">
            <div>
              <div class="asset-title">📦 ${asset.file_name}</div>
              <div class="asset-meta">${asset.path} (${(asset.size / 1024).toFixed(1)} KB)</div>
            </div>
            <a href="${asset.short_url}" target="_blank" class="ui-btn-action" style="padding: 4px 8px; font-size: 11px;">打开</a>
          </div>
        `).join("")}
      </div>

      <!-- 工作区文件目录树 -->
      <div class="file-browser-box">
        <div class="ui-panel-header" style="padding: 0 0 8px 0;">
          <div class="ui-panel-title-group">
            <span class="ui-panel-title">工作区文件</span>
            <span class="ui-panel-meta">PC Agent Workspace</span>
          </div>
          <button class="ui-btn-action" id="btn-refresh-files">刷新目录</button>
        </div>

        <div class="file-breadcrumb">
          <span>📂 d:/Go_All/myai/mobile/src</span>
          <button class="ui-btn-action" style="padding: 2px 6px; font-size: 10px;">上一级</button>
        </div>

        <div style="display: flex; flex-direction: column; max-height: 240px; overflow-y: auto;">
          ${files.map(f => `
            <div class="file-row" data-filename="${f.name}">
              <div class="file-row-left">
                <span class="file-icon">${f.is_dir ? '📁' : '📄'}</span>
                <span class="file-name">${f.name}</span>
              </div>
              <span class="file-size">${f.is_dir ? '目录' : (f.size / 1024).toFixed(1) + ' KB'}</span>
            </div>
          `).join("")}
        </div>
      </div>

      <!-- 选中文件代码预览 -->
      ${selectedFilePreview ? `
        <div class="file-browser-box">
          <div class="ui-panel-header" style="padding: 0 0 8px 0;">
            <div class="ui-panel-title-group">
              <span class="ui-panel-title" style="font-size: 14px;">📄 ${selectedFilePreview.path.split('/').pop()}</span>
              <span class="ui-panel-meta">${selectedFilePreview.path}</span>
            </div>
            <button class="ui-btn-action ui-btn-primary" id="btn-attach-preview" style="font-size: 11px;">附加到对话</button>
          </div>
          <pre style="background: #25231f; color: #f7f5f0; padding: 10px; border-radius: 8px; font-family: var(--font-mono); font-size: 11px; overflow-x: auto; max-height: 180px;">${escapeCode(selectedFilePreview.content)}</pre>
        </div>
      ` : ''}
    </div>
  `;
}

function escapeCode(code) {
  if (!code) return "";
  return code
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}
