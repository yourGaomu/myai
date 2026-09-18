/**
 * MyAI Mobile UI Prototype - PlanScreen
 * 规划执行面板：目标总览、步骤完成度进度条、步骤状态卡片与执行控制
 */

export function renderPlanScreen(state) {
  const { plan } = state;
  const completedCount = plan.steps.filter(s => s.status === 'completed').length;
  const totalCount = plan.steps.length;
  const progressPercent = Math.round((completedCount / totalCount) * 100);

  return `
    <div class="plan-container">
      <!-- 目标概览卡片 -->
      <div class="plan-goal-box">
        <div style="font-size: 11px; font-weight: 900; color: var(--text-muted); text-transform: uppercase;">
          🎯 计划执行总目标
        </div>
        <div style="font-size: 14px; font-weight: 900; color: var(--ink); line-height: 1.35; margin-top: 2px;">
          ${plan.goal}
        </div>
      </div>

      <!-- 进度条与状态 -->
      <div class="ui-panel" style="padding: 12px; gap: 8px;">
        <div style="display: flex; justify-content: space-between; font-size: 12px; font-weight: 900;">
          <span>执行进度 (${completedCount}/${totalCount})</span>
          <span style="color: var(--blue-accent);">${progressPercent}%</span>
        </div>
        <div class="plan-progress-track">
          <div class="plan-progress-fill" style="width: ${progressPercent}%;"></div>
        </div>
      </div>

      <!-- 步骤清单 -->
      <div style="display: flex; flex-direction: column; gap: 8px;">
        ${plan.steps.map(step => `
          <div class="plan-step-card">
            <span class="step-num-badge">${step.order}</span>
            <div style="flex: 1; display: flex; flex-direction: column; gap: 2px;">
              <div style="font-size: 13px; font-weight: 900; color: var(--text-primary);">
                ${step.title}
              </div>
              <div style="font-size: 11px; color: var(--text-secondary); line-height: 1.4;">
                ${step.description}
              </div>
            </div>
            <span class="status-pill ${step.status === 'completed' ? 'online' : step.status === 'running' ? 'busy' : ''}" style="font-size: 9px; min-height: 22px; padding: 0 6px;">
              ${step.status === 'completed' ? '已完成' : step.status === 'running' ? '运行中' : '等待'}
            </span>
          </div>
        `).join("")}
      </div>

      <!-- 底部操作按钮 -->
      <div style="display: flex; gap: 10px; margin-top: 6px;">
        <button class="ui-btn-action" id="btn-plan-to-chat" style="flex: 1; height: 42px; justify-content: center; font-size: 13px;">
          返回对话
        </button>
        <button class="ui-btn-action ui-btn-primary" id="btn-execute-plan" style="flex: 1; height: 42px; justify-content: center; font-size: 13px;">
          ⚡ 执行计划
        </button>
      </div>
    </div>
  `;
}
