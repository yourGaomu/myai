import type { ChatAttachment, ChatItem } from "../types/chat";
import type { AgentRunSnapshot, TokenUsage } from "../protocol";
import { parseSharedAsset } from "./toolAssets";

export type ToolCallStep = {
  id: string;
  toolCallID?: string;
  name: string;
  arguments?: string;
  result?: string;
  error?: string;
  errorCode?: string;
  truncated?: boolean;
  status: "running" | "completed" | "error";
  duration?: string;
  createdAt?: string;
  completedAt?: string;
};

export type AgentTurnTimelineStep =
  | {
      type: "thought";
      id: string;
      text: string;
    }
  | {
      type: "tool_group";
      id: string;
      tools: ToolCallStep[];
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
  timeline: AgentTurnTimelineStep[];
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

function formatTurnDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0 || seconds > 86400) {
    return "";
  }
  if (seconds < 1) {
    return "1秒";
  }
  if (seconds < 60) {
    return `${Math.max(1, Math.round(seconds))}秒`;
  }
  if (seconds < 3600) {
    const mins = Math.floor(seconds / 60);
    const secs = Math.round(seconds % 60);
    return secs > 0 ? `${mins}分钟 ${secs}秒` : `${mins}分钟`;
  }
  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  return mins > 0 ? `${hours}小时 ${mins}分钟` : `${hours}小时`;
}

function findMatchingRun(
  turn: AgentTurnItem,
  runs: AgentRunSnapshot[],
  precedingUserAt?: string,
  nextUserAt?: string,
): AgentRunSnapshot | undefined {
  if (!runs || runs.length === 0) {
    return undefined;
  }
  if (turn.requestID) {
    const byReq = runs.find((r) => r.run.request_id === turn.requestID);
    if (byReq) {
      return byReq;
    }
  }
  const userStart = precedingUserAt ? Date.parse(precedingUserAt) : Number.NaN;
  const userEnd = nextUserAt ? Date.parse(nextUserAt) : Number.POSITIVE_INFINITY;
  if (Number.isFinite(userStart)) {
    const byWindow = runs.find(({ run }) => {
      const runStart = Date.parse(run.started_at);
      return Number.isFinite(runStart) && runStart >= userStart - 2000 && runStart < userEnd;
    });
    if (byWindow) {
      return byWindow;
    }
  }
  return undefined;
}

function pushThoughtStep(timeline: AgentTurnTimelineStep[], id: string, rawText?: string) {
  const cleaned = (rawText || "").trim();
  if (!cleaned || cleaned === "(empty assistant message)") {
    return;
  }
  const last = timeline[timeline.length - 1];
  if (last && last.type === "thought") {
    if (!last.text.includes(cleaned)) {
      last.text = `${last.text}\n\n${cleaned}`;
    }
    return;
  }
  timeline.push({
    type: "thought",
    id,
    text: cleaned,
  });
}

function pushToolStep(timeline: AgentTurnTimelineStep[], tool: ToolCallStep) {
  const last = timeline[timeline.length - 1];
  if (last && last.type === "tool_group") {
    last.tools.push(tool);
    return;
  }
  timeline.push({
    type: "tool_group",
    id: `tool-group-${tool.id}`,
    tools: [tool],
  });
}

function findMatchingPendingToolStep(
  tools: ToolCallStep[],
  toolName: string,
  toolCallID?: string,
  toolArguments?: string,
): ToolCallStep | undefined {
  if (toolCallID) {
    const byCallID = tools.find(
      (t) => t.result === undefined && t.toolCallID && t.toolCallID === toolCallID,
    );
    if (byCallID) {
      return byCallID;
    }
  }
  const normalizedArgs = (toolArguments || "").trim();
  if (normalizedArgs) {
    const byNameAndArgs = tools.find(
      (t) =>
        t.result === undefined &&
        t.name === toolName &&
        (t.arguments || "").trim() === normalizedArgs,
    );
    if (byNameAndArgs) {
      return byNameAndArgs;
    }
  }
  return tools.find((t) => t.result === undefined && t.name === toolName);
}

