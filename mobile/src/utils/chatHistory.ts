import type { SessionHistoryMessage } from "../protocol";
import type { ChatItem } from "../types/chat";
import { newRequestID } from "./ids";

export function historyMessageToChatItem(message: SessionHistoryMessage): ChatItem {
  const role = chatRoleFromHistory(message.role);
  return {
    id: message.id || newRequestID(),
    role,
    status: role === "assistant" ? "done" : undefined,
    text: historyMessageText(message),
    reasoning: message.reasoning,
    toolName: message.tool_name,
    toolArguments: message.tool_arguments,
    toolError: message.tool_error,
    usage: message.usage,
  };
}

function chatRoleFromHistory(role?: string): ChatItem["role"] {
  switch (role) {
    case "user":
      return "user";
    case "assistant":
      return "assistant";
    case "tool_call":
      return "tool_call";
    case "tool":
      return "tool";
    default:
      return "event";
  }
}

function historyMessageText(message: SessionHistoryMessage) {
  switch (message.role) {
    case "tool_call":
      return "";
    case "tool":
      return message.content || "";
    case "assistant":
      return message.content || "(empty assistant message)";
    case "user":
      return message.content || "(empty user message)";
    default:
      return message.content || message.tool_arguments || message.tool_error || `(message: ${message.role})`;
  }
}
