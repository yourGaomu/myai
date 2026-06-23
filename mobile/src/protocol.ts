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
  | "session_delete"
  | "session_delete_result"
  | "session_restore"
  | "session_restore_result"
  | "session_changed"
  | "session_history"
  | "session_history_result"
  | "session_permission_set"
  | "session_permission_set_result"
  | "session_context_set"
  | "session_context_set_result"
  | "session_compact"
  | "session_compact_result"
  | "model_list"
  | "model_list_result"
  | "model_switch"
  | "model_switch_result"
  | "skill_list"
  | "skill_list_result"
  | "skill_reload"
  | "skill_reload_result"
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
  usage?: TokenUsage;
  context?: ContextInfo;
  compact?: CompactInfo;
};

export type TokenUsage = {
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
  reasoning_tokens?: number;
  prompt_cached_tokens?: number;
  available?: boolean;
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
  usage?: TokenUsage;
  last_usage?: TokenUsage;
  deleted?: boolean;
  deleted_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type SessionListResultPayload = {
  current_session_id?: string;
  sessions?: SessionSummary[];
  include_deleted?: boolean;
};

export type SessionChangedPayload = SessionListResultPayload & {
  session?: SessionSummary;
};

export type SessionHistoryPayload = {
  session_id?: string;
};

export type SessionDeletePayload = {
  session_id: string;
};

export type SessionRestorePayload = {
  session_id: string;
};

export type SessionHistoryMessage = {
  id: string;
  role: "user" | "assistant" | "tool_call" | "tool" | string;
  content?: string;
  reasoning?: string;
  tool_call_id?: string;
  tool_name?: string;
  tool_arguments?: string;
  tool_error?: string;
  usage?: TokenUsage;
  created_at?: string;
};

export type SessionHistoryResultPayload = {
  session_id: string;
  messages?: SessionHistoryMessage[];
  count?: number;
};

export type SessionPermissionSetPayload = {
  session_id?: string;
  mode: string;
};

export type SessionContextSetPayload = {
  session_id?: string;
  window_k: number;
};

export type SessionCompactPayload = {
  session_id?: string;
};

export type ContextInfo = {
  window_k?: number;
  full_tokens?: number;
  selected_tokens?: number;
  summary_tokens?: number;
  prefix_tokens?: number;
  cacheable_tokens?: number;
  full_messages?: number;
  selected_messages?: number;
  compacted_messages?: number;
  has_summary?: boolean;
  truncated?: boolean;
  summary_version?: number;
  summary_hash?: string;
  prefix_hash?: string;
};

export type CompactInfo = {
  triggered?: boolean;
  reason?: string;
  before_tokens?: number;
  after_tokens?: number;
  new_messages?: number;
  compacted_messages?: number;
  summary_tokens?: number;
  summary_version?: number;
  summary_hash?: string;
  prefix_hash?: string;
  cacheable_tokens?: number;
};

export type SessionSettingsResultPayload = {
  current_session_id?: string;
  session?: SessionSummary;
  sessions?: SessionSummary[];
  context?: ContextInfo;
  message?: string;
};

export type ModelSummary = {
  id: string;
  name?: string;
  provider?: string;
  model_name?: string;
  enabled?: boolean;
  is_default?: boolean;
};

export type ModelListResultPayload = {
  current_model_id?: string;
  models?: ModelSummary[];
};

export type ModelSwitchPayload = {
  model_id: string;
};

export type ModelSwitchResultPayload = {
  current_model_id?: string;
  models?: ModelSummary[];
  session?: SessionSummary;
  message?: string;
};

export type SkillSummary = {
  name: string;
  description?: string;
  path?: string;
  triggers?: string[];
  updated_at?: string;
};

export type SkillListResultPayload = {
  root?: string;
  skills?: SkillSummary[];
  count?: number;
  reloaded?: boolean;
  message?: string;
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
