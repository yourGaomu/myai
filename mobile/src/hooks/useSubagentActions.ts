import { useCallback } from "react";

import type { RelayMessage, SubagentDefinition } from "../protocol";
import type { PendingAction } from "../types/app";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  clientToken: string;
  onError: (message: string) => void;
  sendEnvelope: SendEnvelope;
  sessionID: string;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

export function useSubagentActions({ clientToken, onError, sendEnvelope, sessionID, startPending, stopPending }: Args) {
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
  const cancelTask = useCallback((taskID: string) => send("subagent_task_cancel", { task_id: taskID }), [send]);
  const applyTask = useCallback((taskID: string) => send("subagent_task_apply", { task_id: taskID }), [send]);
  const discardTask = useCallback((taskID: string) => send("subagent_task_discard", { task_id: taskID }), [send]);

  return {
    applyTask,
    cancelTask,
    checkTask,
    createDefinition,
    deleteDefinition,
    discardTask,
    requestDefinitions,
    requestTasks,
    updateDefinition,
  };
}
