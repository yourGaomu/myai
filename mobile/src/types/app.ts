export type PermissionState = {
  requestID: string;
  sessionID: string;
  name: string;
  permission: string;
  arguments: string;
};

export type UserQuestionState = {
  questionID: string;
  requestID: string;
  sessionID: string;
  title: string;
  questions: import("../protocol").UserQuestionItemPayload[];
  sourceTaskID?: string;
  submitted?: boolean;
};

export type SessionPermissionMode = "readonly" | "ask" | "full";
export type SessionAgentMode = "chat" | "plan";

export type ViewMode = "chat" | "knowledge" | "files" | "changes" | "changeDetail" | "sessions" | "settings" | "plan";

export type PendingAction =
  | "connect"
  | "pair"
  | "sessions"
  | "models"
  | "skills"
	| "plugins"
	| "mcp"
  | "settings"
  | "generation"
  | "context"
  | "plan"
  | "assets"
  | "files"
  | "changes"
  | "history"
  | "diff"
  | "revert"
  | "upload"
	| "pause"
	| "knowledge"
	| "memory"
	| "subagents"
	| "intent";
