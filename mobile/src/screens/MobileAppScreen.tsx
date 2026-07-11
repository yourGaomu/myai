import { useCallback, useRef } from "react";
import type { ScrollView } from "react-native";

import { AppHeader } from "../components/layout/AppHeader";
import { BottomDock } from "../components/layout/BottomDock";
import { useAndroidNavigationBar } from "../hooks/useAndroidNavigationBar";
import { useAssetState } from "../hooks/useAssetState";
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
import { useNavigationActions } from "../hooks/useNavigationActions";
import { useNormalizedRelayUrl } from "../hooks/useNormalizedRelayUrl";
import { usePairingActions } from "../hooks/usePairingActions";
import { usePendingActions } from "../hooks/usePendingActions";
import { useRelayConnection } from "../hooks/useRelayConnection";
import { useRemoteResultAppliers } from "../hooks/useRemoteResultAppliers";
import { useRemoteMessageHandler } from "../hooks/useRemoteMessageHandler";
import { useRemoteRuntimeRefs } from "../hooks/useRemoteRuntimeRefs";
import { useRelaySender } from "../hooks/useRelaySender";
import { useRemoteRequests } from "../hooks/useRemoteRequests";
import { useSessionModelActions } from "../hooks/useSessionModelActions";
import { useSessionModelState } from "../hooks/useSessionModelState";
import { useSessionSettingsActions } from "../hooks/useSessionSettingsActions";
import { useSkillState } from "../hooks/useSkillState";
import { MobileMainContent } from "./MobileMainContent";
import { MobileScreenShell } from "./MobileScreenShell";
import { buttonFeedback } from "../utils/buttonFeedback";
import { historyMessageToChatItem } from "../utils/chatHistory";

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
    userID,
  } = useMobileSettings({ onTokenRestored: handleTokenRestored });
  const {
    clearModels,
    clearSessions,
    currentModelID,
    deletedSessions,
    models,
    sessionCompacts,
    sessionContexts,
    sessions,
    setCurrentModelID,
    setDeletedSessions,
    setModels,
    setSessionCompact,
    setSessionContext,
    setSessions,
  } = useSessionModelState();
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
    assets,
    clearAssets,
    setAssets,
  } = useAssetState();
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
  const { isBusy, pendingActions, startPending, stopPending } = usePendingActions();

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
  void sessionChatsVersion;
  const currentUsage = currentChat.lastUsage || null;
  const currentSessionBusy = Boolean(currentChat.pendingRequestID);
  const currentPauseBusy = Boolean(pendingActions.pause);
  const uiBusy = isBusy || currentSessionBusy;
  const { bottomSafePadding, chatPanelHeight, topSafePadding } = useMobileLayoutMetrics({ hasUsage: Boolean(currentUsage) });
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

  const {
    refreshRemoteState,
    requestAssets,
    requestChanges,
    requestFiles,
    requestHistory,
    requestModels,
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
  const { deleteSession, loadSession, newSession, restoreSession, switchModel } = useSessionModelActions({
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
    compactSession,
    executePlan,
    setAgentMode,
    setContextWindowK,
    setPermissionMode,
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
    addUserMessage: (targetSessionID, message) => addMessage(targetSessionID, "user", message),
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
    applySkillList,
    applySessionChanged,
    applySessionHistoryDelta,
    applySessionHistoryMeta,
    applySessionHistory,
    applySessionList,
    applySessionSettings,
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
  });

  const handleRemoteMessage = useRemoteMessageHandler({
    activeRequestIDRef,
    addErrorMessage: (targetSessionID, message) => addMessage(targetSessionID, "error", message),
    addEventMessage: (targetSessionID, message) => addMessage(targetSessionID, "event", message),
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
    applySkillList,
    applySessionChanged,
    applySessionHistoryDelta,
    applySessionHistoryMeta,
    applySessionHistory,
    applySessionList,
    applySessionSettings,
    clearSessionPendingRequest,
    completeAssistant,
    currentFilePath: filePath,
    getSessionChat,
    historySessionIDRef,
    markAssistantError,
    mergeSessionChats,
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

  const connect = useRelayConnection({
    addErrorMessage: (message) => addMessage(sessionID, "error", message),
    clientToken,
    normalizedRelayURL,
    onConnected: refreshRemoteState,
    onMessage: handleRemoteMessage,
    setConnected,
    setStatus,
    socketRef,
    startPending,
    stopPending,
  });
  const {
    openChanges,
    openChat,
    openFiles,
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
          connected={connected}
          isBusy={uiBusy}
          lastUsage={currentUsage}
          messageInput={messageInput}
          onChangeMessage={setMessageInput}
          onChangesPress={openChanges}
          onChatPress={openChat}
          onFilesPress={openFiles}
          onPause={pauseSession}
          onRemoveAttachedFile={removeAttachedFile}
          onSend={sendUserMessage}
          onSessionsPress={openSessions}
          onSettingsPress={toggleSettings}
          onUploadFile={uploadLocalFile}
          pendingPause={currentPauseBusy}
          pendingSend={Boolean(currentChat.pendingRequestID)}
          pendingUpload={pendingActions.upload}
          viewMode={viewMode}
        />
      }
      bottomSafePadding={bottomSafePadding}
      scrollRef={chatScrollRef}
      topSafePadding={topSafePadding}
    >
      <AppHeader
        buttonFeedback={buttonFeedback}
        connected={connected}
        deviceID={deviceID}
        isBusy={uiBusy}
        onToggleSettings={toggleSettings}
        status={status}
        userID={userID}
        viewMode={viewMode}
      />

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
          chatPanelHeight,
          chatScrollRef,
          messages: currentChat.messages,
          onRegenerate: regenerateSession,
          pendingHistorySessionID: pendingHistorySessionIDRef.current,
          pendingRequestID: currentChat.pendingRequestID,
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
          currentModelID,
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
          onRefreshSessions: requestSessions,
          onRefreshSkills: requestSkills,
          onReloadSkills: reloadSkills,
          onAssetBaseURLChange: setAssetBaseURL,
          onRelayURLChange: setRelayURL,
          onSetAgentMode: setAgentMode,
          onSetContextWindowK: setContextWindowK,
          onSetPermissionMode: setPermissionMode,
          onSwitchModel: switchModel,
          onUserIDChange: setUserID,
          relayURL,
          sessionID,
          sessions,
          setupVisible,
          skillMessage,
          skillRoot,
          skills,
          userID,
        }}
      />
    </MobileScreenShell>
  );
}
