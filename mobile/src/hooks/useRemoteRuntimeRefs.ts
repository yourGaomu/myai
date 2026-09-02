import { useEffect, useRef } from "react";

export type RelaySocketHandle = {
  readonly readyState: number;
  send: (data: string) => void;
  close: () => void;
};

// 这些值参与异步 WebSocket 回调但不直接渲染，因此使用 Ref 避免闭包读取旧状态。
export function useRemoteRuntimeRefs(currentSessionID: string) {
  const socketRef = useRef<RelaySocketHandle | null>(null);
  const activeRequestIDRef = useRef("");
  const sessionIDRef = useRef("");
  const historySessionIDRef = useRef("");
  const pendingHistorySessionIDRef = useRef("");
  const requestSessionMapRef = useRef<Record<string, string>>({});

  useEffect(() => {
    sessionIDRef.current = currentSessionID;
  }, [currentSessionID]);

  return {
    activeRequestIDRef,
    historySessionIDRef,
    pendingHistorySessionIDRef,
    requestSessionMapRef,
    sessionIDRef,
    socketRef,
  };
}
