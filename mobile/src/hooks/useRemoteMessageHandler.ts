import { useCallback, useRef, type RefObject } from "react";

import type {
  AIMemoryCandidateListResultPayload,
  AIMemoryCandidateMutationResultPayload,
  AIMemoryExtractionJobListResultPayload,
  AIMemoryDreamResultPayload,
  AIMemoryListResultPayload,
  AgentRun,
  AgentRunCompletedPayload,
  AgentRunEventPayload,
  AgentRunListResultPayload,
  AgentRunStartedPayload,
  AssetListResultPayload,
  AssetSummary,
  AssistantDeltaPayload,
  AssistantDonePayload,
  BackgroundTurnEventPayload,
  ChangeDiffResultPayload,
  ChangeRevertResultPayload,
  ChangesListResultPayload,
  ErrorPayload,
  FileListResultPayload,
  FileReadResultPayload,
  HistoryDiffResultPayload,
  HistoryListResultPayload,
  HistoryRevertResultPayload,
  ImageModelConfigAddResultPayload,
  ImageModelConfigMutationResultPayload,
  ImageModelConfigTestResultPayload,
  ImageModelListResultPayload,
  IntentConfigResultPayload,
  IntentConfigTestResultPayload,
  IntentTraceClearResultPayload,
  IntentTraceGetResultPayload,
  IntentTraceListResultPayload,
  IntentTracePayload,
  KnowledgeCatalogResultPayload,
  KnowledgeDocumentListResultPayload,
  KnowledgeProfileListResultPayload,
  KnowledgeSearchPreviewResultPayload,
  ModelListResultPayload,
  ModelConfigAddResultPayload,
  ModelConfigMutationResultPayload,
  ModelConfigTestResultPayload,
  ModelSwitchResultPayload,
  PluginListResultPayload,
  PluginMutationResultPayload,
  PermissionAskPayload,
  RelayMessage,
  SessionGenerationResultPayload,
  SessionListResultPayload,
  SessionPlanExecuteUpdatePayload,
  SessionSettingsResultPayload,
  SessionPauseResultPayload,
  SkillListResultPayload,
  SubagentDefinitionListResultPayload,
  SubagentDefinitionMutationResultPayload,
  SubagentTaskDetailResultPayload,
  SubagentTaskEventsResultPayload,
  SubagentTaskListResultPayload,
  SubagentTaskResultPayload,
  TokenUsage,
  ToolCallPayload,
  ToolResultPayload,
  UserQuestionPayload,
  UserQuestionListResultPayload,
  UserQuestionResolvedPayload,
  UserMessageQueuedPayload,
} from "../protocol";
import type { PendingAction, PermissionState, QuestionSync, RelayScope, UserQuestionState } from "../types/app";
import { assetSummariesForSession } from "../utils/toolAssets";
import { validateQuestionChoices } from "../utils/userQuestion";
import type { SessionChatState } from "./useChatMessages";

