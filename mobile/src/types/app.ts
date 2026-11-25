export type PermissionState = {
  approvalID: string;
  requestID: string;
  sessionID: string;
  name: string;
  permission: string;
  arguments: string;
  requireFreshConsent?: boolean;
};

export type UserQuestionDraftAnswer = {
  selected: string[];
  other: string;
};

export type UserQuestionAnswerSubmission = {
  structuredAnswers: Record<string, import("../protocol").UserQuestionStructuredAnswerPayload>;
  adoptedDefaults?: boolean;
};

export type UserQuestionState = {
  questionID: string;
  requestID: string;
  sessionID: string;
  title: string;
  questions: import("../protocol").UserQuestionItemPayload[];
  sourceTaskID?: string;
  submitted?: boolean;
  submittedSummary?: string;
  draftAnswers?: Record<string, UserQuestionDraftAnswer>;
};

export type RelayScope = { pairingID: string; connectionID: string };

export type QuestionSync = {
  requestID: string;
  connectionID: string;
  submittedQuestions: ReadonlyMap<string, UserQuestionState>;
};

export type SessionPermissionMode = "readonly" | "ask" | "read_write" | "execute" | "full";
export type SessionAgentMode = "chat" | "plan";

export type ViewMode =
  | "chat"
  | "knowledge"
  | "files"
  | "changes"
  | "changeDetail"
  | "sessions"
  | "trash"
  | "settings"
  | "plan";

export type PendingAction =
  | "connect"
  | "pair"
  | "sessions"
  | "models"
  | "imageModels"
  | "skills"
	| "plugins"
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
