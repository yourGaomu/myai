import { useCallback, useRef, useState } from "react";

import type { AssetSummary, TokenUsage, ToolResultPayload } from "../protocol";
import { mergeAssetSummaries } from "../utils/toolAssets";
import type { PermissionState, UserQuestionState } from "../types/app";
import type { ChatAttachment, ChatItem, ChatMessageStatus } from "../types/chat";
import { attachmentKey } from "../utils/attachments";
import { newRequestID } from "../utils/ids";

export type SessionChatState = {
  activeAssistantID: string;
  historyIDs: ReadonlySet<string>;
  historyConfirmed: boolean;
  finishedRequests: ReadonlySet<string>;
  lastUsage: TokenUsage | null;
  messages: ChatItem[];
  pendingPermission: PermissionState | null;
  pendingPermissions: PermissionState[];
  pendingQuestions: UserQuestionState[];
  pendingRequestID: string;
};

type HistorySource = "cache" | "server";

function emptySessionChatState(): SessionChatState {
  return {
    activeAssistantID: "",
    historyIDs: new Set(),
    historyConfirmed: false,
    finishedRequests: new Set(),
    lastUsage: null,
    messages: [],
    pendingPermission: null,
    pendingPermissions: [],
    pendingQuestions: [],
    pendingRequestID: "",
  };
}

function chatSessionKey(sessionID: string) {
  return sessionID.trim();
}

function lookupSessionChat(chats: Record<string, SessionChatState>, key: string) {
  return Object.prototype.hasOwnProperty.call(chats, key) ? chats[key] : emptySessionChatState();
}

