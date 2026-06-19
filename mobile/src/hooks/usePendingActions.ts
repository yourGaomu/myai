import { useCallback, useMemo, useState } from "react";

import type { PendingAction } from "../types/app";

const initialPendingActions: Record<PendingAction, boolean> = {
  connect: false,
  pair: false,
  send: false,
  sessions: false,
  models: false,
  settings: false,
  files: false,
  changes: false,
  history: false,
  diff: false,
  revert: false,
};

export function usePendingActions() {
  const [pendingActions, setPendingActions] = useState<Record<PendingAction, boolean>>(initialPendingActions);

  const startPending = useCallback((action: PendingAction) => {
    setPendingActions((current) => ({ ...current, [action]: true }));
  }, []);

  const stopPending = useCallback((action: PendingAction) => {
    setPendingActions((current) => ({ ...current, [action]: false }));
  }, []);

  const isBusy = useMemo(
    () => Object.values(pendingActions).some(Boolean),
    [pendingActions],
  );

  return {
    isBusy,
    pendingActions,
    startPending,
    stopPending,
  };
}
