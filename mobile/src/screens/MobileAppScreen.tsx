import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { AppState, BackHandler, Platform, type AppStateStatus, type ScrollView } from "react-native";

import { ActionBar } from "../components/layout/ActionBar";
import { AppHeader, describeConnection } from "../components/layout/AppHeader";
import { HeaderIconButton, ScreenHeader } from "../components/layout/ScreenHeader";
import { PermissionSheet } from "../components/permissions/PermissionSheet";
import { SessionSheet } from "../components/session/SessionSheet";
import { settingsSectionTitles, type SettingsSection } from "../components/settings/SettingsHome";
import { PairingScreen } from "../components/startup/PairingScreen";
import { BottomDock } from "../components/layout/BottomDock";
import { SideDrawer } from "../components/navigation/SideDrawer";
import { useAndroidNavigationBar } from "../hooks/useAndroidNavigationBar";
import { useAssetState } from "../hooks/useAssetState";
import { useAgentRunState } from "../hooks/useAgentRunState";
import { useChangeHistoryActions } from "../hooks/useChangeHistoryActions";
import { useChangeHistoryState } from "../hooks/useChangeHistoryState";
import { useChatActions } from "../hooks/useChatActions";
import { useChatMessages } from "../hooks/useChatMessages";
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
import { useRelayConnection } from "../hooks/useRelayConnection";
import {
  stopRelayForegroundService,
} from "../native/relayForegroundService";
import { useRemoteResultAppliers } from "../hooks/useRemoteResultAppliers";
import { useRemoteMessageHandler } from "../hooks/useRemoteMessageHandler";
import { useRemoteRuntimeRefs } from "../hooks/useRemoteRuntimeRefs";
import { useRelaySender } from "../hooks/useRelaySender";
import { useRemoteRequests } from "../hooks/useRemoteRequests";
import { useSessionModelActions } from "../hooks/useSessionModelActions";
import { useSessionModelState } from "../hooks/useSessionModelState";
import { useSessionSettingsActions } from "../hooks/useSessionSettingsActions";
import { useSessionGenerationState } from "../hooks/useSessionGenerationState";
import { useSkillState } from "../hooks/useSkillState";
import { useSubagentActions } from "../hooks/useSubagentActions";
import { useSubagentInspection } from "../hooks/useSubagentInspection";
import { useSubagentState } from "../hooks/useSubagentState";
import { useIntentSettings } from "../hooks/useIntentSettings";
import { useMCPState } from "../hooks/useMCPState";
import { SubagentInspectionModal } from "../components/subagents/SubagentInspectionModal";
import { MobileMainContent } from "./MobileMainContent";
import { MobileScreenShell } from "./MobileScreenShell";
import { buttonFeedback } from "../utils/buttonFeedback";
import { historyMessageToChatItem } from "../utils/chatHistory";
import { newRequestID } from "../utils/ids";
import { formatBytes } from "../utils/format";
import { modelDisplayName, permissionLabel } from "../utils/session";
import { baseName, dirName, toolTarget } from "../utils/toolDisplay";

