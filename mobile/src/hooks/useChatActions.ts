import { useCallback, useRef, type RefObject } from "react";

import type { AgentRunSnapshot, PermissionApprovalScope, PermissionResultPayload, RelayMessage, TokenUsage } from "../protocol";
import type { PermissionState, RelayScope, UserQuestionAnswerSubmission, UserQuestionState } from "../types/app";
import { formatSubmittedCardSummary } from "../utils/userQuestion";
import type { ChatAttachment } from "../types/chat";
import { userMessageEcho } from "../utils/attachments";
import { buildChatTurns } from "../utils/chatRenderItems";
import { newRequestID } from "../utils/ids";
import type { SessionChatState } from "./useChatMessages";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  activeRequestIDRef: RefObject<string>;
  addEventMessage: (sessionID: string, message: string) => void;
  addUserMessage: (sessionID: string, message: string, requestID: string, attachments: ChatAttachment[]) => void;
  attachedFiles: ChatAttachment[];
  getSessionChat: (sessionID: string) => SessionChatState;
  getSessionRuns: (sessionID: string) => AgentRunSnapshot[];
  messageInput: string;
  pendingPermission: PermissionState | null;
  relayScopeRef: RefObject<RelayScope>;
  requestSessionMapRef: RefObject<Record<string, string>>;
  requestUserQuestionSync: () => boolean;
  resetActiveAssistant: (sessionID: string) => void;
  sendEnvelope: SendEnvelope;
  sendMessageWithFiles: (content: string, requestID: string) => boolean;
  sessionID: string;
  sessionIDRef: RefObject<string>;
  setSessionLastUsage: (sessionID: string, usage: TokenUsage | null) => void;
  setSessionPendingQuestion: (sessionID: string, question: UserQuestionState | null) => void;
  setSessionPendingRequest: (sessionID: string, requestID: string) => void;
  startPausePending: () => void;
  stopPausePending: () => void;
};

