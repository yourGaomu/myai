import { useCallback, type RefObject } from "react";

import type { RelayMessage, TokenUsage } from "../protocol";
import type { PendingAction, SessionAgentMode, SessionPermissionMode } from "../types/app";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  activeRequestIDRef: RefObject<string>;
  clearSessionPendingRequest: (sessionID: string, requestID?: string) => void;
  historySessionIDRef: RefObject<string>;
  requestSessionMapRef: RefObject<Record<string, string>>;
  resetActiveAssistant: (sessionID: string) => void;
  sendEnvelope: SendEnvelope;
  sessionID: string;
  setSessionLastUsage: (sessionID: string, usage: TokenUsage | null) => void;
  setSessionPendingPermission: (sessionID: string, permission: null) => void;
  setSessionPendingRequest: (sessionID: string, requestID: string) => void;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

// 封装会话级命令。每个动作只负责发送协议和维护 pending，最终 Session 状态由服务端响应回填。
export function useSessionSettingsActions({
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
  startPending,
  stopPending,
}: Args) {
  const setPermissionMode = useCallback(
    (mode: SessionPermissionMode) => {
      const targetSessionID = sessionID.trim();
      if (!targetSessionID || !mode) {
        return;
      }

      startPending("settings");
      if (!sendEnvelope("session_permission_set", {
        request_id: newRequestID(),
        session_id: targetSessionID,
        payload: { session_id: targetSessionID, mode },
      })) {
        stopPending("settings");
      }
    },
    [sendEnvelope, sessionID, startPending, stopPending],
  );

  const setAgentMode = useCallback(
    (mode: SessionAgentMode) => {
      const targetSessionID = sessionID.trim();
      if (!targetSessionID || !mode) {
        return;
      }

      // 不直接修改本地 mode；等待 session_mode_set_result，保证刷新后仍与服务端一致。
      startPending("settings");
      if (!sendEnvelope("session_mode_set", {
        request_id: newRequestID(),
        session_id: targetSessionID,
        payload: { session_id: targetSessionID, mode },
      })) {
        stopPending("settings");
      }
    },
    [sendEnvelope, sessionID, startPending, stopPending],
  );

  const setContextWindowK = useCallback(
    (windowK: number) => {
      const targetSessionID = sessionID.trim();
      if (!targetSessionID || !windowK) {
        return;
      }

      startPending("settings");
      if (!sendEnvelope("session_context_set", {
        request_id: newRequestID(),
        session_id: targetSessionID,
        payload: { session_id: targetSessionID, window_k: windowK },
      })) {
        stopPending("settings");
      }
    },
    [sendEnvelope, sessionID, startPending, stopPending],
  );

  const compactSession = useCallback(() => {
    const targetSessionID = sessionID.trim();
    if (!targetSessionID) {
      return;
    }

    startPending("settings");
    if (!sendEnvelope("session_compact", {
      request_id: newRequestID(),
      session_id: targetSessionID,
      payload: { session_id: targetSessionID },
    })) {
      stopPending("settings");
    }
  }, [sendEnvelope, sessionID, startPending, stopPending]);

  const executePlan = useCallback(() => {
    const targetSessionID = sessionID.trim();
    if (!targetSessionID) {
      return;
    }

    // Plan 执行会持续返回 delta 和步骤更新，先建立 request -> session 映射才能正确归并流式事件。
    const requestID = newRequestID();
    activeRequestIDRef.current = requestID;
    requestSessionMapRef.current[requestID] = targetSessionID;
    resetActiveAssistant(targetSessionID);
    historySessionIDRef.current = targetSessionID;
    setSessionPendingPermission(targetSessionID, null);
    setSessionLastUsage(targetSessionID, null);
    setSessionPendingRequest(targetSessionID, requestID);
    startPending("plan");
    if (!sendEnvelope("session_plan_execute", {
      request_id: requestID,
      session_id: targetSessionID,
      payload: { session_id: targetSessionID },
    })) {
      activeRequestIDRef.current = "";
      delete requestSessionMapRef.current[requestID];
      clearSessionPendingRequest(targetSessionID, requestID);
      stopPending("plan");
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
    startPending,
    stopPending,
  ]);

  return {
    compactSession,
    executePlan,
    setAgentMode,
    setContextWindowK,
    setPermissionMode,
  };
}
