/**
 * MyAI Mobile UI Prototype - Mock Plan Data
 */

export const mockPlan = {
  id: "plan-101",
  goal: "为 MyAI Mobile 移动端构建高保真产品 UI 原型系统 (remote_ui)",
  status: "running",
  steps: [
    {
      order: 1,
      id: "step-1",
      title: "分析安卓客户端工程架构与视觉设计系统",
      description: "提取设计色调令牌 (--ink, --yellow, --cyan)、边框粗细与圆角规范。",
      status: "completed"
    },
    {
      order: 2,
      id: "step-2",
      title: "创建模块化样式系统与 Android 真机物理外壳",
      description: "实现包含状态栏、摄像头打孔、底栏手势条的精准机身框架。",
      status: "completed"
    },
    {
      order: 3,
      id: "step-3",
      title: "实现 8 大核心屏幕组件与响应式数据绑定",
      description: "复原 Chat、Files、Changes、Diff、Knowledge、Plan、Sessions 与 Settings 面板。",
      status: "running"
    },
    {
      order: 4,
      id: "step-4",
      title: "集成工作台演示外壳与交互模拟控制器",
      description: "支持一键模拟流式打字、工具调用、权限审批弹窗与模式切换。",
      status: "pending"
    }
  ]
};
