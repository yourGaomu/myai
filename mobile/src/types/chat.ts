import type { FileReadResultPayload, TokenUsage, UploadedAssetPayload } from "../protocol";

export type WorkspaceFileAttachment = FileReadResultPayload & {
  kind: "workspace_file";
};

export type UploadedAssetAttachment = UploadedAssetPayload & {
  kind: "uploaded_asset";
  local_uri?: string;
};

export type ChatAttachment = WorkspaceFileAttachment | UploadedAssetAttachment;

export type ChatMessageStatus = "streaming" | "done" | "paused" | "error" | "tool_running";

export type ChatItem = {
  id: string;
  requestID?: string;
  role: "user" | "assistant" | "event" | "error" | "tool_call" | "tool";
  status?: ChatMessageStatus;
  text: string;
  reasoning?: string;
  toolName?: string;
  toolArguments?: string;
  toolError?: string;
  usage?: TokenUsage;
};
