import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import { BackHandler, Platform, type ScrollView } from "react-native";

import { ActionBar } from "../components/layout/ActionBar";
import { AppHeader, describeConnection } from "../components/layout/AppHeader";
import { HeaderIconButton, ScreenHeader } from "../components/layout/ScreenHeader";
import { ChatWallpaperLayer } from "../components/chat/ChatWallpaperLayer";
import { ConfirmModal } from "../components/common/ConfirmModal";
import { ResponsiveFormModal } from "../components/common/ResponsiveFormModal";
import { PermissionSheet } from "../components/permissions/PermissionSheet";
import { SessionRenameEditor } from "../components/session/SessionRenameEditor";
import { SessionSheet } from "../components/session/SessionSheet";
import { settingsSectionTitles, type SettingsSection } from "../components/settings/SettingsHome";
import { PairingScreen } from "../components/startup/PairingScreen";
import { BottomDock } from "../components/layout/BottomDock";
import { SideDrawer } from "../components/navigation/SideDrawer";
import type { ChangeEntry, RelayMessage } from "../protocol";
import type { QuestionSync } from "../types/app";
import type { RelayPairing } from "../utils/relayPairing";
import { AssetPreviewConnectionContext, useAssetImagePreview } from "../hooks/useAssetImagePreview";
import { useAssetState } from "../hooks/useAssetState";
import { useAssetUpload } from "../hooks/useAssetUpload";
import { useAgentRunState } from "../hooks/useAgentRunState";
import { useChangeHistoryActions } from "../hooks/useChangeHistoryActions";
import { useChangeHistoryState } from "../hooks/useChangeHistoryState";
import { useChatActions } from "../hooks/useChatActions";
import { getSubmittedQuestions, useChatMessages } from "../hooks/useChatMessages";
import { useFileActions } from "../hooks/useFileActions";
import { useFileState } from "../hooks/useFileState";
import { useMobileDerivedState } from "../hooks/useMobileDerivedState";
import { useMobileLayoutMetrics } from "../hooks/useMobileLayoutMetrics";
import { useMobileSettings } from "../hooks/useMobileSettings";
import { useMobileUiState } from "../hooks/useMobileUiState";
import { useKnowledgeActions } from "../hooks/useKnowledgeActions";
import { useKnowledgeState } from "../hooks/useKnowledgeState";
import { useAIMemoryActions } from "../hooks/useAIMemoryActions";
import { useAIMemoryState } from "../hooks/useAIMemoryState";
import { useNavigationActions } from "../hooks/useNavigationActions";
import { useNormalizedRelayUrl } from "../hooks/useNormalizedRelayUrl";
import { usePairingActions } from "../hooks/usePairingActions";
import { usePendingActions } from "../hooks/usePendingActions";
import { usePluginActions } from "../hooks/usePluginActions";
import { usePluginState } from "../hooks/usePluginState";
import { useRelayConnection, type ConnectionScope } from "../hooks/useRelayConnection";
import { stopLegacyRelayForegroundService, stopRelayForegroundService } from "../native/relayForegroundService";
import { useRemoteResultAppliers } from "../hooks/useRemoteResultAppliers";
import { useRemoteMessageHandler } from "../hooks/useRemoteMessageHandler";
import { useRemoteRuntimeRefs } from "../hooks/useRemoteRuntimeRefs";
import { useRelaySender } from "../hooks/useRelaySender";
import { useRemoteRequests } from "../hooks/useRemoteRequests";
import { useSessionContext } from "../hooks/useSessionContext";
import { useSessionModelActions } from "../hooks/useSessionModelActions";
import { useSessionModelState } from "../hooks/useSessionModelState";
import { useSessionRename } from "../hooks/useSessionRename";
import { useSessionSettingsActions } from "../hooks/useSessionSettingsActions";
import { useSessionGenerationState } from "../hooks/useSessionGenerationState";
import { useSkillState } from "../hooks/useSkillState";
import { useSubagentActions } from "../hooks/useSubagentActions";
import { useSubagentInspection } from "../hooks/useSubagentInspection";
import { useSubagentState } from "../hooks/useSubagentState";
import { useIntentSettings } from "../hooks/useIntentSettings";
import { useMCPState } from "../hooks/useMCPState";
import { useMCPTrust } from "../hooks/useMCPTrust";
import { SubagentInspectionModal } from "../components/subagents/SubagentInspectionModal";
import { MobileMainContent } from "./MobileMainContent";
import { MobileScreenShell } from "./MobileScreenShell";
import { buttonFeedback } from "../utils/buttonFeedback";
import { agentActivity, shouldHoldDoneActivity, type AgentActivity } from "../utils/agentActivity";
import { newRequestID } from "../utils/ids";
import { formatBytes } from "../utils/format";
import { activityToMotionSlot } from "../utils/motionRegistry";
import { describeSessionContext, formatContextUsage } from "../utils/sessionContext";
import { modelDisplayName, permissionLabel } from "../utils/session";
import { baseName, dirName, toolTarget } from "../utils/toolDisplay";

export function MobileAppScreen() {
  const { commitPairing, currentPairingRef, editRelayURL, pairing, relayURL, relayURLRef, relayScopeRef, settingsLoaded } = useMobileSettings();
  const normalizedRelayURL = useNormalizedRelayUrl(relayURL);
  const onPaired = useCallback((next: RelayPairing) => {
    const previous = currentPairingRef.current;
    // Invalidate the old handle synchronously, before React remounts the workspace.
    commitPairing(next);
    if (previous) void stopRelayForegroundService(previous.pairingID).catch(() => undefined);
  }, [commitPairing, currentPairingRef]);
  const { bindCode, cancelPairing, pairDevice, pairingError, pairingPending, setBindCode } = usePairingActions({
    relayURLRef,
    onPaired,
  });
  const handleRelayURLChange = (value: string) => {
    if (value !== relayURLRef.current) cancelPairing();
    editRelayURL(value);
  };
  const { bottomSafePadding, topSafePadding } = useMobileLayoutMetrics();

  useEffect(() => {
    if (settingsLoaded && !pairing) {
      void stopRelayForegroundService("").catch(() => undefined);
      void stopLegacyRelayForegroundService().catch(() => undefined);
    }
  }, [settingsLoaded, pairing]);

  if (!settingsLoaded) return null;
  if (!pairing) {
    return (
      <PairingScreen
        bindCode={bindCode}
        bottomInset={bottomSafePadding}
        errorMessage={pairingError}
        normalizedRelayURL={normalizedRelayURL}
        onBindCodeChange={setBindCode}
        onPair={() => { void pairDevice(); }}
        onRelayURLChange={handleRelayURLChange}
        pending={pairingPending}
        relayURL={relayURL}
        topInset={topSafePadding}
      />
    );
  }
  return (
    <PairedWorkspace
      key={pairing.pairingID}
      pairing={pairing}
      currentPairingRef={currentPairingRef}
      relayScopeRef={relayScopeRef}
      bindCode={bindCode}
      onBindCodeChange={setBindCode}
      onPair={() => { void pairDevice(); }}
      pairingError={pairingError}
      pairingPending={pairingPending}
      relayURL={relayURL}
      normalizedRelayURL={normalizedRelayURL}
      onRelayURLChange={handleRelayURLChange}
    />
  );
}

type PairedWorkspaceProps = {
  pairing: RelayPairing;
  currentPairingRef: RefObject<RelayPairing | null>;
  relayScopeRef: RefObject<ConnectionScope>;
  bindCode: string;
  onBindCodeChange: (value: string) => void;
  onPair: () => void;
  pairingError: string;
  pairingPending: boolean;
  relayURL: string;
  normalizedRelayURL: string;
  onRelayURLChange: (value: string) => void;
};

