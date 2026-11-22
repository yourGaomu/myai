import type { RefObject } from "react";
import type { ScrollView } from "react-native";

import type {
  ChangeDiffResultPayload,
  AgentRunSnapshot,
  ChangeEntry,
  CompactInfo,
  ContextInfo,
  GenerationSettings,
  SessionGenerationPreferences,
  AssetSummary,
  FileEntry,
  FileReadResultPayload,
  HistoryCheckpoint,
  HistoryDiffResultPayload,
  IntentConfigPayload,
  IntentConfigResultPayload,
  IntentConfigTestResultPayload,
  IntentTracePayload,
  ModelSummary,
  Plan,
  PluginInfo,
  SkillSummary,
  SessionSummary,
  KnowledgeBase,
  KnowledgeCategory,
  KnowledgeDocument,
  KnowledgeIndexProfile,
  KnowledgeIndexingJob,
  KnowledgeSearchPreviewResultPayload,
  AIMemory,
  AIMemoryCandidate,
  AIMemoryDreamRun,
  AIMemoryExtractionJob,
  AIMemoryInput,
  AIMemoryListPayload,
  RAGSettings,
  SubagentDefinition,
  SubagentTaskEvent,
  SubagentTask,
} from "../protocol";
import type { PendingAction, PermissionState, SessionAgentMode, SessionPermissionMode, UserQuestionState, ViewMode } from "../types/app";
import type { ChatItem } from "../types/chat";
import type { DocumentImportStatus, KnowledgeBaseChanges } from "../types/knowledge";
import type { ButtonFeedback } from "../types/ui";
import type { ModelConfigDraft } from "../hooks/useSessionModelActions";
import type { MCPInventory } from "../hooks/useMCPState";
import type { ModelDeleteFeedback } from "../utils/modelDelete";
import type { SettingsSection } from "../components/settings/SettingsHome";
import type { MotionSlot } from "../utils/motionRegistry";

export type MainContentCommonProps = {
  buttonFeedback: ButtonFeedback;
  clientToken: string;
  pendingActions: Record<PendingAction, boolean>;
  viewMode: ViewMode;
};

