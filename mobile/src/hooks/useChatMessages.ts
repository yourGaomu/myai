import { useCallback, useRef, useState } from "react";

import type { TokenUsage } from "../protocol";
import type { PermissionState } from "../types/app";
import type { ChatItem, ChatMessageStatus } from "../types/chat";
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

export function useChatMessages() {
  const sessionChatsRef = useRef<Record<string, SessionChatState>>({});
  const [sessionChatsVersion, setSessionChatsVersion] = useState(0);

  const commitSessionChats = useCallback((next: Record<string, SessionChatState>) => {
    sessionChatsRef.current = next;
    setSessionChatsVersion((value) => value + 1);
  }, []);

  const updateSessionChat = useCallback((sessionID: string, updater: (current: SessionChatState) => SessionChatState) => {
    const key = chatSessionKey(sessionID);
    const current = sessionChatsRef.current[key] || emptySessionChatState();
    const next = {
      ...sessionChatsRef.current,
      [key]: updater(current),
    };
    commitSessionChats(next);
  }, [commitSessionChats]);

  const getSessionChat = useCallback((sessionID: string) => {
    return sessionChatsRef.current[chatSessionKey(sessionID)] || emptySessionChatState();
  }, []);

  const addMessage = useCallback(
    (sessionID: string, role: ChatItem["role"], text: string) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        messages: [...current.messages, { id: newRequestID(), role, text }],
      }));
    },
    [updateSessionChat],
  );

  const addToolCall = useCallback(
    (sessionID: string, name: string, argumentsText: string, requestID?: string) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        messages: [
          ...current.messages.map((item) =>
            item.role === "assistant" && (item.requestID === requestID || item.id === current.activeAssistantID)
              ? { ...item, status: "tool_running" as ChatMessageStatus }
              : item,
          ),
          {
            id: newRequestID(),
            requestID,
            role: "tool_call",
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
    (sessionID: string, name: string, argumentsText: string, result: string, failed: boolean, requestID?: string) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        messages: [
          ...current.messages.map((item) =>
            item.role === "assistant" && (item.requestID === requestID || item.id === current.activeAssistantID)
              ? { ...item, status: "streaming" as ChatMessageStatus }
              : item,
          ),
          {
            id: newRequestID(),
            requestID,
            role: "tool",
            text: result,
            toolName: name,
            toolArguments: argumentsText,
            toolError: failed ? result : "",
          },
        ],
      }));
    },
    [updateSessionChat],
  );

  const appendAssistant = useCallback(
    (sessionID: string, requestID: string | undefined, text: string) => {
      if (!text) {
        return;
      }

      updateSessionChat(sessionID, (current) => {
        const assistantID = findAssistantID(current, requestID) || current.activeAssistantID;
        if (!assistantID) {
          const id = newRequestID();
          return {
            ...current,
            activeAssistantID: id,
            messages: [...current.messages, { id, requestID, role: "assistant", status: "streaming", text }],
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
                  status: item.status === "paused" || item.status === "error" ? item.status : "streaming",
                  text: item.text + text,
                }
              : item,
          ),
        };
      });
    },
    [updateSessionChat],
  );

  const completeAssistant = useCallback(
    (sessionID: string, requestID: string | undefined, status: ChatMessageStatus, usage?: TokenUsage | null, content?: string) => {
      updateSessionChat(sessionID, (current) => {
        const assistantID = findAssistantID(current, requestID) || current.activeAssistantID;
        if (!assistantID) {
          if (!content && status === "done") {
            return { ...current, activeAssistantID: "" };
          }
          const id = newRequestID();
          return {
            ...current,
            activeAssistantID: "",
            messages: [
              ...current.messages,
              {
                id,
                requestID,
                role: "assistant",
                status,
                text: content || "",
                usage: usage || undefined,
              },
            ],
          };
        }

        return {
          ...current,
          activeAssistantID: current.activeAssistantID === assistantID ? "" : current.activeAssistantID,
          messages: current.messages.map((item) =>
            item.id === assistantID
              ? {
                  ...item,
                  requestID: item.requestID || requestID,
                  status,
                  text: item.text || content || "",
                  usage: usage || item.usage,
                }
              : item,
          ),
        };
      });
    },
    [updateSessionChat],
  );

  const markAssistantError = useCallback(
    (sessionID: string, requestID: string | undefined, message?: string) => {
      updateSessionChat(sessionID, (current) => {
        const assistantID = findAssistantID(current, requestID) || current.activeAssistantID;
        if (!assistantID) {
          const id = newRequestID();
          return {
            ...current,
            activeAssistantID: "",
            messages: [
              ...current.messages,
              {
                id,
                requestID,
                role: "assistant",
                status: "error",
                text: message || "Request failed.",
              },
            ],
          };
        }

        return {
          ...current,
          activeAssistantID: current.activeAssistantID === assistantID ? "" : current.activeAssistantID,
          messages: current.messages.map((item) =>
            item.id === assistantID
              ? {
                  ...item,
                  requestID: item.requestID || requestID,
                  status: "error",
                  text: item.text || message || "Request failed.",
                }
              : item,
          ),
        };
      });
    },
    [updateSessionChat],
  );

  const resetActiveAssistant = useCallback(
    (sessionID: string) => {
      updateSessionChat(sessionID, (current) => ({ ...current, activeAssistantID: "" }));
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

  const setSessionLastUsage = useCallback(
    (sessionID: string, usage: TokenUsage | null) => {
      updateSessionChat(sessionID, (current) => ({ ...current, lastUsage: usage }));
    },
    [updateSessionChat],
  );

  const setSessionPendingPermission = useCallback(
    (sessionID: string, permission: PermissionState | null) => {
      updateSessionChat(sessionID, (current) => ({ ...current, pendingPermission: permission }));
    },
    [updateSessionChat],
  );

  const setSessionPendingRequest = useCallback(
    (sessionID: string, requestID: string) => {
      updateSessionChat(sessionID, (current) => ({ ...current, pendingRequestID: requestID }));
    },
    [updateSessionChat],
  );

  const clearSessionPendingRequest = useCallback(
    (sessionID: string, requestID?: string) => {
      updateSessionChat(sessionID, (current) => {
        if (requestID && current.pendingRequestID && current.pendingRequestID !== requestID) {
          return current;
        }
        return { ...current, pendingRequestID: "" };
      });
    },
    [updateSessionChat],
  );

  const hasPendingRequest = useCallback((sessionID: string) => {
    return Boolean(getSessionChat(sessionID).pendingRequestID);
  }, [getSessionChat]);

  const mergeSessionChats = useCallback((fromSessionID: string, toSessionID: string) => {
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
  }, [commitSessionChats]);

  return {
    addMessage,
    addToolCall,
    addToolResult,
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

function findAssistantID(current: SessionChatState, requestID?: string) {
  if (!requestID) {
    return "";
  }

  for (let index = current.messages.length - 1; index >= 0; index -= 1) {
    const message = current.messages[index];
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
  return [...fromMessages, ...toMessages.filter((message) => !seen.has(message.id))];
}
