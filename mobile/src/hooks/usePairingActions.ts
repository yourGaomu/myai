import { useCallback } from "react";
import { Alert } from "react-native";

import { pairWithRelay } from "../services/pairing";
import type { PendingAction } from "../types/app";
import { messageFromError } from "../utils/format";

type Args = {
  addErrorMessage: (message: string) => void;
  bindCode: string;
  normalizedRelayURL: string;
  setBindCode: (bindCode: string) => void;
  setClientToken: (token: string) => void;
  setDeviceID: (deviceID: string) => void;
  setStatus: (status: string) => void;
  setUserID: (userID: string) => void;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

// 配对通过 HTTP 换取 client_token；后续 WebSocket 请求都使用该 token 鉴权。
export function usePairingActions({
  addErrorMessage,
  bindCode,
  normalizedRelayURL,
  setBindCode,
  setClientToken,
  setDeviceID,
  setStatus,
  setUserID,
  startPending,
  stopPending,
}: Args) {
  const pairDevice = useCallback(async () => {
    const code = bindCode.trim();
    if (!code) {
      Alert.alert("Bind code required", "Enter the code printed by the PC Agent.");
      return;
    }

    startPending("pair");
    setStatus("Pairing");
    try {
      const data = await pairWithRelay(normalizedRelayURL, code);
      setUserID(data.user_id || "local");
      setDeviceID(data.device_id || "pc-local");
      setClientToken(data.client_token || "");
      setBindCode("");
      setStatus(`Paired ${data.user_id}/${data.device_id}`);
    } catch (error) {
      setStatus("Pair failed");
      addErrorMessage(messageFromError(error));
    } finally {
      stopPending("pair");
    }
  }, [
    addErrorMessage,
    bindCode,
    normalizedRelayURL,
    setBindCode,
    setClientToken,
    setDeviceID,
    setStatus,
    setUserID,
    startPending,
    stopPending,
  ]);

  return { pairDevice };
}
