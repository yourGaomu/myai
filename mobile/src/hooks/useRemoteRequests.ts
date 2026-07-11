import { useCallback, useEffect, useRef, type RefObject } from "react";

import type { RelayMessage, SessionHistoryMessage, SessionHistoryMetaPayload } from "../protocol";
import type { PendingAction } from "../types/app";
import { loadCachedSessionHistory } from "../storage/sessionHistoryCache";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;
const remoteStateTimeoutMs = 8000;
const timeoutActions: PendingAction[] = ["sessions", "models", "skills", "assets"];

type Args = {
  clearAssets: () => void;
  clearFileEntries: () => void;
  clearHistory: () => void;
  clearModels: () => void;
  clearSessions: () => void;
  clearSkills: () => void;
  clearWorkspaceChanges: () => void;
  clientToken: string;
  currentFilePath: string;
  currentSessionID: string;
  pendingHistorySessionIDRef: RefObject<string>;
  replaceHistoryMessages: (sessionID: string, messages: SessionHistoryMessage[]) => void;
  sendEnvelope: SendEnvelope;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

// 集中定义所有只读远程请求，并统一处理 pending、超时和会话历史缓存协商。
export function useRemoteRequests({
  clearAssets,
  clearFileEntries,
  clearHistory,
  clearModels,
  clearSessions,
  clearSkills,
  clearWorkspaceChanges,
  clientToken,
  currentFilePath,
  currentSessionID,
  pendingHistorySessionIDRef,
  replaceHistoryMessages,
  sendEnvelope,
  startPending,
  stopPending,
}: Args) {
  const remoteStateTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (remoteStateTimeoutRef.current) {
        clearTimeout(remoteStateTimeoutRef.current);
      }
    };
  }, []);

  const stopRemoteStateLoadingLater = useCallback(() => {
    if (remoteStateTimeoutRef.current) {
      clearTimeout(remoteStateTimeoutRef.current);
    }
    remoteStateTimeoutRef.current = setTimeout(() => {
      timeoutActions.forEach(stopPending);
      remoteStateTimeoutRef.current = null;
    }, remoteStateTimeoutMs);
  }, [stopPending]);

  const requestSessions = useCallback(() => {
    if (!clientToken) {
      clearSessions();
      stopPending("sessions");
      return false;
    }
    startPending("sessions");
    if (!sendEnvelope("session_list", { request_id: newRequestID() })) {
      stopPending("sessions");
      return false;
    }
    return true;
  }, [clearSessions, clientToken, sendEnvelope, startPending, stopPending]);

  const requestDeletedSessions = useCallback(() => {
    if (!clientToken) {
      return false;
    }
    startPending("sessions");
    if (!sendEnvelope("session_list", {
      request_id: newRequestID(),
      payload: { include_deleted: true },
    })) {
      stopPending("sessions");
      return false;
    }
    return true;
  }, [clientToken, sendEnvelope, startPending, stopPending]);

  const requestModels = useCallback(() => {
    if (!clientToken) {
      clearModels();
      stopPending("models");
      return false;
    }
    startPending("models");
    if (!sendEnvelope("model_list", { request_id: newRequestID() })) {
      stopPending("models");
      return false;
    }
    return true;
  }, [clearModels, clientToken, sendEnvelope, startPending, stopPending]);

  const requestSkills = useCallback(() => {
    if (!clientToken) {
      clearSkills();
      stopPending("skills");
      return false;
    }
    startPending("skills");
    if (!sendEnvelope("skill_list", { request_id: newRequestID() })) {
      stopPending("skills");
      return false;
    }
    return true;
  }, [clearSkills, clientToken, sendEnvelope, startPending, stopPending]);

  const reloadSkills = useCallback(() => {
    if (!clientToken) {
      clearSkills();
      stopPending("skills");
      return false;
    }
    startPending("skills");
    if (!sendEnvelope("skill_reload", { request_id: newRequestID() })) {
      stopPending("skills");
      return false;
    }
    return true;
  }, [clearSkills, clientToken, sendEnvelope, startPending, stopPending]);

  const requestSessionHistoryFull = useCallback(
    (nextSessionID = currentSessionID) => {
      const targetSessionID = nextSessionID.trim();
      if (!clientToken || !targetSessionID) {
        pendingHistorySessionIDRef.current = "";
        stopPending("sessions");
        return false;
      }

      startPending("sessions");
      pendingHistorySessionIDRef.current = targetSessionID;
      if (!sendEnvelope("session_history", {
        request_id: newRequestID(),
        session_id: targetSessionID,
        payload: { session_id: targetSessionID },
      })) {
        pendingHistorySessionIDRef.current = "";
        stopPending("sessions");
        return false;
      }
      return true;
    },
    [clientToken, currentSessionID, pendingHistorySessionIDRef, sendEnvelope, startPending, stopPending],
  );

  const requestSessionHistoryDelta = useCallback(
    (nextSessionID = currentSessionID) => {
      const targetSessionID = nextSessionID.trim();
      if (!clientToken || !targetSessionID) {
        pendingHistorySessionIDRef.current = "";
        stopPending("sessions");
        return false;
      }

      startPending("sessions");
      pendingHistorySessionIDRef.current = targetSessionID;
      // 先显示手机缓存，再发送 meta 判断是否需要增量或全量同步，减少长会话等待时间。
      void loadCachedSessionHistory(targetSessionID)
        .then(({ meta }) => {
          if (!sendEnvelope("session_history_delta", {
            request_id: newRequestID(),
            session_id: targetSessionID,
            payload: {
              after_message_id: meta.local_last_message_id || "",
              limit: 100,
              session_id: targetSessionID,
            },
          })) {
            pendingHistorySessionIDRef.current = "";
            stopPending("sessions");
          }
        })
        .catch(() => {
          requestSessionHistoryFull(targetSessionID);
        });
      return true;
    },
    [clientToken, currentSessionID, pendingHistorySessionIDRef, requestSessionHistoryFull, sendEnvelope, startPending, stopPending],
  );

  const sendHistoryMeta = useCallback(
    (sessionID: string, meta: SessionHistoryMetaPayload) => {
      const payload: SessionHistoryMetaPayload = {
        ...meta,
        session_id: sessionID,
      };
      if (!payload.local_last_message_created_at) {
        delete payload.local_last_message_created_at;
      }
      return sendEnvelope("session_history_meta", {
        request_id: newRequestID(),
        session_id: sessionID,
        payload,
      });
    },
    [sendEnvelope],
  );

  const requestSessionHistory = useCallback(
    (nextSessionID = currentSessionID) => {
      const targetSessionID = nextSessionID.trim();
      if (!clientToken || !targetSessionID) {
        pendingHistorySessionIDRef.current = "";
        stopPending("sessions");
        return false;
      }

      startPending("sessions");
      pendingHistorySessionIDRef.current = targetSessionID;
      void loadCachedSessionHistory(targetSessionID)
        .then(({ messages, meta }) => {
          if (messages.length > 0) {
            replaceHistoryMessages(targetSessionID, messages);
          }
          if (!sendHistoryMeta(targetSessionID, meta)) {
            pendingHistorySessionIDRef.current = "";
            stopPending("sessions");
          }
        })
        .catch(() => {
          if (!sendHistoryMeta(targetSessionID, emptyHistoryMeta(targetSessionID))) {
            pendingHistorySessionIDRef.current = "";
            stopPending("sessions");
          }
        });
      return true;
    },
    [clientToken, currentSessionID, pendingHistorySessionIDRef, replaceHistoryMessages, sendHistoryMeta, startPending, stopPending],
  );

  const requestAssets = useCallback(
    (nextSessionID = currentSessionID) => {
      const targetSessionID = nextSessionID.trim();
      if (!clientToken || !targetSessionID) {
        clearAssets();
        stopPending("assets");
        return false;
      }

      startPending("assets");
      if (!sendEnvelope("asset_list", {
        request_id: newRequestID(),
        session_id: targetSessionID,
        payload: { session_id: targetSessionID, limit: 100 },
      })) {
        stopPending("assets");
        return false;
      }
      return true;
    },
    [clearAssets, clientToken, currentSessionID, sendEnvelope, startPending, stopPending],
  );

  const requestFiles = useCallback(
    (path = currentFilePath) => {
      if (!clientToken) {
        clearFileEntries();
        stopPending("files");
        return false;
      }
      startPending("files");
      if (!sendEnvelope("file_list", {
        request_id: newRequestID(),
        payload: { path, limit: 200 },
      })) {
        stopPending("files");
        return false;
      }
      return true;
    },
    [clearFileEntries, clientToken, currentFilePath, sendEnvelope, startPending, stopPending],
  );

  const requestChanges = useCallback(() => {
    if (!clientToken) {
      clearWorkspaceChanges();
      stopPending("changes");
      return false;
    }

    startPending("changes");
    if (!sendEnvelope("changes_list", {
      request_id: newRequestID(),
      payload: { limit: 200 },
    })) {
      stopPending("changes");
      return false;
    }
    return true;
  }, [clearWorkspaceChanges, clientToken, sendEnvelope, startPending, stopPending]);

  const requestHistory = useCallback(() => {
    if (!clientToken) {
      clearHistory();
      stopPending("history");
      return false;
    }

    startPending("history");
    if (!sendEnvelope("history_list", {
      request_id: newRequestID(),
      payload: { limit: 50 },
    })) {
      stopPending("history");
      return false;
    }
    return true;
  }, [clearHistory, clientToken, sendEnvelope, startPending, stopPending]);

  const refreshRemoteState = useCallback(() => {
    requestSessions();
    requestModels();
    requestSkills();
    requestAssets();
    stopRemoteStateLoadingLater();
  }, [requestAssets, requestModels, requestSessions, requestSkills, stopRemoteStateLoadingLater]);

  return {
    reloadSkills,
    refreshRemoteState,
    requestAssets,
    requestChanges,
    requestFiles,
    requestHistory,
    requestModels,
    requestSkills,
    requestDeletedSessions,
    requestSessionHistoryDelta,
    requestSessionHistoryFull,
    requestSessionHistory,
    requestSessions,
  };
}

function emptyHistoryMeta(sessionID: string): SessionHistoryMetaPayload {
  return {
    session_id: sessionID,
    local_message_count: 0,
    local_last_message_id: "",
    local_history_version: 0,
  };
}
