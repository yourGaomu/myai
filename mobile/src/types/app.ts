export type PermissionState = {
  requestID: string;
  sessionID: string;
  name: string;
  permission: string;
  arguments: string;
};

export type SessionPermissionMode = "readonly" | "ask" | "full";
export type SessionAgentMode = "chat" | "plan";

export type ViewMode = "chat" | "files" | "changes" | "changeDetail" | "sessions" | "settings" | "plan";

export type PendingAction =
  | "connect"
  | "pair"
  | "sessions"
  | "models"
  | "skills"
  | "settings"
  | "plan"
  | "assets"
  | "files"
  | "changes"
  | "history"
  | "diff"
  | "revert"
  | "upload"
  | "pause";
