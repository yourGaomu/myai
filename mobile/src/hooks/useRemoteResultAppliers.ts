import { useMemo, type RefObject } from "react";

import type {
  ChangeDiffResultPayload,
  ChangeEntry,
  ChangeRevertResultPayload,
  ChangesListResultPayload,
  ContextInfo,
  FileEntry,
  FileListResultPayload,
  FileReadResultPayload,
  HistoryCheckpoint,
  HistoryDiffResultPayload,
  HistoryListResultPayload,
  HistoryRevertResultPayload,
  ModelListResultPayload,
  ModelSummary,
  ModelSwitchResultPayload,
  SessionChangedPayload,
  SessionHistoryResultPayload,
  SessionListResultPayload,
  SessionSettingsResultPayload,
  SessionSummary,
  TokenUsage,
} from "../protocol";
import type { PermissionState, ViewMode } from "../types/app";
import type { ChatItem } from "../types/chat";
import { historyMessageToChatItem } from "../utils/chatHistory";
import { shortID } from "../utils/ids";
import { findSessionUsage, upsertSession } from "../utils/session";

type Args = {
  addEventMessage: (sessionID: string, message: string) => void;
  filePath: string;
  hasPendingRequest: (sessionID: string) => boolean;
  historyDiff: HistoryDiffResultPayload | null;
  historySessionIDRef: RefObject<string>;
  pendingHistorySessionIDRef: RefObject<string>;
  replaceMessages: (sessionID: string, messages: ChatItem[]) => void;
  requestChanges: () => boolean;
  requestFiles: (path?: string) => boolean;
  requestHistory: () => boolean;
  requestSessionHistory: (sessionID?: string) => boolean;
  resetActiveAssistant: (sessionID: string) => void;
  selectedChange: string;
  sessionIDRef: RefObject<string>;
  setChangeDiff: (diff: ChangeDiffResultPayload | null) => void;
  setChanges: (changes: ChangeEntry[]) => void;
  setChangesClean: (clean: boolean) => void;
  setChangesMessage: (message: string) => void;
  setCurrentModelID: (modelID: string) => void;
  setFileEntries: (entries: FileEntry[]) => void;
  setFileParent: (parent: string) => void;
  setFilePath: (path: string) => void;
  setFilePreview: (preview: FileReadResultPayload | null) => void;
  setHistoryCheckpoints: (checkpoints: HistoryCheckpoint[]) => void;
  setHistoryDiff: (diff: HistoryDiffResultPayload | null) => void;
  setHistoryMessage: (message: string) => void;
  setModels: (models: ModelSummary[]) => void;
  setSelectedChange: (path: string) => void;
  setSessionLastUsage: (sessionID: string, usage: TokenUsage | null) => void;
  setSessionContext: (sessionID: string, context: ContextInfo) => void;
  setSessionID: (sessionID: string) => void;
  setSessionPendingPermission: (sessionID: string, permission: PermissionState | null) => void;
  setDeletedSessions: (sessions: SessionSummary[]) => void;
  setSessions: (updater: SessionSummary[] | ((current: SessionSummary[]) => SessionSummary[])) => void;
  setViewMode: (mode: ViewMode) => void;
};