type Args = {
  acceptAgentRunList: (message: RelayMessage) => boolean;
  acceptAssetList: (message: RelayMessage) => boolean;
  acceptSessionList: (message: RelayMessage) => boolean;
  handleRemoteQueryError: (message: RelayMessage) => boolean;
  handleSessionHistoryMessage: (message: RelayMessage) => boolean;
  handleAssetUploadMessage: (message: RelayMessage) => boolean;
  activeRequestIDRef: RefObject<string>;
  addErrorMessage: (sessionID: string, message: string) => void;
  addEventMessage: (sessionID: string, message: string) => void;
  upsertIntentTrace: (sessionID: string, trace: IntentTracePayload) => void;
  addToolCall: (
    sessionID: string,
    name: string,
    argumentsText: string,
    requestID?: string,
    toolCallID?: string,
  ) => void;
  addToolResult: (
    sessionID: string,
    name: string,
    argumentsText: string,
    result: string,
    failed: boolean,
    requestID?: string,
    details?: ToolResultPayload,
    toolCallID?: string,
  ) => void;
  applyRunCompleted: (payload?: AgentRunCompletedPayload) => void;
  applyRunEvent: (payload?: AgentRunEventPayload) => void;
  applyRunList: (payload?: AgentRunListResultPayload) => void;
  applyRunStarted: (payload?: AgentRunStartedPayload) => void;
  applyAssistantPlan: (
    sessionID: string,
    plan?: NonNullable<AssistantDonePayload["plan"]>,
  ) => void;
  appendAssistant: (
    sessionID: string,
    requestID: string | undefined,
    text: string,
    reasoning?: string,
  ) => void;
  applyAssetList: (payload?: AssetListResultPayload) => void;
  applyChangeDiff: (payload?: ChangeDiffResultPayload) => void;
  applyChangeRevert: (payload?: ChangeRevertResultPayload) => void;
  applyChangesList: (payload?: ChangesListResultPayload) => void;
  applyFileList: (payload?: FileListResultPayload) => void;
  applyFileRead: (payload?: FileReadResultPayload) => void;
  applyHistoryDiff: (payload?: HistoryDiffResultPayload) => void;
  applyHistoryList: (payload?: HistoryListResultPayload) => void;
  applyHistoryRevert: (payload?: HistoryRevertResultPayload) => void;
  applyIntentConfigQuery: (payload?: IntentConfigResultPayload, requestID?: string) => void;
  applyIntentConfigSet: (payload?: IntentConfigResultPayload, requestID?: string) => void;
  applyIntentConfigTest: (payload?: IntentConfigTestResultPayload, requestID?: string) => void;
  applyIntentTraceList: (payload?: IntentTraceListResultPayload, requestID?: string) => void;
  applyIntentTraceGet: (payload?: IntentTraceGetResultPayload, requestID?: string) => void;
  applyIntentTraceClear: (payload?: IntentTraceClearResultPayload, requestID?: string) => void;
  applyIntentError: (message: string, requestID?: string) => boolean;
  applyKnowledgeCatalog: (payload?: KnowledgeCatalogResultPayload) => void;
  applyKnowledgeDocuments: (payload?: KnowledgeDocumentListResultPayload) => void;
  applyKnowledgeProfiles: (payload?: KnowledgeProfileListResultPayload) => void;
  applyKnowledgeSearch: (payload?: KnowledgeSearchPreviewResultPayload) => void;
  applyKnowledgeError: (message: string) => void;
  applyAIMemories: (payload?: AIMemoryListResultPayload) => void;
  applyAIMemoryCandidates: (payload?: AIMemoryCandidateListResultPayload) => void;
  applyAIMemoryCandidateMutation: (payload?: AIMemoryCandidateMutationResultPayload) => void;
  applyAIMemoryExtractionJobs: (payload?: AIMemoryExtractionJobListResultPayload) => void;
  applyAIMemoryDreamResult: (payload?: AIMemoryDreamResultPayload) => void;
  applyAIMemoryError: (message: string) => void;
  applyModelList: (payload?: ModelListResultPayload) => void;
  applyModelConfigAdd: (payload?: ModelConfigAddResultPayload) => void;
  applyModelConfigTest: (payload?: ModelConfigTestResultPayload) => void;
  applyModelConfigMutation: (payload?: ModelConfigMutationResultPayload) => void;
  applyModelConfigError: (message: string) => void;
  applyImageModelList: (payload?: ImageModelListResultPayload) => void;
  applyImageModelConfigAdd: (payload?: ImageModelConfigAddResultPayload) => void;
  applyImageModelConfigTest: (payload?: ImageModelConfigTestResultPayload) => void;
  applyImageModelConfigMutation: (payload?: ImageModelConfigMutationResultPayload) => void;
  applyImageModelConfigError: (message: string) => void;
  handleModelDeleteMessage: (message: RelayMessage) => boolean;
  handleSessionRenameMessage?: (message: RelayMessage) => boolean;
  isModelOperationPending: boolean;
  applyModelSwitch: (payload?: ModelSwitchResultPayload) => void;
  applySessionChanged: (message: RelayMessage) => boolean;
  applySessionCommandError: (requestID?: string) => boolean;
  hasPendingNavigation: () => boolean;
  applySessionList: (payload?: SessionListResultPayload) => void;
  applySessionSettings: (payload?: SessionSettingsResultPayload, sessionID?: string) => void;
  applySessionGenerationPreferences: (payload?: SessionGenerationResultPayload) => void;
  applySessionGenerationError: (message: string) => void;
  applySkillList: (payload?: SkillListResultPayload) => void;
  applyPluginList: (payload?: PluginListResultPayload | PluginMutationResultPayload) => void;
  applySubagentDefinitionList: (payload?: SubagentDefinitionListResultPayload) => void;
  applySubagentDefinitionMutation: (payload?: SubagentDefinitionMutationResultPayload) => void;
  applySubagentTaskList: (payload?: SubagentTaskListResultPayload) => void;
  applySubagentTaskResult: (payload?: SubagentTaskResultPayload) => void;
  applySubagentTaskDetailResult?: (
    payload?: SubagentTaskDetailResultPayload,
    requestID?: string,
  ) => void;
  applySubagentTaskEventsResult?: (
    payload?: SubagentTaskEventsResultPayload,
    requestID?: string,
  ) => void;
  applyRealtimeSubagentTaskEvent?: (
    payload?: SubagentTaskResultPayload,
  ) => void;
  clearSessionPendingRequest: (sessionID: string, requestID?: string) => void;
  markUserMessageQueued: (sessionID: string, requestID: string) => void;
  markUserMessageDelivered: (sessionID: string, requestID: string) => void;
  completeAssistant: (
    sessionID: string,
    requestID: string | undefined,
    status: "done" | "paused",
    usage?: TokenUsage | null,
    content?: string,
    reasoning?: string,
    messageID?: string,
    runID?: string,
    assets?: AssetSummary[],
  ) => void;
  currentFilePath: string;
  getSessionChat: (sessionID: string) => SessionChatState;
  pendingHistorySessionIDRef: RefObject<string>;
  questionSyncRef: RefObject<QuestionSync | null>;
  relayScopeRef: RefObject<RelayScope>;
  hasRunForRequest: (sessionID: string, requestID?: string) => boolean;
  isGenerationOperationPending: boolean;
  isKnowledgeOperationPending: boolean;
  isMemoryOperationPending: boolean;
  markAssistantError: (
    sessionID: string,
    requestID: string | undefined,
    message?: string,
  ) => void;
  mergeSessionChats: (fromSessionID: string, toSessionID: string) => void;
  mergeSessionRuns: (fromSessionID: string, toSessionID: string) => void;
  addUnreadSession?: (sessionID: string) => void;
  requestAgentRuns?: (sessionID?: string) => boolean;
  requestSessionHistory?: (sessionID?: string) => boolean;
  requestSessionHistoryFull: (sessionID?: string) => boolean;
  requestUserQuestionSync: () => boolean;
  startBackgroundAssistant?: (sessionID: string, turnID: string, title?: string) => void;
  requestChanges: () => boolean;
  requestAssets: (sessionID?: string) => boolean;
  requestFiles: (path?: string) => boolean;
  requestHistory: () => boolean;
  requestModels: () => boolean;
  requestSkills: () => boolean;
  requestDeletedSessions: () => boolean;
  requestSessions: () => boolean;
  requestSessionMapRef: RefObject<Record<string, string>>;
  sessionIDRef: RefObject<string>;
  setSessionLastUsage: (sessionID: string, usage: TokenUsage | null) => void;
  setSessionCompact: (
    sessionID: string,
    compact: NonNullable<AssistantDonePayload["compact"]>,
  ) => void;
  setSessionContext: (
    sessionID: string,
    context: NonNullable<AssistantDonePayload["context"]>,
    source: "live",
  ) => void;
  setSessionPendingPermission: (
    sessionID: string,
    permission: PermissionState | null,
  ) => void;
  setSessionPendingQuestion: (sessionID: string, question: UserQuestionState | null) => void;
  removeSessionPendingQuestion: (sessionID: string, questionID: string, requestID?: string) => void;
  removeSessionPendingPermission: (sessionID: string, requestID: string, approvalID?: string) => void;
  settleSessionRun: (sessionID: string, requestID: string, status: "done" | "paused" | "error") => void;
  removePendingQuestions: (questionIDs: string[]) => void;
  setSessionID: (sessionID: string) => void;
  stopPending: (action: PendingAction) => void;
};

