import { useMemo, useRef, type RefObject } from "react";

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
  ModelConfigAddResultPayload,
  ModelConfigMutationResultPayload,
  ModelConfigTestResultPayload,
  ModelSummary,
  ModelSwitchResultPayload,
  Plan,
  RelayMessage,
  SessionChangedPayload,
  SessionGenerationResultPayload,
  SessionListResultPayload,
  SessionSettingsResultPayload,
  SkillListResultPayload,
  SkillSummary,
  SessionSummary,
  TokenUsage,
} from "../protocol";
import type { RelayScope, ViewMode } from "../types/app";
import type { SessionNavigationState } from "./useSessionModelActions";
import { shortID } from "../utils/ids";
import { findSessionUsage, mergeSessionList, upsertSession } from "../utils/session";

type Args = {
  addEventMessage: (sessionID: string, message: string) => void;
  filePath: string;
  hasPendingRequest: (sessionID: string) => boolean;
  historyDiff: HistoryDiffResultPayload | null;
  historySessionIDRef: RefObject<string>;
  pendingHistorySessionIDRef: RefObject<string>;
  relayScopeRef: RefObject<RelayScope>;
  sessionNavigationRef: RefObject<SessionNavigationState>;
  requestAssets: (sessionID?: string) => boolean;
  requestChanges: () => boolean;
  requestFiles: (path?: string) => boolean;
  requestHistory: () => boolean;
  requestSessionHistory: (sessionID?: string) => boolean;
  requestSubagentTasks: (sessionID?: string) => boolean;
  selectedChange: string;
  sessionIDRef: RefObject<string>;
  setChangeDiff: (diff: ChangeDiffResultPayload | null) => void;
  setAssets: (sessionID: string, assets: AssetSummary[]) => void;
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
  setModelMessage: (message: string) => void;
  setModelMessageError: (error: boolean) => void;
  setSelectedChange: (path: string) => void;
  setSkillMessage: (message: string) => void;
  setSkillRoot: (root: string) => void;
  setSkills: (skills: SkillSummary[]) => void;
  setSessionLastUsage: (sessionID: string, usage: TokenUsage | null) => void;
  setSessionContext: (sessionID: string, context: ContextInfo) => void;
  setSessionGenerationPreferences: (payload?: SessionGenerationResultPayload) => void;
  setSessionID: (sessionID: string) => void;
  setDeletedSessions: (updater: SessionSummary[] | ((current: SessionSummary[]) => SessionSummary[])) => void;
  setSessions: (updater: SessionSummary[] | ((current: SessionSummary[]) => SessionSummary[])) => void;
  setViewMode: (mode: ViewMode) => void;
};

