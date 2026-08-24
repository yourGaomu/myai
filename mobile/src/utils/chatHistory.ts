import type { SessionHistoryMessage } from "../protocol";
import type { ChatItem } from "../types/chat";
import { displayMessageText, parseAttachedFiles } from "./attachments";
import { newRequestID } from "./ids";

export function historyMessageToChatItem(
  message: SessionHistoryMessage,
): ChatItem {
  const role = chatRoleFromHistory(message.role);
  return {
    id: message.id || newRequestID(),
    attachments: role === "user" ? parseAttachedFiles(message.content || "") : undefined,
    createdAt: message.created_at,
    role,
    status: role === "assistant" ? "done" : undefined,
    text: historyMessageText(message),
    reasoning: message.reasoning,
    toolName: message.tool_name,
    toolArguments: message.tool_arguments,
    toolError:
      message.tool_error ||
      (message.tool_status && message.tool_status !== "success"
        ? message.tool_error_code || message.tool_status
        : undefined),
    toolStatus: message.tool_status,
    toolErrorCode: message.tool_error_code,
    toolTruncated: message.tool_truncated,
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
      return displayMessageText(message.content || "") || (parseAttachedFiles(message.content || "").length > 0 ? "" : "(empty user message)");
    default:
      return (
        message.content ||
        message.tool_arguments ||
        message.tool_error ||
        `(message: ${message.role})`
      );
  }
}