function buildTimelineFromRunEvents(snapshot: AgentRunSnapshot): {
  timeline: AgentTurnTimelineStep[];
  tools: ToolCallStep[];
} {
  const timeline: AgentTurnTimelineStep[] = [];
  const tools: ToolCallStep[] = [];
  const sorted = [...snapshot.events].sort((a, b) => a.sequence - b.sequence);

  for (const ev of sorted) {
    if (ev.type === "reasoning") {
      pushThoughtStep(timeline, `run-thought-${ev.id}`, ev.content);
    } else if (ev.type === "tool_call") {
      const step: ToolCallStep = {
        id: ev.id,
        name: ev.tool_name || ev.title || "tool",
        arguments: ev.arguments,
        status: "running",
        createdAt: ev.created_at,
      };
      tools.push(step);
      pushToolStep(timeline, step);
    } else if (ev.type === "tool_result") {
      const toolName = ev.tool_name || ev.title || "tool";
      const failed =
        Boolean(ev.error_message || ev.error_code) ||
        Boolean(ev.status && ev.status !== "success" && ev.status !== "succeeded");
      const existing = findMatchingPendingToolStep(tools, toolName, undefined, ev.arguments);
      if (existing) {
        existing.result = ev.content || ev.error_message || "";
        existing.error = failed ? ev.error_message || ev.content || ev.error_code : undefined;
        existing.errorCode = ev.error_code;
        existing.truncated = ev.truncated;
        if (!existing.arguments && ev.arguments) {
          existing.arguments = ev.arguments;
        }
        existing.status = failed ? "error" : "completed";
        existing.completedAt = ev.created_at;
        if (existing.createdAt && existing.completedAt) {
          const diff = (Date.parse(existing.completedAt) - Date.parse(existing.createdAt)) / 1000;
          if (Number.isFinite(diff) && diff >= 0) {
            existing.duration = diff < 10 ? `${diff.toFixed(1)}s` : `${Math.round(diff)}s`;
          }
        }
      } else {
        const step: ToolCallStep = {
          id: ev.id,
          name: toolName,
          arguments: ev.arguments,
          result: ev.content || ev.error_message || "",
          error: failed ? ev.error_message || ev.content || ev.error_code : undefined,
          errorCode: ev.error_code,
          truncated: ev.truncated,
          status: failed ? "error" : "completed",
          createdAt: ev.created_at,
          completedAt: ev.created_at,
        };
        tools.push(step);
        pushToolStep(timeline, step);
      }
    }
  }

  return { timeline, tools };
}

/**
 * 将平铺的 ChatItem 消息序列聚合为结构化的 Agent Turn 与 User Turn 列表，
 * 并生成「思考段落 ↔ 工具调用」按时间顺序交错的 timeline。
 */
