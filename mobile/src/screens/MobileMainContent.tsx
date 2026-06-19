import { ChangeDetailPanel } from "../components/changes/ChangeDetailPanel";
import { ChangesPanel } from "../components/changes/ChangesPanel";
import { ChatPanel } from "../components/chat/ChatPanel";
import { FilesPanel } from "../components/files/FilesPanel";
import { PermissionPrompt } from "../components/permissions/PermissionPrompt";
import { SettingsPanel } from "../components/settings/SettingsPanel";
import { SessionsPanel } from "../components/sessions/SessionsPanel";
import type { MobileMainContentProps } from "./mobileContentProps";

export function MobileMainContent({
  changes,
  chat,
  common,
  files,
  permission,
  sessions,
  settings,
}: MobileMainContentProps) {
  return (
    <>
      {settings.setupVisible ? (
        <SettingsPanel
          activeModel={settings.activeModel}
          activeSession={settings.activeSession}
          bindCode={settings.bindCode}
          buttonFeedback={common.buttonFeedback}
          clientToken={common.clientToken}
          connected={settings.connected}
          context={settings.context}
          currentModelID={settings.currentModelID}
          deviceID={settings.deviceID}
          models={settings.models}
          normalizedRelayURL={settings.normalizedRelayURL}
          onBindCodeChange={settings.onBindCodeChange}
          onClose={settings.onCloseSettings}
          onConnect={settings.onConnect}
          onDeviceIDChange={settings.onDeviceIDChange}
          onLoadSession={settings.onLoadSession}
          onNewSession={settings.onNewSession}
          onPair={settings.onPair}
          onCompactSession={settings.onCompactSession}
          onRefreshModels={settings.onRefreshModels}
          onRefreshSessions={settings.onRefreshSessions}
          onRelayURLChange={settings.onRelayURLChange}
          onSetContextWindowK={settings.onSetContextWindowK}
          onSetPermissionMode={settings.onSetPermissionMode}
          onSwitchModel={settings.onSwitchModel}
          onUserIDChange={settings.onUserIDChange}
          pendingActions={common.pendingActions}
          relayURL={settings.relayURL}
          sessionID={settings.sessionID}
          sessions={settings.sessions}
          userID={settings.userID}
        />
      ) : null}

      {common.viewMode === "chat" ? (
        <ChatPanel
          buttonFeedback={common.buttonFeedback}
          chatScrollRef={chat.chatScrollRef}
          height={chat.chatPanelHeight}
          loadingHistory={common.pendingActions.sessions && Boolean(chat.pendingHistorySessionID)}
          messages={chat.messages}
          showAssistantLoading={Boolean(chat.pendingRequestID) && !chat.activeAssistantID}
        />
      ) : null}

      {common.viewMode === "files" ? (
        <FilesPanel
          buttonFeedback={common.buttonFeedback}
          clientToken={common.clientToken}
          fileEntries={files.fileEntries}
          fileParent={files.fileParent}
          filePath={files.filePath}
          filePreview={files.filePreview}
          filePreviewAttached={files.filePreviewAttached}
          onAttachFilePreview={files.onAttachFilePreview}
          onGoToParent={files.onGoToParent}
          onOpenFileEntry={files.onOpenFileEntry}
          onRefresh={files.onRefreshFiles}
          pendingFiles={common.pendingActions.files}
        />
      ) : null}

      {common.viewMode === "changes" ? (
        <ChangesPanel
          buttonFeedback={common.buttonFeedback}
          changes={changes.changes}
          changesClean={changes.changesClean}
          changesMessage={changes.changesMessage}
          clientToken={common.clientToken}
          historyCheckpoints={changes.historyCheckpoints}
          historyMessage={changes.historyMessage}
          onOpenChange={changes.onOpenChange}
          onPreviewHistory={changes.onPreviewHistory}
          onRefreshChanges={changes.onRefreshChanges}
          onRefreshHistory={changes.onRefreshHistory}
          onRevertHistory={changes.onRevertHistory}
          pendingChanges={common.pendingActions.changes}
          pendingDiff={common.pendingActions.diff}
          pendingHistory={common.pendingActions.history}
          pendingRevert={common.pendingActions.revert}
          selectedChange={changes.selectedChange}
        />
      ) : null}

      {common.viewMode === "changeDetail" ? (
        <ChangeDetailPanel
          buttonFeedback={common.buttonFeedback}
          canOpenSelectedChangeFile={changes.canOpenSelectedChangeFile}
          canRevertSelectedChange={changes.canRevertSelectedChange}
          changeDiff={changes.changeDiff}
          historyDiff={changes.historyDiff}
          onBack={changes.onBackToChanges}
          onOpenSelectedChangeFile={changes.onOpenSelectedChangeFile}
          onRevertHistory={changes.onRevertHistory}
          onRevertSelectedChange={changes.onRevertSelectedChange}
          pendingFiles={common.pendingActions.files}
          pendingRevert={common.pendingActions.revert}
        />
      ) : null}

      {common.viewMode === "sessions" ? (
        <SessionsPanel
          buttonFeedback={common.buttonFeedback}
          clientToken={common.clientToken}
          onNewSession={settings.onNewSession}
          onSelectSession={sessions.onSelectSession}
          pendingSessions={common.pendingActions.sessions}
          sessionID={settings.sessionID}
          sessions={settings.sessions}
        />
      ) : null}

      {permission.pendingPermission ? (
        <PermissionPrompt
          buttonFeedback={common.buttonFeedback}
          onAllow={permission.onAllowPermission}
          onDeny={permission.onDenyPermission}
          permission={permission.pendingPermission}
        />
      ) : null}
    </>
  );
}
