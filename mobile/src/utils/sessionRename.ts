import type { ErrorPayload, RelayMessage, SessionRenamePayload, SessionRenameResultPayload, SessionSummary } from "../protocol";
import type { RelayScope } from "../types/app";
import { newRequestID } from "./ids";
import { sessionTitleRevision } from "./session";

const requestType = "session_rename";
const requestPrefix = "session-rename-";
const renameTimeoutMs = 30_000;
const notSentMessage = "改名请求未发出，请在中继连接恢复后重试。";
const disconnectedMessage = "连接中断，改名结果未确认。重连后会刷新会话列表。";
const timeoutMessage = "等待改名确认超时，请刷新会话列表确认结果。";
const unavailableMessage = "此会话已删除或不在会话列表中。";

export const maxSessionTitleLength = 100;

export function validateSessionTitle(input: string) {
  // Use the same Unicode White_Space set as the Agent's strings.TrimSpace.
  const title = input.replace(/^[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/gu, "");
  const length = Array.from(title).length;
  const error = length === 0 ? "请输入会话标题。" : length > maxSessionTitleLength ? "标题最多可包含 100 个字符。" : "";
  return { title, length, error };
}

export type SessionRenameFeedback = {
  sessionID: string;
  title: string;
  status: "pending" | "success" | "error" | "unconfirmed" | "unavailable";
  message: string;
};

type PendingRename = {
  sessionID: string;
  title: string;
  previousRevision: number;
  requestID: string;
  scope: RelayScope;
  deadline: number;
  timeout: ReturnType<typeof setTimeout>;
};

type SendRename = (type: typeof requestType, envelope: Partial<RelayMessage<SessionRenamePayload>>) => boolean;

export function createSessionRenameTracker({
  getScope, getSession, onFeedback, onRenamed, refreshSessions,
}: {
  getScope: () => RelayScope;
  getSession: (sessionID: string) => SessionSummary | undefined;
  onFeedback: (feedback: SessionRenameFeedback) => void;
  onRenamed: (result: SessionRenameResultPayload) => void;
  refreshSessions: () => void;
}) {
  const pendingByID = new Map<string, PendingRename>();
  const pendingBySession = new Map<string, PendingRename>();
  const unconfirmed = new Map<string, { request: PendingRename; message: string }>();

  function report(request: Pick<PendingRename, "sessionID" | "title">, status: SessionRenameFeedback["status"], message = "") {
    onFeedback({ sessionID: request.sessionID, title: request.title, status, message });
  }

  function remove(request: PendingRename) {
    clearTimeout(request.timeout);
    pendingByID.delete(request.requestID);
    pendingBySession.delete(request.sessionID);
  }

  function sameConnection(request: PendingRename) {
    const scope = getScope();
    return request.scope.pairingID === scope.pairingID && request.scope.connectionID === scope.connectionID;
  }

  function markUnconfirmed(request: PendingRename, message: string) {
    remove(request);
    unconfirmed.set(request.sessionID, { request, message });
    report(request, "unconfirmed", message);
  }

  function start(sessionID: string, input: string, send: SendRename) {
    if (pendingBySession.has(sessionID) || unconfirmed.has(sessionID)) return false;
    const { title, error } = validateSessionTitle(input);
    const target = { sessionID, title };
    const session = sessionID && sessionID === sessionID.trim() ? getSession(sessionID) : undefined;
    if (!session || session.id !== sessionID || session.deleted) {
      report(target, "unavailable", unavailableMessage);
      return false;
    }
    if (error) {
      report(target, "error", error);
      return false;
    }
    const scope = { ...getScope() };
    if (!scope.pairingID || !scope.connectionID) {
      report(target, "error", notSentMessage);
      return false;
    }

    const requestID = requestPrefix + newRequestID();
    const request: PendingRename = {
      ...target,
      previousRevision: sessionTitleRevision(session.title_revision),
      requestID,
      scope,
      deadline: Date.now() + renameTimeoutMs,
      timeout: setTimeout(() => {
        if (pendingByID.get(requestID) === request) expire(request);
      }, renameTimeoutMs),
    };
    pendingByID.set(requestID, request);
    pendingBySession.set(sessionID, request);
    report(request, "pending");
    let sent = false;
    try {
      sent = send(requestType, { request_id: requestID, session_id: sessionID, payload: { session_id: sessionID, title } });
    } catch {
      // The sender could not write the request to the connection.
    }
    if (!sent && pendingByID.get(requestID) === request) {
      remove(request);
      report(request, "error", notSentMessage);
    }
    return sent;
  }

  function expire(request: PendingRename) {
    const connected = sameConnection(request);
    markUnconfirmed(request, connected ? timeoutMessage : disconnectedMessage);
    if (connected) refreshSessions();
  }

  function invalidateSession(sessionID: string) {
    const pending = pendingBySession.get(sessionID);
    const request = pending ?? unconfirmed.get(sessionID)?.request;
    if (pending) remove(pending);
    unconfirmed.delete(sessionID);
    if (request) report(request, "unavailable", unavailableMessage);
  }

  function handleMessage(message: RelayMessage) {
    const requestID = message.request_id;
    const isRenameID = typeof requestID === "string" && requestID.startsWith(requestPrefix);
    const error = message.type === "error" ? message.payload as (ErrorPayload & { transport_error?: boolean }) | undefined : undefined;
    const isResult = message.type === "session_rename_result";
    if (!isResult && (message.type !== "error" || (!isRenameID && error?.request_type !== requestType))) return false;

    const request = isRenameID ? pendingByID.get(requestID) : undefined;
    if (!request || message.session_id !== request.sessionID) return true;
    if ((!isResult && error?.request_type && error.request_type !== requestType) || !sameConnection(request)) return true;
    if (Date.now() >= request.deadline) {
      expire(request);
      return true;
    }
    if (isResult) {
      const result = message.payload as SessionRenameResultPayload | undefined;
      if (!result || result.session_id !== request.sessionID || typeof result.title !== "string" ||
          validateSessionTitle(result.title).error || sessionTitleRevision(result.title_revision) <= request.previousRevision) return true;
      const session = getSession(request.sessionID);
      if (!session || session.deleted) {
        invalidateSession(request.sessionID);
        return true;
      }
      remove(request);
      onRenamed(result);
      report(request, "success");
    } else if (error?.transport_error) {
      markUnconfirmed(request, "无法确认中继是否收到改名请求，请刷新会话列表确认结果。");
      refreshSessions();
    } else {
      remove(request);
      report(request, "error", typeof error?.message === "string" && error.message.trim() ? error.message : "改名失败，请重试。");
    }
    return true;
  }

  function reconcile(sessions: SessionSummary[]) {
    const { pairingID, connectionID } = getScope();
    if (!pairingID || !connectionID) return;
    const active = new Map(sessions.filter((session) => !session.deleted).map((session) => [session.id, session]));
    for (const sessionID of pendingBySession.keys()) {
      if (!active.has(sessionID)) invalidateSession(sessionID);
    }
    for (const [sessionID, { request }] of unconfirmed) {
      if (request.scope.pairingID !== pairingID) continue;
      const session = active.get(sessionID);
      if (!session) {
        invalidateSession(sessionID);
        continue;
      }
      unconfirmed.delete(sessionID);
      if (session.title === request.title && sessionTitleRevision(session.title_revision) > request.previousRevision) {
        report(request, "success");
      } else {
        report(request, "error", "已刷新会话列表，尚未确认改名。你可以检查后重新保存。");
      }
    }
  }

  function getFeedback(sessionID: string): SessionRenameFeedback | undefined {
    const pending = pendingBySession.get(sessionID);
    if (pending) return { sessionID, title: pending.title, status: "pending", message: "" };
    const unknown = unconfirmed.get(sessionID);
    if (unknown) return { sessionID, title: unknown.request.title, status: "unconfirmed", message: unknown.message };
  }

  return {
    start, handleMessage, reconcile, invalidateSession, getFeedback,
    disconnect: () => {
      for (const request of pendingByID.values()) markUnconfirmed(request, disconnectedMessage);
    },
    clear: () => {
      for (const request of pendingByID.values()) clearTimeout(request.timeout);
      pendingByID.clear();
      pendingBySession.clear();
      unconfirmed.clear();
    },
  };
}
