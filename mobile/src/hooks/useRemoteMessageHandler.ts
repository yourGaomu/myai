import { useCallback, type RefObject } from "react";

import type {
  AssetListResultPayload,
  AssistantDeltaPayload,
  AssistantDonePayload,
  ChangeDiffResultPayload,
  ChangeRevertResultPayload,
  ChangesListResultPayload,
  ErrorPayload,
  FileListResultPayload,
  FileReadResultPayload,
  HistoryDiffResultPayload,
  HistoryListResultPayload,
  HistoryRevertResultPayload,
  ModelListResultPayload,
  ModelSwitchResultPayload,
  PermissionAskPayload,
  RelayMessage,
  SessionChangedPayload,
  SessionHistoryDeltaResultPayload,
  SessionHistoryMetaResultPayload,
  SessionHistoryResultPayload,
  SessionListResultPayload,
  SessionSettingsResultPayload,
  SessionPauseResultPayload,
  SkillListResultPayload,
  TokenUsage,
  ToolCallPayload,
  ToolResultPayload,
} from "../protocol";
import type { PendingAction, PermissionState } from "../types/app";
import { shortID } from "../utils/ids";

type Args = {
  activeRequestIDRef: RefObject<string>;
  addErrorMessage: (sessionID: string, message: string) => void;
  addEventMessage: (sessionID: string, message: string) => void;
  addToolCall: (sessionID: string, name: string, argumentsText: string, requestID?: string) => void;
  addToolResult: (sessionID: string, name: string, argumentsText: string, result: string, failed: boolean, requestID?: string) => void;
  appendAssistant: (sessionID: string, requestID: string | undefined, text: string, reasoning?: string) => void;
  applyAssetList: (payload?: AssetListResultPayload) => void;
  applyChangeDiff: (payload?: ChangeDiffResultPayload) => void;
  applyChangeRevert: (payload?: ChangeRevertResultPayload) => void;
  applyChangesList: (payload?: ChangesListResultPayload) => void;
  applyFileList: (payload?: FileListResultPayload) => void;
  applyFileRead: (payload?: FileReadResultPayload) => void;
  applyHistoryDiff: (payload?: HistoryDiffResultPayload) => void;
  applyHistoryList: (payload?: HistoryListResultPayload) => void;
  applyHistoryRevert: (payload?: HistoryRevertResultPayload) => void;
  applyModelList: (payload?: ModelListResultPayload) => void;
  applyModelSwitch: (payload?: ModelSwitchResultPayload) => void;
  applySessionChanged: (payload?: SessionChangedPayload) => void;
  applySessionHistoryDelta: (payload?: SessionHistoryDeltaResultPayload) => void;
  applySessionHistoryMeta: (payload?: SessionHistoryMetaResultPayload) => void;
  applySessionHistory: (payload?: SessionHistoryResultPayload) => void;
  applySessionList: (payload?: SessionListResultPayload) => void;
  applySessionSettings: (payload?: SessionSettingsResultPayload) => void;
  applySkillList: (payload?: SkillListResultPayload) => void;
  clearSessionPendingRequest: (sessionID: string, requestID?: string) => void;
  completeAssistant: (sessionID: string, requestID: string | undefined, status: "done" | "paused", usage?: TokenUsage | null, content?: string, reasoning?: string) => void;
  currentFilePath: string;
  getSessionChat: (sessionID: string) => { activeAssistantID: string; pendingRequestID: string };
  historySessionIDRef: RefObject<string>;
  markAssistantError: (sessionID: string, requestID: string | undefined, message?: string) => void;
  mergeSessionChats: (fromSessionID: string, toSessionID: string) => void;
  requestChanges: () => boolean;
  requestAssets: (sessionID?: string) => boolean;
  requestFiles: (path?: string) => boolean;
  requestHistory: () => boolean;
  requestModels: () => boolean;
  requestSkills: () => boolean;
  requestDeletedSessions: () => boolean;
  requestSessions: () => boolean;
  requestSessionMapRef: RefObject<Record<string, string>>;
  sessionIDRef: RefObject<string>;
  setSessionLastUsage: (sessionID: string, usage: TokenUsage | null) => void;
  setSessionCompact: (sessionID: string, compact: NonNullable<AssistantDonePayload["compact"]>) => void;
  setSessionContext: (sessionID: string, context: NonNullable<AssistantDonePayload["context"]>) => void;
  setSessionPendingPermission: (sessionID: string, permission: PermissionState | null) => void;
  setSessionID: (sessionID: string) => void;
  setStatus: (status: string) => void;
  stopPending: (action: PendingAction) => void;
};

