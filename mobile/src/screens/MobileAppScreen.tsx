import { useCallback, useRef } from "react";
import type { ScrollView } from "react-native";

import { AppHeader } from "../components/layout/AppHeader";
import { BottomDock } from "../components/layout/BottomDock";
import { useAndroidNavigationBar } from "../hooks/useAndroidNavigationBar";
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
import { MobileMainContent } from "./MobileMainContent";
import { MobileScreenShell } from "./MobileScreenShell";
import { buttonFeedback } from "../utils/buttonFeedback";

export function MobileAppScreen() {
  useAndroidNavigationBar();

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
    clientToken,
    deviceID,
    relayURL,
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
    models,
    sessionContexts,
    sessions,
    setCurrentModelID,
    setModels,
    setSessionContext,
    setSessions,
  } = useSessionModelState();
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
    appendAssistant,
    clearMessages,
    clearSessionPendingRequest,
    getSessionChat,
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
  const { bottomSafePadding, chatPanelHeight, topSafePadding } = useMobileLayoutMetrics({ hasUsage: Boolean(currentUsage) });
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
    requestChanges,
    requestFiles,
    requestHistory,
    requestModels,
    requestSessionHistory,
    requestSessions,
  } = useRemoteRequests({
    clearFileEntries,
    clearHistory,
    clearModels,
    clearSessions,
    clearWorkspaceChanges,
    clientToken,
    currentFilePath: filePath,
    currentSessionID: sessionID,
    pendingHistorySessionIDRef,
    sendEnvelope,
    startPending,
    stopPending,
  });
  const { loadSession, newSession, switchModel } = useSessionModelActions({
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
    setContextWindowK,
    setPermissionMode,
  } = useSessionSettingsActions({
    sendEnvelope,
    sessionID,
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
  } = useFileActions({
    addErrorMessage: (message) => addMessage(sessionID, "error", message),
    attachedFiles,
    changeDiffPath: changeDiff?.path,
    fileParent,
    filePreview,
    requestFiles,
    selectedChange,
    sendEnvelope,
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
  const { allowPermission, denyPermission, sendUserMessage } = useChatActions({
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
    startPending,
    stopPending,
  });
  const {
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
  } = useRemoteResultAppliers({
    activeRequestIDRef,
    addEventMessage: (targetSessionID, message) => addMessage(targetSessionID, "event", message),
    clearMessages,
    filePath,
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
    setSessions,
    setViewMode,
  });

  const handleRemoteMessage = useRemoteMessageHandler({
    activeRequestIDRef,
    addErrorMessage: (targetSessionID, message) => addMessage(targetSessionID, "error", message),
    addEventMessage: (targetSessionID, message) => addMessage(targetSessionID, "event", message),
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
    clearSessionPendingRequest,
    currentFilePath: filePath,
    getSessionChat,
    historySessionIDRef,
    mergeSessionChats,
    requestChanges,
    requestFiles,
    requestHistory,
    requestModels,
    requestSessions,
    requestSessionMapRef,
    sessionIDRef,
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
    openSessions,
    selectSession,
    toggleSettings,
  } = useNavigationActions({
    fileEntriesCount: fileEntries.length,
    filePath,
    loadSession,
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
          isBusy={isBusy}
          lastUsage={currentUsage}
          messageInput={messageInput}
          onChangeMessage={setMessageInput}
          onChangesPress={openChanges}
          onChatPress={openChat}
          onFilesPress={openFiles}
          onRemoveAttachedFile={removeAttachedFile}
          onSend={sendUserMessage}
          onSessionsPress={openSessions}
          onSettingsPress={toggleSettings}
          pendingSend={Boolean(currentChat.pendingRequestID)}
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
        isBusy={isBusy}
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
          fileEntries,
          fileParent,
          filePath,
          filePreview,
          filePreviewAttached,
          onAttachFilePreview: attachFilePreview,
          onGoToParent: goToParent,
          onOpenFileEntry: openFileEntry,
          onRefreshFiles: refreshCurrentFiles,
        }}
        permission={{
          onAllowPermission: allowPermission,
          onDenyPermission: denyPermission,
          pendingPermission: currentChat.pendingPermission,
        }}
        sessions={{
          onSelectSession: selectSession,
        }}
        settings={{
          activeModel,
          activeSession,
          bindCode,
          connected,
          context: sessionContexts[sessionID],
          currentModelID,
          deviceID,
          models,
          normalizedRelayURL,
          onBindCodeChange: setBindCode,
          onCloseSettings: openChat,
          onConnect: connect,
          onDeviceIDChange: setDeviceID,
          onLoadSession: loadSession,
          onNewSession: newSession,
          onPair: pairDevice,
          onCompactSession: compactSession,
          onRefreshModels: requestModels,
          onRefreshSessions: requestSessions,
          onRelayURLChange: setRelayURL,
          onSetContextWindowK: setContextWindowK,
          onSetPermissionMode: setPermissionMode,
          onSwitchModel: switchModel,
          onUserIDChange: setUserID,
          relayURL,
          sessionID,
          sessions,
          setupVisible,
          userID,
        }}
      />
    </MobileScreenShell>
  );
}
