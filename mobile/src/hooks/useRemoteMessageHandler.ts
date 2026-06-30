import { useCallback, type RefObject } from "react";

import type {
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
  SessionHistoryResultPayload,
  SessionListResultPayload,
  SessionSettingsResultPayload,
  SessionPauseResultPayload,
  SkillListResultPayload,
  TokenUsage,
  ToolCallPayload,
} from "../protocol";
import type { PendingAction, PermissionState } from "../types/app";
import { shortID } from "../utils/ids";

type Args = {
  activeRequestIDRef: RefObject<string>;
  addErrorMessage: (sessionID: string, message: string) => void;
  addEventMessage: (sessionID: string, message: string) => void;
  addToolCall: (sessionID: string, name: string, argumentsText: string) => void;
  appendAssistant: (sessionID: string, text: string) => void;
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
  applySessionHistory: (payload?: SessionHistoryResultPayload) => void;
  applySessionList: (payload?: SessionListResultPayload) => void;
  applySessionSettings: (payload?: SessionSettingsResultPayload) => void;
  applySkillList: (payload?: SkillListResultPayload) => void;
  clearSessionPendingRequest: (sessionID: string, requestID?: string) => void;
  currentFilePath: string;
  getSessionChat: (sessionID: string) => { activeAssistantID: string };
  historySessionIDRef: RefObject<string>;
  mergeSessionChats: (fromSessionID: string, toSessionID: string) => void;
  requestChanges: () => boolean;
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

export function useRemoteMessageHandler({
  activeRequestIDRef,
  addErrorMessage,
  addEventMessage,
  addToolCall,
  appendAssistant,
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
  applySessionHistory,
  applySessionList,
  applySessionSettings,
  applySkillList,
  clearSessionPendingRequest,
  currentFilePath,
  getSessionChat,
  historySessionIDRef,
  mergeSessionChats,
  requestChanges,
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
      switch (message.type) {
        case "heartbeat":
          setStatus(message.request_id ? `Ack ${shortID(message.request_id)}` : "Connected");
          break;
        case "assistant_delta":
          appendAssistant(resolveChatSessionID(message, requestSessionMapRef, sessionIDRef), (message.payload as AssistantDeltaPayload | undefined)?.content || "");
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
          const streamedAssistantID =
            getSessionChat(targetSessionID).activeAssistantID ||
            (requestSessionID ? getSessionChat(requestSessionID).activeAssistantID : "");
          const payload = (message.payload || {}) as AssistantDonePayload;
          setSessionLastUsage(targetSessionID, payload.usage || null);
          if (payload.context) {
            setSessionContext(targetSessionID, payload.context);
          }
          if (payload.compact) {
            setSessionCompact(targetSessionID, payload.compact);
          }
          if (!streamedAssistantID) {
            appendAssistant(targetSessionID, payload.content || "");
          }
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
          addToolCall(resolveChatSessionID(message, requestSessionMapRef, sessionIDRef), payload.name || "tool", payload.arguments || "");
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
        case "session_permission_set_result":
        case "session_context_set_result":
        case "session_compact_result":
          stopPending("settings");
          applySessionSettings(message.payload as SessionSettingsResultPayload | undefined);
          requestSessions();
          break;
        case "session_pause_result": {
          stopPending("pause");
          const payload = (message.payload || {}) as SessionPauseResultPayload;
          const targetSessionID = resolveChatSessionID(message, requestSessionMapRef, sessionIDRef);
          addEventMessage(targetSessionID, payload.message || (payload.paused ? "Session paused." : "No running task to pause."));
          setStatus(payload.paused ? "Paused" : "Idle");
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
          addErrorMessage(targetSessionID, payload.message || "Remote error");
          setSessionPendingPermission(targetSessionID, null);
          clearSessionPendingRequest(targetSessionID, message.request_id);
          stopPending("sessions");
          stopPending("models");
          stopPending("skills");
          stopPending("files");
          stopPending("changes");
          stopPending("history");
          stopPending("diff");
          stopPending("revert");
          stopPending("settings");
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
      appendAssistant,
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
      applySessionHistory,
      applySessionList,
      applySessionSettings,
      applySkillList,
      clearSessionPendingRequest,
      currentFilePath,
      getSessionChat,
      historySessionIDRef,
      mergeSessionChats,
      requestChanges,
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
