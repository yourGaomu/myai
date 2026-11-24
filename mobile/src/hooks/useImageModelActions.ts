import { useCallback } from "react";

import type {
  ImageModelConfigAddPayload,
  ImageModelConfigDefaultSetPayload,
  ImageModelConfigDeletePayload,
  ImageModelConfigEnabledSetPayload,
  ImageModelConfigTestPayload,
  ImageModelConfigUpdatePayload,
  RelayMessage,
} from "../protocol";
import type { PendingAction } from "../types/app";
import { newRequestID } from "../utils/ids";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;
type Args = {
  sendEnvelope: SendEnvelope;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

export function useImageModelActions({ sendEnvelope, startPending, stopPending }: Args) {
  const send = useCallback((type: RelayMessage["type"], payload?: object) => {
    startPending("imageModels");
    if (!sendEnvelope(type, { request_id: newRequestID(), payload })) {
      stopPending("imageModels");
      return false;
    }
    return true;
  }, [sendEnvelope, startPending, stopPending]);

  const refresh = useCallback(() => send("image_model_list", {}), [send]);
  const add = useCallback((payload: ImageModelConfigAddPayload) => send("image_model_config_add", payload), [send]);
  const test = useCallback((payload: ImageModelConfigTestPayload) => send("image_model_config_test", payload), [send]);
  const update = useCallback((payload: ImageModelConfigUpdatePayload) => send("image_model_config_update", payload), [send]);
  const remove = useCallback((id: string) => send("image_model_config_delete", { id } satisfies ImageModelConfigDeletePayload), [send]);
  const setEnabled = useCallback((id: string, enabled: boolean) => send("image_model_config_enabled_set", { id, enabled } satisfies ImageModelConfigEnabledSetPayload), [send]);
  const setDefault = useCallback((id: string) => send("image_model_config_default_set", { id } satisfies ImageModelConfigDefaultSetPayload), [send]);

  return { add, refresh, remove, setDefault, setEnabled, test, update };
}
