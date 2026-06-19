import { useCallback, useRef, useState } from "react";

import type { TokenUsage } from "../protocol";
import type { PermissionState } from "../types/app";
import type { ChatItem } from "../types/chat";
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
    (sessionID: string, name: string, argumentsText: string) => {
      updateSessionChat(sessionID, (current) => ({
        ...current,
        messages: [
          ...current.messages,
          {
            id: newRequestID(),
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

  const appendAssistant = useCallback(
    (sessionID: string, text: string) => {
      if (!text) {
        return;
      }

      updateSessionChat(sessionID, (current) => {
        const assistantID = current.activeAssistantID;
        if (!assistantID) {
          const id = newRequestID();
          return {
            ...current,
            activeAssistantID: id,
            messages: [...current.messages, { id, role: "assistant", text }],
          };
        }

        return {
          ...current,
          messages: current.messages.map((item) =>
            item.id === assistantID ? { ...item, text: item.text + text } : item,
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
    appendAssistant,
    clearMessages,
    clearSessionPendingRequest,
    getSessionChat,
    hasPendingRequest,
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
