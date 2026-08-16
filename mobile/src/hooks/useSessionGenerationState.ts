import { useCallback, useMemo, useState } from "react";

import type { SessionGenerationPreferences, SessionGenerationResultPayload } from "../protocol";

export function useSessionGenerationState(sessionID: string) {
  const [preferencesBySession, setPreferencesBySession] = useState<Record<string, SessionGenerationPreferences>>({});
  const [status, setStatus] = useState({ error: false, message: "" });

  const applyPreferences = useCallback((payload?: SessionGenerationResultPayload) => {
    const preferences = payload?.preferences;
    const sessionID = preferences?.session_id?.trim();
    if (!preferences || !sessionID) {
      return;
    }
    setPreferencesBySession((current) => ({ ...current, [sessionID]: preferences }));
    setStatus({ error: false, message: payload?.message || "" });
  }, []);

  const applyError = useCallback((error: string) => {
    setStatus({ error: true, message: error.trim() });
  }, []);

  const clearPreferences = useCallback(() => {
    setPreferencesBySession({});
    setStatus({ error: false, message: "" });
  }, []);

  const preferences = useMemo(() => preferencesBySession[sessionID.trim()], [preferencesBySession, sessionID]);

  return {
    applyError,
    applyPreferences,
    clearPreferences,
    preferences,
    status,
  };
}
