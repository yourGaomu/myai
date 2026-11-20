import { Alert, Platform } from "react-native";
import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";

import type { GenerationSettings, ModelSummary, RelayMessage, SessionSummary, TokenUsage } from "../protocol";
import type { PendingAction, RelayScope } from "../types/app";
import { newRequestID } from "../utils/ids";
import { createModelDeleteTracker, type ModelDeleteFeedback } from "../utils/modelDelete";
import { findSessionUsage } from "../utils/session";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

export type ModelConfigDraft = {
  id: string;
  name: string;
  provider: string;
  protocol: string;
  auth_type: string;
  base_url?: string;
  api_key: string;
  model_name: string;
  defaults: GenerationSettings;
};

type NavigationIntent = {
  connectionID: string;
  requestID: string;
  targetSessionID: string;
};

type SessionMutation = NavigationIntent & {
  type: "session_delete" | "session_restore";
  navigationVersion: number;
};

export type SessionNavigationState = {
  current: NavigationIntent | null;
  version: number;
  mutations: Map<string, SessionMutation>;
};

type Args = {
  currentModelID: string;
  historySessionIDRef: RefObject<string>;
  pendingHistorySessionIDRef: RefObject<string>;
  relayScopeRef: RefObject<RelayScope>;
  requestAssets: (sessionID?: string) => boolean;
  requestSessionHistory: (sessionID?: string) => boolean;
  sendEnvelope: SendEnvelope;
  sessionIDRef: RefObject<string>;
  sessions: SessionSummary[];
  setModels: (models: ModelSummary[]) => void;
  setSessionLastUsage: (sessionID: string, usage: TokenUsage | null) => void;
  setSessionID: (sessionID: string) => void;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

// Session selection is local to this client. Remember only the latest explicit
// navigation; a result from another request must not change it.
export function useSessionModelActions({
  currentModelID,
  historySessionIDRef,
  pendingHistorySessionIDRef,
  relayScopeRef,
  requestAssets,
  requestSessionHistory,
  sendEnvelope,
  sessionIDRef,
  sessions,
  setModels,
  setSessionLastUsage,
  setSessionID,
  startPending,
  stopPending,
}: Args) {
  const [modelDeleteFeedback, setModelDeleteFeedback] = useState<ModelDeleteFeedback | null>(null);
  const modelDelete = useMemo(() => createModelDeleteTracker({
    getScope: () => relayScopeRef.current,
    onFeedback: setModelDeleteFeedback,
    onDeleted: setModels,
  }), [relayScopeRef, setModels]);
  useEffect(() => modelDelete.clear, [modelDelete]);

  const sessionNavigationRef = useRef<SessionNavigationState>({ current: null, version: 0, mutations: new Map() });
  const beginNavigation = useCallback((targetSessionID: string) => {
    const intent = {
      connectionID: relayScopeRef.current.connectionID,
      requestID: newRequestID(),
      targetSessionID,
    };
    sessionNavigationRef.current.current = intent;
    sessionNavigationRef.current.version += 1;
    return intent;
  }, [relayScopeRef]);

  const sendSessionMutation = useCallback((type: SessionMutation["type"], targetSessionID: string) => {
    const connectionID = relayScopeRef.current.connectionID;
    const navigation = sessionNavigationRef.current;
    for (const [id, mutation] of navigation.mutations) {
      if (mutation.connectionID !== connectionID) navigation.mutations.delete(id);
    }
    const requestID = newRequestID();
    navigation.mutations.set(requestID, { connectionID, requestID, targetSessionID, type, navigationVersion: navigation.version });
    startPending("sessions");
    if (!sendEnvelope(type, { request_id: requestID, session_id: targetSessionID, payload: { session_id: targetSessionID } })) {
      navigation.mutations.delete(requestID);
      if (!pendingHistorySessionIDRef.current && !navigation.current) stopPending("sessions");
    }
  }, [pendingHistorySessionIDRef, relayScopeRef, sendEnvelope, startPending, stopPending]);

  const newSession = useCallback(() => {
    const intent = beginNavigation("");
    startPending("sessions");
    if (!sendEnvelope("session_new", { request_id: intent.requestID })) {
      sessionNavigationRef.current.current = null;
      if (!pendingHistorySessionIDRef.current) stopPending("sessions");
    }
  }, [beginNavigation, pendingHistorySessionIDRef, sendEnvelope, startPending, stopPending]);

  const loadSession = useCallback(
    (nextSessionID: string) => {
      const targetSessionID = nextSessionID.trim();
      if (!targetSessionID) return;

      const intent = beginNavigation(targetSessionID);
      sessionIDRef.current = targetSessionID;
      setSessionID(targetSessionID);
      historySessionIDRef.current = "";
      pendingHistorySessionIDRef.current = "";
      const usage = findSessionUsage(sessions, targetSessionID);
      if (usage) setSessionLastUsage(targetSessionID, usage);
      startPending("sessions");
      const loadSent = sendEnvelope("session_load", {
        request_id: intent.requestID,
        session_id: targetSessionID,
        payload: { session_id: targetSessionID },
      });
      if (!loadSent) sessionNavigationRef.current.current = null;
      const historySent = requestSessionHistory(targetSessionID);
      requestAssets(targetSessionID);
      if (!loadSent && !historySent) stopPending("sessions");
    },
    [
      beginNavigation,
      historySessionIDRef,
      pendingHistorySessionIDRef,
      requestAssets,
      requestSessionHistory,
      sendEnvelope,
      sessionIDRef,
      sessions,
      setSessionLastUsage,
      setSessionID,
      startPending,
      stopPending,
    ],
  );

  const deleteSession = useCallback(
    (targetSessionID: string) => {
      const nextSessionID = targetSessionID.trim();
      if (!nextSessionID) {
        return;
      }

      const runDelete = () => sendSessionMutation("session_delete", nextSessionID);

      if (Platform.OS === "web") {
        if (typeof window !== "undefined" && window.confirm("Delete session? This session will move to the recycle bin and can be restored later.")) {
          runDelete();
        }
        return;
      }

      Alert.alert("Delete session?", "This session will move to the recycle bin and can be restored later.", [
        { style: "cancel", text: "Cancel" },
        {
          style: "destructive",
          text: "Delete",
          onPress: runDelete,
        },
      ]);
    },
    [sendSessionMutation],
  );

  const restoreSession = useCallback(
    (targetSessionID: string) => {
      const nextSessionID = targetSessionID.trim();
      if (nextSessionID) sendSessionMutation("session_restore", nextSessionID);
    },
    [sendSessionMutation],
  );

  const switchModel = useCallback(
    (modelID: string) => {
      const activeModelID = sessions.find((session) => session.id === sessionIDRef.current)?.model || currentModelID;
      if (!modelID || modelID === activeModelID) {
        return;
      }

      startPending("models");
      if (!sendEnvelope("model_switch", {
        request_id: newRequestID(),
        payload: { model_id: modelID },
      })) {
        stopPending("models");
      }
    },
    [currentModelID, sendEnvelope, sessionIDRef, sessions, startPending, stopPending],
  );

  const addModelConfig = useCallback((config: ModelConfigDraft) => {
    startPending("models");
    if (!sendEnvelope("model_config_add", {
      request_id: newRequestID(),
      payload: config,
    })) {
      stopPending("models");
    }
  }, [sendEnvelope, startPending, stopPending]);

  const testModelConfig = useCallback((config: ModelConfigDraft) => {
    startPending("models");
    if (!sendEnvelope("model_config_test", {
      request_id: newRequestID(),
      payload: config,
    })) {
      stopPending("models");
    }
  }, [sendEnvelope, startPending, stopPending]);

  const updateModelConfig = useCallback((config: ModelConfigDraft) => {
    startPending("models");
    if (!sendEnvelope("model_config_update", { request_id: newRequestID(), payload: config })) {
      stopPending("models");
    }
  }, [sendEnvelope, startPending, stopPending]);

  const setModelEnabled = useCallback((modelID: string, enabled: boolean) => {
    startPending("models");
    if (!sendEnvelope("model_config_enabled_set", {
      request_id: newRequestID(),
      payload: { id: modelID, enabled },
    })) {
      stopPending("models");
    }
  }, [sendEnvelope, startPending, stopPending]);

  const setDefaultModel = useCallback((modelID: string) => {
    startPending("models");
    if (!sendEnvelope("model_config_default_set", {
      request_id: newRequestID(),
      payload: { id: modelID },
    })) {
      stopPending("models");
    }
  }, [sendEnvelope, startPending, stopPending]);

  const deleteModelConfig = useCallback((modelID: string) => {
    modelDelete.start(modelID, sendEnvelope);
  }, [modelDelete, sendEnvelope]);

  return {
    addModelConfig,
    deleteModelConfig,
    deleteSession,
    disconnectModelDelete: modelDelete.disconnect,
    handleModelDeleteMessage: modelDelete.handleMessage,
    modelDeleteFeedback,
    loadSession,
    newSession,
    restoreSession,
    sessionNavigationRef,
    switchModel,
    setDefaultModel,
    setModelEnabled,
    testModelConfig,
    updateModelConfig,
  };
}
