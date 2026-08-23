import type { AgentRunSnapshot } from "../protocol";
import type { ChatItem } from "../types/chat";

export type AgentActivity = "offline" | "idle" | "thinking" | "tool" | "permission" | "uploading";

type AgentActivityInput = {
  connected: boolean;
  messages: ChatItem[];
  pendingPermission: boolean;
  pendingRequestID: string;
  runs: AgentRunSnapshot[];
  uploading: boolean;
};

// 根据当前请求最近的运行事件计算头像状态。UI 只消费这个稳定的状态，不直接理解 Relay 事件细节。
export function agentActivity({
  connected,
  messages,
  pendingPermission,
  pendingRequestID,
  runs,
  uploading,
}: AgentActivityInput): AgentActivity {
  if (uploading) {
    return "uploading";
  }
  if (!connected) {
    return "offline";
  }
  if (pendingPermission) {
    return "permission";
  }
  if (!pendingRequestID) {
    return "idle";
  }

  const run = [...runs]
    .reverse()
    .find((snapshot) => snapshot.run.request_id === pendingRequestID && snapshot.run.status === "running");
  if (run) {
    const latestEvent = [...run.events].reverse().find((event) => event.type !== "progress");
    if (latestEvent?.type === "permission") {
      return "permission";
    }
    if (latestEvent?.type === "tool_call") {
      return "tool";
    }
  }

  const requestMessages = messages.filter((message) => message.requestID === pendingRequestID);
  const lastToolCall = lastIndex(requestMessages, (message) => message.role === "tool_call");
  const lastToolResult = lastIndex(requestMessages, (message) => message.role === "tool");
  if (lastToolCall > lastToolResult) {
    return "tool";
  }
  return "thinking";
}

function lastIndex<T>(items: T[], matches: (item: T) => boolean) {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (matches(items[index])) {
      return index;
    }
  }
  return -1;
}
