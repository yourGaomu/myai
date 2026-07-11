import { useCallback, useState } from "react";

import type { CompactInfo, ContextInfo, ModelSummary, SessionSummary } from "../protocol";

// 统一保存 Session、模型、上下文和压缩结果，服务端 SessionSummary 是主要数据来源。
export function useSessionModelState() {
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [deletedSessions, setDeletedSessions] = useState<SessionSummary[]>([]);
  const [models, setModels] = useState<ModelSummary[]>([]);
  const [sessionContexts, setSessionContexts] = useState<Record<string, ContextInfo>>({});
  const [sessionCompacts, setSessionCompacts] = useState<Record<string, CompactInfo>>({});
  const [currentModelID, setCurrentModelID] = useState("");

  const clearSessions = useCallback(() => {
    setSessions([]);
    setDeletedSessions([]);
    setSessionContexts({});
    setSessionCompacts({});
  }, []);
  const clearModels = useCallback(() => {
    setModels([]);
    setCurrentModelID("");
  }, []);
  const setSessionContext = useCallback((sessionID: string, context: ContextInfo) => {
    if (!sessionID) {
      return;
    }
    setSessionContexts((current) => ({ ...current, [sessionID]: context }));
  }, []);
  const setSessionCompact = useCallback((sessionID: string, compact: CompactInfo) => {
    if (!sessionID) {
      return;
    }
    setSessionCompacts((current) => ({ ...current, [sessionID]: compact }));
  }, []);

  return {
    clearModels,
    clearSessions,
    currentModelID,
    deletedSessions,
    models,
    sessionCompacts,
    sessionContexts,
    sessions,
    setCurrentModelID,
    setDeletedSessions,
    setModels,
    setSessionCompact,
    setSessionContext,
    setSessions,
  };
}
