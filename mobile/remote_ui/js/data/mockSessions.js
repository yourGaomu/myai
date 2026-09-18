/**
 * MyAI Mobile UI Prototype - Mock Sessions Data
 */

export const mockSessions = [
  {
    id: "sess-01",
    title: "构建 Android 原型图 (当前会话)",
    model: "gemini-2.5-pro",
    permission_mode: "ask",
    agent_mode: "chat",
    updated_at: 1774000000000,
    created_at: 1773990000000,
    message_count: 5
  },
  {
    id: "sess-02",
    title: "排查 EAS 构建 APK 报错",
    model: "claude-3-7-sonnet",
    permission_mode: "ask",
    agent_mode: "chat",
    updated_at: 1773950000000,
    created_at: 1773940000000,
    message_count: 14
  },
  {
    id: "sess-03",
    title: "优化 SQLite 向量检索 FTS5 性能",
    model: "deepseek-r1",
    permission_mode: "readonly",
    agent_mode: "plan",
    updated_at: 1773800000000,
    created_at: 1773750000000,
    message_count: 22
  }
];

export const mockDeletedSessions = [
  {
    id: "del-01",
    title: "临时测试指令会话",
    model: "gemini-2.5-flash",
    permission_mode: "ask",
    deleted_at: 1773700000000
  }
];
