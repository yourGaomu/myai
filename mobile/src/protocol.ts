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
};

export type ChangesListResultPayload = {
  repository: boolean;
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
  message?: string;
};
