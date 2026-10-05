import { useCallback, useState } from "react";

import type { MCPReloadResultPayload } from "../protocol";

// MCP 运行时重载状态管理
export function useMCPState() {
  const [mcpMessage, setMcpMessage] = useState("");

  const applyMCPReload = useCallback((payload?: MCPReloadResultPayload) => {
    setMcpMessage(payload?.message || (payload?.reloaded ? "MCP 重载成功" : "MCP 重载失败"));
  }, []);

  const applyMCPError = useCallback((message: string) => {
    setMcpMessage(message || "MCP 重载失败");
  }, []);

  const clearMCPMessage = useCallback(() => {
    setMcpMessage("");
  }, []);

  return {
    applyMCPError,
    applyMCPReload,
    clearMCPMessage,
    mcpMessage,
    setMcpMessage,
  };
}
