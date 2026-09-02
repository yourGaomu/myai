import { useCallback } from "react";

import type { RelayMessage } from "../protocol";
import type { PendingAction } from "../types/app";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  clientToken: string;
  onError: (message: string) => void;
  sendEnvelope: SendEnvelope;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

export function usePluginActions({ clientToken, onError, sendEnvelope, startPending, stopPending }: Args) {
  const setPluginEnabled = useCallback((pluginID: string, enabled: boolean) => {
    if (!clientToken) {
      onError("请先完成配对");
      return false;
    }
    const normalizedID = pluginID.trim();
    if (!normalizedID) {
      onError("缺少插件 ID");
      return false;
    }
    startPending("plugins");
    const sent = sendEnvelope(enabled ? "plugin_enable" : "plugin_disable", {
      request_id: newRequestID(),
      payload: { plugin_id: normalizedID },
    });
    if (!sent) {
      stopPending("plugins");
      onError("请求未发送，请检查 Relay 连接");
    }
    return sent;
  }, [clientToken, onError, sendEnvelope, startPending, stopPending]);

  return { setPluginEnabled };
}
