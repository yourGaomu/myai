import { useCallback, useEffect, useRef, type RefObject } from "react";

import type { RelayMessage } from "../protocol";
import type { PendingAction } from "../types/app";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;
const remoteStateTimeoutMs = 8000;
const timeoutActions: PendingAction[] = ["sessions", "models"];

type Args = {
  clearFileEntries: () => void;
  clearHistory: () => void;
  clearModels: () => void;
  clearSessions: () => void;
  clearWorkspaceChanges: () => void;
  clientToken: string;
  currentFilePath: string;
  currentSessionID: string;
  pendingHistorySessionIDRef: RefObject<string>;
  sendEnvelope: SendEnvelope;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

export function useRemoteRequests({
  clearFileEntries,
  clearHistory,
  clearModels,
  clearSessions,
  clearWorkspaceChanges,
  clientToken,
  currentFilePath,
  currentSessionID,
  pendingHistorySessionIDRef,
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
    stopRemoteStateLoadingLater();
  }, [requestModels, requestSessions, stopRemoteStateLoadingLater]);

  return {
    refreshRemoteState,
    requestChanges,
    requestFiles,
    requestHistory,
    requestModels,
    requestSessionHistory,
    requestSessions,
  };
}
