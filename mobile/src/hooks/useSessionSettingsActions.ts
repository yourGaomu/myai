import { useCallback } from "react";

import type { RelayMessage } from "../protocol";
import type { PendingAction, SessionPermissionMode } from "../types/app";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  sendEnvelope: SendEnvelope;
  sessionID: string;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

export function useSessionSettingsActions({
  sendEnvelope,
  sessionID,
  startPending,
  stopPending,
}: Args) {
  const setPermissionMode = useCallback(
    (mode: SessionPermissionMode) => {
      const targetSessionID = sessionID.trim();
      if (!targetSessionID || !mode) {
        return;
      }

      startPending("settings");
      if (!sendEnvelope("session_permission_set", {
        request_id: newRequestID(),
        session_id: targetSessionID,
        payload: { session_id: targetSessionID, mode },
      })) {
        stopPending("settings");
      }
    },
    [sendEnvelope, sessionID, startPending, stopPending],
  );

  const setContextWindowK = useCallback(
    (windowK: number) => {
      const targetSessionID = sessionID.trim();
      if (!targetSessionID || !windowK) {
        return;
      }

      startPending("settings");
      if (!sendEnvelope("session_context_set", {
        request_id: newRequestID(),
        session_id: targetSessionID,
        payload: { session_id: targetSessionID, window_k: windowK },
      })) {
        stopPending("settings");
      }
    },
    [sendEnvelope, sessionID, startPending, stopPending],
  );

  const compactSession = useCallback(() => {
    const targetSessionID = sessionID.trim();
    if (!targetSessionID) {
      return;
    }

    startPending("settings");
    if (!sendEnvelope("session_compact", {
      request_id: newRequestID(),
      session_id: targetSessionID,
      payload: { session_id: targetSessionID },
    })) {
      stopPending("settings");
    }
  }, [sendEnvelope, sessionID, startPending, stopPending]);

  return {
    compactSession,
    setContextWindowK,
    setPermissionMode,
  };
}