// 所有服务端协议消息都在这里收口：先按 request_id 找到目标 Session，再更新对应领域状态。
export function useRemoteMessageHandler({
  activeRequestIDRef,
  addErrorMessage,
  addEventMessage,
  addToolCall,
  addToolResult,
  appendAssistant,
  applyAssetList,
  applyChangeDiff,
  applyChangeRevert,
  applyChangesList,
  applyFileList,
  applyFileRead,
  applyHistoryDiff,
  applyHistoryList,
  applyHistoryRevert,
  applyModelList,
  applyModelSwitch,
  applySessionChanged,
  applySessionHistoryDelta,
  applySessionHistoryMeta,
  applySessionHistory,
  applySessionList,
  applySessionSettings,
  applySkillList,
  clearSessionPendingRequest,
  completeAssistant,
  currentFilePath,
  getSessionChat,
  historySessionIDRef,
  markAssistantError,
  mergeSessionChats,
  requestChanges,
  requestAssets,
  requestFiles,
  requestHistory,
  requestModels,
  requestSkills,
  requestDeletedSessions,
  requestSessions,
  requestSessionMapRef,
  sessionIDRef,
  setSessionCompact,
  setSessionContext,
  setSessionLastUsage,
  setSessionPendingPermission,
  setSessionID,
  setStatus,
  stopPending,
}: Args) {
  return useCallback(
    (message: RelayMessage) => {
      // switch 分组与 Go 的 protocol.MessageType 一一对应；新增协议时必须同步补充这里的终态清理。
      switch (message.type) {
        case "heartbeat":
          setStatus(message.request_id ? `Ack ${shortID(message.request_id)}` : "Connected");
          break;
        case "assistant_delta":
          {
            const payload = (message.payload || {}) as AssistantDeltaPayload;
            appendAssistant(
              resolveChatSessionID(message, requestSessionMapRef, sessionIDRef),
              message.request_id,
              payload.content || "",
              payload.reasoning || "",
            );
          }
          break;
        case "assistant_done": {
          const requestSessionID = message.request_id ? requestSessionMapRef.current[message.request_id] || "" : "";
          const targetSessionID = message.session_id || requestSessionID || sessionIDRef.current;
          if (message.session_id && requestSessionID !== message.session_id) {
            mergeSessionChats(requestSessionID, message.session_id);
            if (message.request_id) {
              requestSessionMapRef.current[message.request_id] = message.session_id;
            }
          }
          const payload = (message.payload || {}) as AssistantDonePayload;
          setSessionLastUsage(targetSessionID, payload.usage || null);
          if (payload.context) {
            setSessionContext(targetSessionID, payload.context);
          }
          if (payload.compact) {
            setSessionCompact(targetSessionID, payload.compact);
          }
          completeAssistant(
            targetSessionID,
            message.request_id,
            payload.paused ? "paused" : "done",
            payload.usage || null,
            payload.content || payload.message || (payload.paused ? "Session task paused." : ""),
            payload.reasoning || "",
          );
          if (message.session_id && (!sessionIDRef.current || sessionIDRef.current === requestSessionID)) {
            setSessionID(message.session_id);
            historySessionIDRef.current = message.session_id;
          }
          setStatus(payload.paused ? "Paused" : "Done");
          setSessionPendingPermission(targetSessionID, null);
          clearSessionPendingRequest(targetSessionID, message.request_id);
          if (requestSessionID && requestSessionID !== targetSessionID) {
            clearSessionPendingRequest(requestSessionID, message.request_id);
          }
          requestSessions();
          requestModels();
          requestSkills();
          requestAssets(targetSessionID);
          requestFiles(currentFilePath);
          requestChanges();
          requestHistory();
          if (!message.request_id || activeRequestIDRef.current === message.request_id) {
            activeRequestIDRef.current = "";
          }
          if (message.request_id) {
            delete requestSessionMapRef.current[message.request_id];
          }
          break;
        }
        case "tool_call": {
          const payload = (message.payload || {}) as ToolCallPayload;
          addToolCall(resolveChatSessionID(message, requestSessionMapRef, sessionIDRef), payload.name || "tool", payload.arguments || "", message.request_id);
          break;
        }
        case "tool_result": {
          const payload = (message.payload || {}) as ToolResultPayload;
          addToolResult(
            resolveChatSessionID(message, requestSessionMapRef, sessionIDRef),
            payload.name || "tool",
            payload.arguments || "",
            payload.result || "",
            Boolean(payload.error),
            message.request_id,
          );
          break;
        }
        case "permission_ask": {
          const payload = (message.payload || {}) as PermissionAskPayload;
          const targetSessionID = resolveChatSessionID(message, requestSessionMapRef, sessionIDRef);
          setSessionPendingPermission(targetSessionID, {
            requestID: message.request_id || "",
            sessionID: targetSessionID,
            name: payload.name || "tool",
            permission: payload.permission || "permission",
            arguments: payload.arguments || "",
          });
          break;
        }
        case "session_list_result":
          stopPending("sessions");
          applySessionList(message.payload as SessionListResultPayload | undefined);
          break;
        case "session_changed":
        case "session_delete_result":
        case "session_restore_result":
          stopPending("sessions");
          applySessionChanged(message.payload as SessionChangedPayload | undefined);
          requestDeletedSessions();
          break;
        case "session_history_result":
          stopPending("sessions");
          applySessionHistory(message.payload as SessionHistoryResultPayload | undefined);
          break;
        case "session_history_meta_result": {
          const payload = message.payload as SessionHistoryMetaResultPayload | undefined;
          if (payload?.up_to_date) {
            stopPending("sessions");
          }
          applySessionHistoryMeta(payload);
          break;
        }
        case "session_history_delta_result": {
          const payload = message.payload as SessionHistoryDeltaResultPayload | undefined;
          if (!payload?.full_sync_required) {
            stopPending("sessions");
          }
          applySessionHistoryDelta(payload);
          break;
        }
        case "session_permission_set_result":
        case "session_mode_set_result":
        case "session_context_set_result":
        case "session_compact_result":
          // 设置以服务端返回的 Session 为准，并重新拉取列表，避免本地乐观状态与持久层不一致。
          stopPending("settings");
          applySessionSettings(message.payload as SessionSettingsResultPayload | undefined);
          requestSessions();
          break;
        case "session_plan_update":
          // Plan 执行中间态只更新当前 Session，不结束 pending，后续还会继续收到步骤状态。
          applySessionSettings(message.payload as SessionSettingsResultPayload | undefined);
          break;
        case "session_plan_execute_result":
          // 只有 execute_result 才表示整个计划结束，可以释放 Plan 按钮和会话运行状态。
          stopPending("plan");
          applySessionSettings(message.payload as SessionSettingsResultPayload | undefined);
          requestSessions();
          break;
        case "session_pause_result": {
          stopPending("pause");
          const payload = (message.payload || {}) as SessionPauseResultPayload;
          const targetSessionID = resolveChatSessionID(message, requestSessionMapRef, sessionIDRef);
          addEventMessage(targetSessionID, payload.message || (payload.paused ? "Session paused." : "No running task to pause."));
          setStatus(payload.paused ? "Paused" : "Idle");
          if (!payload.paused) {
            clearSessionPendingRequest(targetSessionID);
            if (!message.request_id || activeRequestIDRef.current === message.request_id) {
              activeRequestIDRef.current = "";
            }
          }
          if (message.request_id) {
            delete requestSessionMapRef.current[message.request_id];
          }
          break;
        }
        case "model_list_result":
          stopPending("models");
          applyModelList(message.payload as ModelListResultPayload | undefined);
          break;
        case "model_switch_result":
          stopPending("models");
          applyModelSwitch(message.payload as ModelSwitchResultPayload | undefined);
          requestSessions();
          break;
        case "skill_list_result":
        case "skill_reload_result":
          stopPending("skills");
          applySkillList(message.payload as SkillListResultPayload | undefined);
          break;
        case "asset_list_result":
          stopPending("assets");
          applyAssetList(message.payload as AssetListResultPayload | undefined);
          break;
        case "file_list_result":
          stopPending("files");
          applyFileList(message.payload as FileListResultPayload | undefined);
          break;
        case "file_read_result":
          stopPending("files");
          applyFileRead(message.payload as FileReadResultPayload | undefined);
          break;
        case "changes_list_result":
          stopPending("changes");
          applyChangesList(message.payload as ChangesListResultPayload | undefined);
          break;
        case "change_diff_result":
          stopPending("diff");
          applyChangeDiff(message.payload as ChangeDiffResultPayload | undefined);
          break;
        case "change_revert_result":
          stopPending("revert");
          applyChangeRevert(message.payload as ChangeRevertResultPayload | undefined);
          break;
        case "history_list_result":
          stopPending("history");
          applyHistoryList(message.payload as HistoryListResultPayload | undefined);
          break;
        case "history_diff_result":
          stopPending("diff");
          applyHistoryDiff(message.payload as HistoryDiffResultPayload | undefined);
          break;
        case "history_revert_result":
          stopPending("revert");
          applyHistoryRevert(message.payload as HistoryRevertResultPayload | undefined);
          break;
        case "error": {
          const payload = (message.payload || {}) as ErrorPayload;
          const targetSessionID = resolveChatSessionID(message, requestSessionMapRef, sessionIDRef);
          if (isChatRequestError(getSessionChat(targetSessionID), message.request_id)) {
            markAssistantError(targetSessionID, message.request_id, payload.message || "Remote error");
          }
          addErrorMessage(targetSessionID, payload.message || "Remote error");
          setSessionPendingPermission(targetSessionID, null);
          clearSessionPendingRequest(targetSessionID, message.request_id);
          stopPending("sessions");
          stopPending("models");
          stopPending("skills");
          stopPending("assets");
          stopPending("files");
          stopPending("changes");
          stopPending("history");
          stopPending("diff");
          stopPending("revert");
          stopPending("settings");
          stopPending("plan");
          stopPending("pause");
          if (!message.request_id || activeRequestIDRef.current === message.request_id) {
            activeRequestIDRef.current = "";
          }
          if (message.request_id) {
            delete requestSessionMapRef.current[message.request_id];
          }
          break;
        }
        default:
          addEventMessage(sessionIDRef.current, `Message: ${message.type}`);
      }
    },
    [
      activeRequestIDRef,
      addErrorMessage,
      addEventMessage,
      addToolCall,
      addToolResult,
      appendAssistant,
      applyAssetList,
      applyChangeDiff,
      applyChangeRevert,
      applyChangesList,
      applyFileList,
      applyFileRead,
      applyHistoryDiff,
      applyHistoryList,
      applyHistoryRevert,
      applyModelList,
      applyModelSwitch,
      applySessionChanged,
      applySessionHistoryDelta,
      applySessionHistoryMeta,
      applySessionHistory,
      applySessionList,
      applySessionSettings,
      applySkillList,
      clearSessionPendingRequest,
      completeAssistant,
      currentFilePath,
      getSessionChat,
      historySessionIDRef,
      markAssistantError,
      mergeSessionChats,
      requestChanges,
      requestAssets,
      requestFiles,
      requestHistory,
      requestModels,
      requestSkills,
      requestDeletedSessions,
      requestSessions,
      requestSessionMapRef,
      sessionIDRef,
      setSessionCompact,
      setSessionContext,
      setSessionLastUsage,
      setSessionPendingPermission,
      setSessionID,
      setStatus,
      stopPending,
    ],
  );
}

function resolveChatSessionID(
  message: RelayMessage,
  requestSessionMapRef: RefObject<Record<string, string>>,
  sessionIDRef: RefObject<string>,
) {
  return (message.session_id || (message.request_id ? requestSessionMapRef.current[message.request_id] : "") || sessionIDRef.current || "").trim();
}

function isChatRequestError(chat: { activeAssistantID: string; pendingRequestID: string }, requestID?: string) {
  if (!requestID) {
    return false;
  }
  return chat.pendingRequestID === requestID;
}
