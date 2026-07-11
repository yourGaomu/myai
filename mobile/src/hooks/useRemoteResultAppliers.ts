import { useMemo, type RefObject } from "react";

import type {
  AssetListResultPayload,
  AssetSummary,
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
  SessionHistoryDeltaResultPayload,
  SessionHistoryMetaResultPayload,
  SessionHistoryResultPayload,
  SessionListResultPayload,
  SessionSettingsResultPayload,
  SkillListResultPayload,
  SkillSummary,
  SessionSummary,
  TokenUsage,
} from "../protocol";
import type { PermissionState, ViewMode } from "../types/app";
import type { ChatItem } from "../types/chat";
import { appendCachedSessionHistory, replaceCachedSessionHistory } from "../storage/sessionHistoryCache";
import { historyMessageToChatItem } from "../utils/chatHistory";
import { shortID } from "../utils/ids";
import { findSessionUsage, upsertSession } from "../utils/session";

type Args = {
  addEventMessage: (sessionID: string, message: string) => void;
  appendMessages: (sessionID: string, messages: ChatItem[]) => void;
  filePath: string;
  hasPendingRequest: (sessionID: string) => boolean;
  historyDiff: HistoryDiffResultPayload | null;
  historySessionIDRef: RefObject<string>;
  pendingHistorySessionIDRef: RefObject<string>;
  replaceMessages: (sessionID: string, messages: ChatItem[]) => void;
  requestAssets: (sessionID?: string) => boolean;
  requestChanges: () => boolean;
  requestFiles: (path?: string) => boolean;
  requestHistory: () => boolean;
  requestSessionHistoryDelta: (sessionID?: string) => boolean;
  requestSessionHistoryFull: (sessionID?: string) => boolean;
  requestSessionHistory: (sessionID?: string) => boolean;
  resetActiveAssistant: (sessionID: string) => void;
  selectedChange: string;
  sessionIDRef: RefObject<string>;
  setChangeDiff: (diff: ChangeDiffResultPayload | null) => void;
  setAssets: (assets: AssetSummary[]) => void;
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
  setSkillMessage: (message: string) => void;
  setSkillRoot: (root: string) => void;
  setSkills: (skills: SkillSummary[]) => void;
  setSessionLastUsage: (sessionID: string, usage: TokenUsage | null) => void;
  setSessionContext: (sessionID: string, context: ContextInfo) => void;
  setSessionID: (sessionID: string) => void;
  setSessionPendingPermission: (sessionID: string, permission: PermissionState | null) => void;
  setDeletedSessions: (sessions: SessionSummary[]) => void;
  setSessions: (updater: SessionSummary[] | ((current: SessionSummary[]) => SessionSummary[])) => void;
  setViewMode: (mode: ViewMode) => void;
};

