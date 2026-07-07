import { useCallback, useState } from "react";

import type { AssetSummary } from "../protocol";

export function useAssetState() {
  const [assets, setAssets] = useState<AssetSummary[]>([]);

  const clearAssets = useCallback(() => setAssets([]), []);

  return {
    assets,
    clearAssets,
    setAssets,
  };
}
