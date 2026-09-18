import { useCallback, useEffect, useRef } from "react";
import { AppState, Platform, type AppStateStatus, type ScrollView } from "react-native";

import { AppHeader } from "../components/layout/AppHeader";
import { BottomDock } from "../components/layout/BottomDock";
import { SessionModeSwitch } from "../components/session/SessionModeSwitch";
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
import { useSubagentState } from "../hooks/useSubagentState";
import { MobileMainContent } from "./MobileMainContent";
import { MobileScreenShell } from "./MobileScreenShell";
import { buttonFeedback } from "../utils/buttonFeedback";
import { historyMessageToChatItem } from "../utils/chatHistory";
import { agentActivity } from "../utils/agentActivity";

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
  } = useSubagentState();
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
    completeAssistant,
    getSessionChat,
    hasPendingRequest,
    markAssistantError,
    mergeSessionChats,
    replaceMessages,
    resetActiveAssistant,
    sessionChatsVersion,
    setSessionLastUsage,
    setSessionPendingPermission,
    setSessionPendingRequest,
  } = useChatMessages();
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
  const { isBusy, pendingActions, startPending, stopPending } = usePendingActions();

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
  const headerActivity = agentActivity({
    connected,
    messages: currentChat.messages,
    pendingPermission: Boolean(currentChat.pendingPermission),
    pendingRequestID: currentChat.pendingRequestID,
    runs: currentRuns,
    uploading: pendingActions.upload,
  });
  void sessionChatsVersion;
  void agentRunsVersion;
  const currentSessionBusy = Boolean(currentChat.pendingRequestID);
  const currentPauseBusy = Boolean(pendingActions.pause);
  const uiBusy = isBusy || currentSessionBusy;
  const { bottomSafePadding, topSafePadding } = useMobileLayoutMetrics();
  // 从这里开始组装传输能力：Sender 只发送，Requests/Actions 表达命令，Handler 消费响应。
  const sendEnvelope = useRelaySender({
    activeRequestIDRef,
    addErrorMessage: (message) => addMessage(sessionID, "error", message),
    clientToken,
    deviceID,
    sessionID,
    socketRef,
    userID,
  });
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

  const { setPluginEnabled } = usePluginActions({
    clientToken,
    onError: (message) => addMessage(sessionID, "error", message),
    sendEnvelope,
    startPending,
    stopPending,
  });

  const {
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
    requestSessionHistoryDelta,
    requestSessionHistoryFull,
    requestSessionHistory,
    requestSessions,
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
    replaceHistoryMessages: (targetSessionID, messages) => replaceMessages(targetSessionID, messages.map(historyMessageToChatItem)),
    sendEnvelope,
    startPending,
    stopPending,
  });
  const { addModelConfig, deleteModelConfig, deleteSession, loadSession, newSession, restoreSession, setDefaultModel, setModelEnabled, switchModel, testModelConfig, updateModelConfig } = useSessionModelActions({
    activeRequestIDRef,
    currentModelID,
    pendingHistorySessionIDRef,
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
  const { allowPermission, denyPermission, pauseSession, regenerateSession, sendUserMessage } = useChatActions({
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
    applySubagentDefinitionList,
    applySubagentDefinitionMutation,
    applySubagentTaskList,
    applySubagentTaskResult,
    applySessionChanged,
    applySessionHistoryDelta,
    applySessionHistoryMeta,
    applySessionHistory,
    applySessionList,
    applySessionSettings,
    applySessionGenerationPreferences,
    applySessionGenerationError: setSessionGenerationError,
    clearSessionPendingRequest,
    completeAssistant,
    currentFilePath: filePath,
    getSessionChat,
    historySessionIDRef,
    hasRunForRequest,
    isKnowledgeOperationPending: pendingActions.knowledge || (viewMode === "knowledge" && pendingActions.settings),
    isGenerationOperationPending: pendingActions.generation,
    isMemoryOperationPending: pendingActions.memory,
    markAssistantError,
    mergeSessionChats,
    mergeSessionRuns,
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
    setSessionID,
    setStatus,
    stopPending,
  });

  const refreshAllRemoteState = useCallback(() => {
    refreshRemoteState();
    requestCatalog();
    requestProfiles();
    requestAIMemories();
    requestAIMemoryCandidates();
    requestAIMemoryExtractionJobs();
    requestAIMemoryDreamRuns();
    requestSubagentDefinitions();
    requestSubagentTasks();
  }, [refreshRemoteState, requestAIMemories, requestAIMemoryCandidates, requestAIMemoryDreamRuns, requestAIMemoryExtractionJobs, requestCatalog, requestProfiles, requestSubagentDefinitions, requestSubagentTasks]);

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
  const {
    openChanges,
    openChat,
    openFiles,
    openKnowledge,
    openPlan,
    openSessions,
    selectSession,
    toggleSettings,
  } = useNavigationActions({
    fileEntriesCount: fileEntries.length,
    filePath,
    loadSession,
    requestAssets,
    requestChanges,
    requestFiles,
    requestKnowledge: requestCatalog,
    requestSessions,
    setViewMode,
  });
  return (
    <MobileScreenShell
      bottomDock={
        <BottomDock
          attachedFiles={attachedFiles}
          bottomPadding={bottomSafePadding}
          buttonFeedback={buttonFeedback}
          changesActive={changesTabActive}
          messageInput={messageInput}
          onChangeMessage={setMessageInput}
          onChangesPress={openChanges}
          onChatPress={openChat}
          onFilesPress={openFiles}
          onPause={pauseSession}
          onRemoveAttachedFile={removeAttachedFile}
          onSend={sendUserMessage}
          onKnowledgePress={openKnowledge}
          onSettingsPress={toggleSettings}
          onUploadFile={uploadLocalFile}
          pendingPause={currentPauseBusy}
          pendingSend={Boolean(currentChat.pendingRequestID)}
          pendingSettings={pendingActions.settings}
          pendingUpload={pendingActions.upload}
          viewMode={viewMode}
        />
      }
      bottomSafePadding={bottomSafePadding}
      scrollEnabled={viewMode !== "chat"}
      topSafePadding={topSafePadding}
    >
      <AppHeader
        activity={headerActivity}
        buttonFeedback={buttonFeedback}
        connected={connected}
        deviceID={deviceID}
        isBusy={uiBusy}
        onToggleSettings={toggleSettings}
        status={status}
        userID={userID}
        viewMode={viewMode}
      />

      {viewMode === "chat" ? (
        <SessionModeSwitch
          buttonFeedback={buttonFeedback}
          disabled={!connected || !sessionID}
          mode={activeSession?.agent_mode === "plan" ? "plan" : "chat"}
          onChange={setAgentMode}
          pending={pendingActions.settings}
        />
      ) : null}

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
          messages: currentChat.messages,
          onRegenerate: regenerateSession,
          pendingHistorySessionID: pendingHistorySessionIDRef.current,
          pendingRequestID: currentChat.pendingRequestID,
          runs: currentRuns,
          sessionID,
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
          filePreviewAttached,
          onAttachFilePreview: attachFilePreview,
          onGoToParent: goToParent,
          onOpenFileEntry: openFileEntry,
          onRefreshAssets: () => requestAssets(),
          onRefreshFiles: refreshCurrentFiles,
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
        }}
        settings={{
          activeModel,
          activeSession,
          assetBaseURL,
          bindCode,
          compact: sessionCompacts[sessionID],
          connected,
          context: sessionContexts[sessionID],
          generation: generationPreferences,
          generationStatus,
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
          onLoadSession: loadSession,
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
          onRequestContextInfo: requestContextInfo,
          onRequestGenerationPreferences: requestGenerationPreferences,
          onRefreshSessions: requestSessions,
          onRefreshSkills: requestSkills,
          onReloadSkills: reloadSkills,
          onRefreshPlugins: requestPlugins,
          onReloadPlugins: reloadPlugins,
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
          userID,
        }}
      />
    </MobileScreenShell>
  );
}
