import type { ModelSummary, SessionSummary } from "../protocol";

export function upsertSession(current: SessionSummary[], next: SessionSummary) {
  const existingIndex = current.findIndex((item) => item.id === next.id);
  if (existingIndex === -1) {
    return [...current, next];
  }

  const copy = current.slice();
  copy[existingIndex] = preferNewerSession(copy[existingIndex], next);
  return copy;
}

// 设置响应和旧的 session_list_result 可能交错到达。只在有明确时间且 incoming 更新时覆盖。
export function mergeSessionList(current: SessionSummary[], next: SessionSummary[]) {
  const currentByID = new Map(current.map((session) => [session.id, session]));
  return next.map((session) => preferNewerSession(currentByID.get(session.id), session));
}

function preferNewerSession(current: SessionSummary | undefined, next: SessionSummary) {
  if (!current) {
    return next;
  }

  const currentTime = sessionTime(current);
  const nextTime = sessionTime(next);
  if (currentTime !== null && nextTime === null) {
    return current;
  }
  if (currentTime !== null && nextTime !== null && currentTime > nextTime) {
    return current;
  }
  return next;
}

function sessionTime(session: SessionSummary) {
  if (!session.updated_at) {
    return null;
  }
  const time = Date.parse(session.updated_at);
  return Number.isFinite(time) ? time : null;
}

export function findSessionUsage(sessions: SessionSummary[], sessionID: string) {
  return sessions.find((session) => session.id === sessionID)?.last_usage || null;
}

export function modelDisplayName(model: ModelSummary) {
  return model.name || model.model_name || model.id;
}

export function normalizePermission(mode?: string): "readonly" | "ask" | "full" {
  return mode === "readonly" || mode === "full" ? mode : "ask";
}

export function permissionLabel(mode?: string) {
  const normalized = normalizePermission(mode);
  return normalized === "readonly" ? "只读" : normalized === "full" ? "完全开放" : "询问";
}

const protocolLabels: Record<string, string> = {
  "openai-chat-completions": "OpenAI 兼容",
  "anthropic-messages": "Anthropic",
  "google-generative-ai": "Google Gemini",
  "mistral-chat": "Mistral",
  "ollama-chat": "Ollama · 本地",
};

export function modelProviderLabel(model: ModelSummary) {
  return protocolLabels[model.protocol || ""] || model.provider || model.protocol || "";
}

// 侧栏按「今天 / 昨天 / 7 天内 / 更早」分组，组内按更新时间倒序。
export function groupSessionsByDate(sessions: SessionSummary[], now = new Date()) {
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const day = 24 * 60 * 60 * 1000;
  const labels = ["今天", "昨天", "7 天内", "更早"];
  const groups = labels.map((label) => ({ label, sessions: [] as SessionSummary[] }));
  const stamp = (s: SessionSummary) => Date.parse(s.updated_at || s.created_at || "") || 0;
  [...sessions]
    .sort((a, b) => stamp(b) - stamp(a))
    .forEach((session) => {
      const t = stamp(session);
      const index = !t ? 3 : t >= startOfToday ? 0 : t >= startOfToday - day ? 1 : t >= startOfToday - 6 * day ? 2 : 3;
      groups[index].sessions.push(session);
    });
  return groups.filter((group) => group.sessions.length > 0);
}
