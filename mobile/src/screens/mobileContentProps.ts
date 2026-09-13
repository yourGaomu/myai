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
  ModelSummary,
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
import type { PendingAction, PermissionState, SessionAgentMode, SessionPermissionMode, ViewMode } from "../types/app";
import type { ChatItem } from "../types/chat";
import type { KnowledgeBaseChanges } from "../types/knowledge";
import type { ButtonFeedback } from "../types/ui";
import type { ModelConfigDraft } from "../hooks/useSessionModelActions";

export type MainContentCommonProps = {
  buttonFeedback: ButtonFeedback;
  clientToken: string;
  pendingActions: Record<PendingAction, boolean>;
  viewMode: ViewMode;
};

export type SettingsContentProps = {
  activeModel?: ModelSummary;
  activeSession?: SessionSummary;
  assetBaseURL: string;
  compact?: CompactInfo;
  context?: ContextInfo;
  generation?: SessionGenerationPreferences;
  generationStatus?: { error: boolean; message: string };
  bindCode: string;
  connected: boolean;
  currentModelID: string;
  modelMessage: string;
  modelMessageError: boolean;
  deviceID: string;
  models: ModelSummary[];
  normalizedRelayURL: string;
  onBindCodeChange: (value: string) => void;
  onCloseSettings: () => void;
  onConnect: () => void;
  onDeviceIDChange: (value: string) => void;
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
  onRequestContextInfo: () => void;
  onRequestGenerationPreferences: () => void;
  onRefreshSessions: () => void;
  onRefreshSkills: () => void;
  onReloadSkills: () => void;
  onRefreshPlugins: () => void;
  onReloadPlugins: () => void;
  onSetPluginEnabled: (pluginID: string, enabled: boolean) => void;
  onAssetBaseURLChange: (value: string) => void;
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
  onRefreshSubagents: () => void;
  onUpdateSubagentDefinition: (definition: SubagentDefinition) => boolean;
  onUserIDChange: (value: string) => void;
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
  userID: string;
};

export type PlanContentProps = {
  activeSession?: SessionSummary;
  onExecutePlan: () => void;
  onOpenChat: () => void;
  sessionID: string;
};

export type ChatContentProps = {
  activeAssistantID: string;
  chatScrollRef: RefObject<ScrollView | null>;
  messages: ChatItem[];
  runs: AgentRunSnapshot[];
  onRegenerate: () => void;
  pendingHistorySessionID: string;
  pendingRequestID: string;
  sessionID: string;
};

export type FilesContentProps = {
  assets: AssetSummary[];
  fileEntries: FileEntry[];
  fileParent: string;
  filePath: string;
  filePreview: FileReadResultPayload | null;
  filePreviewAttached: boolean;
  onAttachFilePreview: () => void;
  onGoToParent: () => void;
  onOpenFileEntry: (entry: FileEntry) => void;
  onRefreshAssets: () => void;
  onRefreshFiles: () => void;
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
};

export type KnowledgeContentProps = {
  activeSession?: SessionSummary;
  categories: KnowledgeCategory[];
  documents: KnowledgeDocument[];
  jobs: KnowledgeIndexingJob[];
  knowledgeBases: KnowledgeBase[];
  message: string;
  onCreateCategory: (name: string, parentID?: string) => boolean;
  onCreateKnowledgeBase: (name: string, categoryID?: string, profileID?: string) => boolean;
  onDeleteCategory: (categoryID: string, recursive?: boolean) => boolean;
  onDeleteDocument: (knowledgeBaseID: string, documentID: string) => void;
  onDeleteKnowledgeBase: (knowledgeBaseID: string) => boolean;
  onMoveCategory: (categoryID: string, parentID?: string) => boolean;
  onRefresh: () => void;
  onRefreshDocuments: () => void;
  onRetryDocument: (knowledgeBaseID: string, jobID: string) => void;
  onSearch: (query: string, settings?: RAGSettings) => boolean;
  onSelectKnowledgeBase: (knowledgeBaseID: string) => void;
  onSetRAG: (settings: RAGSettings) => boolean;
  onUpdateKnowledgeBase: (base: KnowledgeBase, changes: KnowledgeBaseChanges) => boolean;
  onUploadDocument: () => void;
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