export type SettingsContentProps = {
  activeSection: SettingsSection | null;
  onSelectSection: (section: SettingsSection | null) => void;
  activeModel?: ModelSummary;
  activeSession?: SessionSummary;
  compact?: CompactInfo;
  context?: ContextInfo;
  generation?: SessionGenerationPreferences;
  generationStatus?: { error: boolean; message: string };
  intentConfig: IntentConfigResultPayload | null;
  intentConfigLoading: boolean;
  intentConfigSaving: boolean;
  intentConfigTesting: boolean;
  intentConfigMessage: string;
  intentConfigError: boolean;
  intentTestResult: IntentConfigTestResultPayload | null;
  intentTraces: IntentTracePayload[];
  intentTraceDetails: Record<string, IntentTracePayload>;
  intentTraceDetailErrors: Record<string, string>;
  intentTraceListLoading: boolean;
  intentTraceClearing: boolean;
  intentTraceDetailLoadingID: string;
  intentTraceMessage: string;
  intentTraceError: boolean;
  bindCode: string;
  connected: boolean;
  connectionStatus: string;
  pairedRelayURL: string;
  pairingError: string;
  pairingPending: boolean;
  currentModelID: string;
  modelDeleteFeedback: ModelDeleteFeedback | null;
  modelMessage: string;
  modelMessageError: boolean;
  deviceID: string;
  models: ModelSummary[];
  normalizedRelayURL: string;
  onBindCodeChange: (value: string) => void;
  onCloseSettings: () => void;
  onConnect: () => void;
  onDeleteSession: (sessionID: string) => void;
  onLoadSession: (sessionID: string) => void;
  onNewSession: () => void;
  onPair: () => void;
  onCompactSession: () => void;
  onExecutePlan: () => void;
  onOpenPlan: () => void;
  onRefreshModels: () => void;
  onAddModelConfig: (config: ModelConfigDraft) => void;
  onUpdateModelConfig: (config: ModelConfigDraft) => void;
  onDeleteModelConfig: (modelID: string) => void;
  onSetModelEnabled: (modelID: string, enabled: boolean) => void;
  onSetDefaultModel: (modelID: string) => void;
  onClearModelMessage: () => void;
  onTestModelConfig: (config: ModelConfigDraft) => void;
  onRequestIntentConfig: () => boolean;
  onSaveIntentConfig: (payload: IntentConfigPayload) => boolean;
  onTestIntentConfig: (payload: IntentConfigPayload) => boolean;
  onRequestIntentTraces: (sessionID?: string, limit?: number) => boolean;
  onRequestIntentTraceDetail: (traceID: string) => boolean;
  onClearIntentTraces: (sessionID?: string) => boolean;
  onClearIntentConfigFeedback: () => void;
  onClearIntentTraceFeedback: () => void;
  onRequestContextInfo: () => void;
  onRequestGenerationPreferences: () => void;
  onRefreshSessions: () => void;
  onRefreshSkills: () => void;
  onReloadSkills: () => void;
  onRefreshPlugins: () => void;
  onReloadPlugins: () => void;
  onRefreshMCPInventory: () => void;
  mcpInventory: MCPInventory;
  reloadMCP: () => void;
  mcpReloading: boolean;
  mcpMessage: string;
  onSetPluginEnabled: (pluginID: string, enabled: boolean) => void;
  onRelayURLChange: (value: string) => void;
  onSetAgentMode: (mode: SessionAgentMode) => void;
  onSetContextWindowK: (windowK: number) => void;
  onSetGenerationSettings: (settings: GenerationSettings) => void;
  onSetPermissionMode: (mode: SessionPermissionMode) => void;
  onSetStyleInstruction: (instruction: string) => void;
  onSwitchModel: (modelID: string) => void;
  onApplySubagentTask: (taskID: string) => void;
  onCancelSubagentTask: (taskID: string) => void;
  onCheckSubagentTask: (taskID: string) => void;
  onWaitSubagentTask: (taskID: string) => void;
  onCreateSubagentDefinition: (definition: Omit<SubagentDefinition, "id"> & { id?: string }) => boolean;
  onDeleteSubagentDefinition: (definitionID: string) => void;
  onDiscardSubagentTask: (taskID: string) => void;
  onFollowupSubagentTask: (taskID: string, message: string) => boolean;
  onMessageSubagentTask: (taskID: string, message: string) => boolean;
  onResumeSubagentTask: (taskID: string) => void;
  onInspectSubagentTask?: (taskID: string) => void;
  onRefreshSubagents: () => void;
  onUpdateSubagentDefinition: (definition: SubagentDefinition) => boolean;
  relayURL: string;
  sessionID: string;
  sessions: SessionSummary[];
  skillMessage: string;
  skillRoot: string;
  skills: SkillSummary[];
  pluginRoot: string;
  pluginMessage: string;
  plugins: PluginInfo[];
  subagentDefinitions: SubagentDefinition[];
  subagentEvents: Record<string, SubagentTaskEvent[]>;
  subagentMessage: string;
  subagentTasks: SubagentTask[];
  setupVisible: boolean;
  unreadSessionIDs?: Record<string, boolean>;
  userID: string;
};

export type PlanContentProps = {
  activeSession?: SessionSummary;
  onExecutePlan: () => void;
  onOpenChat: () => void;
  sessionID: string;
};

export type ChatContentProps = {
  assets: AssetSummary[];
  activeAssistantID: string;
  activePlan?: Plan;
  activitySlot?: MotionSlot;
  changesCount?: number;
  chatScrollRef: RefObject<ScrollView | null>;
  connected: boolean;
  deviceID: string;
  greetingName: string;
  onAnswerUserQuestion?: (question: UserQuestionState, answers: Record<string, string>, cancelled?: boolean) => boolean;
  onChangeUserQuestionDraft?: (question: UserQuestionState, answers: Record<string, string>) => void;
  onExecutePlan?: () => void;
  onNearHeaderChange?: (near: boolean) => void;
  onOpenChangePath?: (path: string) => void;
  onOpenChanges: () => void;
  onOpenPlan?: () => void;
  onPickSuggestion: (text: string) => void;
  onScrolledChange: (scrolled: boolean) => void;
  messages: ChatItem[];
  runs: AgentRunSnapshot[];
  onRegenerate: (turnID: string) => void;
  pendingHistorySessionID: string;
  pendingPlan?: boolean;
  pendingQuestions?: UserQuestionState[];
  pendingRequestID: string;
  sessionID: string;
  subagentTasks?: SubagentTask[];
  onInspectSubagent?: (taskID: string) => void;
  topClearance?: number;
  workspaceChanges?: ChangeEntry[];
};

export type FilesContentProps = {
  assets: AssetSummary[];
  changes?: ChangeEntry[];
  fileEntries: FileEntry[];
  fileParent: string;
  filePath: string;
  filePreview: FileReadResultPayload | null;
  onGoToParent: () => void;
  onOpenFileEntry: (entry: FileEntry) => void;
  previewOpen: boolean;
};

