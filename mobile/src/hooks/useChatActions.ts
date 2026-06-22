import { useCallback, type RefObject } from "react";

import type { FileReadResultPayload, PermissionResultPayload, RelayMessage, TokenUsage } from "../protocol";
import type { PermissionState } from "../types/app";
import { userMessageEcho } from "../utils/attachments";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  activeRequestIDRef: RefObject<string>;
  addEventMessage: (sessionID: string, message: string) => void;
  addUserMessage: (sessionID: string, message: string) => void;
  attachedFiles: FileReadResultPayload[];
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
  clearSessionPendingRequest: (sessionID: string, requestID?: string) => void;
};

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
}: Args) {
  const sendUserMessage = useCallback(() => {
    const content = messageInput.trim();
    if (!content && attachedFiles.length === 0) {
      return;
    }

    const targetSessionID = sessionID.trim();
    const requestID = newRequestID();
    activeRequestIDRef.current = requestID;
    requestSessionMapRef.current[requestID] = targetSessionID;
    resetActiveAssistant(targetSessionID);
    if (targetSessionID) {
      historySessionIDRef.current = targetSessionID;
    }
    setSessionPendingPermission(targetSessionID, null);
    setSessionLastUsage(targetSessionID, null);
    setSessionPendingRequest(targetSessionID, requestID);
    addUserMessage(targetSessionID, userMessageEcho(content, attachedFiles));

    const sent = sendMessageWithFiles(content, requestID);
    if (!sent) {
      activeRequestIDRef.current = "";
      delete requestSessionMapRef.current[requestID];
      clearSessionPendingRequest(targetSessionID, requestID);
    }
  }, [
    activeRequestIDRef,
    addUserMessage,
    attachedFiles,
    clearSessionPendingRequest,
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
      sendEnvelope("permission_result", {
        request_id: pendingPermission.requestID,
        session_id: permissionSessionID,
        payload,
      });
      addEventMessage(permissionSessionID, `${allowed ? "Allowed" : "Denied"} ${pendingPermission.name}`);
      setSessionPendingPermission(permissionSessionID, null);
    },
    [addEventMessage, pendingPermission, requestSessionMapRef, sendEnvelope, sessionID, setSessionPendingPermission],
  );
  const allowPermission = useCallback(() => sendPermissionResult(true), [sendPermissionResult]);
  const denyPermission = useCallback(() => sendPermissionResult(false), [sendPermissionResult]);

  return {
    allowPermission,
    denyPermission,
    sendPermissionResult,
    sendUserMessage,
  };
}