// 按 Session 隔离聊天运行态；Ref 保存大量消息，version 只负责通知 React 重新渲染。
export function useChatMessages() {
  const sessionChatsRef = useRef<Record<string, SessionChatState>>({});
  const [sessionChatsVersion, setSessionChatsVersion] = useState(0);

  const commitSessionChats = useCallback(
    (next: Record<string, SessionChatState>) => {
      sessionChatsRef.current = next;
      setSessionChatsVersion((value) => value + 1);
    },
    [],
  );

  const updateSessionChat = useCallback(
    (
      sessionID: string,
      updater: (current: SessionChatState) => SessionChatState,
    ) => {
      const key = chatSessionKey(sessionID);
      const current = lookupSessionChat(sessionChatsRef.current, key);
      const updated = updater(current);
      if (updated === current) return;
      commitSessionChats({ ...sessionChatsRef.current, [key]: updated });
    },
    [commitSessionChats],
  );

  const getSessionChat = useCallback((sessionID: string) => lookupSessionChat(sessionChatsRef.current, chatSessionKey(sessionID)), []);

  const getSessionChats = useCallback(() => sessionChatsRef.current, []);

  const addMessage = useCallback(
    (sessionID: string, role: ChatItem["role"], text: string, requestID?: string, attachments?: ChatAttachment[]) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        messages: [...current.messages, { attachments, id: newRequestID(), requestID, createdAt: new Date().toISOString(), role, text }],
      }));
    },
    [updateSessionChat],
  );

  const markUserMessageQueued = useCallback(
    (sessionID: string, requestID: string) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        messages: current.messages.map((item) =>
          item.role === "user" && item.requestID === requestID && item.status !== "done"
            ? { ...item, status: "queued" }
            : item,
        ),
      }));
    },
    [updateSessionChat],
  );

  const markUserMessageDelivered = useCallback(
    (sessionID: string, requestID: string) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        messages: current.messages.map((item) =>
          item.role === "user" && item.requestID === requestID
            ? { ...item, status: "done" }
            : item,
        ),
      }));
    },
    [updateSessionChat],
  );

  // 1.0 记录工具调用开始：若包含 toolCallID 则明确保存，为并发子任务及子代理任务提供精确关联标识
  const addToolCall = useCallback(
    (
      sessionID: string,
      name: string,
      argumentsText: string,
      requestID?: string,
      toolCallID?: string,
    ) => {
      updateSessionChat(sessionID, (current) => {
        const sameCall = (item: ChatItem) => Boolean(toolCallID && item.toolCallID === toolCallID && item.requestID === requestID);
        if (current.messages.some((item) => item.role === "tool_call" && sameCall(item))) return current;
        const resultIndex = current.messages.findIndex((item) => item.role === "tool" && sameCall(item));
        if (resultIndex < 0 && requestID && current.finishedRequests.has(requestID)) return current;
        const call: ChatItem = {
          id: newRequestID(),
          requestID,
          createdAt: new Date().toISOString(),
          role: "tool_call",
          status: resultIndex < 0 ? "tool_running" : "done",
          text: "",
          toolName: name,
          toolCallID: toolCallID || undefined,
          toolArguments: argumentsText,
        };
        if (resultIndex >= 0) {
          const messages = [...current.messages];
          messages.splice(resultIndex, 0, call);
          return { ...current, messages };
        }
        return {
          ...current,
          activeAssistantID: keepOtherActiveAssistant(current, requestID),
          messages: [
            ...current.messages.map((item) =>
              item.role === "assistant" && item.requestID === requestID && item.status === "streaming"
                ? { ...item, status: "tool_running" as const }
                : item,
            ),
            call,
          ],
        };
      });
    },
    [updateSessionChat],
  );

  // 2.0 记录工具执行返回：优先通过 toolCallID 精确匹配对应调用，防止多并发任务/工具错配
  const addToolResult = useCallback(
    (
      sessionID: string,
      name: string,
      argumentsText: string,
      result: string,
      failed: boolean,
      requestID?: string,
      details?: ToolResultPayload,
      toolCallID?: string,
    ) => {
      updateSessionChat(sessionID, (current) => {
        const targetToolCallID = toolCallID || details?.tool_call_id;
        const finished = Boolean(requestID && current.finishedRequests.has(requestID));
        const sameCall = (item: ChatItem) => Boolean(targetToolCallID && item.toolCallID === targetToolCallID && item.requestID === requestID);
        if (current.messages.some((item) => item.role === "tool" && sameCall(item))) return current;
        const matchingCall = targetToolCallID
          ? current.messages.find((item) => item.role === "tool_call" && sameCall(item))
          : current.messages.find((item) => item.role === "tool_call" && item.requestID === requestID &&
            item.status === "tool_running" && !item.toolCallID && item.toolName === name &&
            (!argumentsText || item.toolArguments?.trim() === argumentsText.trim()));
        if (finished && (!targetToolCallID || !matchingCall)) return current;
        const message: ChatItem = {
          id: newRequestID(),
          requestID,
          createdAt: new Date().toISOString(),
          role: "tool",
          text: result,
          toolName: name,
          toolCallID: targetToolCallID || undefined,
          toolArguments: argumentsText,
          toolError: failed ? details?.error_message || result : "",
          toolStatus: details?.status,
          toolErrorCode: details?.error_code,
          toolTruncated: details?.truncated,
        };
        const messages = current.messages.map((item) => item === matchingCall ? { ...item, status: "done" as const } : item);
        const finalIndex = finished ? messages.findIndex((item) => item.requestID === requestID && item.finalReply) : -1;
        messages.splice(finalIndex < 0 ? messages.length : finalIndex, 0, message);
        return { ...current, activeAssistantID: keepOtherActiveAssistant(current, requestID), messages };
      });
    },
    [updateSessionChat],
  );

  const appendAssistant = useCallback(
    (
      sessionID: string,
      requestID: string | undefined,
      text: string,
      reasoning = "",
    ) => {
      if (!text && !reasoning) {
        return;
      }

      // 同一阶段（未被 tool_call 隔断）的多个 delta 复用同一个 assistant 条目，被工具调用隔断后开启新段落以保持时间序交错。
      updateSessionChat(sessionID, (current) => {
        if (requestID && current.finishedRequests.has(requestID)) return current;
        const assistantID = findStreamingAssistantID(current, requestID);
        if (!assistantID) {
          const id = newRequestID();
          return {
            ...current,
            activeAssistantID: id,
            messages: [
              ...current.messages,
              {
                id,
                requestID,
                createdAt: new Date().toISOString(),
                role: "assistant",
                reasoning,
                status: "streaming",
                text,
              },
            ],
          };
        }

        return {
          ...current,
          activeAssistantID: assistantID,
          messages: current.messages.map((item) =>
            item.id === assistantID
              ? {
                  ...item,
                  requestID: item.requestID || requestID,
                  status:
                    item.status === "paused" || item.status === "error"
                      ? item.status
                      : "streaming",
                  reasoning: appendText(item.reasoning || "", reasoning),
                  text: appendText(item.text, text),
                }
              : item,
          ),
        };
      });
    },
    [updateSessionChat],
  );

  const completeAssistant = useCallback((
    sessionID: string,
    requestID: string | undefined,
    status: ChatMessageStatus,
    usage?: TokenUsage | null,
    content?: string,
    reasoning?: string,
    messageID?: string,
    runID?: string,
    assets?: AssetSummary[],
  ) => {
    updateSessionChat(sessionID, (current) => finishAssistant(current, requestID, status, { content, reasoning, usage, messageID, runID, assets }));
  }, [updateSessionChat]);

  const markAssistantError = useCallback((sessionID: string, requestID: string | undefined, error?: string) => {
    updateSessionChat(sessionID, (current) => finishAssistant(current, requestID, "error", { error: error || "Request failed." }));
  }, [updateSessionChat]);

  const settleSessionRun = useCallback((sessionID: string, requestID: string, status: "done" | "paused" | "error") => {
    if (!requestID) return;
    updateSessionChat(sessionID, (current) => {
      const completedAt = new Date().toISOString();
      const settled = settleLiveRows(current, requestID, status, completedAt);
      return settled.pendingRequestID === requestID ? { ...settled, pendingRequestID: "" } : settled;
    });
  }, [updateSessionChat]);

  const resetActiveAssistant = useCallback(
    (sessionID: string) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        activeAssistantID: "",
      }));
    },
    [updateSessionChat],
  );

  const clearMessages = useCallback(
    (sessionID: string) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        activeAssistantID: "",
        messages: [],
      }));
    },
    [updateSessionChat],
  );

  const replaceMessages = useCallback(
    (sessionID: string, nextMessages: ChatItem[]) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        activeAssistantID: "",
        messages: nextMessages,
      }));
    },
    [updateSessionChat],
  );

  const mergeSessionHistory = useCallback((sessionID: string, messages: ChatItem[], source: HistorySource) => {
    updateSessionChat(sessionID, (current) => mergeHistory(current, messages, source));
  }, [updateSessionChat]);

  const appendMessages = useCallback(
    (sessionID: string, nextMessages: ChatItem[]) => {
      if (nextMessages.length === 0) {
        return;
      }
      updateSessionChat(sessionID, (current) => ({
        ...current,
        messages: mergeMessages(current.messages, nextMessages),
      }));
    },
    [updateSessionChat],
  );

  const setSessionLastUsage = useCallback(
    (sessionID: string, usage: TokenUsage | null) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        lastUsage: usage,
      }));
    },
    [updateSessionChat],
  );

  const setSessionPendingPermission = useCallback((sessionID: string, permission: PermissionState | null) => {
    // Only a response or terminal turn with the matching identity can remove an approval.
    if (!permission?.approvalID || !permission.requestID || permission.sessionID !== sessionID) return;
    updateSessionChat(sessionID, (current) => {
      const index = current.pendingPermissions.findIndex((item) => samePermission(item, permission));
      const permissions = [...current.pendingPermissions];
      if (index === -1) permissions.push(permission);
      else {
        const existing = permissions[index];
        permissions[index] = { ...permission, requireFreshConsent: existing.requireFreshConsent || permission.requireFreshConsent };
      }
      return { ...current, pendingPermissions: permissions, pendingPermission: permissions[0] || null };
    });
  }, [updateSessionChat]);

  const removeSessionPendingPermission = useCallback((sessionID: string, requestID: string, approvalID?: string) => {
    if (!requestID) return;
    updateSessionChat(sessionID, (current) => {
      const permissions = current.pendingPermissions.filter((item) =>
        item.requestID !== requestID || (approvalID !== undefined && item.approvalID !== approvalID),
      );
      if (permissions.length === current.pendingPermissions.length) return current;
      return { ...current, pendingPermissions: permissions, pendingPermission: permissions[0] || null };
    });
  }, [updateSessionChat]);

  const setSessionPendingQuestion = useCallback((sessionID: string, question: UserQuestionState | null) => {
    if (!question?.questionID || question.sessionID !== sessionID) return;
    updateSessionChat(sessionID, (current) => {
      const existing = current.pendingQuestions.find((item) => sameQuestion(item, question));
      const next = existing ? {
        ...question,
        submitted: question.submitted ?? existing.submitted,
        draftAnswers: question.draftAnswers ?? existing.draftAnswers,
      } : question;
      return { ...current, pendingQuestions: existing
        ? current.pendingQuestions.map((item) => item === existing ? next : item)
        : [...current.pendingQuestions, next] };
    });
  }, [updateSessionChat]);

  const setSessionQuestionDraft = useCallback((question: UserQuestionState, answers: Record<string, string>) => {
    updateSessionChat(question.sessionID, (current) => {
      const existing = current.pendingQuestions.find((item) => sameQuestion(item, question));
      if (!existing || existing.submitted) return current;
      const updated = { ...existing, draftAnswers: { ...answers } };
      return { ...current, pendingQuestions: current.pendingQuestions.map((item) => item === existing ? updated : item) };
    });
  }, [updateSessionChat]);

  const removeSessionPendingQuestion = useCallback((sessionID: string, questionID: string, requestID?: string) => {
    updateSessionChat(sessionID, (current) => {
      const pendingQuestions = current.pendingQuestions.filter((item) =>
        item.questionID !== questionID || (requestID !== undefined && item.requestID !== requestID),
      );
      return pendingQuestions.length === current.pendingQuestions.length ? current : { ...current, pendingQuestions };
    });
  }, [updateSessionChat]);

  const removePendingQuestions = useCallback(
    (questionIDs: string[]) => {
      const removedIDs = new Set(questionIDs);
      if (removedIDs.size === 0) {
        return;
      }
      const current = sessionChatsRef.current;
      let changed = false;
      const next = { ...current };
      for (const [sessionID, chat] of Object.entries(current)) {
        const pendingQuestions = chat.pendingQuestions.filter((item) => !removedIDs.has(item.questionID));
        if (pendingQuestions.length !== chat.pendingQuestions.length) {
          next[sessionID] = { ...chat, pendingQuestions };
          changed = true;
        }
      }
      if (changed) {
        commitSessionChats(next);
      }
    },
    [commitSessionChats],
  );

  const setSessionPendingRequest = useCallback(
    (sessionID: string, requestID: string) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        pendingRequestID: requestID,
      }));
    },
    [updateSessionChat],
  );

  const clearSessionPendingRequest = useCallback(
    (sessionID: string, requestID?: string) => {
      updateSessionChat(sessionID, (current) => {
        if (
          requestID &&
          current.pendingRequestID &&
          current.pendingRequestID !== requestID
        ) {
          return current;
        }
        return { ...current, pendingRequestID: "" };
      });
    },
    [updateSessionChat],
  );

  const hasPendingRequest = useCallback(
    (sessionID: string) => {
      return Boolean(getSessionChat(sessionID).pendingRequestID);
    },
    [getSessionChat],
  );

  const mergeSessionChats = useCallback(
    (fromSessionID: string, toSessionID: string) => {
      const fromKey = chatSessionKey(fromSessionID);
      const toKey = chatSessionKey(toSessionID);
      if (!toSessionID || fromKey === toKey) {
        return;
      }

      const current = sessionChatsRef.current;
      const from = current[fromKey];
      if (!from) {
        return;
      }
      const to = current[toKey] || emptySessionChatState();
      const permissions = [...to.pendingPermissions];
      for (const approval of from.pendingPermissions) {
        const permission = { ...approval, sessionID: toKey };
        if (!permissions.some((existing) => samePermission(existing, permission))) permissions.push(permission);
      }
      const questions = [...to.pendingQuestions];
      for (const pending of from.pendingQuestions) {
        const question = { ...pending, sessionID: toKey };
        if (!questions.some((existing) => sameQuestion(existing, question))) questions.push(question);
      }
      const next = {
        ...current,
        [toKey]: {
          ...to,
          historyIDs: new Set([...to.historyIDs, ...from.historyIDs]),
          finishedRequests: new Set([...to.finishedRequests, ...from.finishedRequests]),
          activeAssistantID: to.activeAssistantID || from.activeAssistantID,
          lastUsage: from.lastUsage || to.lastUsage,
          messages: interleaveHistoryAndLive(to.messages, from.messages),
          pendingPermission: permissions[0] || null,
          pendingPermissions: permissions,
          pendingQuestions: questions,
          pendingRequestID: to.pendingRequestID || from.pendingRequestID,
        },
      };
      delete next[fromKey];
      commitSessionChats(next);
    },
    [commitSessionChats],
  );

  const startBackgroundAssistant = useCallback(
    (sessionID: string, turnID: string) => {
      updateSessionChat(sessionID, (current) => {
        const existing = current.messages.find(
          (m) => m.role === "assistant" && m.requestID === turnID,
        );
        if (existing || current.finishedRequests.has(turnID)) {
          return current;
        }
        const id = newRequestID();
        return {
          ...current,
          activeAssistantID: id,
          messages: [
            ...current.messages,
            {
              id,
              requestID: turnID,
              createdAt: new Date().toISOString(),
              role: "assistant",
              status: "streaming",
              text: "",
            },
          ],
        };
      });
    },
    [updateSessionChat],
  );

  return {
    addMessage,
    markUserMessageQueued,
    markUserMessageDelivered,
    addToolCall,
    addToolResult,
    appendMessages,
    appendAssistant,
    clearMessages,
    clearSessionPendingRequest,
    completeAssistant,
    getSessionChat,
    getSessionChats,
    hasPendingRequest,
    markAssistantError,
    mergeSessionChats,
    mergeSessionHistory,
    replaceMessages,
    resetActiveAssistant,
    setSessionLastUsage,
    setSessionPendingPermission,
    removeSessionPendingPermission,
    setSessionPendingQuestion,
    setSessionQuestionDraft,
    settleSessionRun,
    removeSessionPendingQuestion,
    removePendingQuestions,
    setSessionPendingRequest,
    startBackgroundAssistant,
    sessionChats: sessionChatsRef.current,
    sessionChatsVersion,
  };
}