function PairedWorkspace({
  pairing, currentPairingRef, relayScopeRef, bindCode, onBindCodeChange, onPair, pairingError, pairingPending,
  relayURL, normalizedRelayURL, onRelayURLChange,
}: PairedWorkspaceProps) {
  const { clientToken, deviceID, pairingID, userID } = pairing;
  const {
    connected,
    sessionID,
    setConnected,
    setSessionID,
    setStatus,
    setViewMode,
    status,
    viewMode,
  } = useMobileUiState();
  const {
    clearModels,
    clearSessions: clearSessionModels,
    currentModelID,
    deletedSessions,
    modelMessage,
    modelMessageError,
    models,
    sessionCompacts,
    sessions,
    setCurrentModelID,
    setDeletedSessions,
    setModelMessage,
    setModelMessageError,
    setModels,
    setSessionCompact,
    setSessions,
  } = useSessionModelState();
  const {
    applyError: setSessionGenerationError,
    applyPreferences: setSessionGenerationPreferences,
    preferences: generationPreferences,
    status: generationStatus,
  } = useSessionGenerationState(sessionID);
  const {
    clearSkills,
    setSkillMessage,
    setSkillRoot,
    setSkills,
    skillMessage,
    skillRoot,
    skills,
  } = useSkillState();
  const {
    applyPluginList,
    clearPlugins,
    pluginMessage,
    pluginRoot,
    plugins,
  } = usePluginState();
  const {
    assets,
    clearAssets,
    setAssets,
  } = useAssetState(sessionID);
  const {
    applyCatalog: applyKnowledgeCatalog,
    applyDocuments: applyKnowledgeDocuments,
    applyProfiles: applyKnowledgeProfiles,
    applySearchResult: applyKnowledgeSearch,
    categories: knowledgeCategories,
    documentsByBase,
    jobsByBase,
    knowledgeBases,
    message: knowledgeMessage,
    profiles: knowledgeProfiles,
    searchResult: knowledgeSearchResult,
    selectedKnowledgeBaseID,
    setMessage: setKnowledgeMessage,
    setSelectedKnowledgeBaseID,
  } = useKnowledgeState();
  const {
    applyCandidateMutation: applyAIMemoryCandidateMutation,
    applyCandidates: applyAIMemoryCandidates,
    applyExtractionJobs: applyAIMemoryExtractionJobs,
    applyDreamResult: applyAIMemoryDreamResult,
    applyMemories: applyAIMemories,
    candidates: aiMemoryCandidates,
    extractionJobs: aiMemoryExtractionJobs,
    dreamRuns: aiMemoryDreamRuns,
    memories: aiMemories,
    message: aiMemoryMessage,
    setMessage: setAIMemoryMessage,
  } = useAIMemoryState();
  const {
    applyDefinitionList: applySubagentDefinitionList,
    applyDefinitionMutation: applySubagentDefinitionMutation,
    applyTaskList: applySubagentTaskList,
    applyTaskResult: applySubagentTaskResult,
    definitions: subagentDefinitions,
    events: subagentEvents,
    message: subagentMessage,
    setMessage: setSubagentMessage,
    tasks: subagentTasks,
  } = useSubagentState(sessionID);
  const {
    changeDiff,
    changes,
    changesClean,
    changesMessage,
    clearHistory,
    clearWorkspaceChanges,
    historyCheckpoints,
    historyDiff,
    historyMessage,
    selectedChange,
    setChangeDiff,
    setChanges,
    setChangesClean,
    setChangesMessage,
    setHistoryCheckpoints,
    setHistoryDiff,
    setHistoryMessage,
    setSelectedChange,
  } = useChangeHistoryState();
  const {
    attachedFiles,
    clearFileEntries,
    fileEntries,
    fileParent,
    filePath,
    filePreview,
    messageInput,
    setAttachedFiles,
    setFileEntries,
    setFileParent,
    setFilePath,
    setFilePreview,
    setMessageInput,
  } = useFileState({ pairingID, sessionID });
  const {
    addMessage,
    addToolCall,
    addToolResult,
    appendAssistant,
    clearSessionPendingRequest,
    removeSessionPendingQuestion,
    removeSessionPendingPermission,
    completeAssistant,
    getSessionChat,
    getSessionChats,
    hasPendingRequest,
    markAssistantError,
    markUserMessageQueued,
    markUserMessageDelivered,
    mergeSessionChats,
    mergeSessionHistory,
    resetActiveAssistant,
    removePendingQuestions,
    sessionChatsVersion,
    setSessionLastUsage,
    setSessionPendingPermission,
    setSessionPendingQuestion,
    setSessionPendingRequest,
    setSessionQuestionDraft,
    settleSessionRun,
    startBackgroundAssistant,
  } = useChatMessages();
  const [unreadSessionIDs, setUnreadSessionIDs] = useState<Record<string, boolean>>({});
  const [drawerVisible, setDrawerVisible] = useState(false);
  // 新版布局的页面级状态：会话设置弹层、对话是否滚动（顶栏分隔线）、权限确认是否收起、设置详情页、文件预览
  const [sessionSheetVisible, setSessionSheetVisible] = useState(false);
  const [sessionSheetContextFirst, setSessionSheetContextFirst] = useState(false);
  const [chatScrolled, setChatScrolled] = useState(false);
  const [chatNearHeader, setChatNearHeader] = useState(false);
  const [confirmRevertFile, setConfirmRevertFile] = useState(false);
  const [minimizedPermissionID, setMinimizedPermissionID] = useState("");
  const changeDetailReturnToRef = useRef<"chat" | "changes">("changes");
  const [settingsSection, setSettingsSection] = useState<SettingsSection | null>(null);
  const [filePreviewOpen, setFilePreviewOpen] = useState(false);
  const sessionFromPermissionRef = useRef(false);

  useEffect(() => {
    if (sessionID) {
      setUnreadSessionIDs((prev: Record<string, boolean>) => {
        if (!prev[sessionID]) return prev;
        const next = { ...prev };
        delete next[sessionID];
        return next;
      });
    }
  }, [sessionID]);
  const {
    applyRunCompleted,
    applyRunEvent,
    applyRunList,
    applyRunStarted,
    getSessionRuns,
    hasRunForRequest,
    mergeSessionRuns,
    version: agentRunsVersion,
  } = useAgentRunState();
  const { pendingActions, startPending, stopPending } = usePendingActions();

  const chatScrollRef = useRef<ScrollView | null>(null);
  // 远程运行时引用不触发重渲染，用来关联 WebSocket 请求、当前 Session 和流式回答。
  const {
    activeRequestIDRef,
    historySessionIDRef,
    pendingHistorySessionIDRef,
    requestSessionMapRef,
    sessionIDRef,
    socketRef,
  } = useRemoteRuntimeRefs(sessionID);

  const {
    activeModel,
    activeSession,
    canOpenSelectedChangeFile,
    canRevertSelectedChange,
    changesTabActive,
    filePreviewAttached,
    setupVisible,
  } = useMobileDerivedState({
    attachedFiles,
    changeDiff,
    changes,
    currentModelID,
    filePreview,
    models,
    selectedChange,
    sessionID,
    sessions,
    viewMode,
  });
  const currentChat = getSessionChat(sessionID);
  const currentRuns = getSessionRuns(sessionID);
  const isSessionBusy = useCallback((targetSessionID: string) => {
    const chat = getSessionChat(targetSessionID);
    return Boolean(chat.pendingRequestID || chat.pendingPermission || chat.pendingQuestions.length ||
      getSessionRuns(targetSessionID).some(({ run }) => run.status === "running"));
  }, [getSessionChat, getSessionRuns]);
  void sessionChatsVersion;
  void agentRunsVersion;
  const currentPauseBusy = Boolean(pendingActions.pause);
  const { bottomSafePadding, topSafePadding } = useMobileLayoutMetrics();
  const questionSyncRef = useRef<QuestionSync | null>(null);
  const onEnqueueFailureRef = useRef<(message: RelayMessage) => void>(() => undefined);
  const onEnqueueFailure = useCallback((message: RelayMessage) => onEnqueueFailureRef.current(message), []);

  // The sender, heartbeat and inbound callbacks share the authenticated connection.
  const addRelayErrorMessage = useCallback(
    (message: string) => addMessage(sessionIDRef.current, "error", message),
    [addMessage, sessionIDRef],
  );
  const sendEnvelope = useRelaySender({
    activeRequestIDRef,
    addErrorMessage: addRelayErrorMessage,
    currentPairingRef,
    onEnqueueFailure,
    sessionIDRef,
    socketRef,
  });
  const {
    clearSessionContexts,
    handleContextMessage,
    pendingContextSessionID,
    removeSessionContext,
    requestContextInfo,
    resetContextQuery,
    sessionContexts,
    setSessionContext,
  } = useSessionContext({ relayScopeRef, sessionID, sessionIDRef, sendEnvelope });
  const clearSessions = useCallback(() => {
    clearSessionModels();
    clearSessionContexts();
  }, [clearSessionModels, clearSessionContexts]);
  const activeContext = sessionContexts[sessionID];
  const contextPending = Boolean(sessionID) && pendingContextSessionID === sessionID;

  useEffect(() => {
    if (connected && viewMode === "chat" && sessionID) requestContextInfo();
  }, [connected, requestContextInfo, sessionID, viewMode]);

  const mcpTrust = useMCPTrust({ relayScopeRef, sendEnvelope });
  const { afterReload: refreshMCPTrustAfterReload, handleMessage: handleMCPTrustMessage,
    setActive: setMCPTrustActive, syncScope: syncMCPTrustScope } = mcpTrust;
  const {
    backFromDetail: backFromMCPDetail,
    closeDetail: closeMCPDetail,
    detail: mcpDetail,
    inventory: mcpInventory,
    loadMoreDetail: loadMoreMCPDetail,
    observeSessionPolicy: observeMCPPolicy,
    openDetail: openMCPDetail,
    openTool: openMCPTool,
    retryDetail: retryMCPDetail,
    reloading: mcpReloading,
    reloadMessage: mcpMessage,
    refresh: requestMCPInventory,
    reload: reloadMCP,
    handleDisconnect: disconnectMCP,
    handleMessage: handleMCPMessage,
  } = useMCPState({ relayScopeRef, sessionIDRef, sessionID, sendEnvelope, onReloaded: refreshMCPTrustAfterReload });
  const mcpSectionActive = viewMode === "settings" && settingsSection === "plugin";
  const visibleMCPDetail = mcpSectionActive && connected ? mcpDetail : null;
  useLayoutEffect(() => {
    setMCPTrustActive(mcpSectionActive && !mcpDetail);
  }, [mcpDetail, mcpSectionActive, setMCPTrustActive]);
  const mcpDetailNavKey = visibleMCPDetail
    ? JSON.stringify([visibleMCPDetail.serverName, visibleMCPDetail.selectedToolID])
    : undefined;

  useLayoutEffect(() => {
    if (!mcpSectionActive) closeMCPDetail();
  }, [closeMCPDetail, mcpSectionActive]);

  useEffect(() => {
    if (mcpSectionActive) requestMCPInventory();
  }, [mcpSectionActive, requestMCPInventory]);

  const { uploadMobileAsset, handleAssetUploadMessage, disconnectAssetUploads, reconnectAssetUploads } = useAssetUpload({
    sendEnvelope,
    relayScopeRef,
  });

  const requestUserQuestionSync = useCallback(() => {
    const { connectionID } = relayScopeRef.current;
    if (!connectionID) return false;
    const requestID = newRequestID();
    const chats = getSessionChats();
    const knownQuestionIDs = Object.values(chats).flatMap((chat) => chat.pendingQuestions.map((question) => question.questionID));
    const sync: QuestionSync = { requestID, connectionID, submittedQuestions: getSubmittedQuestions(chats) };
    questionSyncRef.current = sync;
    const sent = sendEnvelope("user_question_list", {
      request_id: requestID,
      session_id: "",
      payload: { known_question_ids: knownQuestionIDs },
    });
    if (!sent && questionSyncRef.current === sync) questionSyncRef.current = null;
    return sent;
  }, [getSessionChats, sendEnvelope]);

  const activeKnowledgeBase = knowledgeBases.find((base) => base.id === selectedKnowledgeBaseID);
  const {
    applyTask: applySubagentTask,
    cancelTask: cancelSubagentTask,
    checkTask: checkSubagentTask,
    createDefinition: createSubagentDefinition,
    deleteDefinition: deleteSubagentDefinition,
    discardTask: discardSubagentTask,
    followupTask: followupSubagentTask,
    messageTask: messageSubagentTask,
    requestDefinitions: requestSubagentDefinitions,
    requestTaskDetail,
    requestTaskEvents,
    requestTasks: requestSubagentTasks,
    resumeTask: resumeSubagentTask,
    updateDefinition: updateSubagentDefinition,
    waitTask: waitSubagentTask,
  } = useSubagentActions({
    activeRequestIDRef,
    clientToken,
    isSessionBusy,
    sessionIDRef,
    onError: setSubagentMessage,
    onResumeStarted: () => setViewMode("chat"),
    requestSessionMapRef,
    resetActiveAssistant,
    sendEnvelope,
    sessionID,
    setSessionLastUsage,
    setSessionPendingRequest,
    startPending,
    stopPending,
  });

  // 1.0 初始化子代理实时查看 Hook：管理只读详情、事件回放分页、实时缓冲与多 Run 状态
  const {
    agentRuns: inspectionAgentRuns,
    assets: inspectionAssets,
    closeInspection,
    error: inspectionError,
    handleDetailResult: applySubagentTaskDetailResult,
    handleEventsResult: applySubagentTaskEventsResult,
    handleRealtimeTaskEvent: applyRealtimeSubagentTaskEvent,
    hasMoreMessages: inspectionHasMoreMessages,
    inspectedTask,
    inspectedTaskID,
    inspectionOriginID,
    loadingDetail: inspectionLoadingDetail,
    loadingEvents: inspectionLoadingEvents,
    loadingMoreMessages: inspectionLoadingMoreMessages,
    loadMoreMessages: loadMoreInspectionMessages,
    messages: inspectionMessages,
    openInspection,
    readOnly: inspectionReadOnly,
    refreshInspection,
    runs: inspectionRuns,
    selectedRunID: inspectionSelectedRunID,
    selectRun: selectInspectionRun,
    sessionReady: inspectionSessionReady,
    taskEvents: inspectionTaskEvents,
  } = useSubagentInspection({
    currentSessionID: sessionID,
    requestTaskDetail,
    requestTaskEvents,
    tasks: subagentTasks,
  });
  const inspectionVisible = Boolean(inspectedTaskID && inspectionOriginID === sessionID);
  const { previewConnection, syncPreviewConnection, handleAssetPreviewMessage } = useAssetImagePreview({
    relayScopeRef, sessionIDRef, sessionID, sendEnvelope,
    inspectionSessionID: inspectionVisible ? inspectedTask?.child_session_id : undefined,
  });

  const { setPluginEnabled } = usePluginActions({
    clientToken,
    onError: (message) => addMessage(sessionID, "error", message),
    sendEnvelope,
    startPending,
    stopPending,
  });

  const {
    intentConfig,
    intentConfigLoading,
    intentConfigSaving,
    intentConfigTesting,
    intentConfigMessage,
    intentConfigError,
    intentTestResult,
    intentTraces,
    intentTraceDetails,
    intentTraceDetailErrors,
    intentTraceListLoading,
    intentTraceClearing,
    intentTraceDetailLoadingID,
    intentTraceMessage,
    intentTraceError,
    clearIntentConfigFeedback,
    clearIntentTraceFeedback,
    requestIntentConfig,
    saveIntentConfig,
    testIntentConfig,
    requestIntentTraces,
    requestIntentTraceDetail,
    clearIntentTraces,
    applyIntentConfigQuery,
    applyIntentConfigSet,
    applyIntentConfigTest,
    applyIntentTraceList,
    applyIntentTraceGet,
    applyIntentTraceClear,
    applyIntentError,
  } = useIntentSettings({
    clientToken,
    sendEnvelope,
    sessionID,
    startPending,
    stopPending,
  });

  const {
    acceptAgentRunList,
    acceptAssetList,
    acceptSessionList,
    handleRemoteQueryError,
    handleSessionHistoryMessage,
    resetRemoteQueries,
    refreshRemoteState,
    requestAssets,
    requestChanges,
    requestFiles,
    requestHistory,
    requestModels,
    requestPlugins,
    reloadPlugins,
    reloadSkills,
    requestDeletedSessions,
    requestSkills,
    requestSessionHistoryFull,
    requestSessionHistory,
    requestSessions,
    requestAgentRuns,
  } = useRemoteRequests({
    clearAssets,
    clearFileEntries,
    clearHistory,
    clearModels,
    clearSessions,
    clearSkills,
    clearPlugins,
    clearWorkspaceChanges,
    clientToken,
    currentFilePath: filePath,
    historySessionIDRef,
    mergeSessionHistory,
    pendingHistorySessionIDRef,
    relayScopeRef,
    requestSubagentTasks,
    sendEnvelope,
    sessionIDRef,
    startPending,
    stopPending,
  });
  const getRenameScope = useCallback(() => {
    const scope = relayScopeRef.current;
    const socket = socketRef.current;
    return currentPairingRef.current === pairing && socket?.pairing === pairing &&
      scope.pairingID === pairingID && scope.connectionID === socket.connectionID
      ? scope : { pairingID: "", connectionID: "" };
  }, [currentPairingRef, pairing, pairingID, relayScopeRef, socketRef]);
  const {
    draft: renameDraft,
    open: openRename,
    close: closeRename,
    edit: editRename,
    save: saveRename,
    tracker: renameTracker,
  } = useSessionRename({ getScope: getRenameScope, refreshSessions: requestSessions, sendEnvelope, sessions, setSessions });
  const canRenameSessions = connected && Boolean(getRenameScope().connectionID);
  const openListRename = useCallback((targetSessionID: string) => {
    openRename(targetSessionID, "list");
  }, [openRename]);

  const {
    addModelConfig,
    deleteModelConfig,
    deleteSession,
    disconnectModelDelete,
    handleModelDeleteMessage,
    loadSession,
    modelDeleteFeedback,
    newSession,
    restoreSession,
    sessionNavigationRef,
    setDefaultModel,
    setModelEnabled,
    switchModel,
    testModelConfig,
    updateModelConfig,
  } = useSessionModelActions({
    currentModelID,
    historySessionIDRef,
    pendingHistorySessionIDRef,
    relayScopeRef,
    requestAssets,
    requestSessionHistory,
    sendEnvelope,
    sessionIDRef,
    sessions,
    setModels,
    setSessionLastUsage,
    setSessionID,
    startPending,
    stopPending,
  });
  const {
    createCategory,
    createKnowledgeBase,
    deleteCategory,
    deleteDocument,
    deleteKnowledgeBase,
    moveCategory,
    requestCatalog,
    requestDocuments,
    requestProfiles,
    retryDocument,
    searchKnowledge,
    setRAGSettings,
    updateKnowledgeBase,
    cancelDocumentUpload,
    disconnectKnowledgeUploads,
    dismissDocumentImport,
    documentImportStatus,
    handleKnowledgeUploadMessage,
    reconcileDocumentImport,
    reconnectKnowledgeUploads,
    uploadingDocument,
    uploadDocument,
  } = useKnowledgeActions({
    uploadMobileAsset,
    activeKnowledgeBase,
    applyDocuments: applyKnowledgeDocuments,
    clientToken,
    onError: addRelayErrorMessage,
    pairingID,
    relayScopeRef,
    sendEnvelope,
    sessionID,
    startPending,
    stopPending,
  });
  const {
    approveCandidate: approveAIMemoryCandidate,
    createMemory: createAIMemory,
    deleteMemory: deleteAIMemory,
    rejectCandidate: rejectAIMemoryCandidate,
    requestCandidates: requestAIMemoryCandidates,
    requestExtractionJobs: requestAIMemoryExtractionJobs,
    requestDreamRuns: requestAIMemoryDreamRuns,
    requestMemories: requestAIMemories,
    retryExtractionJob: retryAIMemoryExtractionJob,
    runDream: runAIMemoryDream,
    restoreMemory: restoreAIMemory,
    updateMemory: updateAIMemory,
  } = useAIMemoryActions({
    clientToken,
    sendEnvelope,
    startPending,
    stopPending,
  });
  const {
    compactSession,
    executePlan,
    requestGenerationPreferences,
    setAgentMode,
    setContextWindowK,
    setGenerationSettings,
    setPermissionMode,
    setStyleInstruction,
  } = useSessionSettingsActions({
    activeRequestIDRef,
    isSessionBusy,
    sessionIDRef,
    requestSessionMapRef,
    resetActiveAssistant,
    sendEnvelope,
    sessionID,
    setSessionLastUsage,
    setSessionPendingRequest,
    startPending,
    stopPending,
  });
  const {
    attachFilePreview,
    goToParent,
    openFileEntry,
    openSelectedChangeFile,
    refreshCurrentFiles,
    removeAttachedFile,
    sendMessageWithFiles,
    cancelFileUpload,
    disconnectFileUpload,
    uploadLocalFile,
  } = useFileActions({
    uploadMobileAsset,
    addErrorMessage: addRelayErrorMessage,
    attachedFiles,
    changeDiffPath: changeDiff?.path,
    fileParent,
    filePreview,
    pairingID,
    relayScopeRef,
    requestFiles,
    selectedChange,
    sendEnvelope,
    sessionID,
    sessionIDRef,
    setAttachedFiles,
    setFilePreview,
    setMessageInput,
    setViewMode,
    startPending,
    stopPending,
  });
  const {
    executeRevertSelectedChange,
    openChangeEntry,
    previewHistoryCheckpoint,
    revertHistoryCheckpoint,
    revertSelectedChange,
  } = useChangeHistoryActions({
    canRevertSelectedChange,
    changeDiff,
    historyCheckpoints,
    selectedChange,
    sendEnvelope,
    setChangeDiff,
    setHistoryDiff,
    setSelectedChange,
    startPending,
    stopPending,
  });

  const {
    answerUserQuestion,
    allowPermission,
    allowPermissionSession,
    denyPermission,
    pauseSession,
    regenerateSession,
    sendUserMessage,
  } = useChatActions({
    activeRequestIDRef,
    addEventMessage: (targetSessionID, message) => addMessage(targetSessionID, "event", message),
    addUserMessage: (targetSessionID, message, requestID, attachments) => addMessage(targetSessionID, "user", message, requestID, attachments),
    attachedFiles,
    getSessionChat,
    getSessionRuns,
    messageInput,
    pendingPermission: currentChat.pendingPermission,
    relayScopeRef,
    requestSessionMapRef,
    requestUserQuestionSync,
    resetActiveAssistant,
    sendEnvelope,
    sendMessageWithFiles,
    sessionID,
    sessionIDRef,
    setSessionLastUsage,
    setSessionPendingQuestion,
    setSessionPendingRequest,
    startPausePending: () => startPending("pause"),
    stopPausePending: () => stopPending("pause"),
  });
  const {
    applyAssistantPlan,
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
  } = useRemoteResultAppliers({
    addEventMessage: (targetSessionID, message) => addMessage(targetSessionID, "event", message),
    filePath,
    hasPendingRequest,
    historyDiff,
    historySessionIDRef,
    pendingHistorySessionIDRef,
    relayScopeRef,
    onSessionDeleted: renameTracker.invalidateSession,
    onSessionList: renameTracker.reconcile,
    onSessionPolicyObserved: observeMCPPolicy,
    removeSessionContext,
    requestAssets,
    requestChanges,
    requestFiles,
    requestHistory,
    requestSessionHistory,
    requestSubagentTasks,
    selectedChange,
    sessionIDRef,
    sessionNavigationRef,
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
  });

  const handleRemoteMessage = useRemoteMessageHandler({
    acceptAgentRunList,
    acceptAssetList,
    acceptSessionList,
    handleRemoteQueryError,
    handleSessionHistoryMessage,
    handleAssetUploadMessage,
    activeRequestIDRef,
    addErrorMessage: (targetSessionID, message) => addMessage(targetSessionID, "error", message),
    addEventMessage: (targetSessionID, message) => addMessage(targetSessionID, "event", message),
    addToolCall,
    addToolResult,
    applyRunCompleted,
    applyRunEvent,
    applyRunList,
    applyRunStarted,
    applyAssistantPlan,
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
    applyIntentConfigQuery,
    applyIntentConfigSet,
    applyIntentConfigTest,
    applyIntentTraceList,
    applyIntentTraceGet,
    applyIntentTraceClear,
    applyIntentError,
    applyKnowledgeCatalog,
    applyKnowledgeDocuments,
    applyKnowledgeProfiles,
    applyKnowledgeSearch,
    applyKnowledgeError: setKnowledgeMessage,
    applyAIMemories,
    applyAIMemoryCandidates,
    applyAIMemoryCandidateMutation,
    applyAIMemoryExtractionJobs,
    applyAIMemoryDreamResult,
    applyAIMemoryError: setAIMemoryMessage,
    applyModelList,
    applyModelConfigAdd,
    applyModelConfigTest,
    applyModelConfigMutation,
    applyModelConfigError,
    handleModelDeleteMessage,
    handleSessionRenameMessage: renameTracker.handleMessage,
    isModelOperationPending: pendingActions.models,
    applyModelSwitch,
    applySkillList,
    applyPluginList,
    applySubagentDefinitionList,
    applySubagentDefinitionMutation,
    applySubagentTaskList,
    applySubagentTaskResult,
    applyRealtimeSubagentTaskEvent,
    applySubagentTaskDetailResult,
    applySubagentTaskEventsResult,
    applySessionChanged,
    applySessionCommandError,
    hasPendingNavigation,
    applySessionList,
    applySessionSettings,
    applySessionGenerationPreferences,
    applySessionGenerationError: setSessionGenerationError,
    clearSessionPendingRequest,
    markUserMessageQueued,
    markUserMessageDelivered,
    completeAssistant,
    currentFilePath: filePath,
    getSessionChat,
    pendingHistorySessionIDRef,
    questionSyncRef,
    relayScopeRef,
    hasRunForRequest,
    isKnowledgeOperationPending: pendingActions.knowledge || (viewMode === "knowledge" && pendingActions.settings),
    isGenerationOperationPending: pendingActions.generation,
    isMemoryOperationPending: pendingActions.memory,
    markAssistantError,
    mergeSessionChats,
    mergeSessionRuns,
    addUnreadSession: (targetID: string) => {
      setUnreadSessionIDs((prev: Record<string, boolean>) => (prev[targetID] ? prev : { ...prev, [targetID]: true }));
    },
    requestAgentRuns,
    requestSessionHistory,
    requestSessionHistoryFull,
    requestUserQuestionSync,
    startBackgroundAssistant,
    requestChanges,
    requestAssets,
    requestFiles,
    requestHistory,
    requestModels,
    requestDeletedSessions,
    requestSkills,
    requestSessions,
    requestSessionMapRef,
    sessionIDRef,
    setSessionCompact,
    setSessionContext,
    setSessionLastUsage,
    setSessionPendingPermission,
    setSessionPendingQuestion,
    removeSessionPendingQuestion,
    removeSessionPendingPermission,
    removePendingQuestions,
    settleSessionRun,
    setSessionID,
    stopPending,
  });

  const refreshAllRemoteState = useCallback(() => {
    refreshRemoteState();
    const selected = sessionIDRef.current;
    if (selected) requestSessionHistory(selected);
    for (const [id, chat] of Object.entries(getSessionChats())) {
      if (id && id !== selected && chat.pendingRequestID) requestAgentRuns(id);
    }
    requestIntentConfig();
    requestCatalog();
    requestProfiles();
    requestAIMemories();
    requestAIMemoryCandidates();
    requestAIMemoryExtractionJobs();
    requestAIMemoryDreamRuns();
    requestSubagentDefinitions();
    if (inspectedTaskID) {
      refreshInspection();
    }
  }, [getSessionChats, inspectedTaskID, refreshInspection, refreshRemoteState, requestAIMemories, requestAIMemoryCandidates, requestAIMemoryDreamRuns, requestAIMemoryExtractionJobs, requestAgentRuns, requestCatalog, requestIntentConfig, requestProfiles, requestSessionHistory, requestSubagentDefinitions, sessionIDRef]);

  const handleConnected = useCallback(() => {
    syncPreviewConnection();
    reconnectAssetUploads();
    reconnectKnowledgeUploads();
    requestUserQuestionSync();
    requestMCPInventory();
    syncMCPTrustScope();
    refreshAllRemoteState();
  }, [reconnectAssetUploads, reconnectKnowledgeUploads, refreshAllRemoteState, requestMCPInventory, requestUserQuestionSync, syncMCPTrustScope, syncPreviewConnection]);

  const handleDisconnected = useCallback(() => {
    syncPreviewConnection();
    questionSyncRef.current = null;
    resetRemoteQueries();
    resetContextQuery();
    disconnectMCP();
    syncMCPTrustScope();
    disconnectModelDelete();
    renameTracker.disconnect();
    disconnectFileUpload();
    disconnectKnowledgeUploads();
    disconnectAssetUploads();
    stopPending("pause");
  }, [disconnectAssetUploads, disconnectFileUpload, disconnectKnowledgeUploads, disconnectMCP, disconnectModelDelete, renameTracker, resetContextQuery, resetRemoteQueries, stopPending, syncMCPTrustScope, syncPreviewConnection]);

  const dispatchRelayMessage = useCallback((message: RelayMessage) => {
    if (handleMCPTrustMessage(message) || handleContextMessage(message) || handleMCPMessage(message) || handleAssetPreviewMessage(message) || handleKnowledgeUploadMessage(message)) return;
    handleRemoteMessage(message);
  }, [handleMCPTrustMessage, handleContextMessage, handleMCPMessage, handleAssetPreviewMessage, handleKnowledgeUploadMessage, handleRemoteMessage]);
  useLayoutEffect(() => {
    onEnqueueFailureRef.current = dispatchRelayMessage;
  }, [dispatchRelayMessage]);

  const connect = useRelayConnection({
    pairing,
    currentPairingRef,
    relayScopeRef,
    onConnected: handleConnected,
    onDisconnected: handleDisconnected,
    onMessage: dispatchRelayMessage,
    setConnected,
    setStatus,
    socketRef,
    startPending,
    stopPending,
  });

  const handleLoadSession = useCallback(
    (targetSessionID: string) => {
      setUnreadSessionIDs((prev: Record<string, boolean>) => {
        if (!prev[targetSessionID]) return prev;
        const next = { ...prev };
        delete next[targetSessionID];
        return next;
      });
      loadSession(targetSessionID);
    },
    [loadSession],
  );

  const {
    openChanges,
    openChat,
    openFiles,
    openKnowledge,
    openPlan,
    openSessions,
    openTrash,
    selectSession,
  } = useNavigationActions({
    fileEntriesCount: fileEntries.length,
    filePath,
    loadSession: handleLoadSession,
    requestAssets,
    requestChanges,
    requestFiles,
    requestHistory,
    requestKnowledge: requestCatalog,
    requestSessions,
    setViewMode,
  });

  const openChangeFromList = useCallback(
    (entry: ChangeEntry) => {
      changeDetailReturnToRef.current = "changes";
      openChangeEntry(entry);
    },
    [openChangeEntry],
  );

  const openChangeByPath = useCallback(
    (rawPath: string) => {
      const normalized = rawPath.replace(/\\/g, "/").replace(/^\.\//, "");
      const matched = changes.find(
        (entry) =>
          entry.path === normalized ||
          normalized.endsWith(`/${entry.path}`) ||
          entry.path.endsWith(`/${normalized}`),
      );
      changeDetailReturnToRef.current = "chat";
      openChangeEntry(
        matched || {
          path: normalized,
          restorable: true,
          status: "modified",
        },
      );
    },
    [changes, openChangeEntry],
  );

  const pendingPermission = currentChat.pendingPermission;
  const permissionSheetVisible = Boolean(pendingPermission) && minimizedPermissionID !== pendingPermission?.approvalID;
  const sessionRunning = currentRuns.some(({ run }) => run.status === "running");
  const runningSubagentCount = useMemo(
    () =>
      subagentTasks.filter((t) =>
        t.status === "running" ||
        t.status === "queued" ||
        t.status === "waiting_permission" ||
        t.status === "waiting_subagents",
      ).length,
    [subagentTasks],
  );
  const rawActivity = useMemo(
    () =>
      agentActivity({
        connected,
        messages: currentChat.messages,
        pendingPermission: Boolean(pendingPermission),
        pendingQuestion: currentChat.pendingQuestions.length > 0,
        pendingRequestID: currentChat.pendingRequestID,
        planRunning: activeSession?.current_plan?.status === "running",
        runningSubagentCount,
        runs: currentRuns,
        uploading: pendingActions.upload,
      }),
    [
      activeSession?.current_plan?.status,
      connected,
      currentChat.messages,
      currentChat.pendingQuestions.length,
      currentChat.pendingRequestID,
      currentRuns,
      pendingActions.upload,
      pendingPermission,
      runningSubagentCount,
    ],
  );
  const [doneHoldSessionID, setDoneHoldSessionID] = useState("");
  const prevActivityRef = useRef<{ sessionID: string; activity: AgentActivity }>({ sessionID, activity: rawActivity });
  useEffect(() => {
    const prev = prevActivityRef.current;
    prevActivityRef.current = { sessionID, activity: rawActivity };
    if (prev.sessionID !== sessionID || rawActivity !== "idle") {
      if (doneHoldSessionID) setDoneHoldSessionID("");
      return;
    }
    const latestMessage = currentChat.messages[currentChat.messages.length - 1];
    const latestFailed = Boolean(
      latestMessage &&
      (latestMessage.role === "error" || latestMessage.status === "error" || latestMessage.status === "paused"),
    );
    if (!shouldHoldDoneActivity(prev.activity, rawActivity, latestFailed)) {
      return;
    }
    setDoneHoldSessionID(sessionID);
    const timer = setTimeout(() => {
      setDoneHoldSessionID((current) => (current === sessionID ? "" : current));
    }, 2400);
    return () => clearTimeout(timer);
  }, [currentChat.messages, doneHoldSessionID, rawActivity, sessionID]);
  const currentActivity: AgentActivity = rawActivity === "idle" && doneHoldSessionID === sessionID ? "done" : rawActivity;
  const currentActivitySlot = activityToMotionSlot(currentActivity);
  const isEmptyHome =
    currentChat.messages.length === 0 &&
    currentRuns.length === 0 &&
    !activeSession?.current_plan &&
    currentChat.pendingQuestions.length === 0;
  const chatBusy = Boolean(currentChat.pendingRequestID) || sessionRunning || runningSubagentCount > 0;
  const planMode = activeSession?.agent_mode === "plan";
  const activePermissionLabel = permissionLabel(activeSession?.permission_mode);
  const connectionLabel = describeConnection(status, connected, deviceID).label;

  // 读文件结果到达时进入预览；离开文件页时回到列表
  useEffect(() => {
    if (filePreview) {
      setFilePreviewOpen(true);
    }
  }, [filePreview]);
  useEffect(() => {
    if (viewMode !== "files") {
      setFilePreviewOpen(false);
    }
    if (viewMode !== "settings") {
      setSettingsSection(null);
    }
  }, [viewMode]);

  // 安卓返回键：先退出二级详情，再回到对话；对话页交给系统处理
  useEffect(() => {
    if (Platform.OS !== "android") {
      return;
    }
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      if (viewMode === "files" && filePreviewOpen) {
        setFilePreviewOpen(false);
        return true;
      }
      if (viewMode === "settings" && settingsSection) {
        if (visibleMCPDetail) backFromMCPDetail();
        else setSettingsSection(null);
        return true;
      }
      if (viewMode === "changeDetail") {
        (changeDetailReturnToRef.current === "chat" ? openChat : openChanges)();
        return true;
      }
      if (viewMode !== "chat") {
        openChat();
        return true;
      }
      return false;
    });
    return () => subscription.remove();
  }, [backFromMCPDetail, filePreviewOpen, openChanges, openChat, settingsSection, viewMode, visibleMCPDetail]);

  const openSessionSheet = useCallback(() => {
    sessionFromPermissionRef.current = false;
    setSessionSheetContextFirst(false);
    setSessionSheetVisible(true);
  }, []);
  const openContextSheet = useCallback(() => {
    sessionFromPermissionRef.current = false;
    setSessionSheetContextFirst(true);
    setSessionSheetVisible(true);
  }, []);
  const openSheetRename = useCallback(() => {
    openRename(sessionIDRef.current, "sheet");
  }, [openRename, sessionIDRef]);
  const closeSessionSheet = useCallback(() => {
    closeRename("sheet");
    setSessionSheetVisible(false);
    // 从权限确认里点「修改策略」进来的，关掉后回到权限确认
    if (sessionFromPermissionRef.current) {
      sessionFromPermissionRef.current = false;
      setMinimizedPermissionID("");
    }
  }, [closeRename]);
  const sheetRenameDraft = renameDraft?.entry === "sheet" && renameDraft.sessionID === sessionID ? renameDraft : null;
  const listRenameDraft = renameDraft?.entry === "list" ? renameDraft : null;
  const startNewSession = useCallback(() => {
    setChatScrolled(false);
    setDrawerVisible(false);
    setSessionSheetVisible(false);
    newSession();
    openChat();
  }, [newSession, openChat]);
  const pickSuggestion = useCallback((text: string) => setMessageInput(text), [setMessageInput]);
  const insertReference = useCallback((snippet: string) => {
    const clean = snippet.trim();
    if (!clean) return;
    const existing = messageInput.trim();
    setMessageInput(existing ? `${existing}\n\n${clean}` : clean);
    openChat();
  }, [messageInput, openChat, setMessageInput]);

  const sessionHasRunningAgent = useMemo(() => {
    return currentRuns.some(({ run }) => run.status === "running");
  }, [currentRuns]);

  const canPauseSession = Boolean(
    currentChat.pendingRequestID ||
    sessionHasRunningAgent ||
    currentChat.messages.some(
      (m) =>
        (m.role === "assistant" && (m.status === "streaming" || m.status === "tool_running")) ||
        (m.role === "tool_call" && m.status === "tool_running"),
    ),
  );

  const refreshButton = (onPress: () => void, busy: boolean) => (
    <HeaderIconButton accessibilityLabel="刷新" disabled={busy} icon="refresh-cw" muted onPress={onPress} size={19} />
  );
  let header: ReactNode;
  if (viewMode === "chat") {
    const { used, window } = describeSessionContext(activeContext, activeSession?.context_window_k);
    header = (
      <AppHeader
        activity={currentActivity}
        busy={chatBusy}
        connected={connected}
        contextUsage={sessionID ? formatContextUsage(used, window) : undefined}
        deviceID={deviceID}
        emptyHome={isEmptyHome}
        nearHeader={chatNearHeader}
        onNewSession={startNewSession}
        onOpenContext={openContextSheet}
        onOpenMenu={() => setDrawerVisible(true)}
        onOpenSession={openSessionSheet}
        planMode={planMode}
        status={status}
        title={activeSession?.title?.trim() || "新对话"}
        topInset={topSafePadding}
        waiting={Boolean(pendingPermission) || currentChat.pendingQuestions.length > 0}
      />
    );
  } else {
    let title = "设置";
    let subtitle: string | undefined;
    let onBack: () => void = openChat;
    let right: ReactNode = null;
    if (viewMode === "files") {
      if (filePreviewOpen && filePreview) {
        title = filePreview.name || baseName(filePreview.path);
        subtitle = [dirName(filePreview.path), formatBytes(filePreview.size || 0)].filter(Boolean).join(" · ");
        onBack = () => setFilePreviewOpen(false);
      } else {
        title = "文件";
        right = refreshButton(() => { refreshCurrentFiles(); requestAssets(); }, pendingActions.files);
      }
    } else if (viewMode === "changes") {
      title = "变更";
      right = refreshButton(() => { requestChanges(); requestHistory(); }, pendingActions.changes || pendingActions.history);
    } else if (viewMode === "changeDetail") {
      title = changeDiff ? baseName(changeDiff.path) : historyDiff ? "快照对比" : "差异";
      subtitle = changeDiff ? dirName(changeDiff.path) || undefined : undefined;
      onBack = changeDetailReturnToRef.current === "chat" ? openChat : openChanges;
    } else if (viewMode === "sessions") {
      title = "全部对话";
      right = refreshButton(() => { requestSessions(); requestDeletedSessions(); }, pendingActions.sessions);
    } else if (viewMode === "trash") {
      title = "回收站";
      right = refreshButton(() => requestDeletedSessions(), pendingActions.sessions);
    } else if (viewMode === "knowledge") {
      title = "知识库与记忆";
      right = refreshButton(() => { requestCatalog(); requestProfiles(); requestDocuments(selectedKnowledgeBaseID); }, pendingActions.knowledge);
    } else if (viewMode === "plan") {
      title = "计划";
    } else if (settingsSection) {
      title = visibleMCPDetail
        ? (visibleMCPDetail.selectedToolID ? "MCP 工具详情" : "MCP 工具目录")
        : settingsSectionTitles[settingsSection];
      onBack = visibleMCPDetail ? backFromMCPDetail : () => setSettingsSection(null);
    }
    header = <ScreenHeader onBack={onBack} right={right} subtitle={subtitle} title={title} topInset={topSafePadding} />;
  }

  let dock: ReactNode = null;
  if (viewMode === "chat") {
    dock = (
      <BottomDock
        attachedFiles={attachedFiles}
        bottomPadding={bottomSafePadding}
        buttonFeedback={buttonFeedback}
        canPause={canPauseSession}
        messageInput={messageInput}
        modelLabel={activeModel ? modelDisplayName(activeModel) : activeSession?.model || "模型"}
        onBrowseKnowledge={openKnowledge}
        onBrowseWorkspace={openFiles}
        onChangeMessage={setMessageInput}
        onFocus={() => {
          requestAnimationFrame(() => {
            chatScrollRef.current?.scrollToEnd({ animated: true });
          });
        }}
        onOpenPermission={() => setMinimizedPermissionID("")}
        onOpenSession={openSessionSheet}
        onPause={pauseSession}
        onRemoveAttachedFile={removeAttachedFile}
        onSend={sendUserMessage}
        onTogglePlan={() => setAgentMode(planMode ? "chat" : "plan")}
        onUploadFile={uploadLocalFile}
        onCancelUpload={cancelFileUpload}
        pendingPause={currentPauseBusy}
        pendingSend={Boolean(currentChat.pendingRequestID) || pendingActions.settings}
        pendingUpload={pendingActions.upload}
        permissionSummary={pendingPermission && !permissionSheetVisible ? toolTarget(pendingPermission.name, pendingPermission.arguments) : undefined}
        planMode={planMode}
        planPending={pendingActions.settings || !sessionID}
        viewMode={viewMode}
      />
    );
  } else if (viewMode === "files" && filePreviewOpen && filePreview) {
    dock = (
      <ActionBar
        actions={[{
          disabled: filePreview.binary || !filePreview.content,
          icon: filePreviewAttached ? "check" : "paperclip",
          label: filePreviewAttached ? "已添加到对话" : "添加到对话",
          onPress: () => {
            if (!filePreviewAttached) attachFilePreview();
            openChat();
          },
          tone: filePreviewAttached ? "secondary" : "primary",
        }]}
        bottomPadding={bottomSafePadding}
      />
    );
  } else if (viewMode === "changeDetail" && (changeDiff || historyDiff)) {
    dock = (
      <ActionBar
        actions={changeDiff ? [
          { disabled: !canOpenSelectedChangeFile, icon: "file-text", label: "打开文件", loading: pendingActions.files, onPress: openSelectedChangeFile, tone: "secondary" },
          {
            disabled: !canRevertSelectedChange,
            icon: "rotate-ccw",
            label: "还原此文件",
            loading: pendingActions.revert,
            onPress: () => {
              if (!changeDiff.expected_hash) {
                revertSelectedChange();
              } else {
                setConfirmRevertFile(true);
              }
            },
            tone: "danger",
          },
        ] : [
          { icon: "rotate-ccw", label: "回滚到这个快照", loading: pendingActions.revert, onPress: () => historyDiff && revertHistoryCheckpoint(historyDiff.checkpoint_id), tone: "danger" },
        ]}
        bottomPadding={bottomSafePadding}
      />
    );
  }

  return (
    <MobileScreenShell
      backgroundLayer={
        viewMode === "chat" ? (
          <ChatWallpaperLayer active={!drawerVisible && !sessionSheetVisible} />
        ) : undefined
      }
      bottomDock={dock}
      bottomSafePadding={dock ? 0 : bottomSafePadding}
      floatingHeader={viewMode === "chat"}
      header={header}
      scrollEnabled={viewMode !== "chat" && viewMode !== "changeDetail"}
      scrollKey={mcpDetailNavKey}
    >
      <AssetPreviewConnectionContext value={previewConnection}>
      <MobileMainContent
        changes={{
          canOpenSelectedChangeFile,
          canRevertSelectedChange,
          changeDiff,
          changes,
          changesClean,
          changesMessage,
          historyCheckpoints,
          historyDiff,
          historyMessage,
          onBackToChanges: openChanges,
          onOpenChange: openChangeFromList,
          onOpenSelectedChangeFile: openSelectedChangeFile,
          onPreviewHistory: previewHistoryCheckpoint,
          onRefreshChanges: requestChanges,
          onRefreshHistory: requestHistory,
          onRevertHistory: revertHistoryCheckpoint,
          onRevertSelectedChange: revertSelectedChange,
          selectedChange,
        }}
        chat={{
          activeAssistantID: currentChat.activeAssistantID,
          assets,
          activePlan: activeSession?.current_plan,
          activitySlot: currentActivitySlot,
          changesCount: changes.length,
          chatScrollRef,
          connected,
          deviceID,
          greetingName: userID.trim() && userID.trim() !== "local" ? userID.trim() : "",
          onAnswerUserQuestion: answerUserQuestion,
          onChangeUserQuestionDraft: setSessionQuestionDraft,
          onExecutePlan: executePlan,
          onNearHeaderChange: setChatNearHeader,
          onOpenChangePath: openChangeByPath,
          onOpenChanges: openChanges,
          onOpenPlan: openPlan,
          onPickSuggestion: pickSuggestion,
          onScrolledChange: setChatScrolled,
          messages: currentChat.messages,
          onInspectSubagent: openInspection,
          onRegenerate: regenerateSession,
          pendingHistorySessionID: pendingHistorySessionIDRef.current,
          pendingPlan: pendingActions.plan,
          pendingQuestions: currentChat.pendingQuestions,
          pendingRequestID: currentChat.pendingRequestID,
          runs: currentRuns,
          sessionID,
          subagentTasks,
          topClearance: topSafePadding + 88,
          workspaceChanges: changes,
        }}
        common={{
          buttonFeedback,
          clientToken,
          pendingActions: { ...pendingActions, context: contextPending },
          viewMode,
        }}
        files={{
          assets,
          changes,
          fileEntries,
          fileParent,
          filePath,
          filePreview,
          previewOpen: filePreviewOpen,
          onGoToParent: goToParent,
          onOpenFileEntry: openFileEntry,
        }}
        knowledge={{
          activeSession,
          aiCandidates: aiMemoryCandidates,
          aiDreamRuns: aiMemoryDreamRuns,
          aiExtractionJobs: aiMemoryExtractionJobs,
          aiMemories,
          aiMemoryMessage,
          categories: knowledgeCategories,
          documentImportStatus,
          documents: documentsByBase[selectedKnowledgeBaseID] || [],
          jobs: jobsByBase[selectedKnowledgeBaseID] || [],
          knowledgeBases,
          message: knowledgeMessage,
          onCreateCategory: createCategory,
          onCreateKnowledgeBase: createKnowledgeBase,
          onDeleteCategory: deleteCategory,
          onDeleteDocument: deleteDocument,
          onDeleteKnowledgeBase: deleteKnowledgeBase,
          onDismissDocumentImport: dismissDocumentImport,
          onInsertReference: insertReference,
          onMoveCategory: moveCategory,
          onReconcileDocumentImport: reconcileDocumentImport,
          onRefresh: () => { requestCatalog(); requestProfiles(); },
          onRefreshDocuments: () => requestDocuments(selectedKnowledgeBaseID),
          onRetryDocument: retryDocument,
          onSearch: searchKnowledge,
          onSelectKnowledgeBase: (knowledgeBaseID) => { setSelectedKnowledgeBaseID(knowledgeBaseID); requestDocuments(knowledgeBaseID); },
          onSetRAG: setRAGSettings,
          onUpdateKnowledgeBase: updateKnowledgeBase,
          onUploadDocument: uploadDocument,
          onCancelUpload: cancelDocumentUpload,
          uploadingDocument,
          onApproveAIMemoryCandidate: approveAIMemoryCandidate,
          onCreateAIMemory: createAIMemory,
          onDeleteAIMemory: deleteAIMemory,
          onRefreshAIMemories: requestAIMemories,
          onRefreshAIMemoryCandidates: requestAIMemoryCandidates,
          onRefreshAIMemoryDreamRuns: requestAIMemoryDreamRuns,
          onRefreshAIMemoryExtractionJobs: requestAIMemoryExtractionJobs,
          onRejectAIMemoryCandidate: rejectAIMemoryCandidate,
          onRestoreAIMemory: restoreAIMemory,
          onRetryAIMemoryExtractionJob: retryAIMemoryExtractionJob,
          onRunAIMemoryDream: runAIMemoryDream,
          onUpdateAIMemory: updateAIMemory,
          profiles: knowledgeProfiles,
          searchResult: knowledgeSearchResult,
        }}
        permission={{
          onAllowPermission: allowPermission,
          onDenyPermission: denyPermission,
          pendingPermission: currentChat.pendingPermission,
          onAnswerUserQuestion: answerUserQuestion,
          onChangeUserQuestionDraft: setSessionQuestionDraft,
          pendingQuestions: currentChat.pendingQuestions,
        }}
        plan={{
          activeSession,
          onExecutePlan: executePlan,
          onOpenChat: openChat,
          sessionID,
        }}
        sessions={{
          canRenameSessions,
          deletedSessions,
          onDeleteSession: deleteSession,
          onRefreshDeletedSessions: requestDeletedSessions,
          onRenameSession: openListRename,
          onRestoreSession: restoreSession,
          onSelectSession: selectSession,
          unreadSessionIDs,
        }}
        settings={{
          activeSection: settingsSection,
          onSelectSection: setSettingsSection,
          activeModel,
          activeSession,
          bindCode,
          compact: sessionCompacts[sessionID],
          connected,
          connectionStatus: status === "Not paired" ? "等待连接" : status,
          pairedRelayURL: pairing.relayURL,
          pairingError,
          pairingPending,
          context: activeContext,
          generation: generationPreferences,
          generationStatus,
          intentConfig,
          intentConfigLoading,
          intentConfigSaving,
          intentConfigTesting,
          intentConfigMessage,
          intentConfigError,
          intentTestResult,
          intentTraces,
          intentTraceDetails,
          intentTraceDetailErrors,
          intentTraceListLoading,
          intentTraceClearing,
          intentTraceDetailLoadingID,
          intentTraceMessage,
          intentTraceError,
          currentModelID,
          modelDeleteFeedback,
          modelMessage,
          modelMessageError,
          deviceID,
          models,
          normalizedRelayURL,
          onBindCodeChange,
          onCloseSettings: openChat,
          onConnect: connect,
          onDeleteSession: deleteSession,
          onLoadSession: selectSession,
          onNewSession: newSession,
          onPair,
          onCompactSession: compactSession,
          onExecutePlan: executePlan,
          onOpenPlan: openPlan,
          onRefreshModels: requestModels,
          onAddModelConfig: addModelConfig,
          onUpdateModelConfig: updateModelConfig,
          onDeleteModelConfig: deleteModelConfig,
          onSetModelEnabled: setModelEnabled,
          onSetDefaultModel: setDefaultModel,
          onClearModelMessage: () => {
            setModelMessage("");
            setModelMessageError(false);
          },
          onTestModelConfig: testModelConfig,
          onRequestIntentConfig: requestIntentConfig,
          onSaveIntentConfig: saveIntentConfig,
          onTestIntentConfig: testIntentConfig,
          onRequestIntentTraces: requestIntentTraces,
          onRequestIntentTraceDetail: requestIntentTraceDetail,
          onClearIntentTraces: clearIntentTraces,
          onClearIntentConfigFeedback: clearIntentConfigFeedback,
          onClearIntentTraceFeedback: clearIntentTraceFeedback,
          onRequestContextInfo: requestContextInfo,
          onRequestGenerationPreferences: requestGenerationPreferences,
          onRefreshSessions: requestSessions,
          onRefreshSkills: requestSkills,
          onReloadSkills: reloadSkills,
          onRefreshPlugins: requestPlugins,
          onReloadPlugins: reloadPlugins,
          mcp: {
            detail: visibleMCPDetail, inventory: mcpInventory, loadMoreDetail: loadMoreMCPDetail,
            openDetail: openMCPDetail, openTool: openMCPTool, refresh: requestMCPInventory,
            reload: reloadMCP, reloading: mcpReloading, reloadMessage: mcpMessage, retryDetail: retryMCPDetail,
            trust: mcpTrust,
          },
          onSetPluginEnabled: setPluginEnabled,
          onRelayURLChange,
          onSetAgentMode: setAgentMode,
          onSetContextWindowK: setContextWindowK,
          onSetGenerationSettings: setGenerationSettings,
          onSetPermissionMode: setPermissionMode,
          onSetStyleInstruction: setStyleInstruction,
          onSwitchModel: switchModel,
          onApplySubagentTask: applySubagentTask,
          onCancelSubagentTask: cancelSubagentTask,
          onCheckSubagentTask: checkSubagentTask,
          onWaitSubagentTask: waitSubagentTask,
          onCreateSubagentDefinition: createSubagentDefinition,
          onDeleteSubagentDefinition: deleteSubagentDefinition,
          onDiscardSubagentTask: discardSubagentTask,
          onFollowupSubagentTask: followupSubagentTask,
          onMessageSubagentTask: messageSubagentTask,
          onResumeSubagentTask: resumeSubagentTask,
          onInspectSubagentTask: openInspection,
          onRefreshSubagents: () => { requestSubagentDefinitions(); requestSubagentTasks(); },
          onUpdateSubagentDefinition: updateSubagentDefinition,
          relayURL,
          sessionID,
          sessions,
          setupVisible,
          skillMessage,
          skillRoot,
          skills,
          plugins,
          pluginMessage,
          pluginRoot,
          subagentDefinitions,
          subagentEvents,
          subagentMessage,
          subagentTasks,
          unreadSessionIDs,
          userID,
        }}
      />

      {/* 2.0 子代理只读实时查看面板（不调用 LoadSession，不改变主会话 ID） */}
      <SubagentInspectionModal
        agentRuns={inspectionAgentRuns}
        assets={inspectionAssets}
        buttonFeedback={buttonFeedback}
        hasMoreMessages={inspectionHasMoreMessages}
        loadingDetail={inspectionLoadingDetail}
        loadingEvents={inspectionLoadingEvents}
        loadingMoreMessages={inspectionLoadingMoreMessages}
        messages={inspectionMessages}
        onClose={closeInspection}
        onLoadMoreMessages={loadMoreInspectionMessages}
        onRefresh={refreshInspection}
        onSelectRun={selectInspectionRun}
        readOnly={inspectionReadOnly}
        runs={inspectionRuns}
        selectedRunID={inspectionSelectedRunID}
        sessionReady={inspectionSessionReady}
        task={inspectedTask}
        taskEvents={inspectionTaskEvents}
        visible={inspectionVisible}
      />
      </AssetPreviewConnectionContext>

      {/* 全局侧栏：新对话、文件、变更、知识库、按日期分组的会话、设置、回收站 */}
      <SideDrawer
        activeSessionID={sessionID}
        busySessionID={chatBusy ? sessionID : undefined}
        buttonFeedback={buttonFeedback}
        changesCount={changes.length}
        connected={connected}
        deletedCount={deletedSessions.length}
        deviceID={deviceID}
        onClose={() => setDrawerVisible(false)}
        onNewSession={startNewSession}
        onOpenSettings={() => {
          setDrawerVisible(false);
          setViewMode("settings");
        }}
        onSelectSession={(targetSessionID) => {
          setDrawerVisible(false);
          setChatScrolled(false);
          setChatNearHeader(false);
          handleLoadSession(targetSessionID);
          openChat();
        }}
        onSelectViewMode={(mode) => {
          setDrawerVisible(false);
          if (mode === "chat") openChat();
          else if (mode === "files") openFiles();
          else if (mode === "changes") openChanges();
          else if (mode === "knowledge") openKnowledge();
          else if (mode === "sessions") openSessions();
          else if (mode === "trash") {
            requestDeletedSessions();
            openTrash();
          } else if (mode === "settings") setViewMode("settings");
        }}
        sessions={sessions}
        unreadSessionIDs={unreadSessionIDs}
        userID={userID}
        viewMode={viewMode}
        visible={drawerVisible}
      />

      {/* 工具调用权限确认：拒绝 / 允许本次 / 本会话始终允许同类操作 */}
      <PermissionSheet
        deviceID={deviceID}
        onAllow={() => {
          setMinimizedPermissionID("");
          allowPermission();
        }}
        onAllowSession={pendingPermission?.requireFreshConsent ? undefined : () => {
          setMinimizedPermissionID("");
          allowPermissionSession();
        }}
        onDeny={() => {
          setMinimizedPermissionID("");
          denyPermission();
        }}
        onMinimize={() => setMinimizedPermissionID(pendingPermission?.approvalID || "")}
        onOpenPolicy={() => {
          setMinimizedPermissionID(pendingPermission?.approvalID || "");
          sessionFromPermissionRef.current = true;
          setSessionSheetContextFirst(false);
          setSessionSheetVisible(true);
        }}
        permission={pendingPermission}
        policyLabel={activePermissionLabel}
        visible={permissionSheetVisible}
      />

      <ConfirmModal
        confirmLabel="还原文件"
        danger
        icon="rotate-ccw"
        message={`确定将「${changeDiff?.path || selectedChange}」恢复为修改前的基线版本吗？未保存内容将被覆盖。`}
        onCancel={() => setConfirmRevertFile(false)}
        onConfirm={() => {
          setConfirmRevertFile(false);
          executeRevertSelectedChange();
        }}
        pending={pendingActions.revert}
        title="确认还原文件"
        visible={confirmRevertFile}
      />

      <ResponsiveFormModal
        buttonFeedback={buttonFeedback}
        footer={null}
        onClose={() => closeRename("list")}
        title="重命名对话"
        visible={Boolean(listRenameDraft)}
      >
        {listRenameDraft ? (
          <SessionRenameEditor
            connected={canRenameSessions}
            draft={listRenameDraft}
            onCancel={() => closeRename("list")}
            onChange={editRename}
            onSave={saveRename}
          />
        ) : null}
      </ResponsiveFormModal>

      {/* 会话设置：模型、对话/规划、工具权限、上下文 */}
      <SessionSheet
        activeSession={activeSession}
        connected={connected}
        connectionLabel={connectionLabel}
        context={activeContext}
        contextFirst={sessionSheetContextFirst}
        currentModelID={currentModelID}
        models={models}
        onClose={closeSessionSheet}
        onCompact={compactSession}
        onReconnect={connect}
        onRefreshUsage={requestSessions}
        onRename={activeSession && !activeSession.deleted ? openSheetRename : undefined}
        canRename={canRenameSessions}
        onRequestContext={requestContextInfo}
        onSetAgentMode={setAgentMode}
        onSetPermissionMode={setPermissionMode}
        onSwitchModel={switchModel}
        pendingContext={contextPending}
        pendingModels={pendingActions.models}
        pendingSettings={pendingActions.settings}
        renameEditor={sheetRenameDraft ? (
          <SessionRenameEditor
            connected={canRenameSessions}
            draft={sheetRenameDraft}
            onCancel={() => closeRename("sheet")}
            onChange={editRename}
            onSave={saveRename}
          />
        ) : null}
        sessionID={sessionID}
        visible={sessionSheetVisible}
      />
    </MobileScreenShell>
  );
}

