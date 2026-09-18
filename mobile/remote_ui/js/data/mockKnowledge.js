/**
 * MyAI Mobile UI Prototype - Mock Knowledge & Memory Data
 */

export const mockKnowledgeBases = [
  {
    id: "kb-01",
    name: "MyAI 技术架构与规范",
    description: "涵盖服务端中继协议、PC Agent 通信与移动端状态流",
    document_count: 18,
    chunk_count: 342,
    created_at: 1773000000000
  },
  {
    id: "kb-02",
    name: "Android 原生与 EAS 打包指南",
    description: "Gradle 构建、EAS CLI 打包 APK/AAB 与 OTA 差分更新",
    document_count: 6,
    chunk_count: 94,
    created_at: 1773500000000
  }
];

export const mockCategories = [
  { id: "cat-01", name: "架构设计", count: 8 },
  { id: "cat-02", name: "API 接口协议", count: 5 },
  { id: "cat-03", name: "移动端适配", count: 5 }
];

export const mockDocuments = [
  {
    id: "doc-01",
    title: "DEVELOPER_FLOW_GUIDE.md",
    size: 41699,
    status: "indexed",
    category: "架构设计",
    updated_at: "2026-09-18"
  },
  {
    id: "doc-02",
    title: "PROJECT_ARCHITECTURE_GUIDE.md",
    size: 32157,
    status: "indexed",
    category: "架构设计",
    updated_at: "2026-09-17"
  },
  {
    id: "doc-03",
    title: "FEATURE_AI_MEMORY_IMPLEMENTATION.md",
    size: 22607,
    status: "indexed",
    category: "API 接口协议",
    updated_at: "2026-09-16"
  }
];

export const mockAIMemories = [
  {
    id: "mem-01",
    category: "用户习惯",
    importance: "High",
    content: "用户偏好简洁直观的粗线条设计（Neo-Brutalism），且要求移动端支持免构建纯静态原型预览。",
    source: "Chat #42",
    created_at: "2026-09-18 22:30"
  },
  {
    id: "mem-02",
    category: "工程约定",
    importance: "Medium",
    content: "移动端工程所有静态原型图统一存放在 mobile/remote_ui 目录下。",
    source: "User Explicit",
    created_at: "2026-09-18 22:50"
  }
];

export const mockMemoryCandidates = [
  {
    id: "cand-01",
    extracted_text: "用户需要 UI 原型具备分层架构（Tokens、Screens、Store、Data）。",
    status: "pending_approval",
    confidence: 0.94
  }
];
