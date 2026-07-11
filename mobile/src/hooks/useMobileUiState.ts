import { useState } from "react";

import type { ViewMode } from "../types/app";

// 保存连接、当前视图和当前 Session 等页面级 UI 状态。
export function useMobileUiState() {
  const [bindCode, setBindCode] = useState("");
  const [sessionID, setSessionID] = useState("");
  const [connected, setConnected] = useState(false);
  const [status, setStatus] = useState("Not paired");
  const [viewMode, setViewMode] = useState<ViewMode>("chat");

  return {
    bindCode,
    connected,
    sessionID,
    setBindCode,
    setConnected,
    setSessionID,
    setStatus,
    setViewMode,
    status,
    viewMode,
  };
}
