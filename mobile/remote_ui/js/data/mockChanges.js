/**
 * MyAI Mobile UI Prototype - Mock Changes & Diff Data
 */

export const mockChanges = [
  {
    path: "mobile/src/screens/MobileAppScreen.tsx",
    index_status: "M",
    worktree_status: "M",
    kind: "modified"
  },
  {
    path: "mobile/src/components/chat/Composer.tsx",
    index_status: "M",
    worktree_status: " ",
    kind: "modified"
  },
  {
    path: "mobile/remote_ui/index.html",
    index_status: "?",
    worktree_status: "?",
    kind: "added"
  },
  {
    path: "mobile/legacy-preview.html",
    index_status: "D",
    worktree_status: " ",
    kind: "deleted"
  }
];

export const mockHistoryCheckpoints = [
  {
    checkpoint_id: "chk-89b3f1",
    message: "完成移动端底部停靠栏 5Tab 架构重构与状态绑定",
    created_at: 1774000000000,
    files_count: 5
  },
  {
    checkpoint_id: "chk-71a2e4",
    message: "新增 AI 记忆中心抽取与知识库 RAG 增强",
    created_at: 1773950000000,
    files_count: 12
  }
];

export const mockDiffDetail = {
  path: "mobile/src/screens/MobileAppScreen.tsx",
  additions: 14,
  deletions: 3,
  diff_text: `@@ -865,9 +865,14 @@ export function MobileAppScreen() {
           onSend={sendUserMessage}
           onKnowledgePress={openKnowledge}
+          onPlanPress={openPlan}
           onSettingsPress={toggleSettings}
           onUploadFile={uploadLocalFile}
-          pendingPause={currentPauseBusy}
+          pendingPause={currentPauseBusy || isProcessing}
+          showVoiceInput={true}
           pendingSend={Boolean(currentChat.pendingRequestID)}
@@ -881,3 +886,9 @@ export function MobileAppScreen() {
         activity={headerActivity}
         buttonFeedback={buttonFeedback}
+        enableHaptics={true}
+        themeMode="neo-brutalism"`
};
