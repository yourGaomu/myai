import { useCallback, useState } from "react";

import type { ContextInfo, ModelSummary, SessionSummary } from "../protocol";

export function useSessionModelState() {
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [models, setModels] = useState<ModelSummary[]>([]);
  const [sessionContexts, setSessionContexts] = useState<Record<string, ContextInfo>>({});
  const [currentModelID, setCurrentModelID] = useState("");

  const clearSessions = useCallback(() => {
    setSessions([]);
    setSessionContexts({});
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

  return {
    clearModels,
    clearSessions,
    currentModelID,
    models,
    sessionContexts,
    sessions,
    setCurrentModelID,
    setModels,
    setSessionContext,
    setSessions,
  };
}
