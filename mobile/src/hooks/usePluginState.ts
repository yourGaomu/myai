import { useCallback, useState } from "react";

import type { PluginInfo, PluginListResultPayload, PluginMutationResultPayload } from "../protocol";

// 插件状态只保存 Agent 端的发现结果；插件进程始终运行在电脑 Agent 上。
export function usePluginState() {
  const [pluginRoot, setPluginRoot] = useState("");
  const [pluginMessage, setPluginMessage] = useState("");
  const [plugins, setPlugins] = useState<PluginInfo[]>([]);

  const applyPluginList = useCallback((payload?: PluginListResultPayload | PluginMutationResultPayload) => {
    setPlugins(payload?.plugins || []);
    if ("root" in (payload || {})) {
      setPluginRoot((payload as PluginListResultPayload)?.root || "");
    }
    setPluginMessage(payload?.message || "");
  }, []);

  const clearPlugins = useCallback(() => {
    setPluginRoot("");
    setPluginMessage("");
    setPlugins([]);
  }, []);

  return {
    applyPluginList,
    clearPlugins,
    pluginMessage,
    pluginRoot,
    plugins,
  };
}
