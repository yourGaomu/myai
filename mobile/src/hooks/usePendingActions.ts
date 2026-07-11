import { useCallback, useMemo, useState } from "react";

import type { PendingAction } from "../types/app";

const initialPendingActions: Record<PendingAction, boolean> = {
  connect: false,
  pair: false,
  sessions: false,
  models: false,
  skills: false,
  settings: false,
  plan: false,
  assets: false,
  files: false,
  changes: false,
  history: false,
  diff: false,
  revert: false,
  upload: false,
  pause: false,
};

// 各功能独立维护 pending，避免一个慢请求错误地锁住所有无关页面操作。
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
