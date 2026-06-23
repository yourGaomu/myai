import type { RefObject } from "react";
import type { ScrollView } from "react-native";

import type {
  ChangeDiffResultPayload,
  ChangeEntry,
  CompactInfo,
  ContextInfo,
  FileEntry,
  FileReadResultPayload,
  HistoryCheckpoint,
  HistoryDiffResultPayload,
  ModelSummary,
  SkillSummary,
  SessionSummary,
} from "../protocol";
import type { PendingAction, PermissionState, SessionPermissionMode, ViewMode } from "../types/app";
import type { ChatItem } from "../types/chat";
import type { ButtonFeedback } from "../types/ui";

export type MainContentCommonProps = {
  buttonFeedback: ButtonFeedback;
  clientToken: string;
  pendingActions: Record<PendingAction, boolean>;
  viewMode: ViewMode;
};

export type SettingsContentProps = {
  activeModel?: ModelSummary;
  activeSession?: SessionSummary;
  compact?: CompactInfo;
  context?: ContextInfo;
  bindCode: string;
  connected: boolean;
  currentModelID: string;
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
  onRefreshModels: () => void;
  onRefreshSessions: () => void;
  onRefreshSkills: () => void;
  onReloadSkills: () => void;
  onRelayURLChange: (value: string) => void;
  onSetContextWindowK: (windowK: number) => void;
  onSetPermissionMode: (mode: SessionPermissionMode) => void;
  onSwitchModel: (modelID: string) => void;
  onUserIDChange: (value: string) => void;
  relayURL: string;
  sessionID: string;
  sessions: SessionSummary[];
  skillMessage: string;
  skillRoot: string;
  skills: SkillSummary[];
  setupVisible: boolean;
  userID: string;
};

export type ChatContentProps = {
  activeAssistantID: string;
  chatPanelHeight: number;
  chatScrollRef: RefObject<ScrollView | null>;
  messages: ChatItem[];
  pendingHistorySessionID: string;
  pendingRequestID: string;
};

export type FilesContentProps = {
  fileEntries: FileEntry[];
  fileParent: string;
  filePath: string;
  filePreview: FileReadResultPayload | null;
  filePreviewAttached: boolean;
  onAttachFilePreview: () => void;
  onGoToParent: () => void;
  onOpenFileEntry: (entry: FileEntry) => void;
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
  permission: PermissionContentProps;
  sessions: SessionsContentProps;
  settings: SettingsContentProps;
};
