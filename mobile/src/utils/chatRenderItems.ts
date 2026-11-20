import type { ChatAttachment, ChatItem } from "../types/chat";
import type { AgentRunSnapshot, TokenUsage } from "../protocol";
import { parseSharedAsset } from "./toolAssets";

export type ToolCallStep = {
  id: string;
  toolCallID?: string;
  runID?: string;
  name: string;
  arguments?: string;
  result?: string;
  error?: string;
  errorCode?: string;
  truncated?: boolean;
  status: "running" | "completed" | "error";
  toolStatus?: string;
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
  runID?: string;
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

// Ambiguous request ids (for example, Plan steps) cannot identify a Run.
function indexUniqueRuns(runs: AgentRunSnapshot[], keys: (run: AgentRunSnapshot) => Array<string | undefined>) {
  const index = new Map<string, AgentRunSnapshot | null>();
  for (const snapshot of runs) {
    for (const key of new Set(keys(snapshot))) {
      if (key) index.set(key, index.has(key) ? null : snapshot);
    }
  }
  return index;
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

function findToolStep(tools: ToolCallStep[], toolName: string, toolCallID?: string, toolArguments?: string) {
  if (toolCallID) return tools.find((tool) => tool.toolCallID === toolCallID);
  // Only unkeyed legacy rows may be matched by name/arguments.
  const pending = tools.filter((tool) => !tool.toolCallID && tool.status === "running" && tool.name === toolName);
  const args = toolArguments?.trim();
  return (args ? pending.find((tool) => tool.arguments?.trim() === args) : undefined) || pending[0];
}

function updateToolDuration(tool: ToolCallStep) {
  if (!tool.createdAt || !tool.completedAt) return;
  const seconds = (Date.parse(tool.completedAt) - Date.parse(tool.createdAt)) / 1000;
  if (Number.isFinite(seconds) && seconds >= 0) {
    tool.duration = seconds < 10 ? `${seconds.toFixed(1)}s` : `${Math.round(seconds)}s`;
  }
}

function addToolStep(timeline: AgentTurnTimelineStep[], tools: ToolCallStep[], step: ToolCallStep) {
  const existing = step.toolCallID
    ? tools.find((tool) => tool.toolCallID === step.toolCallID)
    : step.status !== "running" ? findToolStep(tools, step.name, undefined, step.arguments) : undefined;
  if (!existing) {
    tools.push(step);
    pushToolStep(timeline, step);
    return;
  }
  existing.runID ||= step.runID;
  existing.arguments ||= step.arguments;
  if (step.status === "running") {
    // A result can precede its call. Take the start time without resetting the result.
    existing.createdAt = step.createdAt || existing.createdAt;
  } else if (existing.status === "running") {
    existing.status = step.status;
    existing.result = step.result;
    existing.error = step.error;
    existing.errorCode = step.errorCode;
    existing.truncated = step.truncated;
    existing.toolStatus = step.toolStatus;
    existing.completedAt = step.completedAt;
  }
  updateToolDuration(existing);
}

function buildTimelineFromRunEvents(snapshot: AgentRunSnapshot): {
  timeline: AgentTurnTimelineStep[];
  tools: ToolCallStep[];
} {
  const timeline: AgentTurnTimelineStep[] = [];
  const tools: ToolCallStep[] = [];
  const sorted = [...snapshot.events].sort((a, b) => a.sequence - b.sequence);

  for (const event of sorted) {
    if (event.type === "reasoning") {
      pushThoughtStep(timeline, `run-thought-${event.id}`, event.content);
      continue;
    }
    if (event.type !== "tool_call" && event.type !== "tool_result") continue;
    const isResult = event.type === "tool_result";
    const failed = isResult && (Boolean(event.error_message || event.error_code) ||
      Boolean(event.status && !["success", "succeeded", "skipped"].includes(event.status)));
    addToolStep(timeline, tools, {
      id: event.id,
      toolCallID: event.tool_call_id,
      runID: snapshot.run.id,
      name: event.tool_name || event.title || "tool",
      arguments: event.arguments,
      status: isResult ? (failed ? "error" : "completed") : "running",
      result: isResult ? event.content || event.error_message || "" : undefined,
      error: failed ? event.error_message || event.content || event.error_code : undefined,
      errorCode: event.error_code,
      truncated: event.truncated,
      toolStatus: event.status,
      createdAt: isResult ? undefined : event.created_at,
      completedAt: isResult ? event.created_at : undefined,
    });
  }

  return { timeline, tools };
}

/**
 * 将平铺的 ChatItem 消息序列聚合为结构化的 Agent Turn 与 User Turn 列表，
 * 并生成「思考段落 ↔ 工具调用」按时间顺序交错的 timeline。
 */
export function buildChatTurns(messages: ChatItem[], runs: AgentRunSnapshot[] = []): ChatRenderItem[] {
  const items: ChatRenderItem[] = [];
  const runsByID = new Map(runs.map((snapshot) => [snapshot.run.id, snapshot]));
  const runsByRequest = indexUniqueRuns(runs, ({ run }) => [run.request_id]);
  const runsByReply = indexUniqueRuns(runs, ({ events }) => events.filter((event) => event.type === "answer").map((event) => event.message_id));
  const runsByTool = indexUniqueRuns(runs, ({ events }) => events.map((event) => event.tool_call_id));
  const runForMessage = (message: ChatItem) =>
    (message.runID ? runsByID.get(message.runID) : undefined) ||
    (message.historyID ? runsByReply.get(message.historyID) : undefined) ||
    (message.toolCallID ? runsByTool.get(message.toolCallID) : undefined) ||
    (message.requestID ? runsByRequest.get(message.requestID) : undefined);
  const replyOwners = new Map<string, ChatItem>();
  const runToolsInChat = new Map<string, Set<string>>();
  const runsWithFinalReply = new Set<string>();
  const isFinalReply = (message: ChatItem) => message.role === "assistant" &&
    (message.finalReply || Boolean(message.historyID && runsByReply.get(message.historyID)));
  for (const message of messages) {
    const snapshot = runForMessage(message);
    if (!snapshot || (message.role !== "assistant" && message.role !== "tool" && message.role !== "tool_call")) continue;
    const runID = snapshot.run.id;
    if (!runsWithFinalReply.has(runID) || isFinalReply(message)) replyOwners.set(runID, message);
    if (isFinalReply(message)) runsWithFinalReply.add(runID);
    if (message.toolCallID) {
      const toolIDs = runToolsInChat.get(runID) || new Set<string>();
      toolIDs.add(message.toolCallID);
      runToolsInChat.set(runID, toolIDs);
    }
  }
  let currentAgentTurn: AgentTurnItem | null = null;
  let lastUserCreatedAt: string | undefined;

  const flushAgentTurn = () => {
    if (!currentAgentTurn) return;

    const turn = currentAgentTurn;
    const matchingRun = turn.runID ? runsByID.get(turn.runID) : undefined;
    const owner = turn.runID ? replyOwners.get(turn.runID) : undefined;
    const ownsRunReply = owner !== undefined && turn.messages.includes(owner);

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
        const isFinalAnswerMessage = isFinalReply(msg) || (idx === finalAssistantIdx && !hasToolsAfterFinalAssistant);

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
      } else if (msg.role === "tool_call" || msg.role === "tool") {
        const isResult = msg.role === "tool";
        const failed = isResult && (Boolean(msg.toolError || msg.toolErrorCode) ||
          Boolean(msg.toolStatus && !["success", "succeeded", "skipped"].includes(msg.toolStatus)));
        addToolStep(timeline, tools, {
          id: msg.id,
          toolCallID: msg.toolCallID,
          runID: runForMessage(msg)?.run.id,
          name: msg.toolName || "tool",
          arguments: msg.toolArguments,
          status: isResult ? (failed ? "error" : "completed") : "running",
          result: isResult ? msg.text : undefined,
          error: msg.toolError,
          errorCode: msg.toolErrorCode,
          truncated: msg.toolTruncated,
          toolStatus: msg.toolStatus,
          createdAt: isResult ? undefined : msg.createdAt,
          completedAt: isResult ? msg.completedAt || msg.createdAt : undefined,
        });
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

    if (isSingleTrailingAssistantAfterTools) {
      const trailingAssistant = turn.messages[assistantIndices[0]];
      const paragraphs = (trailingAssistant.reasoning || "").split(/\n\s*\n/).filter((part) => part.trim());
      const interleaved: AgentTurnTimelineStep[] = [];
      const perTool = Math.ceil(paragraphs.length / tools.length);
      for (const [index, tool] of tools.entries()) {
        const thought = paragraphs.slice(index * perTool, (index + 1) * perTool).join("\n\n");
        pushThoughtStep(interleaved, `hist-thought-${turn.id}-${index}`, thought);
        pushToolStep(interleaved, tool);
      }
      if (interleaved.length > 0) {
        turn.timeline = interleaved;
      } else {
        pushThoughtStep(timeline, `thought-single-${trailingAssistant.id}`, trailingAssistant.reasoning);
      }
    }
    if (turn.timeline.length === 0) turn.timeline = timeline;
    turn.tools = tools;

    if (matchingRun) {
      const fromRun = buildTimelineFromRunEvents(matchingRun);
      const elsewhere = runToolsInChat.get(matchingRun.run.id);
      if (ownsRunReply && tools.length === 0 && !elsewhere?.size && fromRun.tools.length > 0) {
        turn.timeline = fromRun.timeline;
        turn.tools = fromRun.tools;
      } else {
        for (const step of fromRun.tools) {
          if (!step.toolCallID) continue;
          const present = tools.some((tool) => tool.toolCallID === step.toolCallID);
          if (present || (ownsRunReply && !elsewhere?.has(step.toolCallID))) {
            addToolStep(turn.timeline, tools, step);
          }
        }
      }
    }

    const hasFinishedReply = turn.messages.some((message) => isFinalReply(message) ||
      (message.role === "assistant" && ["done", "paused", "error"].includes(message.status || "")));
    if (turn.status === "completed" && !hasFinishedReply && turn.tools.some((tool) => tool.status === "running")) {
      turn.status = "running";
    }
    if (matchingRun?.run.finished_at && turn.status === "running") {
      turn.status = matchingRun.run.status === "paused" ? "paused" : matchingRun.run.status === "failed" ? "error" : "completed";
    }

    turn.text = answerParts.join("\n\n");
    // 运行快照只给拥有该回复的片段补正文，避免把同一答案重复显示在工具片段里。
    if (!turn.text && ownsRunReply && matchingRun) {
      const answers = matchingRun.events.filter((event) => event.type === "answer" && event.content);
      const answer = answers.find((event) => event.message_id && event.message_id === owner?.historyID) ||
        (!runsWithFinalReply.has(matchingRun.run.id) && answers.length === 1 ? answers[0] : undefined);
      if (answer?.content) turn.text = answer.content;
    }

    if (matchingRun?.run.title && (!turn.model || turn.model === "MyAI Agent")) {
      turn.model = matchingRun.run.title;
    }

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
      flushAgentTurn();
      lastUserCreatedAt = message.createdAt;
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

    // Background continuations do not always have a visible user message between them.
    const runID = message.runID || runForMessage(message)?.run.id;
    if (currentAgentTurn) {
      const differentRun = runID && currentAgentTurn.runID && runID !== currentAgentTurn.runID;
      const differentRequest = message.requestID && currentAgentTurn.requestID && message.requestID !== currentAgentTurn.requestID;
      const previousFinalReply = currentAgentTurn.messages.find(isFinalReply);
      const nextReply = isFinalReply(message) && previousFinalReply && message.historyID !== previousFinalReply.historyID;
      const differentKnownTurn = differentRun || differentRequest || nextReply;
      if (differentKnownTurn) flushAgentTurn();
    }

    if (!currentAgentTurn) {
      currentAgentTurn = {
        type: "agent_turn",
        id: `turn-agent-${message.id}`,
        primaryMessageID: message.id,
        requestID: message.requestID,
        runID,
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
    currentAgentTurn.runID ||= runID;
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