export function MobileAppScreen() {
  useAndroidNavigationBar();

  // 页面本身只充当 composition root：状态、协议和业务动作分别由专用 Hook 管理。
  const {
    bindCode,
    connected,
    sessionID,
    setBindCode,
    setConnected,
    setSessionID,
    setStatus,
    setViewMode,
    status,
    viewMode,
  } = useMobileUiState();
  const handleTokenRestored = useCallback(() => setStatus("Paired"), [setStatus]);
  const {
    assetBaseURL,
    clientToken,
    deviceID,
    relayURL,
    setAssetBaseURL,
    setClientToken,
    setDeviceID,
    setRelayURL,
    setUserID,
    settingsLoaded,
    userID,
  } = useMobileSettings({ onTokenRestored: handleTokenRestored });
  const {
    clearModels,
    clearSessions,
    currentModelID,
    deletedSessions,
    modelMessage,
    modelMessageError,
    models,
    sessionCompacts,
    sessionContexts,
    sessions,
    setCurrentModelID,
    setDeletedSessions,
    setModelMessage,
    setModelMessageError,
    setModels,
    setSessionCompact,
    setSessionContext,
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
    applyMCPError,
    applyMCPReload,
    mcpMessage,
  } = useMCPState();
  const {
    assets,
    clearAssets,
    setAssets,
  } = useAssetState();
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
  } = useFileState();
  const {
    addMessage,
    addToolCall,
    addToolResult,
    appendMessages,
    appendAssistant,
    clearSessionPendingRequest,
    removeSessionPendingQuestion,
    completeAssistant,
    getSessionChat,
    hasPendingRequest,
    markAssistantError,
    markUserMessageQueued,
    markUserMessageDelivered,
    mergeSessionChats,
    replaceMessages,
    resetActiveAssistant,
    removePendingQuestions,
    sessionChatsVersion,
    sessionChats,
    setSessionLastUsage,
    setSessionPendingPermission,
    setSessionPendingQuestion,
    setSessionPendingRequest,
    startBackgroundAssistant,
  } = useChatMessages();
  const [unreadSessionIDs, setUnreadSessionIDs] = useState<Record<string, boolean>>({});
  const [drawerVisible, setDrawerVisible] = useState(false);
  // 新版布局的页面级状态：会话设置弹层、对话是否滚动（顶栏分隔线）、权限确认是否收起、设置详情页、文件预览
  const [sessionSheetVisible, setSessionSheetVisible] = useState(false);
  const [chatScrolled, setChatScrolled] = useState(false);
  const [minimizedPermissionID, setMinimizedPermissionID] = useState("");
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
  const appActiveRef = useRef(AppState.currentState === "active");
  const reconnectOnForegroundRef = useRef(false);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptRef = useRef(0);
  const connectRef = useRef<() => void>(() => undefined);
  // 远程运行时引用不触发重渲染，用来关联 WebSocket 请求、当前 Session 和流式回答。
  const {
    activeRequestIDRef,
    historySessionIDRef,
    pendingHistorySessionIDRef,
    requestSessionMapRef,
    sessionIDRef,
    socketRef,
  } = useRemoteRuntimeRefs(sessionID);

  const normalizedRelayURL = useNormalizedRelayUrl(relayURL);
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
  void sessionChatsVersion;
  void agentRunsVersion;
  const currentPauseBusy = Boolean(pendingActions.pause);
  const { bottomSafePadding, topSafePadding } = useMobileLayoutMetrics();
  const questionSyncKeyRef = useRef("");

  // 从这里开始组装传输能力：Sender 只发送，Requests/Actions 表达命令，Handler 消费响应。
  const addRelayErrorMessage = useCallback(
    (message: string) => addMessage(sessionID, "error", message),
    [addMessage, sessionID],
  );
  const sendEnvelope = useRelaySender({
    activeRequestIDRef,
    addErrorMessage: addRelayErrorMessage,
    clientToken,
    deviceID,
    sessionID,
    socketRef,
    userID,
  });

  useEffect(() => {
    if (!connected || !clientToken) {
      questionSyncKeyRef.current = "";
      return;
    }
    const syncKey = `${clientToken}:${deviceID}`;
    if (questionSyncKeyRef.current === syncKey) {
      return;
    }
    questionSyncKeyRef.current = syncKey;
    const knownQuestionIDs = Object.values(sessionChats).flatMap((chat) => chat.pendingQuestions.map((question) => question.questionID));
    sendEnvelope("user_question_list", { request_id: newRequestID(), payload: { known_question_ids: knownQuestionIDs } });
  }, [clientToken, connected, deviceID, sendEnvelope, sessionChats]);

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
    clearSessionPendingRequest,
    historySessionIDRef,
    onError: setSubagentMessage,
    onResumeStarted: () => setViewMode("chat"),
    requestSessionMapRef,
    resetActiveAssistant,
    sendEnvelope,
    sessionID,
    setSessionLastUsage,
    setSessionPendingPermission,
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

  const replaceHistoryMessages = useCallback(
    (targetSessionID: string, messages: Parameters<typeof historyMessageToChatItem>[0][]) => {
      replaceMessages(targetSessionID, messages.map(historyMessageToChatItem));
    },
    [replaceMessages],
  );
  const hasSessionMessages = useCallback(
    (targetSessionID: string) => getSessionChat(targetSessionID).messages.length > 0,
    [getSessionChat],
  );

  const {
    refreshRemoteState,
    requestAssets,
    requestChanges,
    requestFiles,
    requestHistory,
    requestModels,
    requestPlugins,
    reloadPlugins,
    reloadMCP,
    reloadSkills,
    requestDeletedSessions,
    requestSkills,
    requestSessionHistoryDelta,
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
    currentSessionID: sessionID,
    pendingHistorySessionIDRef,
    replaceHistoryMessages,
    requestSubagentTasks,
    sendEnvelope,
    startPending,
    stopPending,
  });
  const { addModelConfig, deleteModelConfig, deleteSession, loadSession, newSession, restoreSession, setDefaultModel, setModelEnabled, switchModel, testModelConfig, updateModelConfig } = useSessionModelActions({
    activeRequestIDRef,
    currentModelID,
    historySessionIDRef,
    pendingHistorySessionIDRef,
    requestAssets,
    requestSessionHistory,
    resetActiveAssistant,
    sendEnvelope,
    sessionIDRef,
    sessions,
    setSessionLastUsage,
    setSessionPendingPermission,
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
    uploadDocument,
  } = useKnowledgeActions({
    activeKnowledgeBase,
    assetBaseURL,
    clientToken,
    onError: (message) => addMessage(sessionID, "error", message),
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
    requestContextInfo,
    requestGenerationPreferences,
    setAgentMode,
    setContextWindowK,
    setGenerationSettings,
    setPermissionMode,
    setStyleInstruction,
  } = useSessionSettingsActions({
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
  });
  const {
    attachFilePreview,
    goToParent,
    openFileEntry,
    openSelectedChangeFile,
    refreshCurrentFiles,
    removeAttachedFile,
    sendMessageWithFiles,
    uploadLocalFile,
  } = useFileActions({
    addErrorMessage: (message) => addMessage(sessionID, "error", message),
    assetBaseURL,
    attachedFiles,
    changeDiffPath: changeDiff?.path,
    fileParent,
    filePreview,
    requestFiles,
    selectedChange,
    sendEnvelope,
    sessionID,
    setAttachedFiles,
    setFilePreview,
    setMessageInput,
    setViewMode,
    startPending,
    stopPending,
  });
  const {
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

  const { pairDevice } = usePairingActions({
    addErrorMessage: (message) => addMessage(sessionID, "error", message),
    bindCode,
    normalizedRelayURL,
    setBindCode,
    setClientToken,
    setDeviceID,
    setStatus,
    setUserID,
    startPending,
    stopPending,
  });
  const { answerUserQuestion, allowPermission, denyPermission, pauseSession, regenerateSession, sendUserMessage } = useChatActions({
    activeRequestIDRef,
    addEventMessage: (targetSessionID, message) => addMessage(targetSessionID, "event", message),
    addUserMessage: (targetSessionID, message, requestID, attachments) => addMessage(targetSessionID, "user", message, requestID, attachments),
    attachedFiles,
    historySessionIDRef,
    messageInput,
    pendingPermission: currentChat.pendingPermission,
    requestSessionMapRef,
    resetActiveAssistant,
    sendEnvelope,
    sendMessageWithFiles,
    sessionID,
    clearSessionPendingRequest,
    setSessionLastUsage,
    setSessionPendingPermission,
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
    applySessionHistoryDelta,
    applySessionHistoryMeta,
    applySessionHistory,
    applySessionList,
    applySessionSettings,
    applySessionGenerationPreferences,
  } = useRemoteResultAppliers({
    addEventMessage: (targetSessionID, message) => addMessage(targetSessionID, "event", message),
    appendMessages,
    filePath,
    hasPendingRequest,
    hasSessionMessages,
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
    requestSubagentTasks,
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
    setSessionPendingPermission,
    setDeletedSessions,
    setSessions,
    setViewMode,
  });

  const handleRemoteMessage = useRemoteMessageHandler({
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
    isModelOperationPending: pendingActions.models,
    applyModelSwitch,
    applySkillList,
    applyPluginList,
    applyMCPReload,
    applyMCPError,
    isMCPOperationPending: pendingActions.mcp,
    applySubagentDefinitionList,
    applySubagentDefinitionMutation,
    applySubagentTaskList,
    applySubagentTaskResult,
    applyRealtimeSubagentTaskEvent,
    applySubagentTaskDetailResult,
    applySubagentTaskEventsResult,
    applySessionChanged,
    applySessionHistoryDelta,
    applySessionHistoryMeta,
    applySessionHistory,
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
    historySessionIDRef,
    pendingHistorySessionIDRef,
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
    removePendingQuestions,
    setSessionID,
    setStatus,
    stopPending,
  });

  const refreshAllRemoteState = useCallback(() => {
    refreshRemoteState();
    if (sessionID) {
      requestAgentRuns(sessionID);
      requestSessionHistory(sessionID);
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
  }, [inspectedTaskID, refreshInspection, refreshRemoteState, requestAIMemories, requestAIMemoryCandidates, requestAIMemoryDreamRuns, requestAIMemoryExtractionJobs, requestAgentRuns, requestCatalog, requestIntentConfig, requestProfiles, requestSessionHistory, requestSubagentDefinitions, sessionID]);

  const scheduleReconnect = useCallback(() => {
    if (!clientToken) {
      return;
    }
    // Android's native Foreground Service owns the socket and its reconnect loop.
    // Do not create a second JS connection when its state changes in the background.
    if (Platform.OS === "android") {
      setStatus("Reconnecting");
      return;
    }
    if (!appActiveRef.current) {
      reconnectOnForegroundRef.current = true;
      return;
    }
    if (reconnectTimerRef.current) {
      return;
    }

    const attempt = reconnectAttemptRef.current;
    reconnectAttemptRef.current = Math.min(attempt + 1, 6);
    const delay = Math.min(30000, 500 * 2 ** attempt);
    setStatus(attempt === 0 ? "Reconnecting" : `Reconnecting (${attempt + 1})`);
    reconnectTimerRef.current = setTimeout(() => {
      reconnectTimerRef.current = null;
      if ((!appActiveRef.current && Platform.OS !== "android") || !clientToken) {
        reconnectOnForegroundRef.current = Boolean(clientToken);
        return;
      }
      connectRef.current();
    }, delay);
  }, [clientToken, setStatus]);

  const handleConnected = useCallback(() => {
    reconnectAttemptRef.current = 0;
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    refreshAllRemoteState();
  }, [refreshAllRemoteState]);

  const connect = useRelayConnection({
    addErrorMessage: (message) => addMessage(sessionID, "error", message),
    clientToken,
    deviceID,
    normalizedRelayURL,
    onConnected: handleConnected,
    onDisconnected: scheduleReconnect,
    onMessage: handleRemoteMessage,
    setConnected,
    setStatus,
    socketRef,
    startPending,
    stopPending,
    userID,
  });

  useEffect(() => {
    connectRef.current = connect;
  }, [connect]);

  useEffect(() => {
    if (!settingsLoaded || !clientToken || AppState.currentState !== "active") {
      return;
    }

    const timer = setTimeout(() => {
      if (!socketRef.current) {
        connectRef.current();
      }
    }, 300);

    return () => clearTimeout(timer);
  }, [clientToken, deviceID, normalizedRelayURL, settingsLoaded, userID]);

  useEffect(() => {
    appActiveRef.current = AppState.currentState === "active";
    let previousState: AppStateStatus = AppState.currentState;

    const subscription = AppState.addEventListener("change", (nextState) => {
      const enteringBackground = nextState === "background" || nextState === "inactive";
      const returningToForeground =
        (previousState === "background" || previousState === "inactive") && nextState === "active";

      appActiveRef.current = nextState === "active";

      if (enteringBackground) {
        // Android/iOS may suspend or close the socket while the app is backgrounded.
        // Keep the retry intent even if the close event races with this lifecycle event.
        reconnectOnForegroundRef.current = reconnectOnForegroundRef.current || Boolean(clientToken);
      }

      if (returningToForeground && reconnectOnForegroundRef.current && clientToken) {
        reconnectOnForegroundRef.current = false;
        // 如果当前已有正在连接或已经处于 OPEN 状态的 Socket，无需重复发起重连
        const currentSocket = socketRef.current;
        if (currentSocket && (currentSocket.readyState === WebSocket.OPEN || currentSocket.readyState === WebSocket.CONNECTING)) {
          return;
        }
        if (reconnectTimerRef.current) {
          clearTimeout(reconnectTimerRef.current);
        }
        setStatus("Restoring connection");
        // Let the OS restore network interfaces before opening the replacement socket.
        reconnectTimerRef.current = setTimeout(() => {
          reconnectTimerRef.current = null;
          connectRef.current();
        }, 300);
      }

      previousState = nextState;
    });

    return () => {
      subscription.remove();
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
    };
  }, [clientToken]);

  useEffect(() => {
    if (!settingsLoaded) {
      return;
    }
    if (clientToken && normalizedRelayURL && userID.trim() && deviceID.trim()) {
      return;
    }
    void stopRelayForegroundService().catch((error) => {
      console.warn("Relay foreground service stop failed", error);
    });
  }, [clientToken, deviceID, normalizedRelayURL, settingsLoaded, userID]);

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

  const pendingPermission = currentChat.pendingPermission;
  const permissionSheetVisible = Boolean(pendingPermission) && minimizedPermissionID !== pendingPermission?.requestID;
  const sessionRunning = currentRuns.some(({ run }) => run.status === "running");
  const chatBusy = Boolean(currentChat.pendingRequestID) || sessionRunning;
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
      if (!settingsLoaded || !clientToken) return false;
      if (viewMode === "files" && filePreviewOpen) {
        setFilePreviewOpen(false);
        return true;
      }
      if (viewMode === "settings" && settingsSection) {
        setSettingsSection(null);
        return true;
      }
      if (viewMode === "changeDetail") {
        openChanges();
        return true;
      }
      if (viewMode !== "chat") {
        openChat();
        return true;
      }
      return false;
    });
    return () => subscription.remove();
  }, [clientToken, filePreviewOpen, openChanges, openChat, settingsLoaded, settingsSection, viewMode]);

  const openSessionSheet = useCallback(() => {
    sessionFromPermissionRef.current = false;
    setSessionSheetVisible(true);
  }, []);
  const closeSessionSheet = useCallback(() => {
    setSessionSheetVisible(false);
    // 从权限确认里点「修改策略」进来的，关掉后回到权限确认
    if (sessionFromPermissionRef.current) {
      sessionFromPermissionRef.current = false;
      setMinimizedPermissionID("");
    }
  }, []);
  const startNewSession = useCallback(() => {
    setChatScrolled(false);
    setDrawerVisible(false);
    setSessionSheetVisible(false);
    newSession();
    openChat();
  }, [newSession, openChat]);
  const pickSuggestion = useCallback((text: string) => setMessageInput(text), [setMessageInput]);

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

  // 未配对：全屏配对页。配对成功写入 client_token 后，上面的自动连接副作用会接着发起连接。
  if (settingsLoaded && !clientToken) {
    return (
      <PairingScreen
        bindCode={bindCode}
        bottomInset={bottomSafePadding}
        failed={/pair failed/i.test(status)}
        normalizedRelayURL={normalizedRelayURL}
        onBindCodeChange={setBindCode}
        onPair={pairDevice}
        onRelayURLChange={setRelayURL}
        pending={pendingActions.pair}
        relayURL={relayURL}
        topInset={topSafePadding}
      />
    );
  }

  const refreshButton = (onPress: () => void, busy: boolean) => (
    <HeaderIconButton accessibilityLabel="刷新" disabled={busy} icon="refresh-cw" muted onPress={onPress} size={19} />
  );
  let header: ReactNode;
  if (viewMode === "chat") {
    header = (
      <AppHeader
        busy={chatBusy}
        connected={connected}
        deviceID={deviceID}
        onNewSession={startNewSession}
        onOpenMenu={() => setDrawerVisible(true)}
        onOpenSession={openSessionSheet}
        planMode={planMode}
        scrolled={chatScrolled}
        status={status}
        title={activeSession?.title?.trim() || "新对话"}
        topInset={topSafePadding}
        waiting={Boolean(pendingPermission)}
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
      onBack = openChanges;
    } else if (viewMode === "sessions") {
      title = "全部对话";
      right = refreshButton(() => { requestSessions(); requestDeletedSessions(); }, pendingActions.sessions);
    } else if (viewMode === "knowledge") {
      title = "知识库与记忆";
      right = refreshButton(() => { requestCatalog(); requestProfiles(); }, pendingActions.knowledge);
    } else if (viewMode === "plan") {
      title = "计划";
    } else if (settingsSection) {
      title = settingsSectionTitles[settingsSection];
      onBack = () => setSettingsSection(null);
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
          { disabled: !canRevertSelectedChange, icon: "rotate-ccw", label: "还原此文件", loading: pendingActions.revert, onPress: revertSelectedChange, tone: "danger" },
        ] : [
          { icon: "rotate-ccw", label: "回滚到这个快照", loading: pendingActions.revert, onPress: () => historyDiff && revertHistoryCheckpoint(historyDiff.checkpoint_id), tone: "danger" },
        ]}
        bottomPadding={bottomSafePadding}
      />
    );
  }

  return (
    <MobileScreenShell
      bottomDock={dock}
      bottomSafePadding={dock ? 0 : bottomSafePadding}
      header={header}
      scrollEnabled={viewMode !== "chat"}
    >
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
          onOpenChange: openChangeEntry,
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
          chatScrollRef,
          connected,
          deviceID,
          greetingName: userID.trim() && userID.trim() !== "local" ? userID.trim() : "",
          onOpenChanges: openChanges,
          onPickSuggestion: pickSuggestion,
          onScrolledChange: setChatScrolled,
          messages: currentChat.messages,
          onInspectSubagent: openInspection,
          onRegenerate: regenerateSession,
          pendingHistorySessionID: pendingHistorySessionIDRef.current,
          pendingRequestID: currentChat.pendingRequestID,
          runs: currentRuns,
          sessionID,
          subagentTasks,
        }}
        common={{
          buttonFeedback,
          clientToken,
          pendingActions,
          viewMode,
        }}
        files={{
          assets,
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
          documents: documentsByBase[selectedKnowledgeBaseID] || [],
          jobs: jobsByBase[selectedKnowledgeBaseID] || [],
          knowledgeBases,
          message: knowledgeMessage,
          onCreateCategory: createCategory,
          onCreateKnowledgeBase: createKnowledgeBase,
          onDeleteCategory: deleteCategory,
          onDeleteDocument: deleteDocument,
          onDeleteKnowledgeBase: deleteKnowledgeBase,
          onMoveCategory: moveCategory,
          onRefresh: () => { requestCatalog(); requestProfiles(); },
          onRefreshDocuments: () => requestDocuments(selectedKnowledgeBaseID),
          onRetryDocument: retryDocument,
          onSearch: searchKnowledge,
          onSelectKnowledgeBase: (knowledgeBaseID) => { setSelectedKnowledgeBaseID(knowledgeBaseID); requestDocuments(knowledgeBaseID); },
          onSetRAG: setRAGSettings,
          onUpdateKnowledgeBase: updateKnowledgeBase,
          onUploadDocument: uploadDocument,
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
          pendingQuestions: currentChat.pendingQuestions,
        }}
        plan={{
          activeSession,
          onExecutePlan: executePlan,
          onOpenChat: openChat,
          sessionID,
        }}
        sessions={{
          deletedSessions,
          onDeleteSession: deleteSession,
          onRefreshDeletedSessions: requestDeletedSessions,
          onRestoreSession: restoreSession,
          onSelectSession: selectSession,
          unreadSessionIDs,
        }}
        settings={{
          activeSection: settingsSection,
          onSelectSection: setSettingsSection,
          activeModel,
          activeSession,
          assetBaseURL,
          bindCode,
          compact: sessionCompacts[sessionID],
          connected,
          context: sessionContexts[sessionID],
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
          modelMessage,
          modelMessageError,
          deviceID,
          models,
          normalizedRelayURL,
          onBindCodeChange: setBindCode,
          onCloseSettings: openChat,
          onConnect: connect,
          onDeleteSession: deleteSession,
          onDeviceIDChange: setDeviceID,
          onLoadSession: selectSession,
          onNewSession: newSession,
          onPair: pairDevice,
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
          reloadMCP,
          mcpReloading: pendingActions.mcp,
          mcpMessage,
          onSetPluginEnabled: setPluginEnabled,
          onAssetBaseURLChange: setAssetBaseURL,
          onRelayURLChange: setRelayURL,
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
          onUserIDChange: setUserID,
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
        visible={Boolean(inspectedTaskID)}
      />

      {/* 全局侧栏：新对话、文件、变更、知识库、按日期分组的会话、设置 */}
      <SideDrawer
        activeSessionID={sessionID}
        busySessionID={chatBusy ? sessionID : undefined}
        buttonFeedback={buttonFeedback}
        changesCount={changes.length}
        connected={connected}
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
          else if (mode === "settings") setViewMode("settings");
        }}
        sessions={sessions}
        unreadSessionIDs={unreadSessionIDs}
        userID={userID}
        viewMode={viewMode}
        visible={drawerVisible}
      />

      {/* 工具调用权限确认：点遮罩收成输入框上方的提示条，请求保持待确认 */}
      <PermissionSheet
        deviceID={deviceID}
        onAllow={() => {
          setMinimizedPermissionID("");
          allowPermission();
        }}
        onDeny={() => {
          setMinimizedPermissionID("");
          denyPermission();
        }}
        onMinimize={() => setMinimizedPermissionID(pendingPermission?.requestID || "")}
        onOpenPolicy={() => {
          setMinimizedPermissionID(pendingPermission?.requestID || "");
          sessionFromPermissionRef.current = true;
          setSessionSheetVisible(true);
        }}
        permission={pendingPermission}
        policyLabel={activePermissionLabel}
        visible={permissionSheetVisible}
      />

      {/* 会话设置：模型、对话/规划、工具权限、上下文 */}
      <SessionSheet
        activeSession={activeSession}
        connected={connected}
        connectionLabel={connectionLabel}
        context={sessionContexts[sessionID]}
        currentModelID={currentModelID}
        models={models}
        onClose={closeSessionSheet}
        onCompact={compactSession}
        onReconnect={connect}
        onRequestContext={requestContextInfo}
        onSetAgentMode={setAgentMode}
        onSetPermissionMode={setPermissionMode}
        onSwitchModel={switchModel}
        pendingModels={pendingActions.models}
        pendingSettings={pendingActions.settings}
        visible={sessionSheetVisible}
      />
    </MobileScreenShell>
  );
}

