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
