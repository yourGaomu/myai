/**
 * MyAI Mobile UI Prototype - PermissionModal Component
 * 危险操作拦截审批弹窗
 */

export function renderPermissionModal(permission) {
  if (!permission) return "";

  return `
    <div class="permission-modal-overlay">
      <div class="permission-card">
        <div class="permission-title">
          <span class="permission-badge-warn">危险操作授权</span>
          <span>${permission.name} 申请权限</span>
        </div>
        <div style="font-size: 13px; font-weight: 800; color: var(--ink);">
          权限项：${permission.permission}
        </div>
        <div class="permission-content-box">
          ${permission.arguments}
        </div>
        <div class="permission-actions">
          <button id="btn-perm-deny" class="permission-btn btn-deny">
            拒绝执行
          </button>
          <button id="btn-perm-allow" class="permission-btn btn-allow">
            确认允许
          </button>
        </div>
      </div>
    </div>
  `;
}