export function useRemoteResultAppliers({
  addEventMessage,
  filePath,
  hasPendingRequest,
  historyDiff,
  historySessionIDRef,
  pendingHistorySessionIDRef,
  replaceMessages,
  requestChanges,
  requestFiles,
  requestHistory,
  requestSessionHistory,
  resetActiveAssistant,
  selectedChange,
  sessionIDRef,
  setChangeDiff,
  setChanges,
  setChangesClean,
  setChangesMessage,
  setCurrentModelID,
  setFileEntries,
  setFileParent,
  setFilePath,
  setFilePreview,
  setHistoryCheckpoints,
  setHistoryDiff,
  setHistoryMessage,
  setModels,
  setSelectedChange,
  setSessionLastUsage,
  setSessionContext,
  setSessionID,
  setSessionPendingPermission,
  setDeletedSessions,
  setSessions,
  setViewMode,
}: Args) {
  return useMemo(() => {
    const applySessionList = (payload?: SessionListResultPayload) => {
      const nextSessions = payload?.sessions || [];
      if (payload?.include_deleted) {
        setDeletedSessions(nextSessions);
        return;
      }

      setSessions(nextSessions);
      if (payload?.current_session_id && !sessionIDRef.current) {
        setSessionID(payload.current_session_id);
        sessionIDRef.current = payload.current_session_id;
        setSessionLastUsage(payload.current_session_id, findSessionUsage(nextSessions, payload.current_session_id));
        if (historySessionIDRef.current !== payload.current_session_id && !hasPendingRequest(payload.current_session_id)) {
          requestSessionHistory(payload.current_session_id);
        }
      } else if (sessionIDRef.current) {
        setSessionLastUsage(sessionIDRef.current, findSessionUsage(nextSessions, sessionIDRef.current));
      }
    };

    const applySessionChanged = (payload?: SessionChangedPayload) => {
      const nextSessions = payload?.sessions || [];
      setSessions(nextSessions);
      if (payload?.current_session_id) {
        setSessionID(payload.current_session_id);
        sessionIDRef.current = payload.current_session_id;
        setSessionLastUsage(payload.current_session_id, findSessionUsage(nextSessions, payload.current_session_id));
        setSessionPendingPermission(payload.current_session_id, null);
        historySessionIDRef.current = "";
        requestSessionHistory(payload.current_session_id);
      } else if (payload?.session?.id) {
        setSessionID(payload.session.id);
        sessionIDRef.current = payload.session.id;
        setSessionLastUsage(payload.session.id, payload.session.last_usage || null);
        setSessionPendingPermission(payload.session.id, null);
        historySessionIDRef.current = "";
        requestSessionHistory(payload.session.id);
      }
    };

    const applySessionSettings = (payload?: SessionSettingsResultPayload) => {
      if (!payload) {
        return;
      }

      const nextSessionID = payload.session?.id || payload.current_session_id || sessionIDRef.current;
      if (payload.sessions) {
        setSessions(payload.sessions);
      } else if (payload.session?.id) {
        setSessions((current) => upsertSession(current, payload.session as SessionSummary));
      }

      if (nextSessionID) {
        setSessionID(nextSessionID);
        sessionIDRef.current = nextSessionID;
        setSessionLastUsage(nextSessionID, payload.session?.last_usage || findSessionUsage(payload.sessions || [], nextSessionID));
        if (payload.context) {
          setSessionContext(nextSessionID, payload.context);
        }
      }

      if (payload.message) {
        addEventMessage(nextSessionID || sessionIDRef.current, payload.message);
      }
    };

    const applySessionHistory = (payload?: SessionHistoryResultPayload) => {
      if (!payload) {
        return;
      }
      const targetSessionID = pendingHistorySessionIDRef.current || sessionIDRef.current;
      if (payload.session_id && targetSessionID && payload.session_id !== targetSessionID) {
        return;
      }
      if (payload.session_id && hasPendingRequest(payload.session_id)) {
        return;
      }

      resetActiveAssistant(payload.session_id);
      pendingHistorySessionIDRef.current = "";
      historySessionIDRef.current = payload.session_id;
      if (payload.session_id) {
        setSessionID(payload.session_id);
        sessionIDRef.current = payload.session_id;
      }
      replaceMessages(payload.session_id, (payload.messages || []).map(historyMessageToChatItem));
    };

    const applyModelList = (payload?: ModelListResultPayload) => {
      setModels(payload?.models || []);
      if (payload?.current_model_id) {
        setCurrentModelID(payload.current_model_id);
      }
    };

    const applyModelSwitch = (payload?: ModelSwitchResultPayload) => {
      setModels(payload?.models || []);
      if (payload?.current_model_id) {
        setCurrentModelID(payload.current_model_id);
      }
      if (payload?.session?.id) {
        setSessionID(payload.session.id);
        setSessions((current) => upsertSession(current, payload.session as SessionSummary));
        setSessionLastUsage(payload.session.id, payload.session.last_usage || null);
      }
      addEventMessage(payload?.session?.id || sessionIDRef.current, payload?.message || `Model switched to ${payload?.current_model_id || "selected model"}`);
    };

    const applyFileList = (payload?: FileListResultPayload) => {
      setFilePath(payload?.path || ".");
      setFileParent(payload?.parent || "");
      setFileEntries(payload?.entries || []);
    };

    const applyFileRead = (payload?: FileReadResultPayload) => {
      if (!payload) {
        return;
      }
      setFilePreview(payload);
      setViewMode("files");
    };

    const applyChangesList = (payload?: ChangesListResultPayload) => {
      const nextChanges = payload?.entries || [];
      setChanges(nextChanges);
      setChangesClean(Boolean(payload?.clean));
      setChangesMessage(payload?.message || "");
      if (selectedChange && !nextChanges.some((entry) => entry.path === selectedChange)) {
        setSelectedChange("");
        setChangeDiff(null);
      }
    };

    const applyChangeDiff = (payload?: ChangeDiffResultPayload) => {
      if (!payload) {
        return;
      }
      setSelectedChange(payload.path || "");
      setChangeDiff(payload);
      setHistoryDiff(null);
      setViewMode("changeDetail");
    };

    const applyChangeRevert = (payload?: ChangeRevertResultPayload) => {
      if (!payload) {
        return;
      }
      addEventMessage(sessionIDRef.current, payload.message || `Reverted ${payload.path}`);
      setSelectedChange("");
      setChangeDiff(null);
      setViewMode("changes");
      requestChanges();
      requestHistory();
      requestFiles(filePath);
    };

    const applyHistoryList = (payload?: HistoryListResultPayload) => {
      const checkpoints = payload?.checkpoints || [];
      setHistoryCheckpoints(checkpoints);
      setHistoryMessage(checkpoints.length === 0 ? "No file history recorded yet" : "");
      if (historyDiff && !checkpoints.some((checkpoint) => checkpoint.id === historyDiff.checkpoint_id)) {
        setHistoryDiff(null);
      }
    };

    const applyHistoryDiff = (payload?: HistoryDiffResultPayload) => {
      if (!payload) {
        return;
      }
      setSelectedChange("");
      setChangeDiff(null);
      setHistoryDiff(payload);
      setHistoryMessage(payload.message || "");
      setViewMode("changeDetail");
    };

    const applyHistoryRevert = (payload?: HistoryRevertResultPayload) => {
      if (!payload) {
        return;
      }
      addEventMessage(sessionIDRef.current, payload.message || `Reverted checkpoint ${shortID(payload.checkpoint_id)}`);
      setSelectedChange("");
      setChangeDiff(null);
      setHistoryDiff(null);
      setViewMode("changes");
      requestChanges();
      requestHistory();
      requestFiles(filePath);
    };

    return {
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
    };
  }, [
    addEventMessage,
    filePath,
    hasPendingRequest,
    historyDiff,
    historySessionIDRef,
    pendingHistorySessionIDRef,
    replaceMessages,
    requestChanges,
    requestFiles,
    requestHistory,
    requestSessionHistory,
    resetActiveAssistant,
    selectedChange,
    sessionIDRef,
    setChangeDiff,
    setChanges,
    setChangesClean,
    setChangesMessage,
    setCurrentModelID,
    setFileEntries,
    setFileParent,
    setFilePath,
    setFilePreview,
    setHistoryCheckpoints,
    setHistoryDiff,
    setHistoryMessage,
    setModels,
    setSelectedChange,
    setSessionLastUsage,
    setSessionContext,
    setSessionID,
    setSessionPendingPermission,
    setDeletedSessions,
    setSessions,
    setViewMode,
  ]);
}
