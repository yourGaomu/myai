export type PermissionState = {
  requestID: string;
  sessionID: string;
  name: string;
  permission: string;
  arguments: string;
};

export type SessionPermissionMode = "readonly" | "ask" | "full";

export type ViewMode = "chat" | "files" | "changes" | "changeDetail" | "sessions" | "settings";

export type PendingAction =
  | "connect"
  | "pair"
  | "sessions"
  | "models"
  | "skills"
  | "settings"
  | "files"
  | "changes"
  | "history"
  | "diff"
  | "revert";