// 将协议 DTO 归并到各页面状态；这里是远程数据到本地视图模型的统一映射入口。
export function useRemoteResultAppliers({
  addEventMessage,
  filePath,
  hasPendingRequest,
  historyDiff,
  historySessionIDRef,
  pendingHistorySessionIDRef,
  relayScopeRef,
  sessionNavigationRef,
  requestAssets,
  requestChanges,
  requestFiles,
  requestHistory,
  requestSessionHistory,
  requestSubagentTasks,
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
  setModelMessage,
  setModelMessageError,
  setSelectedChange,
  setSkillMessage,
  setSkillRoot,
  setSkills,
  setSessionLastUsage,
  setSessionContext,
  setSessionGenerationPreferences,
  setSessionID,
  setDeletedSessions,
  setSessions,
  setViewMode,
}: Args) {
  const filePathRef = useRef(filePath);
  filePathRef.current = filePath;
  const selectedChangeRef = useRef(selectedChange);
  selectedChangeRef.current = selectedChange;
  const historyDiffRef = useRef(historyDiff);
  historyDiffRef.current = historyDiff;
  const hasPendingRequestRef = useRef(hasPendingRequest);
  hasPendingRequestRef.current = hasPendingRequest;
  const addEventMessageRef = useRef(addEventMessage);
  addEventMessageRef.current = addEventMessage;
  const requestAssetsRef = useRef(requestAssets);
  requestAssetsRef.current = requestAssets;
  const requestChangesRef = useRef(requestChanges);
  requestChangesRef.current = requestChanges;
  const requestFilesRef = useRef(requestFiles);
  requestFilesRef.current = requestFiles;
  const requestHistoryRef = useRef(requestHistory);
  requestHistoryRef.current = requestHistory;
  const requestSessionHistoryRef = useRef(requestSessionHistory);
  requestSessionHistoryRef.current = requestSessionHistory;
  const requestSubagentTasksRef = useRef(requestSubagentTasks);
  requestSubagentTasksRef.current = requestSubagentTasks;

  return useMemo(() => {
    const applyAssistantPlan = (targetSessionID: string, plan?: Plan) => {
      const sessionID = targetSessionID.trim();
      if (!sessionID || !plan || (plan.session_id && plan.session_id !== sessionID)) return;
      setSessions((current) => current.map((item) => {
        if (item.id !== sessionID) return item;
        const previous = item.current_plan;
        if (previous?.id === plan.id && (previous.revision ?? 0) > (plan.revision ?? 0)) return item;
        return { ...item, current_plan: plan };
      }));
    };

    const selectSession = (sessionID: string) => {
      if (sessionIDRef.current !== sessionID) {
        sessionIDRef.current = sessionID;
        historySessionIDRef.current = "";
        pendingHistorySessionIDRef.current = "";
        setSessionID(sessionID);
      }
    };

    const ensureHistory = (sessionID: string) => {
      if (historySessionIDRef.current !== sessionID && pendingHistorySessionIDRef.current !== sessionID) {
        requestSessionHistoryRef.current(sessionID);
      }
    };

    const hasPendingNavigation = () => {
      const connectionID = relayScopeRef.current.connectionID;
      const navigation = sessionNavigationRef.current;
      return Boolean(connectionID && (
        navigation.current?.connectionID === connectionID ||
        [...navigation.mutations.values()].some((mutation) => mutation.connectionID === connectionID)
      ));
    };

    const applySessionList = (payload?: SessionListResultPayload) => {
      if (!payload) return;
      const nextSessions = payload.sessions || [];
      if (payload.include_deleted) {
        setDeletedSessions(nextSessions);
        return;
      }
      setSessions((current) => mergeSessionList(current, nextSessions));
      const localSessionID = sessionIDRef.current.trim();
      // A list can initialize a blank workspace, but not navigate an existing one.
      const currentSessionID = localSessionID || (!hasPendingNavigation() ? payload.current_session_id?.trim() : "");
      if (!currentSessionID) return;
      if (!localSessionID) {
        selectSession(currentSessionID);
        requestAssetsRef.current(currentSessionID);
      }
      const usage = findSessionUsage(nextSessions, currentSessionID);
      if (usage) setSessionLastUsage(currentSessionID, usage);
      if (!hasPendingRequestRef.current(currentSessionID)) ensureHistory(currentSessionID);
    };

    const applySessionChanged = (message: RelayMessage) => {
      const payload = message.payload as SessionChangedPayload | undefined;
      const requestID = message.request_id;
      const connectionID = relayScopeRef.current.connectionID;
      if (!requestID || !connectionID || !payload) return false;
      const navigation = sessionNavigationRef.current;
      const nextSessions = payload.sessions || [];
      const targetSessionID = (payload.current_session_id || payload.session?.id || "").trim();
      if (message.type === "session_changed") {
        const intent = navigation.current;
        if (intent?.requestID !== requestID || intent.connectionID !== connectionID) return false;
        if (!targetSessionID || (intent.targetSessionID && targetSessionID !== intent.targetSessionID)) return false;
        navigation.current = null;
        setSessions((current) => payload.sessions
          ? mergeSessionList(current, nextSessions)
          : payload.session?.id ? upsertSession(current, payload.session) : current);
        selectSession(targetSessionID);
      } else {
        const mutation = navigation.mutations.get(requestID);
        if (!mutation || mutation.connectionID !== connectionID || message.type !== mutation.type + "_result") return false;
        navigation.mutations.delete(requestID);
        const affectedID = mutation.targetSessionID;
        if (mutation.type === "session_restore") {
          const restored = nextSessions.find((session) => session.id === affectedID);
          if (restored) setSessions((current) => upsertSession(current, restored));
          setDeletedSessions((current) => current.filter((session) => session.id !== affectedID));
          return true;
        }
        const fallback = nextSessions.find((session) => session.id === targetSessionID) || payload.session;
        setSessions((current) => {
          const active = current.filter((session) => session.id !== affectedID);
          return fallback?.id && fallback.id !== affectedID ? upsertSession(active, fallback) : active;
        });
        // Only a delete of the still-selected session can pick its fallback.
        if (sessionIDRef.current !== affectedID || mutation.navigationVersion !== navigation.version ||
            (navigation.current?.connectionID === connectionID && !navigation.current.targetSessionID) ||
            !targetSessionID || targetSessionID === affectedID) return true;
        selectSession(targetSessionID);
      }
      const selectedSession = payload.session?.id === targetSessionID ? payload.session : undefined;
      const modelID = selectedSession?.model || nextSessions.find((session) => session.id === targetSessionID)?.model;
      if (modelID) setCurrentModelID(modelID);
      const usage = selectedSession?.last_usage || findSessionUsage(nextSessions, targetSessionID);
      if (usage) setSessionLastUsage(targetSessionID, usage);
      ensureHistory(targetSessionID);
      requestAssetsRef.current(targetSessionID);
      return true;
    };

    const applySessionCommandError = (requestID?: string) => {
      const connectionID = relayScopeRef.current.connectionID;
      if (!connectionID || !requestID) return false;
      const navigation = sessionNavigationRef.current;
      if (navigation.current?.requestID === requestID && navigation.current.connectionID === connectionID) {
        navigation.current = null;
        return true;
      }
      if (navigation.mutations.get(requestID)?.connectionID !== connectionID) return false;
      navigation.mutations.delete(requestID);
      return true;
    };

    const applySessionSettings = (payload?: SessionSettingsResultPayload, envelopeSessionID?: string) => {
      if (!payload) return;
      const sessionID = (payload.session?.id || envelopeSessionID || "").trim();
      if (!sessionID) return;
      const session = payload.session?.id === sessionID ? payload.session : payload.sessions?.find((item) => item.id === sessionID);
      if (session) {
        setSessions((current) => upsertSession(current, session));
        if (session.last_usage) setSessionLastUsage(sessionID, session.last_usage);
      }
      if (payload.context) setSessionContext(sessionID, payload.context);
      if (payload.message) addEventMessageRef.current(sessionID, payload.message);
    };

    const applySessionGenerationPreferences = (payload?: SessionGenerationResultPayload) => {
      setSessionGenerationPreferences(payload);
    };

    const applyModelList = (payload?: ModelListResultPayload) => {
      setModels(payload?.models || []);
      setModelMessageError(false);
      if (payload?.current_model_id) {
        setCurrentModelID(payload.current_model_id);
      }
    };

    const applyModelSwitch = (payload?: ModelSwitchResultPayload) => {
      setModels(payload?.models || []);
      setModelMessageError(false);
      if (payload?.current_model_id) {
        setCurrentModelID(payload.current_model_id);
      }
      if (payload?.session?.id) {
        setSessions((current) => upsertSession(current, payload.session as SessionSummary));
        setSessionLastUsage(payload.session.id, payload.session.last_usage || null);
      }
      addEventMessageRef.current(payload?.session?.id || sessionIDRef.current, payload?.message || `Model switched to ${payload?.current_model_id || "selected model"}`);
    };

    const applyModelConfigAdd = (payload?: ModelConfigAddResultPayload) => {
      setModels(payload?.models || []);
      setModelMessageError(false);
      setModelMessage(payload?.message || "模型已添加。");
    };

    const applyModelConfigTest = (payload?: ModelConfigTestResultPayload) => {
      setModelMessageError(false);
      setModelMessage(payload?.message || "连接测试成功。");
    };

    const applyModelConfigMutation = (payload?: ModelConfigMutationResultPayload) => {
      setModels(payload?.models || []);
      setModelMessageError(false);
      setModelMessage(payload?.message || "模型设置已更新。");
    };

    const applyModelConfigError = (message: string) => {
      setModelMessageError(true);
      setModelMessage(message);
    };

    const applySkillList = (payload?: SkillListResultPayload) => {
      const nextSkills = payload?.skills || [];
      setSkills(nextSkills);
      setSkillRoot(payload?.root || "");
      setSkillMessage(payload?.message || (nextSkills.length === 0 ? "No local skills found" : ""));
      if (payload?.message) {
        addEventMessageRef.current(sessionIDRef.current, payload.message);
      }
    };

    const applyAssetList = (payload?: AssetListResultPayload) => {
      if (payload?.session_id === sessionIDRef.current) setAssets(payload.session_id, payload.assets || []);
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
      if (selectedChangeRef.current && !nextChanges.some((entry) => entry.path === selectedChangeRef.current)) {
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
      addEventMessageRef.current(sessionIDRef.current, payload.message || `Reverted ${payload.path}`);
      setSelectedChange("");
      setChangeDiff(null);
      setViewMode("changes");
      requestChangesRef.current();
      requestHistoryRef.current();
      requestFilesRef.current(filePathRef.current);
    };

    const applyHistoryList = (payload?: HistoryListResultPayload) => {
      const checkpoints = payload?.checkpoints || [];
      setHistoryCheckpoints(checkpoints);
      setHistoryMessage(checkpoints.length === 0 ? "No file history recorded yet" : "");
      const currentHistoryDiff = historyDiffRef.current;
      if (currentHistoryDiff && !checkpoints.some((checkpoint) => checkpoint.id === currentHistoryDiff.checkpoint_id)) {
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
      addEventMessageRef.current(sessionIDRef.current, payload.message || `Reverted checkpoint ${shortID(payload.checkpoint_id)}`);
      setSelectedChange("");
      setChangeDiff(null);
      setHistoryDiff(null);
      setViewMode("changes");
      requestChangesRef.current();
      requestHistoryRef.current();
      requestFilesRef.current(filePathRef.current);
    };

    return {
      applyAssistantPlan,
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
      applyModelConfigAdd,
      applyModelConfigTest,
      applyModelConfigMutation,
      applyModelConfigError,
      applyModelSwitch,
      applySkillList,
      applySessionChanged,
      applySessionCommandError,
      hasPendingNavigation,
      applySessionList,
      applySessionSettings,
      applySessionGenerationPreferences,
    };
  }, [
    historySessionIDRef,
    pendingHistorySessionIDRef,
    relayScopeRef,
    sessionNavigationRef,
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
    setModelMessage,
    setModelMessageError,
    setSelectedChange,
    setSkillMessage,
    setSkillRoot,
    setSkills,
    setSessionLastUsage,
    setSessionContext,
    setSessionGenerationPreferences,
    setSessionID,
    setDeletedSessions,
    setSessions,
    setViewMode,
  ]);
}
