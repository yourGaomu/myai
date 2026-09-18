/**
 * MyAI Mobile UI Prototype - ChangeDetailScreen
 * 代码差异详情：统一差异视图 (Unified Diff)、行级高亮与还原操作
 */

export function renderChangeDetailScreen(state) {
  const { currentDiff } = state;

  return `
    <div class="diff-container">
      <!-- 差异顶部操作栏 -->
      <div class="diff-header-bar">
        <button class="ui-btn-action" id="btn-back-to-changes">
          ❮ 返回变更
        </button>
        <div style="font-size: 13px; font-weight: 900; font-family: var(--font-mono);">
          ${currentDiff.path.split('/').pop()}
        </div>
        <button class="ui-btn-action ui-btn-danger" id="btn-revert-change" style="font-size: 11px;">
          还原文件
        </button>
      </div>

      <!-- 差异统计条 -->
      <div style="display: flex; justify-content: space-between; align-items: center; font-size: 12px; font-weight: 800; padding: 0 4px;">
        <span style="color: var(--text-secondary);">${currentDiff.path}</span>
        <div>
          <span style="color: #7ee787;">+${currentDiff.additions}</span>
          <span style="color: #ffa198; margin-left: 6px;">-${currentDiff.deletions}</span>
        </div>
      </div>

      <!-- 代码差异视图 -->
      <div class="diff-view-box custom-scrollbar">
        ${renderDiffLines(currentDiff.diff_text)}
      </div>
    </div>
  `;
}

function renderDiffLines(diffText) {
  if (!diffText) return "<div style='padding: 10px;'>无差异</div>";
  const lines = diffText.split("\n");

  return lines.map((line, idx) => {
    let type = "normal";
    if (line.startsWith("+")) type = "add";
    else if (line.startsWith("-")) type = "del";
    else if (line.startsWith("@")) type = "meta";

    return `
      <div class="diff-line ${type}">
        <span class="diff-line-num">${idx + 1}</span>
        <span>${escapeHtml(line)}</span>
      </div>
    `;
  }).join("");
}

function escapeHtml(text) {
  if (!text) return "";
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}
