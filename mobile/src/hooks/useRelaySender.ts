import { useCallback, type RefObject } from "react";

import type { RelayMessage } from "../protocol";
import { newRequestID } from "../utils/ids";
import type { RelayPairing } from "../utils/relayPairing";
import type { RelaySendHandle } from "../utils/relayWebSocket";

type Args = {
  activeRequestIDRef: RefObject<string>;
  addErrorMessage: (message: string) => void;
  currentPairingRef: RefObject<RelayPairing | null>;
  onEnqueueFailure: (message: RelayMessage) => void;
  sessionIDRef: RefObject<string>;
  socketRef: RefObject<RelaySendHandle | null>;
};

export function useRelaySender({
  activeRequestIDRef, addErrorMessage, currentPairingRef, onEnqueueFailure, sessionIDRef, socketRef,
}: Args) {
  return useCallback(
    (type: RelayMessage["type"], overrides: Partial<RelayMessage> = {}) => {
      const handlesOwnErrors = type === "asset_image_preview" || type === "mcp_list" || type === "mcp_reload" ||
        type === "session_context_query" || type === "session_rename";
      const socket = socketRef.current;
      if (!socket || currentPairingRef.current !== socket.pairing) {
        if (!handlesOwnErrors) addErrorMessage("中继尚未就绪，请等待连接或检查配对。");
        return false;
      }
      const { userID, deviceID, clientToken } = socket.pairing;
      const envelope: RelayMessage = {
        type,
        request_id: overrides.request_id ?? (activeRequestIDRef.current || newRequestID()),
        user_id: userID,
        device_id: deviceID,
        session_id: overrides.session_id ?? sessionIDRef.current,
        client_token: clientToken,
        payload: overrides.payload ?? {},
      };
      const rejectEnqueue = (definitelyUnsent: boolean) => {
        if (currentPairingRef.current !== socket.pairing) return;
        const isTurn = type === "user_message" || type === "session_regenerate" ||
          type === "session_plan_execute" || type === "subagent_task_resume";
        // A definitive native rejection can arrive after the disconnect event.
        // Only the owning turn may settle from that earlier socket's result.
        if (socketRef.current !== socket && (!definitelyUnsent || !isTurn)) return;
        const control = envelope.payload as { approval_id?: string; question_id?: string };
        onEnqueueFailure({
          type: "error",
          request_id: envelope.request_id,
          session_id: envelope.session_id,
          user_id: userID,
          device_id: deviceID,
          payload: {
            message: "消息未能进入中继连接，请在连接恢复后重试。",
            request_type: type,
            approval_id: control.approval_id,
            question_id: control.question_id,
            transport_error: true,
          },
        });
      };
      try {
        const enqueue = socket.send(JSON.stringify(envelope));
        if (enqueue) void enqueue.then((sent) => { if (!sent) rejectEnqueue(true); }, () => rejectEnqueue(false));
        return true;
      } catch {
        if (!handlesOwnErrors) addErrorMessage("中继连接已断开，请等待自动重连后重试。");
        return false;
      }
    },
    [activeRequestIDRef, addErrorMessage, currentPairingRef, onEnqueueFailure, sessionIDRef, socketRef],
  );
}