// 将协议 DTO 归并到各页面状态；这里是远程数据到本地视图模型的统一映射入口。
export function useRemoteResultAppliers({
  addEventMessage,
  appendMessages,
  filePath,
  hasPendingRequest,
  historyDiff,
  historySessionIDRef,
  pendingHistorySessionIDRef,
  replaceMessages,
  requestAssets,
  requestChanges,
  requestFiles,
  requestHistory,
  requestSessionHistoryDelta,
  requestSessionHistoryFull,
  requestSessionHistory,
  resetActiveAssistant,
  selectedChange,
  sessionIDRef,
  setChangeDiff,
  setAssets,
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
  setSkillMessage,
  setSkillRoot,
  setSkills,
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
      const currentSessionID = (payload?.current_session_id || sessionIDRef.current || "").trim();
      if (!currentSessionID) {
        return;
      }

      if (sessionIDRef.current !== currentSessionID) {
        setSessionID(currentSessionID);
        sessionIDRef.current = currentSessionID;
        requestAssets(currentSessionID);
      }
      setSessionLastUsage(currentSessionID, findSessionUsage(nextSessions, currentSessionID));
      if (
        historySessionIDRef.current !== currentSessionID &&
        pendingHistorySessionIDRef.current !== currentSessionID &&
        !hasPendingRequest(currentSessionID)
      ) {
        requestSessionHistory(currentSessionID);
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
        requestAssets(payload.current_session_id);
      } else if (payload?.session?.id) {
        setSessionID(payload.session.id);
        sessionIDRef.current = payload.session.id;
        setSessionLastUsage(payload.session.id, payload.session.last_usage || null);
        setSessionPendingPermission(payload.session.id, null);
        historySessionIDRef.current = "";
        requestSessionHistory(payload.session.id);
        requestAssets(payload.session.id);
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
        requestAssets(payload.session_id);
      }
      const messages = payload.messages || [];
      void replaceCachedSessionHistory(payload.session_id, messages);
      replaceMessages(payload.session_id, messages.map(historyMessageToChatItem));
    };

    const applySessionHistoryMeta = (payload?: SessionHistoryMetaResultPayload) => {
      if (!payload?.session_id) {
        return;
      }
      if (payload.up_to_date) {
        pendingHistorySessionIDRef.current = "";
        historySessionIDRef.current = payload.session_id;
        setSessionID(payload.session_id);
        sessionIDRef.current = payload.session_id;
        requestAssets(payload.session_id);
        return;
      }
      if (payload.can_delta) {
        requestSessionHistoryDelta(payload.session_id);
        return;
      }
      requestSessionHistoryFull(payload.session_id);
    };

    const applySessionHistoryDelta = (payload?: SessionHistoryDeltaResultPayload) => {
      if (!payload?.session_id) {
        return;
      }
      if (payload.full_sync_required) {
        requestSessionHistoryFull(payload.session_id);
        return;
      }

      const messages = payload.messages || [];
      pendingHistorySessionIDRef.current = "";
      historySessionIDRef.current = payload.session_id;
      setSessionID(payload.session_id);
      sessionIDRef.current = payload.session_id;
      requestAssets(payload.session_id);
      if (messages.length > 0) {
        void appendCachedSessionHistory(payload.session_id, messages);
        appendMessages(payload.session_id, messages.map(historyMessageToChatItem));
      }
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

    const applySkillList = (payload?: SkillListResultPayload) => {
      const nextSkills = payload?.skills || [];
      setSkills(nextSkills);
      setSkillRoot(payload?.root || "");
      setSkillMessage(payload?.message || (nextSkills.length === 0 ? "No local skills found" : ""));
      if (payload?.message) {
        addEventMessage(sessionIDRef.current, payload.message);
      }
    };

    const applyAssetList = (payload?: AssetListResultPayload) => {
      if (!payload) {
        setAssets([]);
        return;
      }
      setAssets(payload.assets || []);
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
      applyAssetList,
      applyChangeRevert,
      applyChangesList,
      applyFileList,
      applyFileRead,
      applyHistoryDiff,
      applyHistoryList,
      applyHistoryRevert,
      applyModelList,
      applyModelSwitch,
      applySkillList,
      applySessionChanged,
      applySessionHistory,
      applySessionHistoryDelta,
      applySessionHistoryMeta,
      applySessionList,
      applySessionSettings,
    };
  }, [
    addEventMessage,
    appendMessages,
    filePath,
    hasPendingRequest,
    historyDiff,
    historySessionIDRef,
    pendingHistorySessionIDRef,
    replaceMessages,
    requestAssets,
    requestChanges,
    requestFiles,
    requestHistory,
    requestSessionHistoryDelta,
    requestSessionHistoryFull,
    requestSessionHistory,
    resetActiveAssistant,
    selectedChange,
    sessionIDRef,
    setChangeDiff,
    setAssets,
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
    setSkillMessage,
    setSkillRoot,
    setSkills,
    setSessionLastUsage,
    setSessionContext,
    setSessionID,
    setSessionPendingPermission,
    setDeletedSessions,
    setSessions,
    setViewMode,
  ]);
}