function appendText(current: string, next: string) {
  if (!next) {
    return current;
  }
  return current + next;
}

function findStreamingAssistantID(current: SessionChatState, requestID?: string) {
  // A tool boundary closes that request's streaming segment. Frames from a
  // different request in the same session do not close or reuse it.
  for (let index = current.messages.length - 1; index >= 0; index--) {
    const item = current.messages[index];
    if (item.requestID !== requestID) continue;
    if (item.role === "assistant") return item.status === "streaming" ? item.id : "";
    if (item.role === "tool_call" || item.role === "tool" || item.role === "user") break;
  }
  return "";
}

function keepOtherActiveAssistant(current: SessionChatState, requestID?: string) {
  const active = current.messages.find((item) => item.id === current.activeAssistantID);
  return active && active.requestID !== requestID ? active.id : "";
}

function rememberFinished(current: ReadonlySet<string>, requestID?: string) {
  if (!requestID || current.has(requestID)) return current;
  const next = new Set(current);
  next.add(requestID);
  if (next.size > 256) next.delete(next.values().next().value!);
  return next;
}

function settleLiveRows(current: SessionChatState, requestID: string | undefined, status: ChatMessageStatus, completedAt: string): SessionChatState {
  const activeAssistantID = keepOtherActiveAssistant(current, requestID);
  const finishedRequests = rememberFinished(current.finishedRequests, requestID);
  let changed = activeAssistantID !== current.activeAssistantID || finishedRequests !== current.finishedRequests;
  const messages = current.messages.map((item) => {
    if (item.requestID !== requestID || !requestID && item.id !== current.activeAssistantID) return item;
    if (item.role === "assistant" && (item.status === "streaming" || item.status === "tool_running")) {
      changed = true;
      return { ...item, status, completedAt };
    }
    if (item.role === "tool_call" && item.status === "tool_running" || item.role === "user" && status === "done" && item.status !== "done") {
      changed = true;
      return { ...item, status: "done" as const, completedAt };
    }
    return item;
  });
  return changed ? { ...current, activeAssistantID, finishedRequests, messages } : current;
}

