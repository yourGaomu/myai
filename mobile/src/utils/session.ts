import type { ModelSummary, SessionSummary } from "../protocol";

export function upsertSession(current: SessionSummary[], next: SessionSummary) {
  const existingIndex = current.findIndex((item) => item.id === next.id);
  if (existingIndex === -1) {
    return [...current, next];
  }

  const copy = current.slice();
  copy[existingIndex] = next;
  return copy;
}

export function findSessionUsage(sessions: SessionSummary[], sessionID: string) {
  return sessions.find((session) => session.id === sessionID)?.last_usage || null;
}

export function modelDisplayName(model: ModelSummary) {
  return model.name || model.model_name || model.id;
}
