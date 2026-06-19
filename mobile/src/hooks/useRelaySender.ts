import { useCallback, type RefObject } from "react";

import type { RelayMessage } from "../protocol";
import { newRequestID } from "../utils/ids";

type Args = {
  activeRequestIDRef: RefObject<string>;
  addErrorMessage: (message: string) => void;
  clientToken: string;
  deviceID: string;
  sessionID: string;
  userID: string;
  socketRef: RefObject<WebSocket | null>;
};

export function useRelaySender({
  activeRequestIDRef,
  addErrorMessage,
  clientToken,
  deviceID,
  sessionID,
  socketRef,
  userID,
}: Args) {
  return useCallback(
    (type: RelayMessage["type"], overrides: Partial<RelayMessage> = {}) => {
      const socket = socketRef.current;
      if (!socket || socket.readyState !== WebSocket.OPEN) {
        addErrorMessage("WebSocket is not connected");
        return false;
      }

      const envelope: RelayMessage = {
        type,
        request_id:
          overrides.request_id === undefined ? activeRequestIDRef.current || newRequestID() : overrides.request_id,
        user_id: userID.trim(),
        device_id: deviceID.trim(),
        session_id: overrides.session_id || sessionID.trim(),
        client_token: clientToken,
        payload: overrides.payload || {},
      };
      socket.send(JSON.stringify(envelope));
      return true;
    },
    [activeRequestIDRef, addErrorMessage, clientToken, deviceID, sessionID, socketRef, userID],
  );
}