function finishAssistant(
  current: SessionChatState,
  requestID: string | undefined,
  status: ChatMessageStatus,
  result: { content?: string; reasoning?: string; usage?: TokenUsage | null; error?: string; messageID?: string; runID?: string; assets?: AssetSummary[] },
): SessionChatState {
  const completedAt = new Date().toISOString();
  const settled = settleLiveRows(current, requestID, status, completedAt);
  const { content, reasoning, usage, error, messageID, runID } = result;
  const persisted = messageID ? settled.messages.find((item) => item.role === "assistant" && (item.historyID === messageID || item.id === messageID)) : undefined;
  let live: ChatItem | undefined;
  for (let index = settled.messages.length - 1; index >= 0; index--) {
    const item = settled.messages[index];
    const matchesRequest = requestID ? item.requestID === requestID : item.id === current.activeAssistantID;
    if (!matchesRequest) continue;
    if (item.role === "assistant") {
      if (messageID && item.historyID && item.historyID !== messageID) break;
      live = item;
      break;
    }
    // A final answer after a tool is a different segment. Pausing or failing
    // after the tool still belongs to the partial answer already on screen.
    if (status === "done" && (item.role === "tool_call" || item.role === "tool" || item.role === "user")) break;
  }
  const target = persisted || live;
  const assets = result.assets?.length || persisted?.assets?.length || live?.assets?.length
    ? mergeAssetSummaries(persisted?.assets || [], live?.assets || [], result.assets || [])
    : undefined;
  if (!target && !messageID && !content?.trim() && !reasoning?.trim() && !assets?.length && status === "done") return settled;
  const existingText = (status === "paused" ? live?.text || persisted?.text : persisted?.text || live?.text) || "";
  let text = content?.trim() ? content : existingText;
  if (status === "paused" && existingText.trim() && (!content?.trim() || !content.startsWith(existingText))) text = existingText;
  if (error) {
    const note = `[错误: ${error}]`;
    text = !existingText ? error : existingText === error || existingText.endsWith(note) ? existingText : `${existingText}\n\n${note}`;
  }
  const replacement: ChatItem = {
    ...target,
    assets,
    id: target?.id || messageID || newRequestID(),
    historyID: messageID || target?.historyID,
    finalReply: true,
    requestID: requestID || target?.requestID,
    runID: runID || target?.runID,
    createdAt: target?.createdAt || completedAt,
    completedAt: target?.completedAt || completedAt,
    role: "assistant",
    status,
    text,
    reasoning: reasoning?.trim() ? reasoning : live?.reasoning || target?.reasoning,
    usage: usage || target?.usage,
  };
  let targetIndex = target ? settled.messages.indexOf(target) : -1;
  let insertAfterAnchor = !target;
  if (requestID) {
    // Server timestamps can precede the local optimistic input (clock skew).
    // Keep this reply attached to its request rather than stranding its user
    // message below the reply, or moving a newer turn with it.
    for (let index = settled.messages.length - 1; index > targetIndex; index--) {
      const item = settled.messages[index];
      if (item !== live && item !== persisted && item.requestID === requestID && (status === "done" || item.role === "user")) {
        targetIndex = index;
        insertAfterAnchor = true;
        break;
      }
    }
  }
  if (targetIndex < 0) return { ...settled, messages: [...settled.messages, replacement] };
  const messages = settled.messages.flatMap((item, index) => {
    if (insertAfterAnchor && index === targetIndex) return [item, replacement];
    if (item === target) return insertAfterAnchor ? [] : [replacement];
    if (persisted && item === live) return [];
    return [item];
  });
  return { ...settled, messages };
}