// 所有服务端协议消息都在这里收口：先按 request_id 找到目标 Session，再更新对应领域状态。
export function useRemoteMessageHandler({
  acceptAgentRunList,
  acceptAssetList,
  acceptSessionList,
  handleRemoteQueryError,
  handleSessionHistoryMessage,
  handleAssetUploadMessage,
  activeRequestIDRef,
  addErrorMessage,
  addEventMessage,
  upsertIntentTrace,
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
  applyKnowledgeError,
  applyAIMemories,
  applyAIMemoryCandidates,
  applyAIMemoryCandidateMutation,
  applyAIMemoryExtractionJobs,
  applyAIMemoryDreamResult,
  applyAIMemoryError,
  applyModelList,
  applyModelConfigAdd,
  applyModelConfigTest,
  applyModelConfigMutation,
  applyModelConfigError,
  applyImageModelList,
  applyImageModelConfigAdd,
  applyImageModelConfigTest,
  applyImageModelConfigMutation,
  applyImageModelConfigError,
  handleModelDeleteMessage,
  handleSessionRenameMessage,
  isModelOperationPending,
  applyModelSwitch,
  applySessionChanged,
  applySessionCommandError,
  hasPendingNavigation,
  applySessionList,
  applySessionSettings,
  applySessionGenerationPreferences,
  applySessionGenerationError,
  applySkillList,
  applyPluginList,
  applySubagentDefinitionList,
  applySubagentDefinitionMutation,
  applySubagentTaskList,
  applySubagentTaskResult,
  applySubagentTaskDetailResult,
  applySubagentTaskEventsResult,
  applyRealtimeSubagentTaskEvent,
  clearSessionPendingRequest,
  markUserMessageQueued,
  markUserMessageDelivered,
  completeAssistant,
  currentFilePath,
  getSessionChat,
  pendingHistorySessionIDRef,
  questionSyncRef,
  relayScopeRef,
  hasRunForRequest,
  isGenerationOperationPending,
  isKnowledgeOperationPending,
  isMemoryOperationPending,
  markAssistantError,
  mergeSessionChats,
  mergeSessionRuns,
  addUnreadSession,
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
  requestSkills,
  requestDeletedSessions,
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
}: Args) {
  const turnSequenceMapRef = useRef<Record<string, number>>({});
  const resolvedUserQuestionIDsRef = useRef(new Set<string>());
  const resolvedPermissionIDsRef = useRef(new Set<string>());
  const backgroundPermissionsRef = useRef<Record<string, { sessionID: string; turnID: string }>>({});
  const backgroundQuestionsRef = useRef<Record<string, { sessionID: string; turnID: string }>>({});

  return useCallback(
    (message: RelayMessage) => {
      if (handleSessionRenameMessage?.(message) || handleAssetUploadMessage(message)) return;

      function forgetRequest(requestID: string) {
        if (activeRequestIDRef.current === requestID) activeRequestIDRef.current = "";
        delete requestSessionMapRef.current[requestID];
      }

      function finishTurnInteractions(sessionID: string, requestID: string) {
        const chat = getSessionChat(sessionID);
        for (const permission of chat.pendingPermissions) {
          if (permission.requestID !== requestID) continue;
          delete backgroundPermissionsRef.current[permission.approvalID];
          rememberResolvedUserQuestion(permission.approvalID, resolvedPermissionIDsRef.current);
        }
        removeSessionPendingPermission(sessionID, requestID);
        for (const question of chat.pendingQuestions) {
          if (question.requestID !== requestID) continue;
          delete backgroundQuestionsRef.current[question.questionID];
          rememberResolvedUserQuestion(question.questionID, resolvedUserQuestionIDsRef.current);
          removeSessionPendingQuestion(sessionID, question.questionID, requestID);
        }
      }

      function resolveTurnSession(frame: RelayMessage): string | null {
        const requestID = frame.request_id;
        if (!requestID) return null;
        const requests = requestSessionMapRef.current;
        const original = Object.prototype.hasOwnProperty.call(requests, requestID) ? requests[requestID] : undefined;
        const reported = frame.session_id?.trim();
        if (original && reported && original !== reported) return null;
        if (original === "" && reported) {
          if (getSessionChat("").pendingRequestID === requestID) {
            mergeSessionChats("", reported);
            mergeSessionRuns("", reported);
          }
          requests[requestID] = reported;
          if (!sessionIDRef.current && !hasPendingNavigation()) {
            sessionIDRef.current = reported;
            setSessionID(reported);
          }
        }
        return reported || original || (original === "" ? "" : null);
      }

      function rememberQuestion(sessionID: string, requestID: string, payload: UserQuestionPayload) {
        if (!sessionID || !requestID || !payload.question_id || !payload.questions?.length ||
            resolvedUserQuestionIDsRef.current.has(payload.question_id)) return false;
        if (payload.schema_version !== 2) {
          addErrorMessage(sessionID, "问答协议版本不匹配，请同时升级 Agent 与移动端后重试。");
          return false;
        }
        try { payload.questions.forEach(validateQuestionChoices); }
        catch (error) {
          addErrorMessage(sessionID, error instanceof Error ? error.message : "问答协议无效，请升级 Agent 与移动端。");
          return false;
        }
        const existing = getSessionChat(sessionID).pendingQuestions.some((question) =>
          question.questionID === payload.question_id && question.requestID === requestID);
        if (!existing) {
          setSessionPendingQuestion(sessionID, {
            questionID: payload.question_id, requestID, sessionID,
            title: payload.title || "需要你确认一些信息", questions: payload.questions,
            sourceTaskID: payload.source_task_id, submitted: false,
          });
          if (sessionID !== sessionIDRef.current) addUnreadSession?.(sessionID);
        }
        return true;
      }

      // switch 分组与 Go 的 protocol.MessageType 一一对应；新增协议时必须同步补充这里的终态清理。
      switch (message.type) {
        case "user_message_queued": {
          const sessionID = resolveTurnSession(message);
          const requestID = message.request_id;
          const payload = (message.payload || {}) as UserMessageQueuedPayload;
          if (sessionID === null || !requestID) break;
          markUserMessageQueued(sessionID, requestID);
          clearSessionPendingRequest(sessionID, requestID);
          forgetRequest(requestID);
          if (payload.content && sessionID && sessionID !== sessionIDRef.current) addUnreadSession?.(sessionID);
          break;
        }
        case "heartbeat":
          break;
        case "assistant_delta": {
          const payload = (message.payload || {}) as AssistantDeltaPayload;
          const sessionID = resolveTurnSession(message);
          if (sessionID !== null) appendAssistant(sessionID, message.request_id, payload.content || "", payload.reasoning || "");
          break;
        }
        case "intent_trace_event": {
          const sessionID = resolveTurnSession(message);
          const trace = message.payload as IntentTracePayload | undefined;
          if (sessionID && trace?.trace_id && trace.session_id === sessionID && trace.request_id === message.request_id) {
            upsertIntentTrace(sessionID, trace);
          }
          break;
        }
        case "agent_run_started":
          if (resolveTurnSession(message) !== null) applyRunStarted(message.payload as AgentRunStartedPayload | undefined);
          break;
        case "agent_run_event":
          applyRunEvent(message.payload as AgentRunEventPayload | undefined);
          break;
        case "agent_run_completed": {
          const payload = message.payload as AgentRunCompletedPayload | undefined;
          applyRunCompleted(payload);
          const run = payload?.run;
          if (run?.request_id && run.request_id === message.request_id && run.session_id === message.session_id &&
              !run.step_id && run.kind !== "internal" && run.status !== "running" &&
              (getSessionChat(run.session_id).pendingRequestID === run.request_id || getSessionChat("").pendingRequestID === run.request_id)) {
            // The live run notification precedes assistant_done. An independent
            // snapshot also covers final frames lost on a socket that stays open.
            requestAgentRuns?.(run.session_id);
          }
          break;
        }
        case "agent_run_list_result": {
          if (!acceptAgentRunList(message)) break;
          const payload = message.payload as AgentRunListResultPayload;
          applyRunList(payload);
          const sessionID = payload.session_id;
          // A background continuation has no pending foreground request. If its
          // terminal frame was lost, link the existing stream to the committed
          // reply before the history snapshot can render a second copy.
          const sessionChat = getSessionChat(sessionID);
          const localRequests = new Set(sessionChat.messages.filter((item) =>
            item.role === "assistant" && item.requestID && !sessionChat.finishedRequests.has(item.requestID) &&
            (item.status === "streaming" || item.status === "tool_running"),
          ).map((item) => item.requestID));
          for (const { run, events } of payload.runs || []) {
            const turnID = run.request_id;
            if (!turnID || run.session_id !== sessionID || run.kind !== "chat" || run.step_id ||
                run.status !== "succeeded" || turnID === sessionChat.pendingRequestID || !localRequests.has(turnID)) continue;
            const answer = events?.find((event) => event.type === "answer" && event.message_id);
            if (!answer) continue;
            completeAssistant(sessionID, turnID, "done", undefined, answer.truncated ? undefined : answer.content, undefined, answer.message_id, run.id);
            finishTurnInteractions(sessionID, turnID);
            forgetRequest(turnID);
          }
          const requestID = sessionChat.pendingRequestID || getSessionChat("").pendingRequestID;
          if (!requestID) break;
          const candidates = (payload.runs || []).filter(({ run }) =>
            run.session_id === sessionID && run.request_id === requestID && !run.step_id && run.kind !== "internal");
          // Plan steps share the request ID. A terminal step cannot settle the enclosing turn.
          if (candidates.some(({ run }) => run.status === "running")) break;
          const finished = candidates.filter(({ run }) =>
            run.status === "succeeded" || run.status === "paused" || run.status === "canceled" || run.status === "failed")
            .sort((a, b) => Date.parse(a.run.finished_at || a.run.started_at) - Date.parse(b.run.finished_at || b.run.started_at)).pop();
          if (!finished || resolveTurnSession({ ...message, request_id: requestID, session_id: sessionID }) === null ||
              getSessionChat(sessionID).pendingRequestID !== requestID) break;
          const status = finished.run.status === "succeeded" ? "done" : finished.run.status === "failed" ? "error" : "paused";
          settleSessionRun(sessionID, requestID, status);
          const answer = finished.events?.find((event) => event.type === "answer" && event.message_id);
          if (status === "done" && answer) {
            completeAssistant(sessionID, requestID, "done", undefined, answer.truncated ? undefined : answer.content, undefined, answer.message_id, finished.run.id);
          }
          markUserMessageDelivered(sessionID, requestID);
          finishTurnInteractions(sessionID, requestID);
          forgetRequest(requestID);
          if (finished.run.kind === "plan") stopPending("plan");
          requestSessionHistoryFull(sessionID);
          break;
        }
        case "background_turn_event": {
          const payload = (message.payload || {}) as BackgroundTurnEventPayload;
          if ((payload.session_id && message.session_id && payload.session_id !== message.session_id) ||
              (payload.turn_id && message.request_id && payload.turn_id !== message.request_id)) break;
          const turnID = payload.turn_id || message.request_id || "";
          const targetSessionID =
            payload.session_id ||
            message.session_id ||
            (turnID ? requestSessionMapRef.current[turnID] : "") ||
            sessionIDRef.current;

          if (payload.kind === "resync_required") {
            for (const [approvalID, identity] of Object.entries(backgroundPermissionsRef.current)) {
              removeSessionPendingPermission(identity.sessionID, identity.turnID, approvalID);
            }
            backgroundPermissionsRef.current = {};
            requestUserQuestionSync();
            const resyncSessionID = targetSessionID || sessionIDRef.current;
            if (resyncSessionID) {
              requestAgentRuns?.(resyncSessionID);
              requestSessionHistory?.(resyncSessionID);
            }
            break;
          }

          if (!turnID || !targetSessionID) {
            break;
          }

          if (turnID) {
            requestSessionMapRef.current[turnID] = targetSessionID;
          }

          const seq = typeof payload.sequence === "number" ? payload.sequence : 0;
          const turnKey = `${targetSessionID}:${turnID}`;
          const lastSeq = turnSequenceMapRef.current[turnKey] || 0;

          const interactionEvent = payload.kind === "permission_ask" || payload.kind === "user_question";
          if (seq > 0 && !(interactionEvent && seq <= lastSeq)) {
            if (seq <= lastSeq) {
              // 实时轮次序号去重
              break;
            }
            if (lastSeq > 0 && seq > lastSeq + 1) {
              // 序号缺口，自动重查校准
              requestAgentRuns?.(targetSessionID);
              requestSessionHistory?.(targetSessionID);
            }
            turnSequenceMapRef.current[turnKey] = seq;
          }

          if (targetSessionID !== sessionIDRef.current) {
            addUnreadSession?.(targetSessionID);
          }

          switch (payload.kind) {
            case "intent_trace": {
              const trace = payload.intent_trace;
              if (trace?.trace_id && trace.session_id === targetSessionID) upsertIntentTrace(targetSessionID, trace);
              break;
            }
            case "permission_ask": {
              const permission = payload.permission;
              const approvalID = payload.approval_id || permission?.approval_id;
              if (!approvalID || !permission || resolvedPermissionIDsRef.current.has(approvalID)) { break; }
              backgroundPermissionsRef.current[approvalID] = { sessionID: targetSessionID, turnID };
              setSessionPendingPermission(targetSessionID, {
                approvalID, requestID: turnID, sessionID: targetSessionID,
                name: permission.name || "tool", permission: permission.permission || "", arguments: permission.arguments || "",
                requireFreshConsent: permission.require_fresh_consent === true,
              });
              break;
            }
            case "permission_resolved": {
              if (payload.approval_id) {
                rememberResolvedUserQuestion(payload.approval_id, resolvedPermissionIDsRef.current);
                delete backgroundPermissionsRef.current[payload.approval_id];
              }
              if (payload.approval_id) removeSessionPendingPermission(targetSessionID, turnID, payload.approval_id);
              break;
            }
            case "user_question": {
              const question = payload.question;
              if (question && rememberQuestion(targetSessionID, turnID, question)) {
                backgroundQuestionsRef.current[question.question_id] = { sessionID: targetSessionID, turnID };
              }
              break;
            }
            case "user_question_resolved": {
              const questionID = payload.question?.question_id;
              if (questionID) {
                delete backgroundQuestionsRef.current[questionID];
                rememberResolvedUserQuestion(questionID, resolvedUserQuestionIDsRef.current);
                removeSessionPendingQuestion(targetSessionID, questionID, turnID);
              }
              break;
            }
            case "started": {
              const run: AgentRun = payload.run || {
                id: payload.run_id || turnID,
                request_id: turnID,
                session_id: targetSessionID,
                kind: "chat",
                title: "正在处理子代理结果",
                status: "running",
                started_at: new Date().toISOString(),
              };
              applyRunStarted({
                run: { ...run, title: run.title || "正在处理子代理结果" },
              });
              startBackgroundAssistant?.(targetSessionID, turnID, "正在处理子代理结果");
              break;
            }

            case "delta": {
              if (payload.run_id && !hasRunForRequest(targetSessionID, turnID)) {
                applyRunStarted({
                  run: {
                    id: payload.run_id,
                    request_id: turnID,
                    session_id: targetSessionID,
                    kind: "chat",
                    title: "正在处理子代理结果",
                    status: "running",
                    started_at: new Date().toISOString(),
                  },
                });
                requestAgentRuns?.(targetSessionID);
              }
              appendAssistant(
                targetSessionID,
                turnID,
                payload.content || "",
                payload.reasoning || "",
              );
              break;
            }

            case "run_event": {
              if (payload.event) {
                applyRunEvent({ event: payload.event });
              }
              break;
            }

            case "completed": {
              finishTurnInteractions(targetSessionID, turnID);
              if (payload.run) {
                applyRunCompleted({ run: payload.run });
              } else if (payload.run_id) {
                applyRunCompleted({
                  run: {
                    id: payload.run_id,
                    request_id: turnID,
                    session_id: targetSessionID,
                    kind: "chat",
                    status:
                      payload.status === "failed"
                        ? "failed"
                        : payload.status === "paused"
                          ? "paused"
                          : "succeeded",
                    error_message: payload.error,
                    started_at: new Date().toISOString(),
                    finished_at: new Date().toISOString(),
                  },
                });
              }

              const isFailed = payload.status === "failed";
              const isPaused = payload.status === "paused";

              if (payload.request_id && !isFailed && !isPaused) {
                markUserMessageDelivered(targetSessionID, payload.request_id);
              }

              if (isFailed) {
                markAssistantError(
                  targetSessionID,
                  turnID,
                  payload.error || "Subagent continuation failed.",
                );
              } else {
                completeAssistant(
                  targetSessionID,
                  turnID,
                  isPaused ? "paused" : "done",
                  null,
                  payload.content,
                  isPaused ? undefined : payload.reasoning,
                  payload.message_id,
                  payload.run?.id || payload.run_id,
                  assetSummariesForSession(payload.assets, targetSessionID),
                );
              }

              delete requestSessionMapRef.current[turnID];
              if (payload.request_id) {
                delete requestSessionMapRef.current[payload.request_id];
              }
              requestSessions();
              if (targetSessionID === sessionIDRef.current) requestAssets(targetSessionID);
              break;
            }
          }
          break;
        }
        case "assistant_done": {
          const requestID = message.request_id;
          const targetSessionID = resolveTurnSession(message);
          if (!requestID || targetSessionID === null) break;
          const payload = (message.payload || {}) as AssistantDonePayload;
          setSessionLastUsage(targetSessionID, payload.usage || null);
          if (payload.context && (!payload.paused || payload.context.window_k || payload.context.request_context)) {
            setSessionContext(targetSessionID, payload.context, "live");
          }
          if (payload.compact) setSessionCompact(targetSessionID, payload.compact);
          if (payload.plan) applyAssistantPlan(targetSessionID, payload.plan);
          if (payload.retrieval && targetSessionID === sessionIDRef.current) applyKnowledgeSearch(payload.retrieval);
          completeAssistant(
            targetSessionID, requestID, payload.paused ? "paused" : "done", payload.usage || null,
            payload.content, payload.paused ? undefined : payload.reasoning, payload.message_id, undefined,
            assetSummariesForSession(payload.assets, targetSessionID),
          );
          if (payload.paused && payload.message) addEventMessage(targetSessionID, payload.message);
          finishTurnInteractions(targetSessionID, requestID);
          markUserMessageDelivered(targetSessionID, requestID);
          clearSessionPendingRequest(targetSessionID, requestID);
          forgetRequest(requestID);
          requestSessions();
          if (targetSessionID === sessionIDRef.current) {
            requestModels();
            requestSkills();
            requestAssets(targetSessionID);
            requestChanges();
            requestHistory();
          } else if (targetSessionID) {
            addUnreadSession?.(targetSessionID);
          }
          break;
        }
        case "tool_call": {
          const payload = (message.payload || {}) as ToolCallPayload;
          const targetSessionID = resolveTurnSession(message);
          if (targetSessionID === null) break;
          addToolCall(
            targetSessionID,
            payload.name || "tool",
            payload.arguments || "",
            message.request_id,
            payload.tool_call_id,
          );
          break;
        }
        case "tool_result": {
          const payload = (message.payload || {}) as ToolResultPayload;
          const targetSessionID = resolveTurnSession(message);
          if (targetSessionID === null) break;
          addToolResult(
            targetSessionID,
            payload.name || "tool",
            payload.arguments || "",
            payload.result || "",
            Boolean(payload.error) ||
              Boolean(payload.status && !["success", "succeeded", "skipped"].includes(payload.status)),
            message.request_id,
            payload,
            payload.tool_call_id,
          );
          break;
        }
        case "permission_ask": {
          const payload = (message.payload || {}) as PermissionAskPayload;
          const sessionID = resolveTurnSession(message);
          if (!sessionID || !message.request_id || !payload.approval_id || resolvedPermissionIDsRef.current.has(payload.approval_id)) break;
          setSessionPendingPermission(sessionID, {
            approvalID: payload.approval_id,
            requestID: message.request_id,
            sessionID,
            name: payload.name || "tool",
            permission: payload.permission || "",
            arguments: payload.arguments || "",
            requireFreshConsent: payload.require_fresh_consent === true,
          });
          if (sessionID !== sessionIDRef.current) addUnreadSession?.(sessionID);
          break;
        }
        case "permission_result": {
          const payload = (message.payload || {}) as { approval_id?: string; allowed?: boolean };
          const sessionID = resolveTurnSession(message);
          if (!sessionID || !message.request_id || !payload.approval_id) break;
          rememberResolvedUserQuestion(payload.approval_id, resolvedPermissionIDsRef.current);
          delete backgroundPermissionsRef.current[payload.approval_id];
          const pending = getSessionChat(sessionID).pendingPermissions.find((permission) =>
            permission.requestID === message.request_id && permission.approvalID === payload.approval_id);
          if (pending) addEventMessage(sessionID, `${payload.allowed ? "Allowed" : "Denied"} ${pending.name}`);
          removeSessionPendingPermission(sessionID, message.request_id, payload.approval_id);
          break;
        }
        case "user_question": {
          const sessionID = resolveTurnSession(message);
          const payload = (message.payload || {}) as UserQuestionPayload;
          if (sessionID && message.request_id) rememberQuestion(sessionID, message.request_id, payload);
          break;
        }
        case "user_question_resolved": {
          const payload = (message.payload || {}) as UserQuestionResolvedPayload;
          const sessionID = resolveTurnSession(message);
          if (!sessionID || !message.request_id || !payload.question_id) break;
          rememberResolvedUserQuestion(payload.question_id, resolvedUserQuestionIDsRef.current);
          const pending = getSessionChat(sessionID).pendingQuestions.some((question) =>
            question.questionID === payload.question_id && question.requestID === message.request_id);
          removeSessionPendingQuestion(sessionID, payload.question_id, message.request_id);
          if (pending && payload.status && payload.status !== "answered") {
            addEventMessage(sessionID, `用户询问已结束：${payload.status}`);
          }
          break;
        }
        case "user_question_list_result": {
          const sync = questionSyncRef.current;
          if (!sync || message.request_id !== sync.requestID || relayScopeRef.current.connectionID !== sync.connectionID) break;
          questionSyncRef.current = null;
          const payload = (message.payload || {}) as UserQuestionListResultPayload;
          const resolvedIDs = payload.resolved_question_ids || [];
          for (const questionID of resolvedIDs) {
            rememberResolvedUserQuestion(questionID, resolvedUserQuestionIDsRef.current);
            delete backgroundQuestionsRef.current[questionID];
          }
          removePendingQuestions(resolvedIDs);
          for (const item of payload.questions || []) {
            const question = item.question;
            if (!item.request_id || !question || !rememberQuestion(item.session_id, item.request_id, question)) continue;
            const existing = getSessionChat(item.session_id).pendingQuestions.find((pending) =>
              pending.questionID === question.question_id && pending.requestID === item.request_id);
            // Only the submission that preceded this query can be retried.
            if (existing?.submitted && existing === sync.submittedQuestions.get(existing.questionID)) {
              setSessionPendingQuestion(item.session_id, { ...existing, submitted: false });
            }
          }
          break;
        }
        case "session_list_result": {
          if (!acceptSessionList(message)) break;
          const payload = message.payload as SessionListResultPayload;
          applySessionList(payload);
          if (!payload.include_deleted && !pendingHistorySessionIDRef.current && !hasPendingNavigation()) stopPending("sessions");
          break;
        }
        case "session_changed":
        case "session_delete_result":
        case "session_restore_result":
          if (applySessionChanged(message)) {
            if (!pendingHistorySessionIDRef.current && !hasPendingNavigation()) stopPending("sessions");
            if (message.type !== "session_changed") requestDeletedSessions();
          }
          break;
        case "session_history_result":
        case "session_history_meta_result":
        case "session_history_delta_result":
          handleSessionHistoryMessage(message);
          break;
        case "session_mode_set_result":
          // 模式响应已经携带服务端最新 Session。不要立刻请求列表，避免旧列表响应覆盖刚切换的模式。
          stopPending("settings");
          applySessionSettings(
            message.payload as SessionSettingsResultPayload | undefined, message.session_id,
          );
          break;
        case "session_permission_set_result":
        case "session_context_set_result":
        case "session_rag_set_result":
        case "session_compact_result":
          // 设置以服务端返回的 Session 为准，并重新拉取列表，避免本地乐观状态与持久层不一致。
          stopPending("settings");
          applySessionSettings(
            message.payload as SessionSettingsResultPayload | undefined, message.session_id,
          );
          requestSessions();
          break;
        case "session_generation_query_result":
        case "session_generation_set_result":
        case "session_style_set_result": {
          stopPending("generation");
          applySessionGenerationPreferences(message.payload as SessionGenerationResultPayload | undefined);
          break;
        }
        case "session_plan_update": {
          const payload = message.payload as
            | SessionPlanExecuteUpdatePayload
            | undefined;
          const targetSessionID = (
            payload?.session_id ||
            message.session_id ||
            sessionIDRef.current
          ).trim();
          applyAssistantPlan(targetSessionID, payload?.plan);
          if (payload?.message) {
            addEventMessage(targetSessionID, payload.message);
          }
          break;
        }
        case "session_plan_execute_result":
          // 只有 execute_result 才表示整个计划结束，可以释放 Plan 按钮和会话运行状态。
          stopPending("plan");
          applySessionSettings(
            message.payload as SessionSettingsResultPayload | undefined, message.session_id,
          );
          requestSessions();
          break;
        case "session_pause_result": {
          stopPending("pause");
          const payload = (message.payload || {}) as SessionPauseResultPayload;
          const targetSessionID = resolveChatSessionID(
            message,
            requestSessionMapRef,
            sessionIDRef,
          );
          addEventMessage(
            targetSessionID,
            payload.message ||
              (payload.paused
                ? "Session paused."
                : "No running task to pause."),
          );
          if (!payload.paused && targetSessionID) requestAgentRuns?.(targetSessionID);
          if (message.request_id) forgetRequest(message.request_id);
          break;
        }
        case "model_list_result":
          stopPending("models");
          applyModelList(message.payload as ModelListResultPayload | undefined);
          break;
        case "model_switch_result":
          stopPending("models");
          applyModelSwitch(
            message.payload as ModelSwitchResultPayload | undefined,
          );
          requestSessions();
          break;
        case "model_config_add_result":
          stopPending("models");
          applyModelConfigAdd(message.payload as ModelConfigAddResultPayload | undefined);
          break;
        case "model_config_test_result":
          stopPending("models");
          applyModelConfigTest(message.payload as ModelConfigTestResultPayload | undefined);
          break;
        case "model_config_mutation_result":
          if (handleModelDeleteMessage(message)) break;
          stopPending("models");
          applyModelConfigMutation(message.payload as ModelConfigMutationResultPayload | undefined);
          break;
        case "image_model_list_result":
          stopPending("imageModels");
          applyImageModelList(message.payload as ImageModelListResultPayload | undefined);
          break;
        case "image_model_config_add_result":
          stopPending("imageModels");
          applyImageModelConfigAdd(message.payload as ImageModelConfigAddResultPayload | undefined);
          break;
        case "image_model_config_test_result":
          stopPending("imageModels");
          applyImageModelConfigTest(message.payload as ImageModelConfigTestResultPayload | undefined);
          break;
        case "image_model_config_mutation_result":
          stopPending("imageModels");
          applyImageModelConfigMutation(message.payload as ImageModelConfigMutationResultPayload | undefined);
          break;
        case "intent_config_query_result":
          applyIntentConfigQuery(message.payload as IntentConfigResultPayload | undefined, message.request_id);
          break;
        case "intent_config_set_result":
          applyIntentConfigSet(message.payload as IntentConfigResultPayload | undefined, message.request_id);
          break;
        case "intent_config_test_result":
          applyIntentConfigTest(message.payload as IntentConfigTestResultPayload | undefined, message.request_id);
          break;
        case "intent_trace_list_result":
          applyIntentTraceList(message.payload as IntentTraceListResultPayload | undefined, message.request_id);
          break;
        case "intent_trace_get_result":
          applyIntentTraceGet(message.payload as IntentTraceGetResultPayload | undefined, message.request_id);
          break;
        case "intent_trace_clear_result":
          applyIntentTraceClear(message.payload as IntentTraceClearResultPayload | undefined, message.request_id);
          break;
        case "skill_list_result":
        case "skill_reload_result":
          stopPending("skills");
          applySkillList(message.payload as SkillListResultPayload | undefined);
          break;
        case "plugin_list_result":
        case "plugin_reload_result":
        case "plugin_mutation_result":
          stopPending("plugins");
          applyPluginList(message.payload as PluginListResultPayload | PluginMutationResultPayload | undefined);
          break;
        case "asset_list_result":
          if (!acceptAssetList(message)) break;
          stopPending("assets");
          applyAssetList(message.payload as AssetListResultPayload | undefined);
          break;
        case "knowledge_catalog_list_result":
        case "knowledge_catalog_mutation_result":
          stopPending("knowledge");
          applyKnowledgeCatalog(message.payload as KnowledgeCatalogResultPayload | undefined);
          break;
        case "knowledge_document_list_result":
        case "knowledge_document_mutation_result":
          stopPending("knowledge");
          applyKnowledgeDocuments(message.payload as KnowledgeDocumentListResultPayload | undefined);
          break;
        case "knowledge_profile_list_result":
          applyKnowledgeProfiles(message.payload as KnowledgeProfileListResultPayload | undefined);
          break;
        case "knowledge_search_preview_result":
          stopPending("knowledge");
          applyKnowledgeSearch(message.payload as KnowledgeSearchPreviewResultPayload | undefined);
          break;
        case "ai_memory_list_result":
        case "ai_memory_mutation_result":
          stopPending("memory");
          applyAIMemories(message.payload as AIMemoryListResultPayload | undefined);
          break;
        case "ai_memory_candidate_list_result":
          stopPending("memory");
          applyAIMemoryCandidates(message.payload as AIMemoryCandidateListResultPayload | undefined);
          break;
        case "ai_memory_candidate_mutation_result":
          stopPending("memory");
          applyAIMemoryCandidateMutation(message.payload as AIMemoryCandidateMutationResultPayload | undefined);
          break;
        case "ai_memory_extraction_job_list_result":
        case "ai_memory_extraction_job_retry_result":
          stopPending("memory");
          applyAIMemoryExtractionJobs(message.payload as AIMemoryExtractionJobListResultPayload | undefined);
          break;
        case "ai_memory_dream_list_result":
        case "ai_memory_dream_run_result":
          stopPending("memory");
          applyAIMemoryDreamResult(message.payload as AIMemoryDreamResultPayload | undefined);
          break;
        case "subagent_definition_list_result":
          stopPending("subagents");
          applySubagentDefinitionList(message.payload as SubagentDefinitionListResultPayload | undefined);
          break;
        case "subagent_definition_mutation_result":
          stopPending("subagents");
          applySubagentDefinitionMutation(message.payload as SubagentDefinitionMutationResultPayload | undefined);
          break;
        case "subagent_task_list_result":
          stopPending("subagents");
          applySubagentTaskList(message.payload as SubagentTaskListResultPayload | undefined);
          break;
        case "subagent_task_result":
          stopPending("subagents");
          applySubagentTaskResult(message.payload as SubagentTaskResultPayload | undefined);
          break;
        case "subagent_task_wait_result":
          stopPending("subagents");
          applySubagentTaskResult(message.payload as SubagentTaskResultPayload | undefined);
          break;
        case "subagent_task_resume_result":
          stopPending("subagents");
          applySubagentTaskResult(message.payload as SubagentTaskResultPayload | undefined);
          break;
        case "subagent_task_event":
          applySubagentTaskResult(message.payload as SubagentTaskResultPayload | undefined);
          applyRealtimeSubagentTaskEvent?.(message.payload as SubagentTaskResultPayload | undefined);
          break;
        case "subagent_task_detail_result":
          applySubagentTaskDetailResult?.(
            message.payload as SubagentTaskDetailResultPayload | undefined,
            message.request_id,
          );
          break;
        case "subagent_task_events_result":
          applySubagentTaskEventsResult?.(
            message.payload as SubagentTaskEventsResultPayload | undefined,
            message.request_id,
          );
          break;
        case "file_list_result":
          stopPending("files");
          applyFileList(message.payload as FileListResultPayload | undefined);
          break;
        case "file_read_result":
          stopPending("files");
          applyFileRead(message.payload as FileReadResultPayload | undefined);
          break;
        case "changes_list_result":
          stopPending("changes");
          applyChangesList(
            message.payload as ChangesListResultPayload | undefined,
          );
          break;
        case "change_diff_result":
          stopPending("diff");
          applyChangeDiff(
            message.payload as ChangeDiffResultPayload | undefined,
          );
          break;
        case "change_revert_result":
          stopPending("revert");
          applyChangeRevert(
            message.payload as ChangeRevertResultPayload | undefined,
          );
          break;
        case "history_list_result":
          stopPending("history");
          applyHistoryList(
            message.payload as HistoryListResultPayload | undefined,
          );
          break;
        case "history_diff_result":
          stopPending("diff");
          applyHistoryDiff(
            message.payload as HistoryDiffResultPayload | undefined,
          );
          break;
        case "history_revert_result":
          stopPending("revert");
          applyHistoryRevert(
            message.payload as HistoryRevertResultPayload | undefined,
          );
          break;
        case "error": {
          if (handleModelDeleteMessage(message)) break;
          const payload = (message.payload || {}) as ErrorPayload;
          const requestID = message.request_id;
          const type = payload.request_type;
          if (type === "heartbeat") break;
          const sessionID = resolveChatSessionID(message, requestSessionMapRef, sessionIDRef);
          if (type === "permission_result" || type === "user_question_answer") {
            if (type === "user_question_answer") {
              const question = getSessionChat(sessionID).pendingQuestions.find((item) =>
                item.requestID === requestID && item.questionID === payload.question_id);
              if (question?.submitted) {
                setSessionPendingQuestion(sessionID, { ...question, submitted: false });
                requestUserQuestionSync();
              }
            }
            addErrorMessage(sessionID, payload.message || "确认失败，请重试。");
            break;
          }
          if (type === "user_question_list") {
            if (questionSyncRef.current?.requestID === requestID) questionSyncRef.current = null;
            break;
          }
          if (type === "session_list" || type === "agent_run_list" || type === "asset_list") {
            if (handleRemoteQueryError(message)) addErrorMessage(sessionID, payload.message || "无法同步会话。");
            break;
          }
          if (type === "session_history" || type === "session_history_meta" || type === "session_history_delta") {
            if (handleSessionHistoryMessage(message)) addErrorMessage(sessionID, payload.message || "无法同步会话历史。");
            break;
          }
          if (type === "session_load" || type === "session_new" || type === "session_delete" || type === "session_restore") {
            if (applySessionCommandError(requestID)) {
              if (!pendingHistorySessionIDRef.current && !hasPendingNavigation()) stopPending("sessions");
              addErrorMessage(sessionID, payload.message || "会话操作失败。");
            }
            break;
          }
          const text = payload.message || "中继请求失败。";
          const isTurnError = !type || type === "user_message" || type === "session_regenerate" ||
            type === "session_plan_execute" || type === "subagent_task_resume";
          if (isTurnError && requestID) {
            const owner = resolveTurnSession(message);
            if (owner !== null && getSessionChat(owner).pendingRequestID === requestID) {
              markAssistantError(owner, requestID, text);
              finishTurnInteractions(owner, requestID);
              clearSessionPendingRequest(owner, requestID);
              forgetRequest(requestID);
              if (!type || type === "session_plan_execute") stopPending("plan");
              if (!type || type === "subagent_task_resume") stopPending("subagents");
              break;
            }
          }
          if (applyIntentError(text, requestID)) break;
          const action = type && pendingActionForError(type);
          if (action) stopPending(action);
          if (action === "models" && isModelOperationPending) applyModelConfigError(text);
          if (action === "imageModels") applyImageModelConfigError(text);
          if (action === "generation" && isGenerationOperationPending) applySessionGenerationError(text);
          if ((action === "knowledge" || type === "session_rag_set") && isKnowledgeOperationPending) applyKnowledgeError(text);
          if (action === "memory" && isMemoryOperationPending) applyAIMemoryError(text);
          if (type === "session_pause" && requestID) forgetRequest(requestID);
          addErrorMessage(sessionID, text);
          break;
        }
        default:
          addEventMessage(sessionIDRef.current, `Message: ${message.type}`);
      }
    },
    [
      acceptAgentRunList,
      acceptAssetList,
      acceptSessionList,
      handleRemoteQueryError,
      handleSessionHistoryMessage,
      handleAssetUploadMessage,
      activeRequestIDRef,
      addErrorMessage,
      addEventMessage,
      upsertIntentTrace,
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
      applyKnowledgeError,
      applyAIMemories,
      applyAIMemoryCandidates,
      applyAIMemoryCandidateMutation,
      applyAIMemoryExtractionJobs,
      applyAIMemoryDreamResult,
      applyAIMemoryError,
      applyModelList,
      applyModelConfigAdd,
      applyModelConfigTest,
      applyModelConfigMutation,
      applyModelConfigError,
      applyImageModelList,
      applyImageModelConfigAdd,
      applyImageModelConfigTest,
      applyImageModelConfigMutation,
      applyImageModelConfigError,
      handleModelDeleteMessage,
      handleSessionRenameMessage,
      isModelOperationPending,
      applyModelSwitch,
      applySessionChanged,
      applySessionCommandError,
      hasPendingNavigation,
      applySessionList,
      applySessionSettings,
      applySessionGenerationPreferences,
      applySessionGenerationError,
      applySkillList,
      applyPluginList,
      applySubagentDefinitionList,
      applySubagentDefinitionMutation,
      applySubagentTaskList,
      applySubagentTaskResult,
      applySubagentTaskDetailResult,
      applySubagentTaskEventsResult,
      applyRealtimeSubagentTaskEvent,
      clearSessionPendingRequest,
      markUserMessageQueued,
      markUserMessageDelivered,
      completeAssistant,
      currentFilePath,
      getSessionChat,
      pendingHistorySessionIDRef,
      questionSyncRef,
      relayScopeRef,
      hasRunForRequest,
      isGenerationOperationPending,
      isKnowledgeOperationPending,
      isMemoryOperationPending,
      markAssistantError,
      mergeSessionChats,
      mergeSessionRuns,
      addUnreadSession,
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
      requestSkills,
      requestDeletedSessions,
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
    ],
  );
}

