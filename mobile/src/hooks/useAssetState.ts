import { useCallback, useState } from "react";

import type { AssetSummary } from "../protocol";

const emptyAssets: AssetSummary[] = [];

// Hide results from the previous session while the new session's list loads.
export function useAssetState(sessionID: string) {
  const [snapshot, setSnapshot] = useState<{ sessionID: string; assets: AssetSummary[] } | null>(null);
  const setAssets = useCallback((targetSessionID: string, assets: AssetSummary[]) => {
    setSnapshot({ sessionID: targetSessionID, assets });
  }, []);
  const clearAssets = useCallback(() => setSnapshot(null), []);

  return { assets: snapshot?.sessionID === sessionID ? snapshot.assets : emptyAssets, clearAssets, setAssets };
}