function samePermission(left: PermissionState, right: PermissionState) {
  return left.approvalID === right.approvalID && left.requestID === right.requestID && left.sessionID === right.sessionID;
}

function sameQuestion(left: UserQuestionState, right: UserQuestionState) {
  return left.questionID === right.questionID && left.requestID === right.requestID && left.sessionID === right.sessionID;
}

export function getSubmittedQuestions(chats: Readonly<Record<string, Pick<SessionChatState, "pendingQuestions">>>) {
  const submitted = new Map<string, UserQuestionState>();
  for (const chat of Object.values(chats)) {
    for (const question of chat.pendingQuestions) {
      if (question.submitted) submitted.set(question.questionID, question);
    }
  }
  return submitted;
}

function mergeHistory(current: SessionChatState, incoming: ChatItem[], source: HistorySource): SessionChatState {
  if (source === "cache" && (current.historyConfirmed || current.historyIDs.size > 0 || incoming.length === 0)) return current;
  const historyIDs = new Set<string>();
  const matched = new Set<string>();
  const byHistoryID = new Map(current.messages.map((item) => [item.historyID || item.id, item]));
  const local = current.messages.filter((item) => !item.historyID && !current.historyIDs.has(item.id));
  const history: ChatItem[] = [];
  for (const stored of incoming) {
    const historyID = stored.historyID || stored.id;
    if (historyIDs.has(historyID)) continue;
    historyIDs.add(historyID);
    const existing = byHistoryID.get(historyID) || (source === "server"
      ? local.find((item) => !matched.has(item.id) && canReconcileLegacyRow(current, item, stored))
      : undefined);
    if (!existing) {
      history.push({ ...stored, historyID });
      continue;
    }
    matched.add(existing.id);
    const preserveLive = existing.status === "streaming" || existing.status === "tool_running" || existing.status === "paused" || existing.status === "error";
    history.push({
      ...stored,
      ...preserveLive && existing,
      id: existing.id,
      historyID,
      finalReply: existing.finalReply || stored.finalReply,
      requestID: existing.requestID || stored.requestID,
      runID: existing.runID || stored.runID,
      assets: existing.assets || stored.assets,
      attachments: existing.attachments || stored.attachments,
      completedAt: existing.completedAt || stored.completedAt,
      status: preserveLive ? existing.status : stored.role === "user" && existing.requestID ? "done" : stored.status,
    });
  }
  const overlay = current.messages.filter((item) => !matched.has(item.id) && !current.historyIDs.has(item.historyID || item.id));
  const messages = interleaveHistoryAndLive(history, overlay);
  return { ...current, historyIDs, historyConfirmed: source === "server", messages };
}