export type ChangesContentProps = {
  canOpenSelectedChangeFile: boolean;
  canRevertSelectedChange: boolean;
  changeDiff: ChangeDiffResultPayload | null;
  changes: ChangeEntry[];
  changesClean: boolean;
  changesMessage: string;
  historyCheckpoints: HistoryCheckpoint[];
  historyDiff: HistoryDiffResultPayload | null;
  historyMessage: string;
  onBackToChanges: () => void;
  onOpenChange: (entry: ChangeEntry) => void;
  onOpenSelectedChangeFile: () => void;
  onPreviewHistory: (checkpointID: string) => void;
  onRefreshChanges: () => void;
  onRefreshHistory: () => void;
  onRevertHistory: (checkpointID: string) => void;
  onRevertSelectedChange: () => void;
  selectedChange: string;
};

export type SessionsContentProps = {
  deletedSessions: SessionSummary[];
  onDeleteSession: (sessionID: string) => void;
  onRefreshDeletedSessions: () => void;
  onRestoreSession: (sessionID: string) => void;
  onSelectSession: (sessionID: string) => void;
  unreadSessionIDs?: Record<string, boolean>;
};

export type KnowledgeContentProps = {
  activeSession?: SessionSummary;
  categories: KnowledgeCategory[];
  documentImportStatus: DocumentImportStatus | null;
  documents: KnowledgeDocument[];
  jobs: KnowledgeIndexingJob[];
  knowledgeBases: KnowledgeBase[];
  message: string;
  onCreateCategory: (name: string, parentID?: string) => boolean;
  onCreateKnowledgeBase: (name: string, categoryID?: string, profileID?: string) => boolean;
  onDeleteCategory: (categoryID: string, recursive?: boolean) => boolean;
  onDeleteDocument: (knowledgeBaseID: string, documentID: string) => void;
  onDeleteKnowledgeBase: (knowledgeBaseID: string) => boolean;
  onDismissDocumentImport: () => void;
  onInsertReference?: (snippet: string) => void;
  onMoveCategory: (categoryID: string, parentID?: string) => boolean;
  onReconcileDocumentImport: () => boolean;
  onRefresh: () => void;
  onRefreshDocuments: () => void;
  onRetryDocument: (knowledgeBaseID: string, jobID: string) => void;
  onSearch: (query: string, settings?: RAGSettings) => boolean;
  onSelectKnowledgeBase: (knowledgeBaseID: string) => void;
  onSetRAG: (settings: RAGSettings) => boolean;
  onUpdateKnowledgeBase: (base: KnowledgeBase, changes: KnowledgeBaseChanges) => boolean;
  onUploadDocument: (knowledgeBaseID: string, knowledgeBaseName?: string) => void;
  onCancelUpload: () => void;
  uploadingDocument: boolean;
  profiles: KnowledgeIndexProfile[];
  searchResult: KnowledgeSearchPreviewResultPayload | null;
  aiMemories: AIMemory[];
  aiCandidates: AIMemoryCandidate[];
  aiDreamRuns: AIMemoryDreamRun[];
  aiExtractionJobs: AIMemoryExtractionJob[];
  aiMemoryMessage: string;
  onApproveAIMemoryCandidate: (candidate: AIMemoryCandidate, memoryID?: string) => boolean;
  onCreateAIMemory: (memory: AIMemoryInput) => boolean;
  onDeleteAIMemory: (memoryID: string) => boolean;
  onRefreshAIMemories: (filter?: AIMemoryListPayload) => boolean;
  onRefreshAIMemoryCandidates: () => boolean;
  onRefreshAIMemoryDreamRuns: () => boolean;
  onRefreshAIMemoryExtractionJobs: () => boolean;
  onRejectAIMemoryCandidate: (candidate: AIMemoryCandidate) => boolean;
  onRestoreAIMemory: (memoryID: string) => boolean;
  onRetryAIMemoryExtractionJob: (job: AIMemoryExtractionJob) => boolean;
  onRunAIMemoryDream: () => boolean;
  onUpdateAIMemory: (memoryID: string, memory: AIMemoryInput) => boolean;
};

export type PermissionContentProps = {
  onAllowPermission: () => void;
  onDenyPermission: () => void;
  pendingPermission: PermissionState | null;
  onAnswerUserQuestion: (question: UserQuestionState, answers: Record<string, string>, cancelled?: boolean) => boolean;
  onChangeUserQuestionDraft?: (question: UserQuestionState, answers: Record<string, string>) => void;
  pendingQuestions: UserQuestionState[];
};

export type MobileMainContentProps = {
  changes: ChangesContentProps;
  chat: ChatContentProps;
  common: MainContentCommonProps;
	files: FilesContentProps;
	knowledge: KnowledgeContentProps;
  permission: PermissionContentProps;
  plan: PlanContentProps;
  sessions: SessionsContentProps;
  settings: SettingsContentProps;
};
