import { useCallback, type RefObject } from "react";

import type { RelayMessage, SubagentDefinition, TokenUsage } from "../protocol";
import type { PendingAction } from "../types/app";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  activeRequestIDRef: RefObject<string>;
  clientToken: string;
  clearSessionPendingRequest: (sessionID: string, requestID?: string) => void;
  historySessionIDRef: RefObject<string>;
  onError: (message: string) => void;
  onResumeStarted: () => void;
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

export function useSubagentActions({
  activeRequestIDRef,
  clientToken,
  clearSessionPendingRequest,
  historySessionIDRef,
  onError,
  onResumeStarted,
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
  const send = useCallback((type: RelayMessage["type"], payload: unknown, targetSessionID = sessionID) => {
    if (!clientToken) {
      onError("请先完成配对");
      return false;
    }
    startPending("subagents");
    const sent = sendEnvelope(type, {
      request_id: newRequestID(),
      session_id: targetSessionID,
      payload,
    });
    if (!sent) {
      stopPending("subagents");
      onError("请求未发送，请检查 Relay 连接");
    }
    return sent;
  }, [clientToken, onError, sendEnvelope, sessionID, startPending, stopPending]);

  const requestDefinitions = useCallback(() => send("subagent_definition_list", {}), [send]);
  const createDefinition = useCallback((definition: Omit<SubagentDefinition, "id"> & { id?: string }) =>
    send("subagent_definition_create", definition), [send]);
  const updateDefinition = useCallback((definition: SubagentDefinition) =>
    send("subagent_definition_update", definition), [send]);
  const deleteDefinition = useCallback((definitionID: string) =>
    send("subagent_definition_delete", { definition_id: definitionID }), [send]);
  const requestTasks = useCallback((targetSessionID = sessionID) =>
    send("subagent_task_list", { session_id: targetSessionID, limit: 100 }, targetSessionID), [send, sessionID]);
  const checkTask = useCallback((taskID: string) => send("subagent_task_check", { task_id: taskID }), [send]);
  const messageTask = useCallback((taskID: string, message: string) =>
    send("subagent_task_message", { task_id: taskID, message }), [send]);
  const followupTask = useCallback((taskID: string, message: string) =>
    send("subagent_task_followup", { task_id: taskID, message }), [send]);
  const waitTask = useCallback((taskID: string) => send("subagent_task_wait", { task_id: taskID, timeout_ms: 30000 }), [send]);
  const cancelTask = useCallback((taskID: string) => send("subagent_task_cancel", { task_id: taskID }), [send]);
  const applyTask = useCallback((taskID: string) => send("subagent_task_apply", { task_id: taskID }), [send]);
  const discardTask = useCallback((taskID: string) => send("subagent_task_discard", { task_id: taskID }), [send]);
  const resumeTask = useCallback((taskID: string) => {
    const targetSessionID = sessionID.trim();
    if (!clientToken || !targetSessionID || !taskID.trim()) {
      onError(!clientToken ? "请先完成配对" : "缺少会话或任务，无法继续主任务");
      return false;
    }
    const requestID = newRequestID();
    activeRequestIDRef.current = requestID;
    requestSessionMapRef.current[requestID] = targetSessionID;
    resetActiveAssistant(targetSessionID);
    historySessionIDRef.current = targetSessionID;
    setSessionPendingPermission(targetSessionID, null);
    setSessionLastUsage(targetSessionID, null);
    setSessionPendingRequest(targetSessionID, requestID);
    startPending("subagents");
    const sent = sendEnvelope("subagent_task_resume", {
      request_id: requestID,
      session_id: targetSessionID,
      payload: { task_id: taskID },
    });
    if (!sent) {
      activeRequestIDRef.current = "";
      delete requestSessionMapRef.current[requestID];
      clearSessionPendingRequest(targetSessionID, requestID);
      stopPending("subagents");
      onError("继续请求未发送，请检查 Relay 连接");
      return false;
    }
    onResumeStarted();
    return true;
  }, [
    activeRequestIDRef,
    clientToken,
    clearSessionPendingRequest,
    historySessionIDRef,
    onError,
    onResumeStarted,
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
    applyTask,
    cancelTask,
    checkTask,
    createDefinition,
    deleteDefinition,
    discardTask,
    followupTask,
    messageTask,
    requestDefinitions,
    requestTasks,
    resumeTask,
    waitTask,
    updateDefinition,
  };
}