function canReconcileLegacyRow(current: SessionChatState, local: ChatItem, stored: ChatItem) {
  if (local.role !== stored.role || local.status === "streaming" || local.status === "tool_running") return false;
  if (local.requestID && current.pendingRequestID === local.requestID) return false;
  if (local.role === "tool_call" || local.role === "tool") return Boolean(local.toolCallID && local.toolCallID === stored.toolCallID);
  if (local.role !== "user" || !local.requestID) return false;
  const localAt = Date.parse(local.createdAt || "");
  const storedAt = Date.parse(stored.createdAt || "");
  if (!Number.isFinite(localAt) || !Number.isFinite(storedAt) || Math.abs(localAt - storedAt) > 5000) return false;
  const localFiles = (local.attachments || []).map(attachmentKey);
  const storedFiles = (stored.attachments || []).map(attachmentKey);
  const sameFiles = localFiles.length === storedFiles.length && localFiles.every((file, index) => file === storedFiles[index]);
  return sameFiles && (local.text.trim() === stored.text.trim() || Boolean(localFiles.length && !local.text.trim()));
}

function interleaveHistoryAndLive(history: ChatItem[], live: ChatItem[]) {
  const messages: ChatItem[] = [];
  let liveIndex = 0;
  for (const item of history) {
    const time = Date.parse(item.createdAt || "");
    while (liveIndex < live.length && Date.parse(live[liveIndex].createdAt || "") < time) {
      messages.push(live[liveIndex++]);
    }
    messages.push(item);
  }
  messages.push(...live.slice(liveIndex));
  return messages;
}

function mergeMessages(fromMessages: ChatItem[], toMessages: ChatItem[]) {
  if (fromMessages.length === 0) {
    return toMessages;
  }
  if (toMessages.length === 0) {
    return fromMessages;
  }

  const seen = new Set(fromMessages.map((message) => message.id));
  return [
    ...fromMessages,
    ...toMessages.filter((message) => !seen.has(message.id)),
  ];
}
