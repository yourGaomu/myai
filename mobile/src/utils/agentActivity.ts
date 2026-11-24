import type { AgentRunSnapshot } from "../protocol";
import type { ChatItem } from "../types/chat";

export type AgentActivity =
  | "offline"
  | "idle"
  | "thinking"
  | "tool"
  | "permission"
  | "uploading"
  | "subagent"
  | "done";

type AgentActivityInput = {
  connected: boolean;
  messages: ChatItem[];
  pendingPermission: boolean;
  pendingQuestion?: boolean;
  pendingRequestID: string;
  planRunning?: boolean;
  runningSubagentCount?: number;
  runs: AgentRunSnapshot[];
  uploading: boolean;
};

// 根据当前请求与后台运行态计算统一角色活动状态。UI 只消费该稳定语义枚举。
export function agentActivity({
  connected,
  messages,
  pendingPermission,
  pendingQuestion = false,
  pendingRequestID,
  planRunning = false,
  runningSubagentCount = 0,
  runs,
  uploading,
}: AgentActivityInput): AgentActivity {
  if (uploading) {
    return "uploading";
  }
  if (!connected) {
    return "offline";
  }
  if (pendingPermission || pendingQuestion) {
    return "permission";
  }

  if (runningSubagentCount > 0) {
    return "subagent";
  }

  if (!pendingRequestID) {
    return planRunning ? "tool" : "idle";
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
      if (/subagent|agent_spawn|spawn_subagent/i.test(latestEvent.tool_name || "")) {
        return "subagent";
      }
      return "tool";
    }
  }

  const requestMessages = messages.filter((message) => message.requestID === pendingRequestID);
  const lastToolCall = lastIndex(requestMessages, (message) => message.role === "tool_call");
  const lastToolResult = lastIndex(requestMessages, (message) => message.role === "tool");
  if (lastToolCall > lastToolResult) {
    const callToolName = requestMessages[lastToolCall]?.toolName || "";
    if (/subagent|agent_spawn|spawn_subagent/i.test(callToolName)) {
      return "subagent";
    }
    return "tool";
  }

  return "thinking";
}

export function shouldHoldDoneActivity(prev: AgentActivity, next: AgentActivity, latestFailed = false): boolean {
  if (latestFailed || next !== "idle") {
    return false;
  }
  return prev === "thinking" || prev === "tool" || prev === "subagent" || prev === "uploading";
}

function lastIndex<T>(items: T[], matches: (item: T) => boolean) {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (matches(items[index])) {
      return index;
    }
  }
  return -1;
}
