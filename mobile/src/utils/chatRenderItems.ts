import type { ChatAttachment, ChatItem } from "../types/chat";
import type { TokenUsage } from "../protocol";
import { parseSharedAsset } from "./toolAssets";

export type ToolCallStep = {
  id: string;
  name: string;
  arguments?: string;
  result?: string;
  error?: string;
  status: "running" | "completed" | "error";
  duration?: string;
  createdAt?: string;
  completedAt?: string;
};

export type AgentTurnItem = {
  type: "agent_turn";
  id: string;
  primaryMessageID: string;
  requestID?: string;
  model?: string;
  status: "running" | "completed" | "error" | "paused";
  reasoning?: string;
  tools: ToolCallStep[];
  text: string;
  attachments?: ChatAttachment[];
  usage?: TokenUsage;
  startedAt?: string;
  completedAt?: string;
  elapsed?: string;
  canRegenerate?: boolean;
  messages: ChatItem[];
};

export type UserTurnItem = {
  type: "user_turn";
  id: string;
  message: ChatItem;
};

export type EventRenderItem = {
  type: "event";
  id: string;
  message: ChatItem;
};

export type ToolActivityGroupItem = {
  id: string;
  messages: ChatItem[];
  names: string[];
  callCount: number;
  resultCount: number;
  failedCount: number;
  permissionCount: number;
  assetCount: number;
};

export type LegacyMessageItem = {
  type: "message";
  id: string;
  message: ChatItem;
};

export type LegacyToolGroupItem = {
  type: "tool_group";
  id: string;
  group: ToolActivityGroupItem;
};

export type ChatRenderItem =
  | UserTurnItem
  | AgentTurnItem
  | EventRenderItem
  | LegacyMessageItem
  | LegacyToolGroupItem;

/**
 * 将平铺的 ChatItem 消息序列聚合为结构化的 Agent Turn 与 User Turn 列表
 */
export function buildChatTurns(messages: ChatItem[]): ChatRenderItem[] {
  const items: ChatRenderItem[] = [];
  let currentAgentTurn: AgentTurnItem | null = null;

  const flushAgentTurn = () => {
    if (currentAgentTurn) {
      // 耗时计算：区分实时运行与历史消息，严禁对已完成历史消息使用 Date.now() 导致出现数万秒
      if (currentAgentTurn.status === "running") {
        if (currentAgentTurn.startedAt) {
          const start = Date.parse(currentAgentTurn.startedAt);
          if (Number.isFinite(start)) {
            const seconds = Math.max(0.1, (Date.now() - start) / 1000);
            if (seconds < 300) {
              currentAgentTurn.elapsed = seconds < 10 ? `${seconds.toFixed(1)}s` : `${Math.round(seconds)}s`;
            }
          }
        }
      } else if (currentAgentTurn.status === "completed") {
        if (currentAgentTurn.startedAt && currentAgentTurn.completedAt) {
          const start = Date.parse(currentAgentTurn.startedAt);
          const end = Date.parse(currentAgentTurn.completedAt);
          if (Number.isFinite(start) && Number.isFinite(end) && end >= start) {
            const seconds = (end - start) / 1000;
            // 只有处于合理 AI 生成区间（如 0.1s ~ 120s）才展示具体秒数，避免跨夜或无效时间差
            if (seconds >= 0.1 && seconds <= 120) {
              currentAgentTurn.elapsed = seconds < 10 ? `${seconds.toFixed(1)}s` : `${Math.round(seconds)}s`;
            }
          }
        } else if (currentAgentTurn.tools && currentAgentTurn.tools.length > 0) {
          // 若无整轮完成时间但有工具步骤耗时，累加工具耗时
          const validToolSecs = currentAgentTurn.tools
            .map((t) => (t.duration ? parseFloat(t.duration) : 0))
            .filter((d) => !Number.isNaN(d) && d > 0);
          if (validToolSecs.length > 0) {
            const total = validToolSecs.reduce((a, b) => a + b, 0);
            if (total > 0 && total <= 120) {
              currentAgentTurn.elapsed = total < 10 ? `${total.toFixed(1)}s` : `${Math.round(total)}s`;
            }
          }
        }
      }

      items.push(currentAgentTurn);
      currentAgentTurn = null;
    }
  };

  for (const message of messages) {
    if (message.role === "user") {
      flushAgentTurn();
      items.push({
        type: "user_turn",
        id: message.id,
        message,
      });
      continue;
    }

    if (message.role === "event" && !isPermissionEvent(message)) {
      flushAgentTurn();
      items.push({
        type: "event",
        id: message.id,
        message,
      });
      continue;
    }

    // Assistant, tool_call, tool, or permission event -> merge into current Agent Turn
    if (!currentAgentTurn) {
      currentAgentTurn = {
        type: "agent_turn",
        id: `turn-agent-${message.id}`,
        primaryMessageID: message.id,
        requestID: message.requestID,
        model: "MyAI Agent",
        status: "completed",
        reasoning: "",
        tools: [],
        text: "",
        attachments: [],
        startedAt: message.createdAt,
        completedAt: message.completedAt,
        messages: [],
      };
    }

    currentAgentTurn.messages.push(message);
    if (message.createdAt && (!currentAgentTurn.startedAt || message.createdAt < currentAgentTurn.startedAt)) {
      currentAgentTurn.startedAt = message.createdAt;
    }
    if (message.completedAt) {
      currentAgentTurn.completedAt = message.completedAt;
    }

    if (message.role === "assistant") {
      if (message.reasoning) {
        currentAgentTurn.reasoning = currentAgentTurn.reasoning
          ? `${currentAgentTurn.reasoning}\n\n${message.reasoning}`
          : message.reasoning;
      }
      if (message.text) {
        currentAgentTurn.text = currentAgentTurn.text
          ? `${currentAgentTurn.text}\n\n${message.text}`
          : message.text;
      }
      if (message.usage) {
        currentAgentTurn.usage = message.usage;
      }
      if (message.attachments?.length) {
        currentAgentTurn.attachments = [
          ...(currentAgentTurn.attachments || []),
          ...message.attachments,
        ];
      }
      if (message.status === "streaming" || message.status === "tool_running") {
        currentAgentTurn.status = "running";
      } else if (message.status === "error" || message.status === "paused") {
        currentAgentTurn.status = message.status;
        currentAgentTurn.canRegenerate = true;
      }
    } else if (message.role === "tool_call") {
      const toolStep: ToolCallStep = {
        id: message.id,
        name: message.toolName || "tool",
        arguments: message.toolArguments,
        status: "running",
        createdAt: message.createdAt,
      };
      currentAgentTurn.tools.push(toolStep);
      if (message.status === "tool_running") {
        currentAgentTurn.status = "running";
      }
    } else if (message.role === "tool") {
      const existing = [...currentAgentTurn.tools].reverse().find(
        (t) => t.name === (message.toolName || "tool") && !t.result,
      );
      if (existing) {
        existing.result = message.text;
        existing.error = message.toolError;
        existing.status = message.toolError ? "error" : "completed";
        existing.completedAt = message.completedAt || message.createdAt;
        if (existing.createdAt && existing.completedAt) {
          const diff = (Date.parse(existing.completedAt) - Date.parse(existing.createdAt)) / 1000;
          if (Number.isFinite(diff) && diff >= 0) {
            existing.duration = `${diff.toFixed(1)}s`;
          }
        }
      } else {
        currentAgentTurn.tools.push({
          id: message.id,
          name: message.toolName || "tool",
          result: message.text,
          error: message.toolError,
          status: message.toolError ? "error" : "completed",
          createdAt: message.createdAt,
          completedAt: message.completedAt,
        });
      }
    } else if (isPermissionEvent(message)) {
      currentAgentTurn.tools.push({
        id: message.id,
        name: "权限验证",
        result: message.text,
        status: message.text.startsWith("Denied") ? "error" : "completed",
      });
    }
  }

  flushAgentTurn();
  return items;
}