function rememberResolvedUserQuestion(questionID: string, resolvedIDs: Set<string>) {
  if (!questionID) {
    return;
  }
  resolvedIDs.add(questionID);
  if (resolvedIDs.size > 256) {
    const oldest = resolvedIDs.values().next().value;
    if (oldest) resolvedIDs.delete(oldest);
  }
}

function resolveChatSessionID(
  message: RelayMessage,
  requestSessionMapRef: RefObject<Record<string, string>>,
  sessionIDRef: RefObject<string>,
) {
  return (
    message.session_id ||
    (message.request_id
      ? requestSessionMapRef.current[message.request_id]
      : "") ||
    sessionIDRef.current ||
    ""
  ).trim();
}

function pendingActionForError(type: RelayMessage["type"]): PendingAction | undefined {
  if (type.startsWith("image_model_")) return "imageModels";
  if (type.startsWith("model_")) return "models";
  if (type.startsWith("skill_")) return "skills";
  if (type.startsWith("plugin_")) return "plugins";
  if (type === "asset_list") return "assets";
  if (type.startsWith("file_")) return "files";
  if (type === "changes_list") return "changes";
  if (type === "history_list") return "history";
  if (type.endsWith("_diff")) return "diff";
  if (type.endsWith("_revert")) return "revert";
  if (type.startsWith("session_generation_") || type === "session_style_set") return "generation";
  if (type === "session_pause") return "pause";
  if (type === "session_plan_execute") return "plan";
  if (type.startsWith("knowledge_")) return "knowledge";
  if (type.startsWith("ai_memory_")) return "memory";
  if (type.startsWith("subagent_")) return "subagents";
  if (type.startsWith("session_")) return "settings";
  return undefined;
}