export function useChatActions({
  activeRequestIDRef, addEventMessage, addUserMessage, attachedFiles, getSessionChat, getSessionRuns,
  messageInput, pendingPermission, relayScopeRef, requestSessionMapRef, requestUserQuestionSync,
  resetActiveAssistant, sendEnvelope, sendMessageWithFiles, sessionID, sessionIDRef, setSessionLastUsage,
  setSessionPendingQuestion, setSessionPendingRequest, startPausePending, stopPausePending,
}: Args) {
  // An event handler from an old screen or socket must not submit to a new one.
  const { pairingID, connectionID } = relayScopeRef.current;
  const isCurrent = useCallback(() => Boolean(
    pairingID && connectionID &&
    relayScopeRef.current.pairingID === pairingID &&
    relayScopeRef.current.connectionID === connectionID &&
    sessionIDRef.current.trim() === sessionID.trim(),
  ), [connectionID, pairingID, relayScopeRef, sessionID, sessionIDRef]);

  const isBusy = useCallback((targetSessionID: string) => {
    const chat = getSessionChat(targetSessionID);
    return Boolean(chat.pendingRequestID || chat.pendingPermission || chat.pendingQuestions.length ||
      chat.messages.some((message) => message.role === "user" && message.status === "queued") ||
      getSessionRuns(targetSessionID).some(({ run }) => run.status === "running"));
  }, [getSessionChat, getSessionRuns]);

  const startChatRequest = useCallback((targetSessionID: string, requestID: string, send: () => boolean) => {
    requestSessionMapRef.current[requestID] = targetSessionID;
    if (!send()) {
      delete requestSessionMapRef.current[requestID];
      return false;
    }
    activeRequestIDRef.current = requestID;
    resetActiveAssistant(targetSessionID);
    setSessionLastUsage(targetSessionID, null);
    setSessionPendingRequest(targetSessionID, requestID);
    return true;
  }, [activeRequestIDRef, requestSessionMapRef, resetActiveAssistant, setSessionLastUsage, setSessionPendingRequest]);

  const sendUserMessage = useCallback(() => {
    const content = messageInput.trim();
    if (!content && attachedFiles.length === 0) return;
    const targetSessionID = sessionID.trim();
    if (!isCurrent()) return;
    // A second tap before React re-renders must not replace a pending request.
    if (getSessionChat(targetSessionID).pendingRequestID) return;
    const requestID = newRequestID();
    if (!startChatRequest(targetSessionID, requestID, () => sendMessageWithFiles(content, requestID))) {
      addEventMessage(targetSessionID, "消息发送失败：未连接到 Relay 服务，请检查连接状态。");
      return;
    }
    addUserMessage(targetSessionID, userMessageEcho(content, attachedFiles), requestID, attachedFiles);
  }, [addEventMessage, addUserMessage, attachedFiles, getSessionChat, isCurrent, messageInput, sendMessageWithFiles, sessionID, startChatRequest]);

  const sendPermissionResult = useCallback((allowed: boolean, scope?: PermissionApprovalScope) => {
    if (!isCurrent() || !pendingPermission) return;
    const targetSessionID = sessionID.trim();
    const current = getSessionChat(targetSessionID).pendingPermission;
    if (!current || current.approvalID !== pendingPermission.approvalID || current.requestID !== pendingPermission.requestID ||
        current.sessionID !== pendingPermission.sessionID || current.sessionID !== targetSessionID) return;
    if (!current.approvalID || !current.requestID) return;
    // A stale remember callback can race a fresh-consent replay. Recheck the live item.
    if (allowed && current.requireFreshConsent && scope !== undefined && scope !== "once") return;
    const payload: PermissionResultPayload = {
      approval_id: current.approvalID,
      allowed,
      ...(allowed ? { scope: scope || "once" } : {}),
    };
    sendEnvelope("permission_result", { request_id: current.requestID, session_id: current.sessionID, payload });
  }, [getSessionChat, isCurrent, pendingPermission, sendEnvelope, sessionID]);

  const allowPermission = useCallback(() => sendPermissionResult(true, "once"), [sendPermissionResult]);
  const allowPermissionSession = useCallback(() => {
    sendPermissionResult(true, pendingPermission?.permission ? "session_permission" : "session_tool");
  }, [pendingPermission?.permission, sendPermissionResult]);
  const denyPermission = useCallback(() => sendPermissionResult(false), [sendPermissionResult]);

  const answerUserQuestion = useCallback((question: UserQuestionState, submission: UserQuestionAnswerSubmission, cancelled = false) => {
    if (!isCurrent() || question.sessionID !== sessionID.trim() || !question.requestID) return false;
    const current = getSessionChat(question.sessionID).pendingQuestions.find((item) =>
      item.sessionID === question.sessionID && item.requestID === question.requestID && item.questionID === question.questionID,
    );
    if (!current || current.submitted) return false;
    const draftAnswers = current.draftAnswers;
    const sent = sendEnvelope("user_question_answer", {
      request_id: current.requestID,
      session_id: current.sessionID,
      payload: {
        schema_version: 2,
        question_id: current.questionID,
        structured_answers: submission.structuredAnswers,
        adopted_defaults: submission.adoptedDefaults || undefined,
        cancelled,
      },
    });
    if (sent) {
      const submittedSummary = cancelled ? "已跳过澄清" : formatSubmittedCardSummary(current.questions, submission);
      setSessionPendingQuestion(current.sessionID, { ...current, draftAnswers, submittedSummary, submitted: true });
      requestUserQuestionSync();
    } else if (!cancelled) {
      setSessionPendingQuestion(current.sessionID, { ...current, draftAnswers });
    }
    return sent;
  }, [getSessionChat, isCurrent, requestUserQuestionSync, sendEnvelope, sessionID, setSessionPendingQuestion]);

  const latestPauseRef = useRef<{ pairingID: string; connectionID: string; sessionID: string; requestID: string } | null>(null);
  const pauseSession = useCallback(() => {
    const targetSessionID = sessionID.trim();
    if (!targetSessionID || !isCurrent()) return;
    const previous = latestPauseRef.current;
    if (previous?.pairingID === pairingID && previous.connectionID === connectionID && previous.sessionID === targetSessionID &&
        requestSessionMapRef.current[previous.requestID] === targetSessionID) return;
    const requestID = newRequestID();
    latestPauseRef.current = { pairingID, connectionID, sessionID: targetSessionID, requestID };
    requestSessionMapRef.current[requestID] = targetSessionID;
    startPausePending();
    if (!sendEnvelope("session_pause", { request_id: requestID, session_id: targetSessionID, payload: { session_id: targetSessionID } })) {
      delete requestSessionMapRef.current[requestID];
      stopPausePending();
    }
  }, [connectionID, isCurrent, pairingID, requestSessionMapRef, sendEnvelope, sessionID, startPausePending, stopPausePending]);

  const regenerateSession = useCallback((turnID: string) => {
    const targetSessionID = sessionID.trim();
    if (!targetSessionID || !turnID || !isCurrent() || isBusy(targetSessionID)) return;
    const chat = getSessionChat(targetSessionID);
    if (latestRegeneratableTurnID(buildChatTurns(chat.messages, getSessionRuns(targetSessionID))) !== turnID) return;
    const requestID = newRequestID();
    startChatRequest(targetSessionID, requestID, () => sendEnvelope("session_regenerate", {
      request_id: requestID,
      session_id: targetSessionID,
      payload: { session_id: targetSessionID },
    }));
  }, [getSessionChat, getSessionRuns, isBusy, isCurrent, sendEnvelope, sessionID, startChatRequest]);

  return { answerUserQuestion, allowPermission, allowPermissionSession, denyPermission, pauseSession, regenerateSession, sendPermissionResult, sendUserMessage };
}

// Regenerate operates on the server's last user input. Stop at the first
// substantive turn from the bottom, even when that turn has no retry control.
export function latestRegeneratableTurnID(items: ReturnType<typeof buildChatTurns>) {
  for (let index = items.length - 1; index >= 0; index--) {
    const item = items[index];
    if (item.type === "user_turn") return undefined;
    if (item.type !== "agent_turn") continue;
    const hasUser = items.slice(0, index).some((earlier) => earlier.type === "user_turn");
    if (hasUser && item.canRegenerate && (item.status === "paused" || item.status === "error")) return item.id;
    return undefined;
  }
  return undefined;
}