/**
 * 保持旧版工具折叠方法的兼容性
 */
export function groupToolActivity(messages: ChatItem[]): ChatRenderItem[] {
  const items: ChatRenderItem[] = [];
  let group: ChatItem[] = [];

  const flushGroup = () => {
    if (group.length === 0) {
      return;
    }

    const first = group[0];
    const last = group[group.length - 1];
    const names = uniqueToolNames(group);
    const failedCount = group.filter((message) => Boolean(message.toolError)).length;
    const callCount = group.filter((message) => message.role === "tool_call").length;
    const resultCount = group.filter((message) => message.role === "tool").length;
    const permissionCount = group.filter(isPermissionEvent).length;
    const assetCount = group.filter((message) => Boolean(parseSharedAsset(message.toolName, message.text))).length;

    items.push({
      type: "tool_group",
      id: `tool-group-${first.id}-${last.id}`,
      group: {
        id: `tool-group-${first.id}-${last.id}`,
        messages: group,
        names,
        callCount,
        resultCount,
        failedCount,
        permissionCount,
        assetCount,
      },
    });
    group = [];
  };

  messages.forEach((message) => {
    if (isToolActivityMessage(message)) {
      group.push(message);
      return;
    }

    flushGroup();
    items.push({ type: "message", id: message.id, message });
  });

  flushGroup();
  return items;
}

export function isPermissionEvent(message: ChatItem) {
  return message.role === "event" && /^(Allowed|Denied)\s+\S+/.test(message.text.trim());
}

function isToolActivityMessage(message: ChatItem) {
  return message.role === "tool_call" || message.role === "tool" || isPermissionEvent(message);
}

function uniqueToolNames(messages: ChatItem[]) {
  const names: string[] = [];
  messages.forEach((message) => {
    const name = message.toolName?.trim();
    if (name && !names.includes(name)) {
      names.push(name);
    }
  });
  return names;
}
