export type MessageType =
  | "heartbeat"
  | "user_message"
  | "assistant_delta"
  | "assistant_done"
  | "tool_call"
  | "permission_ask"
  | "permission_result"
  | "session_list"
  | "session_list_result"
  | "session_new"
  | "session_load"
  | "session_changed"
  | "file_list"
  | "file_list_result"
  | "file_read"
  | "file_read_result"
  | "changes_list"
  | "changes_list_result"
  | "change_diff"
  | "change_diff_result"
  | "change_revert"
  | "change_revert_result"
  | "history_list"
  | "history_list_result"
  | "history_diff"
  | "history_diff_result"
  | "history_revert"
  | "history_revert_result"
  | "error";

export type RelayMessage<TPayload = unknown> = {
  type: MessageType;
  request_id?: string;
  user_id?: string;
  device_id?: string;
  session_id?: string;
  client_token?: string;
  payload?: TPayload;
};

export type PairResponse = {
  user_id: string;
  device_id: string;
  client_token: string;
};

export type AssistantDeltaPayload = {
  content?: string;
};

export type AssistantDonePayload = {
  content?: string;
};

export type ErrorPayload = {
  message?: string;
};

export type ToolCallPayload = {
  name?: string;
  arguments?: string;
};

export type PermissionAskPayload = {
  name?: string;
  arguments?: string;
  permission?: string;
};

export type PermissionResultPayload = {
  allowed: boolean;
};

export type SessionSummary = {
  id: string;
  title?: string;
  model?: string;
  permission_mode?: string;
  context_window_k?: number;
  created_at?: string;
  updated_at?: string;
};

export type SessionListResultPayload = {
  current_session_id?: string;
  sessions?: SessionSummary[];
};

export type SessionChangedPayload = SessionListResultPayload & {
  session?: SessionSummary;
};

export type FileListPayload = {
  path: string;
  include_hidden?: boolean;
  limit?: number;
};

export type FileEntry = {
  path: string;
  name: string;
  type: "file" | "dir";
  size?: number;
  modified_at?: string;
};

export type FileListResultPayload = {
  path: string;
  parent?: string;
  entries?: FileEntry[];
  count?: number;
  truncated?: boolean;
};

export type FileReadPayload = {
  path: string;
};

export type FileReadResultPayload = {
  path: string;
  name: string;
  language: string;
  content?: string;
  size: number;
  truncated: boolean;
  binary: boolean;
};

export type ChangesListPayload = {
  limit?: number;
};

export type ChangeEntry = {
  path: string;
  old_path?: string;
  status: string;
  index_status?: string;
  worktree_status?: string;
  staged?: boolean;
  unstaged?: boolean;
  untracked?: boolean;
  deleted?: boolean;
  renamed?: boolean;
  restorable?: boolean;
};

export type ChangesListResultPayload = {
  repository: boolean;
  source?: string;
  root?: string;
  entries?: ChangeEntry[];
  count?: number;
  truncated?: boolean;
  clean?: boolean;
  message?: string;
};

export type ChangeDiffPayload = {
  path: string;
};

export type ChangeDiffResultPayload = {
  path: string;
  diff?: string;
  truncated: boolean;
  binary: boolean;
  restorable?: boolean;
  message?: string;
};

export type ChangeRevertPayload = {
  path: string;
};

export type ChangeRevertResultPayload = {
  path: string;
  reverted: boolean;
  message?: string;
};

export type HistoryListPayload = {
  limit?: number;
};

export type HistoryCheckpoint = {
  id: string;
  title?: string;
  reason?: string;
  session_id?: string;
  request_id?: string;
  change_count: number;
  created_at?: string;
};

export type HistoryListResultPayload = {
  root?: string;
  checkpoints?: HistoryCheckpoint[];
  count?: number;
};

export type HistoryDiffPayload = {
  checkpoint_id: string;
};

export type HistoryFileDiff = {
  path: string;
  change_type: string;
  diff?: string;
  truncated: boolean;
  binary: boolean;
  restorable?: boolean;
  message?: string;
};

export type HistoryDiffResultPayload = {
  checkpoint_id: string;
  files?: HistoryFileDiff[];
  count?: number;
  message?: string;
};

export type HistoryRevertPayload = {
  checkpoint_id: string;
};

export type HistoryRevertResultPayload = {
  checkpoint_id: string;
  reverted: boolean;
  paths?: string[];
  message?: string;
};
