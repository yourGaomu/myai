import { useCallback, useState } from "react";

import type { AssetSummary } from "../protocol";

// 保存当前会话的共享资源列表；资源上传和协议请求由独立 Action Hook 负责。
export function useAssetState() {
  const [assets, setAssets] = useState<AssetSummary[]>([]);

  const clearAssets = useCallback(() => setAssets([]), []);

  return {
    assets,
    clearAssets,
    setAssets,
  };
}
