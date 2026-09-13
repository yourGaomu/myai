import { useCallback, type RefObject } from "react";

import type { PermissionState } from "../types/app";
import type { ChatAttachment } from "../types/chat";
import type { PermissionResultPayload, RelayMessage, TokenUsage } from "../protocol";
import { userMessageEcho } from "../utils/attachments";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  activeRequestIDRef: RefObject<string>;
  addEventMessage: (sessionID: string, message: string) => void;
  addUserMessage: (sessionID: string, message: string, requestID: string, attachments: ChatAttachment[]) => void;
  attachedFiles: ChatAttachment[];
  historySessionIDRef: RefObject<string>;
  messageInput: string;
  pendingPermission: PermissionState | null;
  requestSessionMapRef: RefObject<Record<string, string>>;
  resetActiveAssistant: (sessionID: string) => void;
  sendEnvelope: SendEnvelope;
  sendMessageWithFiles: (content: string, requestID: string) => boolean;
  sessionID: string;
  setSessionLastUsage: (sessionID: string, usage: TokenUsage | null) => void;
  setSessionPendingPermission: (sessionID: string, permission: PermissionState | null) => void;
  setSessionPendingRequest: (sessionID: string, requestID: string) => void;
  startPausePending: () => void;
  stopPausePending: () => void;
  clearSessionPendingRequest: (sessionID: string, requestID?: string) => void;
};

// 把发送、授权、暂停和重新生成封装成会话命令，并维护 request_id 到 session_id 的关联。
export function useChatActions({
  activeRequestIDRef,
  addEventMessage,
  addUserMessage,
  attachedFiles,
  clearSessionPendingRequest,
  historySessionIDRef,
  messageInput,
  pendingPermission,
  requestSessionMapRef,
  resetActiveAssistant,
  sendEnvelope,
  sendMessageWithFiles,
  sessionID,
  setSessionLastUsage,
  setSessionPendingPermission,
  setSessionPendingRequest,
  startPausePending,
  stopPausePending,
}: Args) {
  const sendUserMessage = useCallback(() => {
    const content = messageInput.trim();
    if (!content && attachedFiles.length === 0) {
      return;
    }

    const targetSessionID = sessionID.trim();
    // request_id 先登记到运行时引用，发送成功后再写入本地消息，避免断线时出现“假发送”记录。
    const requestID = newRequestID();
    activeRequestIDRef.current = requestID;
    requestSessionMapRef.current[requestID] = targetSessionID;

    const sent = sendMessageWithFiles(content, requestID);
    if (!sent) {
      activeRequestIDRef.current = "";
      delete requestSessionMapRef.current[requestID];
      return;
    }

    // Relay 已接收后，流式 delta 到达时即可追加到同一 assistant 消息。
    resetActiveAssistant(targetSessionID);
    if (targetSessionID) {
      historySessionIDRef.current = targetSessionID;
    }
    setSessionPendingPermission(targetSessionID, null);
    setSessionLastUsage(targetSessionID, null);
    setSessionPendingRequest(targetSessionID, requestID);
    addUserMessage(targetSessionID, userMessageEcho(content, attachedFiles), requestID, attachedFiles);
  }, [
    activeRequestIDRef,
    addUserMessage,
    attachedFiles,
    historySessionIDRef,
    messageInput,
    requestSessionMapRef,
    resetActiveAssistant,
    sendMessageWithFiles,
    sessionID,
    setSessionLastUsage,
    setSessionPendingPermission,
    setSessionPendingRequest,
  ]);

  const sendPermissionResult = useCallback(
    (allowed: boolean) => {
      if (!pendingPermission) {
        return;
      }

      const permissionSessionID =
        pendingPermission.sessionID ||
        requestSessionMapRef.current[pendingPermission.requestID] ||
        sessionID;
      const payload: PermissionResultPayload = { allowed };
      const sent = sendEnvelope("permission_result", {
        request_id: pendingPermission.requestID,
        session_id: permissionSessionID,
        payload,
      });
      if (!sent) {
        // Keep the prompt visible so the user can retry after the automatic
        // reconnect instead of losing the only way to resume the request.
        return;
      }
      addEventMessage(permissionSessionID, `${allowed ? "Allowed" : "Denied"} ${pendingPermission.name}`);
      setSessionPendingPermission(permissionSessionID, null);
    },
    [addEventMessage, pendingPermission, requestSessionMapRef, sendEnvelope, sessionID, setSessionPendingPermission],
  );
  const allowPermission = useCallback(() => sendPermissionResult(true), [sendPermissionResult]);
  const denyPermission = useCallback(() => sendPermissionResult(false), [sendPermissionResult]);

  const pauseSession = useCallback(() => {
    const targetSessionID = sessionID.trim();
    if (!targetSessionID) {
      return;
    }

    const requestID = newRequestID();
    requestSessionMapRef.current[requestID] = targetSessionID;
    startPausePending();
    const sent = sendEnvelope("session_pause", {
      request_id: requestID,
      session_id: targetSessionID,
      payload: { session_id: targetSessionID },
    });
    if (!sent) {
      delete requestSessionMapRef.current[requestID];
      stopPausePending();
    }
  }, [requestSessionMapRef, sendEnvelope, sessionID, startPausePending, stopPausePending]);

  const regenerateSession = useCallback(() => {
    const targetSessionID = sessionID.trim();
    if (!targetSessionID) {
      return;
    }

    const requestID = newRequestID();
    activeRequestIDRef.current = requestID;
    requestSessionMapRef.current[requestID] = targetSessionID;
    resetActiveAssistant(targetSessionID);
    historySessionIDRef.current = targetSessionID;
    setSessionPendingPermission(targetSessionID, null);
    setSessionLastUsage(targetSessionID, null);
    setSessionPendingRequest(targetSessionID, requestID);

    const sent = sendEnvelope("session_regenerate", {
      request_id: requestID,
      session_id: targetSessionID,
      payload: { session_id: targetSessionID },
    });
    if (!sent) {
      activeRequestIDRef.current = "";
      delete requestSessionMapRef.current[requestID];
      clearSessionPendingRequest(targetSessionID, requestID);
    }
  }, [
    activeRequestIDRef,
    clearSessionPendingRequest,
    historySessionIDRef,
    requestSessionMapRef,
    resetActiveAssistant,
    sendEnvelope,
    sessionID,
    setSessionLastUsage,
    setSessionPendingPermission,
    setSessionPendingRequest,
  ]);

  return {
    allowPermission,
    denyPermission,
    pauseSession,
    regenerateSession,
    sendPermissionResult,
    sendUserMessage,
  };
}
