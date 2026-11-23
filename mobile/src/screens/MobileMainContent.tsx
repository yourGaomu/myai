import { ChangeDetailPanel } from "../components/changes/ChangeDetailPanel";
import { ChangesPanel } from "../components/changes/ChangesPanel";
import { ChatPanel } from "../components/chat/ChatPanel";
import { FadeSlideView } from "../components/common/MotionEffects";
import { FilesPanel } from "../components/files/FilesPanel";
import { KnowledgeWorkspacePanel } from "../components/knowledge/KnowledgeWorkspacePanel";
import { UserQuestionPrompt } from "../components/permissions/UserQuestionPrompt";
import { PlanPanel } from "../components/plan/PlanPanel";
import { SettingsPanel } from "../components/settings/SettingsPanel";
import { SessionsPanel } from "../components/sessions/SessionsPanel";
import type { MobileMainContentProps } from "./mobileContentProps";

const flexFillStyle = { flex: 1, minHeight: 0 } as const;

export function MobileMainContent({
  changes,
  chat,
  common,
  files,
  knowledge,
  permission,
  plan,
  sessions,
  settings,
}: MobileMainContentProps) {
  return (
    <>
      {settings.setupVisible ? (
        <FadeSlideView direction="right" keyProp={`settings-${settings.activeSection || "home"}`}>
          <SettingsPanel
          activeSection={settings.activeSection}
          onSelectSection={settings.onSelectSection}
          activeModel={settings.activeModel}
          activeSession={settings.activeSession}
          bindCode={settings.bindCode}
          buttonFeedback={common.buttonFeedback}
          canRenameSessions={sessions.canRenameSessions}
          clientToken={common.clientToken}
          compact={settings.compact}
          connected={settings.connected}
          connectionStatus={settings.connectionStatus}
          pairedRelayURL={settings.pairedRelayURL}
          pairingError={settings.pairingError}
          pairingPending={settings.pairingPending}
          context={settings.context}
          generation={settings.generation}
          generationStatus={settings.generationStatus}
          intentConfig={settings.intentConfig}
          intentConfigLoading={settings.intentConfigLoading}
          intentConfigSaving={settings.intentConfigSaving}
          intentConfigTesting={settings.intentConfigTesting}
          intentConfigMessage={settings.intentConfigMessage}
          intentConfigError={settings.intentConfigError}
          intentTestResult={settings.intentTestResult}
          intentTraces={settings.intentTraces}
          intentTraceDetails={settings.intentTraceDetails}
          intentTraceDetailErrors={settings.intentTraceDetailErrors}
          intentTraceListLoading={settings.intentTraceListLoading}
          intentTraceClearing={settings.intentTraceClearing}
          intentTraceDetailLoadingID={settings.intentTraceDetailLoadingID}
          intentTraceMessage={settings.intentTraceMessage}
          intentTraceError={settings.intentTraceError}
          currentModelID={settings.currentModelID}
          modelDeleteFeedback={settings.modelDeleteFeedback}
          modelMessage={settings.modelMessage}
          modelMessageError={settings.modelMessageError}
          deviceID={settings.deviceID}
          models={settings.models}
          normalizedRelayURL={settings.normalizedRelayURL}
          onBindCodeChange={settings.onBindCodeChange}
          onClose={settings.onCloseSettings}
          onConnect={settings.onConnect}
          onDeleteSession={settings.onDeleteSession}
          onLoadSession={settings.onLoadSession}
          onRenameSession={sessions.onRenameSession}
          onNewSession={settings.onNewSession}
          onPair={settings.onPair}
          onCompactSession={settings.onCompactSession}
          onExecutePlan={settings.onExecutePlan}
          onOpenPlan={settings.onOpenPlan}
          onRefreshModels={settings.onRefreshModels}
          onAddModelConfig={settings.onAddModelConfig}
          onUpdateModelConfig={settings.onUpdateModelConfig}
          onDeleteModelConfig={settings.onDeleteModelConfig}
          onSetModelEnabled={settings.onSetModelEnabled}
          onSetDefaultModel={settings.onSetDefaultModel}
          onClearModelMessage={settings.onClearModelMessage}
          onTestModelConfig={settings.onTestModelConfig}
          onRequestIntentConfig={settings.onRequestIntentConfig}
          onSaveIntentConfig={settings.onSaveIntentConfig}
          onTestIntentConfig={settings.onTestIntentConfig}
          onRequestIntentTraces={settings.onRequestIntentTraces}
          onRequestIntentTraceDetail={settings.onRequestIntentTraceDetail}
          onClearIntentTraces={settings.onClearIntentTraces}
          onClearIntentConfigFeedback={settings.onClearIntentConfigFeedback}
          onClearIntentTraceFeedback={settings.onClearIntentTraceFeedback}
          onRequestContextInfo={settings.onRequestContextInfo}
          onRequestGenerationPreferences={settings.onRequestGenerationPreferences}
          onRefreshSessions={settings.onRefreshSessions}
          onRefreshSkills={settings.onRefreshSkills}
          onReloadSkills={settings.onReloadSkills}
          onRefreshPlugins={settings.onRefreshPlugins}
          onReloadPlugins={settings.onReloadPlugins}
          onRefreshMCPInventory={settings.onRefreshMCPInventory}
          mcpInventory={settings.mcpInventory}
          reloadMCP={settings.reloadMCP}
          mcpReloading={settings.mcpReloading}
          mcpMessage={settings.mcpMessage}
          onSetPluginEnabled={settings.onSetPluginEnabled}
          onRelayURLChange={settings.onRelayURLChange}
          onSetAgentMode={settings.onSetAgentMode}
          onSetContextWindowK={settings.onSetContextWindowK}
          onSetGenerationSettings={settings.onSetGenerationSettings}
          onSetPermissionMode={settings.onSetPermissionMode}
          onSetStyleInstruction={settings.onSetStyleInstruction}
          onSwitchModel={settings.onSwitchModel}
          onApplySubagentTask={settings.onApplySubagentTask}
          onCancelSubagentTask={settings.onCancelSubagentTask}
          onCheckSubagentTask={settings.onCheckSubagentTask}
          onWaitSubagentTask={settings.onWaitSubagentTask}
          onCreateSubagentDefinition={settings.onCreateSubagentDefinition}
          onDeleteSubagentDefinition={settings.onDeleteSubagentDefinition}
          onDiscardSubagentTask={settings.onDiscardSubagentTask}
          onFollowupSubagentTask={settings.onFollowupSubagentTask}
          onMessageSubagentTask={settings.onMessageSubagentTask}
          onResumeSubagentTask={settings.onResumeSubagentTask}
          onInspectSubagentTask={settings.onInspectSubagentTask}
          onRefreshSubagents={settings.onRefreshSubagents}
          onUpdateSubagentDefinition={settings.onUpdateSubagentDefinition}
          pendingActions={common.pendingActions}
          relayURL={settings.relayURL}
          sessionID={settings.sessionID}
          sessions={settings.sessions}
          skillMessage={settings.skillMessage}
          skillRoot={settings.skillRoot}
          skills={settings.skills}
          plugins={settings.plugins}
          pluginMessage={settings.pluginMessage}
          pluginRoot={settings.pluginRoot}
          subagentDefinitions={settings.subagentDefinitions}
          subagentEvents={settings.subagentEvents}
          subagentMessage={settings.subagentMessage}
          subagentTasks={settings.subagentTasks}
          unreadSessionIDs={settings.unreadSessionIDs}
          userID={settings.userID}
        />
        </FadeSlideView>
      ) : null}

      {common.viewMode === "chat" ? (
        <ChatPanel
          activeAssistantID={chat.activeAssistantID}
          activePlan={chat.activePlan}
          activitySlot={chat.activitySlot}
          assets={chat.assets}
          buttonFeedback={common.buttonFeedback}
          changesCount={chat.changesCount}
          chatScrollRef={chat.chatScrollRef}
          connected={chat.connected}
          deviceID={chat.deviceID}
          greetingName={chat.greetingName}
          onAnswerUserQuestion={chat.onAnswerUserQuestion}
          onChangeUserQuestionDraft={chat.onChangeUserQuestionDraft}
          onExecutePlan={chat.onExecutePlan}
          onNearHeaderChange={chat.onNearHeaderChange}
          onOpenChangePath={chat.onOpenChangePath}
          onOpenChanges={chat.onOpenChanges}
          onOpenPlan={chat.onOpenPlan}
          onPickSuggestion={chat.onPickSuggestion}
          onScrolledChange={chat.onScrolledChange}
          loadingHistory={
            common.pendingActions.sessions &&
            Boolean(chat.pendingHistorySessionID) &&
            (!chat.sessionID || chat.pendingHistorySessionID === chat.sessionID)
          }
          messages={chat.messages}
          onInspectSubagent={chat.onInspectSubagent}
          onRegenerate={chat.onRegenerate}
          pendingPlan={chat.pendingPlan}
          pendingQuestions={chat.pendingQuestions}
          pendingRequestID={chat.pendingRequestID}
          runs={chat.runs}
          sessionID={chat.sessionID}
          subagentTasks={chat.subagentTasks}
          topClearance={chat.topClearance}
          workspaceChanges={chat.workspaceChanges}
        />
      ) : null}

      {common.viewMode !== "chat" ? (
        <FadeSlideView
          direction="right"
          keyProp={common.viewMode}
          style={common.viewMode === "changeDetail" ? flexFillStyle : undefined}
        >
          {common.viewMode === "knowledge" ? (
        <KnowledgeWorkspacePanel
          aiMemory={{
            candidates: knowledge.aiCandidates,
            dreamRuns: knowledge.aiDreamRuns,
            extractionJobs: knowledge.aiExtractionJobs,
            memories: knowledge.aiMemories,
            message: knowledge.aiMemoryMessage,
            onApproveCandidate: knowledge.onApproveAIMemoryCandidate,
            onCreateMemory: knowledge.onCreateAIMemory,
            onDeleteMemory: knowledge.onDeleteAIMemory,
            onRefreshCandidates: knowledge.onRefreshAIMemoryCandidates,
            onRefreshDreamRuns: knowledge.onRefreshAIMemoryDreamRuns,
            onRefreshExtractionJobs: knowledge.onRefreshAIMemoryExtractionJobs,
            onRefreshMemories: knowledge.onRefreshAIMemories,
            onRejectCandidate: knowledge.onRejectAIMemoryCandidate,
            onRestoreMemory: knowledge.onRestoreAIMemory,
            onRetryExtractionJob: knowledge.onRetryAIMemoryExtractionJob,
            onRunDream: knowledge.onRunAIMemoryDream,
            onUpdateMemory: knowledge.onUpdateAIMemory,
            pending: common.pendingActions.memory,
          }}
          buttonFeedback={common.buttonFeedback}
          onInsertReference={knowledge.onInsertReference}
          knowledge={{
            activeSession: knowledge.activeSession,
            categories: knowledge.categories,
            documentImportStatus: knowledge.documentImportStatus,
            documents: knowledge.documents,
            jobs: knowledge.jobs,
            knowledgeBases: knowledge.knowledgeBases,
            message: knowledge.message,
            onCreateCategory: knowledge.onCreateCategory,
            onCreateKnowledgeBase: knowledge.onCreateKnowledgeBase,
            onDeleteCategory: knowledge.onDeleteCategory,
            onDeleteDocument: knowledge.onDeleteDocument,
            onDeleteKnowledgeBase: knowledge.onDeleteKnowledgeBase,
            onDismissDocumentImport: knowledge.onDismissDocumentImport,
            onMoveCategory: knowledge.onMoveCategory,
            onReconcileDocumentImport: knowledge.onReconcileDocumentImport,
            onRefresh: knowledge.onRefresh,
            onRefreshDocuments: knowledge.onRefreshDocuments,
            onRetryDocument: knowledge.onRetryDocument,
            onSearch: knowledge.onSearch,
            onSelectKnowledgeBase: knowledge.onSelectKnowledgeBase,
            onSetRAG: knowledge.onSetRAG,
            onUpdateKnowledgeBase: knowledge.onUpdateKnowledgeBase,
            onUploadDocument: knowledge.onUploadDocument,
            onCancelUpload: knowledge.onCancelUpload,
            uploadingDocument: knowledge.uploadingDocument,
            pendingKnowledge: common.pendingActions.knowledge,
            pendingSettings: common.pendingActions.settings,
            profiles: knowledge.profiles,
            searchResult: knowledge.searchResult,
          }}
        />
      ) : null}

      {common.viewMode === "plan" ? (
        <PlanPanel
          activeSession={plan.activeSession}
          buttonFeedback={common.buttonFeedback}
          clientToken={common.clientToken}
          onExecutePlan={plan.onExecutePlan}
          onOpenChat={plan.onOpenChat}
          pendingPlan={common.pendingActions.plan}
          sessionID={plan.sessionID}
        />
      ) : null}

      {common.viewMode === "files" ? (
        <FilesPanel
          buttonFeedback={common.buttonFeedback}
          assets={files.assets}
          changes={files.changes}
          clientToken={common.clientToken}
          fileEntries={files.fileEntries}
          fileParent={files.fileParent}
          filePath={files.filePath}
          filePreview={files.filePreview}
          onGoToParent={files.onGoToParent}
          onOpenFileEntry={files.onOpenFileEntry}
          pendingAssets={common.pendingActions.assets}
          pendingFiles={common.pendingActions.files}
          showPreview={files.previewOpen}
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
          onRevertHistory={changes.onRevertHistory}
          pendingChanges={common.pendingActions.changes}
          pendingDiff={common.pendingActions.diff}
          pendingHistory={common.pendingActions.history}
          pendingRevert={common.pendingActions.revert}
          selectedChange={changes.selectedChange}
        />
      ) : null}

      {common.viewMode === "changeDetail" ? (
        <ChangeDetailPanel changeDiff={changes.changeDiff} historyDiff={changes.historyDiff} />
      ) : null}

      {common.viewMode === "sessions" || common.viewMode === "trash" ? (
        <SessionsPanel
          buttonFeedback={common.buttonFeedback}
          canRenameSessions={sessions.canRenameSessions}
          clientToken={common.clientToken}
          deletedSessions={sessions.deletedSessions}
          mode={common.viewMode === "trash" ? "trash" : "active"}
          onDeleteSession={sessions.onDeleteSession}
          onNewSession={settings.onNewSession}
          onRefreshDeletedSessions={sessions.onRefreshDeletedSessions}
          onRenameSession={sessions.onRenameSession}
          onRestoreSession={sessions.onRestoreSession}
          onSelectSession={sessions.onSelectSession}
          pendingSessions={common.pendingActions.sessions}
          sessionID={settings.sessionID}
          sessions={settings.sessions}
          unreadSessionIDs={sessions.unreadSessionIDs}
        />
      ) : null}

      {/* AskUserQuestion 已内嵌渲染在对话流内部；若用户在其他二级页，允许通过提问卡片就地响应 */}
          {permission.pendingQuestions.map((question) => (
            <UserQuestionPrompt
              key={JSON.stringify([question.sessionID, question.requestID, question.questionID])}
              buttonFeedback={common.buttonFeedback}
              onAnswer={(answers, cancelled) => permission.onAnswerUserQuestion(question, answers, cancelled)}
              onChangeDraft={permission.onChangeUserQuestionDraft ? (answers) => permission.onChangeUserQuestionDraft?.(question, answers) : undefined}
              question={question}
            />
          ))}
        </FadeSlideView>
      ) : null}
    </>
  );
}
