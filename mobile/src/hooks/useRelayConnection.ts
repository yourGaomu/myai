import { useCallback, type RefObject } from "react";

import type { RelayMessage } from "../protocol";
import type { PendingAction } from "../types/app";
import { messageFromError } from "../utils/format";
import { websocketURL } from "../utils/relay";

const connectTimeoutMs = 10000;

type Args = {
  addErrorMessage: (message: string) => void;
  clientToken: string;
  normalizedRelayURL: string;
  onConnected: () => void;
  onMessage: (message: RelayMessage) => void;
  setConnected: (connected: boolean) => void;
  setStatus: (status: string) => void;
  socketRef: RefObject<WebSocket | null>;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

// 管理手机到 Relay 的单条 WebSocket 连接；配对 token、消息分发和断线清理由这里统一处理。
export function useRelayConnection({
  addErrorMessage,
  clientToken,
  normalizedRelayURL,
  onConnected,
  onMessage,
  setConnected,
  setStatus,
  socketRef,
  startPending,
  stopPending,
}: Args) {
  return useCallback(() => {
    if (!clientToken) {
      addErrorMessage("Pair this phone before connecting");
      stopPending("connect");
      return;
    }

    const previousSocket = socketRef.current;
    if (previousSocket) {
      previousSocket.onclose = null;
      previousSocket.onerror = null;
      previousSocket.onmessage = null;
      previousSocket.onopen = null;
      previousSocket.close();
    }

    startPending("connect");
    let socket: WebSocket;
    try {
      socket = new WebSocket(websocketURL(normalizedRelayURL));
    } catch (error) {
      socketRef.current = null;
      stopPending("connect");
      setConnected(false);
      setStatus("WebSocket error");
      addErrorMessage(`WebSocket connection failed: ${messageFromError(error)}`);
      return;
    }

    const timeoutID = setTimeout(() => {
      if (socketRef.current !== socket || socket.readyState === WebSocket.OPEN) {
        return;
      }
      socket.onclose = null;
      socket.onerror = null;
      socket.onmessage = null;
      socket.onopen = null;
      socket.close();
      socketRef.current = null;
      stopPending("connect");
      setConnected(false);
      setStatus("Connection timeout");
      addErrorMessage("WebSocket connection timed out. Check the relay URL and make sure the relay server is running.");
    }, connectTimeoutMs);

    const clearConnectTimeout = () => clearTimeout(timeoutID);

    socketRef.current = socket;
    setStatus("Connecting");

    socket.onopen = () => {
      // 连接成功后立即刷新远程状态，避免界面继续展示断线前缓存的 Session。
      clearConnectTimeout();
      if (socketRef.current !== socket) {
        return;
      }
      stopPending("connect");
      setConnected(true);
      setStatus("Connected");
      onConnected();
    };
    socket.onclose = () => {
      clearConnectTimeout();
      if (socketRef.current !== socket) {
        return;
      }
      socketRef.current = null;
      stopPending("connect");
      setConnected(false);
      setStatus("Disconnected");
    };
    socket.onerror = () => {
      clearConnectTimeout();
      if (socketRef.current !== socket) {
        return;
      }
      stopPending("connect");
      setConnected(false);
      setStatus("WebSocket error");
      addErrorMessage("WebSocket connection error");
    };
    socket.onmessage = (event) => {
      // 此处只完成 JSON 解码，具体消息类型由 useRemoteMessageHandler 统一归并到各状态仓库。
      try {
        onMessage(JSON.parse(event.data) as RelayMessage);
      } catch (error) {
        addErrorMessage(`Invalid relay message: ${messageFromError(error)}`);
      }
    };
  }, [
    addErrorMessage,
    clientToken,
    normalizedRelayURL,
    onConnected,
    onMessage,
    setConnected,
    setStatus,
    socketRef,
    startPending,
    stopPending,
  ]);
}
