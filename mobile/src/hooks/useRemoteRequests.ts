import { useCallback, useEffect, useRef, type RefObject } from "react";

import type { AgentRunListResultPayload, AssetListResultPayload, ErrorPayload, RelayMessage, SessionListResultPayload } from "../protocol";
import type { PendingAction, RelayScope } from "../types/app";
import type { ChatItem } from "../types/chat";
import { newRequestID } from "../utils/ids";
import { useSessionHistorySync } from "./useSessionHistorySync";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;
const remoteStateTimeoutMs = 8000;
const timeoutActions: PendingAction[] = ["models", "skills", "plugins"];
const queryTimeoutMs = 15000;

type ReadRequest = RelayScope & { requestID: string; timeout: ReturnType<typeof setTimeout> };

type Args = {
  clearAssets: () => void;
  clearFileEntries: () => void;
  clearHistory: () => void;
  clearModels: () => void;
  clearSessions: () => void;
  clearSkills: () => void;
	clearPlugins: () => void;
  clearWorkspaceChanges: () => void;
  clientToken: string;
  currentFilePath: string;
  historySessionIDRef: RefObject<string>;
  pendingHistorySessionIDRef: RefObject<string>;
  relayScopeRef: RefObject<RelayScope>;
  sessionIDRef: RefObject<string>;
  mergeSessionHistory: (sessionID: string, messages: ChatItem[], source: "cache" | "server") => void;
  requestSubagentTasks: (sessionID: string) => boolean;
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
	clearPlugins,
  clearWorkspaceChanges,
  clientToken,
  currentFilePath,
  historySessionIDRef,
  pendingHistorySessionIDRef,
  relayScopeRef,
  sessionIDRef,
  mergeSessionHistory,
  requestSubagentTasks,
  sendEnvelope,
  startPending,
  stopPending,
}: Args) {
  const remoteStateTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const readRequestsRef = useRef(new Map<string, ReadRequest>());
  const { requestHistoryWithCache, requestSessionHistoryFull, handleSessionHistoryMessage, resetHistorySync } = useSessionHistorySync({
    historySessionIDRef, pendingHistorySessionIDRef, relayScopeRef, sessionIDRef,
    mergeSessionHistory, sendEnvelope, startPending, stopPending,
  });

  const clearReadRequests = useCallback(() => {
    for (const request of readRequestsRef.current.values()) clearTimeout(request.timeout);
    readRequestsRef.current.clear();
    if (remoteStateTimeoutRef.current) clearTimeout(remoteStateTimeoutRef.current);
    remoteStateTimeoutRef.current = null;
  }, []);
  useEffect(() => clearReadRequests, [clearReadRequests]);

  const resetRemoteQueries = useCallback(() => {
    clearReadRequests();
    resetHistorySync();
    stopPending("sessions");
    stopPending("assets");
  }, [clearReadRequests, resetHistorySync, stopPending]);

  const stopRemoteStateLoadingLater = useCallback(() => {
    if (remoteStateTimeoutRef.current) clearTimeout(remoteStateTimeoutRef.current);
    remoteStateTimeoutRef.current = setTimeout(() => {
      timeoutActions.forEach(stopPending);
      remoteStateTimeoutRef.current = null;
    }, remoteStateTimeoutMs);
  }, [stopPending]);

  const sendRead = useCallback((key: string, type: "session_list" | "agent_run_list" | "asset_list", overrides: Partial<RelayMessage>) => {
    const scope = relayScopeRef.current;
    if (!scope.connectionID) return false;
    const requests = readRequestsRef.current;
    clearTimeout(requests.get(key)?.timeout);
    const requestID = newRequestID();
    const request: ReadRequest = {
      ...scope,
      requestID,
      timeout: setTimeout(() => {
        if (requests.get(key)?.requestID !== requestID) return;
        requests.delete(key);
        if (key === "session_list" && !pendingHistorySessionIDRef.current) stopPending("sessions");
        if (key === "asset_list") stopPending("assets");
      }, queryTimeoutMs),
    };
    requests.set(key, request);
    if (sendEnvelope(type, { ...overrides, request_id: requestID })) return true;
    clearTimeout(request.timeout);
    requests.delete(key);
    return false;
  }, [pendingHistorySessionIDRef, relayScopeRef, sendEnvelope, stopPending]);

  const acceptRead = useCallback((key: string, requestID?: string) => {
    const request = readRequestsRef.current.get(key);
    const scope = relayScopeRef.current;
    if (!request || requestID !== request.requestID || scope.pairingID !== request.pairingID ||
        !scope.connectionID || scope.connectionID !== request.connectionID) return false;
    clearTimeout(request.timeout);
    readRequestsRef.current.delete(key);
    return true;
  }, [relayScopeRef]);

  const acceptSessionList = useCallback((message: RelayMessage) => {
    const payload = message.payload as SessionListResultPayload | undefined;
    return Boolean(payload && acceptRead(payload.include_deleted ? "deleted_sessions" : "session_list", message.request_id));
  }, [acceptRead]);

  const acceptAgentRunList = useCallback((message: RelayMessage) => {
    const sessionID = (message.payload as AgentRunListResultPayload | undefined)?.session_id?.trim();
    return Boolean(sessionID && (!message.session_id || message.session_id === sessionID) &&
      acceptRead(`runs:${sessionID}`, message.request_id));
  }, [acceptRead]);

  const acceptAssetList = useCallback((message: RelayMessage) => {
    const sessionID = (message.payload as AssetListResultPayload | undefined)?.session_id?.trim();
    return Boolean(sessionID && sessionID === sessionIDRef.current &&
      (!message.session_id || message.session_id === sessionID) && acceptRead("asset_list", message.request_id));
  }, [acceptRead, sessionIDRef]);

  const handleRemoteQueryError = useCallback((message: RelayMessage) => {
    const type = (message.payload as ErrorPayload | undefined)?.request_type;
    if (type !== "session_list" && type !== "agent_run_list" && type !== "asset_list") return false;
    for (const [key, request] of readRequestsRef.current) {
      if (request.requestID !== message.request_id || !acceptRead(key, message.request_id)) continue;
      if (key === "session_list" && !pendingHistorySessionIDRef.current) stopPending("sessions");
      if (key === "asset_list") stopPending("assets");
      return true;
    }
    return false;
  }, [acceptRead, pendingHistorySessionIDRef, stopPending]);

  const requestSessions = useCallback(() => {
    if (!clientToken) {
      clearSessions();
      stopPending("sessions");
      return false;
    }
    startPending("sessions");
    const sent = sendRead("session_list", "session_list", { session_id: "" });
    if (!sent && !pendingHistorySessionIDRef.current) stopPending("sessions");
    return sent;
  }, [clearSessions, clientToken, pendingHistorySessionIDRef, sendRead, startPending, stopPending]);

  const requestDeletedSessions = useCallback(() => Boolean(clientToken && sendRead("deleted_sessions", "session_list", {
    session_id: "",
    payload: { include_deleted: true },
  })), [clientToken, sendRead]);

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

  const requestPlugins = useCallback(() => {
    if (!clientToken) {
      clearPlugins();
      stopPending("plugins");
      return false;
    }
    startPending("plugins");
    if (!sendEnvelope("plugin_list", { request_id: newRequestID() })) {
      stopPending("plugins");
      return false;
    }
    return true;
  }, [clearPlugins, clientToken, sendEnvelope, startPending, stopPending]);

  const reloadPlugins = useCallback(() => {
    if (!clientToken) {
      clearPlugins();
      stopPending("plugins");
      return false;
    }
    startPending("plugins");
    if (!sendEnvelope("plugin_reload", { request_id: newRequestID() })) {
      stopPending("plugins");
      return false;
    }
    return true;
  }, [clearPlugins, clientToken, sendEnvelope, startPending, stopPending]);

  const requestAgentRuns = useCallback((nextSessionID = sessionIDRef.current) => {
    const sessionID = nextSessionID.trim();
    return Boolean(clientToken && sessionID && sendRead(`runs:${sessionID}`, "agent_run_list", {
      session_id: sessionID,
      payload: { session_id: sessionID, limit: 50 },
    }));
  }, [clientToken, sendRead, sessionIDRef]);

  const requestSessionHistory = useCallback((nextSessionID = sessionIDRef.current) => {
    const sessionID = nextSessionID.trim();
    if (!clientToken || !requestHistoryWithCache(sessionID)) return false;
    requestAgentRuns(sessionID);
    requestSubagentTasks(sessionID);
    return true;
  }, [clientToken, requestAgentRuns, requestHistoryWithCache, requestSubagentTasks, sessionIDRef]);

  const requestAssets = useCallback(
    (nextSessionID = sessionIDRef.current) => {
      const targetSessionID = nextSessionID.trim();
      if (targetSessionID !== sessionIDRef.current) return false;
      if (!clientToken || !targetSessionID) {
        clearAssets();
        stopPending("assets");
        return false;
      }

      startPending("assets");
      if (!sendRead("asset_list", "asset_list", {
        session_id: targetSessionID,
        payload: { session_id: targetSessionID, limit: 100 },
      })) {
        stopPending("assets");
        return false;
      }
      return true;
    },
    [clearAssets, clientToken, sendRead, sessionIDRef, startPending, stopPending],
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
    requestPlugins();
    requestAssets();
    stopRemoteStateLoadingLater();
  }, [requestAssets, requestModels, requestPlugins, requestSessions, requestSkills, stopRemoteStateLoadingLater]);

  return {
    acceptAgentRunList,
    acceptAssetList,
    acceptSessionList,
    handleRemoteQueryError,
    handleSessionHistoryMessage,
    resetRemoteQueries,
    reloadSkills,
    refreshRemoteState,
    requestAssets,
    requestAgentRuns,
    requestChanges,
    requestFiles,
    requestHistory,
    requestModels,
    requestPlugins,
    reloadPlugins,
    requestSkills,
    requestDeletedSessions,
    requestSessionHistoryFull,
    requestSessionHistory,
    requestSessions,
  };
}
