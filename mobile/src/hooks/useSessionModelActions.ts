import { Alert, Platform } from "react-native";
import { useCallback, type RefObject } from "react";

import type { GenerationSettings, RelayMessage, SessionSummary, TokenUsage } from "../protocol";
import type { PendingAction, PermissionState } from "../types/app";
import { newRequestID } from "../utils/ids";
import { findSessionUsage } from "../utils/session";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

export type ModelConfigDraft = {
  id: string;
  name: string;
  provider: string;
  protocol: string;
  auth_type: string;
  base_url: string;
  api_key: string;
  model_name: string;
  defaults: GenerationSettings;
};

type Args = {
  activeRequestIDRef: RefObject<string>;
  currentModelID: string;
  pendingHistorySessionIDRef: RefObject<string>;
  resetActiveAssistant: (sessionID: string) => void;
  sendEnvelope: SendEnvelope;
  sessionIDRef: RefObject<string>;
  sessions: SessionSummary[];
  setSessionLastUsage: (sessionID: string, usage: TokenUsage | null) => void;
  setSessionPendingPermission: (sessionID: string, permission: PermissionState | null) => void;
  setSessionID: (sessionID: string) => void;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

// 会话和模型命令只发送请求，不直接改最终状态；结果统一由远程响应回填。
export function useSessionModelActions({
  activeRequestIDRef,
  currentModelID,
  pendingHistorySessionIDRef,
  resetActiveAssistant,
  sendEnvelope,
  sessionIDRef,
  sessions,
  setSessionLastUsage,
  setSessionPendingPermission,
  setSessionID,
  startPending,
  stopPending,
}: Args) {
  const newSession = useCallback(() => {
    startPending("sessions");
    if (!sendEnvelope("session_new", { request_id: newRequestID() })) {
      stopPending("sessions");
    }
  }, [sendEnvelope, startPending, stopPending]);

  const loadSession = useCallback(
    (nextSessionID: string) => {
      if (!nextSessionID) {
        return;
      }
      setSessionID(nextSessionID);
      sessionIDRef.current = nextSessionID;
      resetActiveAssistant(nextSessionID);
      activeRequestIDRef.current = "";
      pendingHistorySessionIDRef.current = nextSessionID;
      setSessionPendingPermission(nextSessionID, null);
      setSessionLastUsage(nextSessionID, findSessionUsage(sessions, nextSessionID));
      startPending("sessions");
      if (!sendEnvelope("session_load", {
        request_id: newRequestID(),
        session_id: nextSessionID,
        payload: { session_id: nextSessionID },
      })) {
        pendingHistorySessionIDRef.current = "";
        stopPending("sessions");
      }
    },
    [
      activeRequestIDRef,
      pendingHistorySessionIDRef,
      resetActiveAssistant,
      sendEnvelope,
      sessionIDRef,
      sessions,
      setSessionLastUsage,
      setSessionPendingPermission,
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

      const runDelete = () => {
        startPending("sessions");
        if (!sendEnvelope("session_delete", {
          request_id: newRequestID(),
          session_id: nextSessionID,
          payload: { session_id: nextSessionID },
        })) {
          stopPending("sessions");
        }
      };

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
    [sendEnvelope, startPending, stopPending],
  );

  const restoreSession = useCallback(
    (targetSessionID: string) => {
      const nextSessionID = targetSessionID.trim();
      if (!nextSessionID) {
        return;
      }

      startPending("sessions");
      if (!sendEnvelope("session_restore", {
        request_id: newRequestID(),
        session_id: nextSessionID,
        payload: { session_id: nextSessionID },
      })) {
        stopPending("sessions");
      }
    },
    [sendEnvelope, startPending, stopPending],
  );

  const switchModel = useCallback(
    (modelID: string) => {
      if (!modelID || modelID === currentModelID) {
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
    [currentModelID, sendEnvelope, startPending, stopPending],
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
    startPending("models");
    if (!sendEnvelope("model_config_delete", {
      request_id: newRequestID(),
      payload: { id: modelID },
    })) {
      stopPending("models");
    }
  }, [sendEnvelope, startPending, stopPending]);

  return {
    addModelConfig,
    deleteModelConfig,
    deleteSession,
    loadSession,
    newSession,
    restoreSession,
    switchModel,
    setDefaultModel,
    setModelEnabled,
    testModelConfig,
    updateModelConfig,
  };
}
