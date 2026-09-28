import { useCallback, useRef, useState } from "react";

import type { TokenUsage, ToolResultPayload } from "../protocol";
import type { PermissionState } from "../types/app";
import type { ChatAttachment, ChatItem, ChatMessageStatus } from "../types/chat";
import { newRequestID } from "../utils/ids";

type SessionChatState = {
  activeAssistantID: string;
  lastUsage: TokenUsage | null;
  messages: ChatItem[];
  pendingPermission: PermissionState | null;
  pendingRequestID: string;
};

const localSessionID = "__local__";

function emptySessionChatState(): SessionChatState {
  return {
    activeAssistantID: "",
    lastUsage: null,
    messages: [],
    pendingPermission: null,
    pendingRequestID: "",
  };
}

function chatSessionKey(sessionID: string) {
  return sessionID.trim() || localSessionID;
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
      const current = sessionChatsRef.current[key] || emptySessionChatState();
      const next = {
        ...sessionChatsRef.current,
        [key]: updater(current),
      };
      commitSessionChats(next);
    },
    [commitSessionChats],
  );

  const getSessionChat = useCallback((sessionID: string) => {
    return (
      sessionChatsRef.current[chatSessionKey(sessionID)] ||
      emptySessionChatState()
    );
  }, []);

  const addMessage = useCallback(
    (sessionID: string, role: ChatItem["role"], text: string, requestID?: string, attachments?: ChatAttachment[]) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        messages: [...current.messages, { attachments, id: newRequestID(), requestID, createdAt: new Date().toISOString(), role, text }],
      }));
    },
    [updateSessionChat],
  );

  const addToolCall = useCallback(
    (
      sessionID: string,
      name: string,
      argumentsText: string,
      requestID?: string,
    ) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        activeAssistantID: "",
        messages: [
          ...current.messages.map((item) =>
            item.role === "assistant" &&
            (item.requestID === requestID ||
              item.id === current.activeAssistantID)
              ? { ...item, status: "tool_running" as ChatMessageStatus }
              : item,
          ),
          {
            id: newRequestID(),
            requestID,
            createdAt: new Date().toISOString(),
            role: "tool_call",
            status: "tool_running",
            text: "",
            toolName: name,
            toolArguments: argumentsText,
          },
        ],
      }));
    },
    [updateSessionChat],
  );

  const addToolResult = useCallback(
    (
      sessionID: string,
      name: string,
      argumentsText: string,
      result: string,
      failed: boolean,
      requestID?: string,
      details?: ToolResultPayload,
    ) => {
      updateSessionChat(sessionID, (current) => {
        let matchedToolCallID = "";
        for (const item of current.messages) {
          if (
            item.role === "tool_call" &&
            item.status === "tool_running" &&
            item.toolName === name &&
            (!argumentsText || (item.toolArguments || "").trim() === argumentsText.trim())
          ) {
            matchedToolCallID = item.id;
            break;
          }
        }
        if (!matchedToolCallID) {
          for (const item of current.messages) {
            if (item.role === "tool_call" && item.status === "tool_running" && item.toolName === name) {
              matchedToolCallID = item.id;
              break;
            }
          }
        }

        return {
          ...current,
          activeAssistantID: "",
          messages: [
            ...current.messages.map((item) => {
              if (item.id === matchedToolCallID) {
                return { ...item, status: "done" as ChatMessageStatus };
              }
              if (
                item.role === "assistant" &&
                (item.requestID === requestID ||
                  item.id === current.activeAssistantID)
              ) {
                return { ...item, status: "streaming" as ChatMessageStatus };
              }
              return item;
            }),
            {
              id: newRequestID(),
              requestID,
              createdAt: new Date().toISOString(),
              role: "tool",
              text: result,
              toolName: name,
              toolArguments: argumentsText,
              toolError: failed ? details?.error_message || result : "",
              toolStatus: details?.status,
              toolErrorCode: details?.error_code,
              toolTruncated: details?.truncated,
            },
          ],
        };
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
        const assistantID =
          findAssistantID(current, requestID) || current.activeAssistantID;
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

  const completeAssistant = useCallback(
    (
      sessionID: string,
      requestID: string | undefined,
      status: ChatMessageStatus,
      usage?: TokenUsage | null,
      content?: string,
      reasoning?: string,
    ) => {
      const completedAt = new Date().toISOString();
      updateSessionChat(sessionID, (current) => {
        const lastUserIndex = current.messages.reduce(
          (acc, msg, idx) => (msg.role === "user" ? idx : acc),
          -1,
        );
        const isInLatestTurn = (idx: number, item: ChatItem) =>
          (requestID && item.requestID === requestID) || idx > lastUserIndex;

        const hasAnyReasoning = Boolean(
          current.messages.some(
            (item, idx) =>
              item.role === "assistant" &&
              isInLatestTurn(idx, item) &&
              Boolean(item.reasoning?.trim()),
          ),
        );
        const assistantID =
          findAssistantID(current, requestID) || current.activeAssistantID;
        if (!assistantID) {
          const updatedExisting = current.messages.map((item, idx) => {
            if (!isInLatestTurn(idx, item)) {
              return item;
            }
            if (item.role === "tool_call" && item.status === "tool_running") {
              return { ...item, status: "done" as ChatMessageStatus };
            }
            if (
              item.role === "assistant" &&
              (item.requestID === requestID ||
                item.status === "streaming" ||
                item.status === "tool_running")
            ) {
              return {
                ...item,
                completedAt: item.completedAt || completedAt,
                status,
                usage: usage || item.usage,
              };
            }
            return item;
          });
          const fallbackReasoning = hasAnyReasoning ? undefined : reasoning || undefined;
          if (!content && !fallbackReasoning && status === "done") {
            return {
              ...current,
              activeAssistantID: "",
              messages: updatedExisting,
            };
          }
          const id = newRequestID();
          return {
            ...current,
            activeAssistantID: "",
            messages: [
              ...updatedExisting,
              {
                completedAt,
                id,
                requestID,
                createdAt: new Date().toISOString(),
                role: "assistant",
                reasoning: fallbackReasoning,
                status,
                text: content || "",
                usage: usage || undefined,
              },
            ],
          };
        }

        return {
          ...current,
          activeAssistantID:
            current.activeAssistantID === assistantID
              ? ""
              : current.activeAssistantID,
          messages: current.messages.map((item, idx) => {
            if (item.id === assistantID) {
              return {
                ...item,
                completedAt,
                requestID: item.requestID || requestID,
                reasoning:
                  item.reasoning ||
                  (hasAnyReasoning ? undefined : reasoning || undefined),
                status,
                text: item.text || content || "",
                usage: usage || item.usage,
              };
            }
            if (isInLatestTurn(idx, item)) {
              if (item.role === "tool_call" && item.status === "tool_running") {
                return { ...item, status: "done" as ChatMessageStatus };
              }
              if (
                item.role === "assistant" &&
                (item.requestID === requestID ||
                  item.status === "streaming" ||
                  item.status === "tool_running")
              ) {
                return {
                  ...item,
                  completedAt: item.completedAt || completedAt,
                  status,
                };
              }
            }
            return item;
          }),
        };
      });
    },
    [updateSessionChat],
  );

  const markAssistantError = useCallback(
    (sessionID: string, requestID: string | undefined, message?: string) => {
      const completedAt = new Date().toISOString();
      updateSessionChat(sessionID, (current) => {
        const lastUserIndex = current.messages.reduce(
          (acc, msg, idx) => (msg.role === "user" ? idx : acc),
          -1,
        );
        const isInLatestTurn = (idx: number, item: ChatItem) =>
          (requestID && item.requestID === requestID) || idx > lastUserIndex;

        const assistantID =
          findAssistantID(current, requestID) || current.activeAssistantID;
        if (!assistantID) {
          const cleanedMessages = current.messages.map((item, idx) => {
            if (!isInLatestTurn(idx, item)) {
              return item;
            }
            if (item.role === "tool_call" && item.status === "tool_running") {
              return { ...item, status: "done" as ChatMessageStatus };
            }
            if (
              item.role === "assistant" &&
              (item.status === "streaming" || item.status === "tool_running")
            ) {
              return { ...item, completedAt: item.completedAt || completedAt, status: "done" as ChatMessageStatus };
            }
            return item;
          });
          const id = newRequestID();
          return {
            ...current,
            activeAssistantID: "",
            messages: [
              ...cleanedMessages,
              {
                completedAt,
                id,
                requestID,
                createdAt: new Date().toISOString(),
                role: "assistant",
                status: "error",
                text: message || "Request failed.",
              },
            ],
          };
        }

        return {
          ...current,
          activeAssistantID:
            current.activeAssistantID === assistantID
              ? ""
              : current.activeAssistantID,
          messages: current.messages.map((item, idx) => {
            if (item.id === assistantID) {
              return {
                ...item,
                completedAt,
                requestID: item.requestID || requestID,
                status: "error",
                text: item.text || message || "Request failed.",
              };
            }
            if (isInLatestTurn(idx, item)) {
              if (item.role === "tool_call" && item.status === "tool_running") {
                return { ...item, status: "done" as ChatMessageStatus };
              }
              if (
                item.role === "assistant" &&
                (item.status === "streaming" || item.status === "tool_running")
              ) {
                return { ...item, completedAt: item.completedAt || completedAt, status: "done" as ChatMessageStatus };
              }
            }
            return item;
          }),
        };
      });
    },
    [updateSessionChat],
  );

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

  const setSessionPendingPermission = useCallback(
    (sessionID: string, permission: PermissionState | null) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        pendingPermission: permission,
      }));
    },
    [updateSessionChat],
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
      const next = {
        ...current,
        [toKey]: {
          ...to,
          activeAssistantID: from.activeAssistantID || to.activeAssistantID,
          lastUsage: from.lastUsage || to.lastUsage,
          messages: mergeMessages(from.messages, to.messages),
          pendingPermission: from.pendingPermission || to.pendingPermission,
          pendingRequestID: from.pendingRequestID || to.pendingRequestID,
        },
      };
      delete next[fromKey];
      commitSessionChats(next);
    },
    [commitSessionChats],
  );

  return {
    addMessage,
    addToolCall,
    addToolResult,
    appendMessages,
    appendAssistant,
    clearMessages,
    clearSessionPendingRequest,
    completeAssistant,
    getSessionChat,
    hasPendingRequest,
    markAssistantError,
    mergeSessionChats,
    replaceMessages,
    resetActiveAssistant,
    setSessionLastUsage,
    setSessionPendingPermission,
    setSessionPendingRequest,
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

function findAssistantID(current: SessionChatState, requestID?: string) {
  if (!requestID) {
    return "";
  }

  for (let index = current.messages.length - 1; index >= 0; index -= 1) {
    const message = current.messages[index];
    if (message.role === "tool_call" || message.role === "tool" || message.role === "user") {
      return "";
    }
    if (message.role === "assistant" && message.requestID === requestID) {
      return message.id;
    }
  }
  return "";
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
