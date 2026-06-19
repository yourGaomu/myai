import type { TokenUsage } from "../protocol";

export type ChatItem = {
  id: string;
  role: "user" | "assistant" | "event" | "error" | "tool_call" | "tool";
  text: string;
  reasoning?: string;
  toolName?: string;
  toolArguments?: string;
  toolError?: string;
  usage?: TokenUsage;
};
