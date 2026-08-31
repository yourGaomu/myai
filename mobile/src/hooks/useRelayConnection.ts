import { useCallback, useEffect, useRef, type RefObject } from "react";

import type { RelayMessage } from "../protocol";
import type { PendingAction } from "../types/app";
import { messageFromError } from "../utils/format";
import { websocketURL } from "../utils/relay";

const connectTimeoutMs = 10000;
const heartbeatIntervalMs = 25000;

type Args = {
  addErrorMessage: (message: string) => void;
  clientToken: string;
  normalizedRelayURL: string;
  onConnected: () => void;
  onDisconnected: () => void;
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
  onDisconnected,
  onMessage,
  setConnected,
  setStatus,
  socketRef,
  startPending,
  stopPending,
}: Args) {
  const heartbeatTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    return () => {
      if (heartbeatTimerRef.current) {
        clearInterval(heartbeatTimerRef.current);
        heartbeatTimerRef.current = null;
      }
      const socket = socketRef.current;
      if (socket) {
        socket.onclose = null;
        socket.onerror = null;
        socket.onmessage = null;
        socket.onopen = null;
        socket.close();
        socketRef.current = null;
      }
    };
  }, [socketRef]);

  return useCallback(() => {
    if (!clientToken) {
      addErrorMessage("请先在设置中完成手机配对，再连接 Relay。");
      stopPending("connect");
      return;
    }

    const previousSocket = socketRef.current;
    if (heartbeatTimerRef.current) {
      clearInterval(heartbeatTimerRef.current);
      heartbeatTimerRef.current = null;
    }
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
      addErrorMessage(`Relay 连接失败：${messageFromError(error)}`);
      onDisconnected();
      return;
    }

    const timeoutID = setTimeout(() => {
      if (socketRef.current !== socket || socket.readyState === WebSocket.OPEN) {
        return;
      }
      if (heartbeatTimerRef.current) {
        clearInterval(heartbeatTimerRef.current);
        heartbeatTimerRef.current = null;
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
      addErrorMessage("Relay 连接超时，请检查地址、端口和服务器是否已启动。");
      onDisconnected();
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
      // Send one heartbeat immediately so a newly restored connection is
      // visible to the Relay before the periodic keep-alive interval elapses.
      try {
        socket.send(JSON.stringify({
          type: "heartbeat",
          payload: { time: new Date().toISOString() },
        } satisfies RelayMessage));
      } catch (error) {
        console.warn("Relay initial heartbeat failed", error);
        socket.close();
        return;
      }
      heartbeatTimerRef.current = setInterval(() => {
        if (socketRef.current !== socket || socket.readyState !== WebSocket.OPEN) {
          return;
        }
        const heartbeat: RelayMessage = {
          type: "heartbeat",
          payload: { time: new Date().toISOString() },
        };
        try {
          socket.send(JSON.stringify(heartbeat));
        } catch (error) {
          console.warn("Relay heartbeat failed", error);
          socket.close();
        }
      }, heartbeatIntervalMs);
      onConnected();
    };
    socket.onclose = () => {
      clearConnectTimeout();
      if (socketRef.current !== socket) {
        return;
      }
      if (heartbeatTimerRef.current) {
        clearInterval(heartbeatTimerRef.current);
        heartbeatTimerRef.current = null;
      }
      socketRef.current = null;
      stopPending("connect");
      setConnected(false);
      setStatus("Disconnected");
      onDisconnected();
    };
    socket.onerror = () => {
      clearConnectTimeout();
      if (socketRef.current !== socket) {
        return;
      }
      stopPending("connect");
      setConnected(false);
      setStatus("WebSocket error");
      addErrorMessage("Relay 连接异常，请检查网络和服务器日志。");
      onDisconnected();
    };
    socket.onmessage = (event) => {
      // 此处只完成 JSON 解码，具体消息类型由 useRemoteMessageHandler 统一归并到各状态仓库。
      try {
        onMessage(JSON.parse(event.data) as RelayMessage);
      } catch (error) {
        addErrorMessage(`Relay 返回的数据无法解析：${messageFromError(error)}`);
      }
    };
  }, [
    addErrorMessage,
    clientToken,
    normalizedRelayURL,
    onConnected,
    onDisconnected,
    onMessage,
    setConnected,
    setStatus,
    socketRef,
    startPending,
    stopPending,
  ]);
}