export function buildChatTurns(messages: ChatItem[], runs: AgentRunSnapshot[] = []): ChatRenderItem[] {
  const items: ChatRenderItem[] = [];
  let currentAgentTurn: AgentTurnItem | null = null;
  let lastUserCreatedAt: string | undefined;

  const flushAgentTurn = (nextUserCreatedAt?: string) => {
    if (!currentAgentTurn) {
      return;
    }

    const turn = currentAgentTurn;
    const matchingRun = findMatchingRun(turn, runs, lastUserCreatedAt, nextUserCreatedAt);

    // 1. 识别本轮所有消息中的工具调用和助手消息分布
    const assistantIndices: number[] = [];
    let firstToolIndex = -1;
    let lastToolIndex = -1;
    turn.messages.forEach((msg, idx) => {
      if (msg.role === "assistant") {
        assistantIndices.push(idx);
      } else if (msg.role === "tool_call" || msg.role === "tool" || isPermissionEvent(msg)) {
        if (firstToolIndex === -1) firstToolIndex = idx;
        lastToolIndex = idx;
      }
    });

    // 判断是否属于 SQLite 历史加载模式（所有 tool_call/tool 在前，唯一一条携带全量多段 reasoning 的 assistant 在末尾）
    const isSingleTrailingAssistantAfterTools =
      assistantIndices.length === 1 &&
      firstToolIndex !== -1 &&
      assistantIndices[0] > firstToolIndex;

    const timeline: AgentTurnTimelineStep[] = [];
    const tools: ToolCallStep[] = [];
    const answerParts: string[] = [];

    // 找到最后一条位于所有工具之后的 assistant 消息索引，其正文作为最终回复，其余中间 assistant 正文视作过程说明
    const finalAssistantIdx =
      assistantIndices.length > 0
        ? assistantIndices[assistantIndices.length - 1]
        : -1;
    const hasToolsAfterFinalAssistant =
      finalAssistantIdx !== -1 && lastToolIndex > finalAssistantIdx;

    for (let idx = 0; idx < turn.messages.length; idx += 1) {
      const msg = turn.messages[idx];
      if (msg.role === "assistant") {
        const cleanText =
          msg.text && msg.text !== "(empty assistant message)" ? msg.text.trim() : "";
        const isFinalAnswerMessage = idx === finalAssistantIdx && !hasToolsAfterFinalAssistant;

        if (!isSingleTrailingAssistantAfterTools && msg.reasoning?.trim()) {
          pushThoughtStep(timeline, `thought-reasoning-${msg.id}`, msg.reasoning);
        }

        if (cleanText) {
          if (isFinalAnswerMessage) {
            answerParts.push(cleanText);
          } else {
            pushThoughtStep(timeline, `thought-text-${msg.id}`, cleanText);
          }
        }
      } else if (msg.role === "tool_call") {
        const step: ToolCallStep = {
          id: msg.id,
          toolCallID: msg.toolCallID,
          name: msg.toolName || "tool",
          arguments: msg.toolArguments,
          status: "running",
          createdAt: msg.createdAt,
        };
        tools.push(step);
        pushToolStep(timeline, step);
      } else if (msg.role === "tool") {
        const toolName = msg.toolName || "tool";
        const existing = findMatchingPendingToolStep(
          tools,
          toolName,
          msg.toolCallID,
          msg.toolArguments,
        );
        if (existing) {
          existing.result = msg.text;
          existing.error = msg.toolError;
          existing.errorCode = msg.toolErrorCode;
          existing.truncated = msg.toolTruncated;
          if (!existing.arguments && msg.toolArguments) {
            existing.arguments = msg.toolArguments;
          }
          existing.status = msg.toolError ? "error" : "completed";
          existing.completedAt = msg.completedAt || msg.createdAt;
          if (existing.createdAt && existing.completedAt) {
            const diff = (Date.parse(existing.completedAt) - Date.parse(existing.createdAt)) / 1000;
            if (Number.isFinite(diff) && diff >= 0) {
              existing.duration = diff < 10 ? `${diff.toFixed(1)}s` : `${Math.round(diff)}s`;
            }
          }
        } else {
          const step: ToolCallStep = {
            id: msg.id,
            toolCallID: msg.toolCallID,
            name: toolName,
            arguments: msg.toolArguments,
            result: msg.text,
            error: msg.toolError,
            errorCode: msg.toolErrorCode,
            truncated: msg.toolTruncated,
            status: msg.toolError ? "error" : "completed",
            createdAt: msg.createdAt,
            completedAt: msg.completedAt || msg.createdAt,
          };
          tools.push(step);
          pushToolStep(timeline, step);
        }
      } else if (isPermissionEvent(msg)) {
        const step: ToolCallStep = {
          id: msg.id,
          name: "权限验证",
          result: msg.text,
          status: msg.text.startsWith("Denied") ? "error" : "completed",
          createdAt: msg.createdAt,
        };
        tools.push(step);
        pushToolStep(timeline, step);
      }
    }

    // 2. 如果是 SQLite 历史加载模式（所有 tool 在前，单条 assistant 在后且含多段 reasoning），
    //    优先使用 matchingRun.events 的精确序列；若无 matchingRun，则将 reasoning 段落与工具步骤按顺序交错编排
    if (isSingleTrailingAssistantAfterTools) {
      const trailingAssistant = turn.messages[assistantIndices[0]];
      const rawReasoning = trailingAssistant.reasoning?.trim() || "";
      const runHasInterleaved =
        matchingRun &&
        matchingRun.events.some((e) => e.type === "reasoning") &&
        matchingRun.events.some((e) => e.type === "tool_call" || e.type === "tool_result");

      if (runHasInterleaved && matchingRun) {
        const fromRun = buildTimelineFromRunEvents(matchingRun);
        if (fromRun.tools.length > 0) {
          turn.timeline = fromRun.timeline;
          turn.tools = fromRun.tools;
        }
      }

      if (turn.timeline.length === 0) {
        const paragraphs = rawReasoning
          ? rawReasoning
              .split(/\n\s*\n/)
              .map((p) => p.trim())
              .filter(Boolean)
          : [];

        if (paragraphs.length > 0 && tools.length > 0) {
          const interleaved: AgentTurnTimelineStep[] = [];
          const P = paragraphs.length;
          const T = tools.length;

          if (P >= T) {
            // 每个工具前分配至少一段思考，剩余思考放在最后一个工具之后
            const perTool = Math.max(1, Math.floor(P / (T + 1)));
            let pCursor = 0;
            for (let tIdx = 0; tIdx < T; tIdx += 1) {
              const take = tIdx === 0 ? Math.max(1, perTool) : perTool;
              const chunk = paragraphs.slice(pCursor, Math.min(P, pCursor + take));
              pCursor += chunk.length;
              if (chunk.length > 0) {
                pushThoughtStep(
                  interleaved,
                  `hist-thought-${turn.id}-${tIdx}`,
                  chunk.join("\n\n"),
                );
              }
              pushToolStep(interleaved, tools[tIdx]);
            }
            if (pCursor < P) {
              pushThoughtStep(
                interleaved,
                `hist-thought-${turn.id}-tail`,
                paragraphs.slice(pCursor).join("\n\n"),
              );
            }
          } else {
            // 思考段落少于工具数量：每段思考后跟随若干个工具调用
            const toolsPerPara = Math.ceil(T / P);
            let tCursor = 0;
            for (let pIdx = 0; pIdx < P; pIdx += 1) {
              pushThoughtStep(
                interleaved,
                `hist-thought-${turn.id}-${pIdx}`,
                paragraphs[pIdx],
              );
              const sliceEnd = pIdx === P - 1 ? T : Math.min(T, tCursor + toolsPerPara);
              while (tCursor < sliceEnd) {
                pushToolStep(interleaved, tools[tCursor]);
                tCursor += 1;
              }
            }
            while (tCursor < T) {
              pushToolStep(interleaved, tools[tCursor]);
              tCursor += 1;
            }
          }

          turn.timeline = interleaved;
          turn.tools = tools;
        } else {
          if (rawReasoning) {
            pushThoughtStep(timeline, `thought-single-${trailingAssistant.id}`, rawReasoning);
          }
          turn.timeline = timeline;
          turn.tools = tools;
        }
      }
    } else if (tools.length === 0 && matchingRun && matchingRun.events.length > 0) {
      // 3. 如果 messages 中尚无工具记录，但 matchingRun.events 中有工具事件，从 run 事件回填
      const fromRun = buildTimelineFromRunEvents(matchingRun);
      if (fromRun.timeline.length > 0) {
        turn.timeline = fromRun.timeline;
        turn.tools = fromRun.tools;
      } else {
        turn.timeline = timeline;
        turn.tools = tools;
      }
    } else {
      turn.timeline = timeline;
      turn.tools = tools;
    }

    const hasFinishedAssistant = turn.messages.some(
      (m) =>
        m.role === "assistant" &&
        (m.status === "done" || m.status === "paused" || m.status === "error"),
    );
    if (!hasFinishedAssistant && turn.tools.some((t) => t.status === "running")) {
      turn.status = "running";
    }

    turn.text = answerParts.join("\n\n");

    // 4. 计算整轮耗时（优先使用 AgentRun 的 started_at/finished_at，其次使用消息时间戳区间）
    const startCandidates: number[] = [];
    const endCandidates: number[] = [];

    if (matchingRun?.run.started_at) {
      const t = Date.parse(matchingRun.run.started_at);
      if (Number.isFinite(t)) startCandidates.push(t);
    }
    if (matchingRun?.run.finished_at) {
      const t = Date.parse(matchingRun.run.finished_at);
      if (Number.isFinite(t)) endCandidates.push(t);
    }
    if (lastUserCreatedAt) {
      const t = Date.parse(lastUserCreatedAt);
      if (Number.isFinite(t)) startCandidates.push(t);
    }
    for (const msg of turn.messages) {
      if (msg.createdAt) {
        const t = Date.parse(msg.createdAt);
        if (Number.isFinite(t)) {
          startCandidates.push(t);
          endCandidates.push(t);
        }
      }
      if (msg.completedAt) {
        const t = Date.parse(msg.completedAt);
        if (Number.isFinite(t)) {
          endCandidates.push(t);
        }
      }
    }

    if (turn.status === "running") {
      const startMs = startCandidates.length > 0 ? Math.min(...startCandidates) : Number.NaN;
      if (Number.isFinite(startMs)) {
        const seconds = Math.max(1, (Date.now() - startMs) / 1000);
        if (seconds < 7200) {
          turn.elapsed = formatTurnDuration(seconds);
        }
      }
    } else if (startCandidates.length > 0 && endCandidates.length > 0) {
      const startMs = Math.min(...startCandidates);
      const endMs = Math.max(...endCandidates);
      const seconds = (endMs - startMs) / 1000;
      if (seconds >= 0.5 && seconds <= 7200) {
        turn.elapsed = formatTurnDuration(seconds);
      }
    }

    items.push(turn);
    currentAgentTurn = null;
  };

  for (const message of messages) {
    if (message.role === "user") {
      flushAgentTurn(message.createdAt);
      lastUserCreatedAt = message.createdAt;
      items.push({
        type: "user_turn",
        id: message.id,
        message,
      });
      continue;
    }

    if (message.role === "event" && !isPermissionEvent(message)) {
      flushAgentTurn(message.createdAt);
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
        timeline: [],
        text: "",
        attachments: [],
        startedAt: message.createdAt,
        completedAt: message.completedAt,
        messages: [],
      };
    }

    currentAgentTurn.messages.push(message);
    if (!currentAgentTurn.requestID && message.requestID) {
      currentAgentTurn.requestID = message.requestID;
    }
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
    } else if (message.role === "tool_call" && message.status === "tool_running") {
      currentAgentTurn.status = "running";
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
