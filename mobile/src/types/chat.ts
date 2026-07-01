import type { TokenUsage } from "../protocol";

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
