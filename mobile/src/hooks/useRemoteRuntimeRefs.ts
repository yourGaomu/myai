import { useEffect, useRef } from "react";

export function useRemoteRuntimeRefs(currentSessionID: string) {
  const socketRef = useRef<WebSocket | null>(null);
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
