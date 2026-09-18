import { useCallback, useEffect, useRef, type RefObject } from "react";
import { Platform } from "react-native";

import type { RelayMessage } from "../protocol";
import {
  drainRelayForegroundServiceMessages,
  getRelayForegroundServiceState,
  hasRelayForegroundService,
  sendRelayForegroundServiceMessage,
  startRelayForegroundService,
  subscribeRelayForegroundService,
  type RelayForegroundServiceState,
} from "../native/relayForegroundService";
import type { PendingAction } from "../types/app";
import { messageFromError } from "../utils/format";
import { websocketURL } from "../utils/relay";
import type { RelaySocketHandle } from "./useRemoteRuntimeRefs";

const connectTimeoutMs = 10000;
const heartbeatIntervalMs = 25000;

type Args = {
  addErrorMessage: (message: string) => void;
  clientToken: string;
  deviceID: string;
  normalizedRelayURL: string;
  onConnected: () => void;
  onDisconnected: () => void;
  onMessage: (message: RelayMessage) => void;
  setConnected: (connected: boolean) => void;
  setStatus: (status: string) => void;
  socketRef: RefObject<RelaySocketHandle | null>;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
  userID: string;
};

function isNativeRelayAvailable() {
  return Platform.OS === "android" && hasRelayForegroundService();
}

// 管理手机到 Relay 的连接。Android 使用原生 Foreground Service 托管 WebSocket，
// 其他平台继续使用 JS WebSocket，保证 Expo Go 和 iOS 的开发体验不变。
export function useRelayConnection({
  addErrorMessage,
  clientToken,
  deviceID,
  normalizedRelayURL,
  onConnected,
  onDisconnected,
  onMessage,
  setConnected,
  setStatus,
  socketRef,
  startPending,
  stopPending,
  userID,
}: Args) {
  const heartbeatTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const nativeConnectedRef = useRef(false);
  const onConnectedRef = useRef(onConnected);
  const onDisconnectedRef = useRef(onDisconnected);
  const onMessageRef = useRef(onMessage);
  const addErrorMessageRef = useRef(addErrorMessage);
  const clientTokenRef = useRef(clientToken);

  useEffect(() => {
    onConnectedRef.current = onConnected;
    onDisconnectedRef.current = onDisconnected;
    onMessageRef.current = onMessage;
    addErrorMessageRef.current = addErrorMessage;
    clientTokenRef.current = clientToken;
  }, [addErrorMessage, clientToken, onConnected, onDisconnected, onMessage]);

  const consumeNativeMessages = useCallback(async () => {
    const messages = await drainRelayForegroundServiceMessages();
    for (const rawMessage of messages) {
      try {
        onMessageRef.current(JSON.parse(rawMessage) as RelayMessage);
      } catch (error) {
        addErrorMessageRef.current(`Relay 返回的数据无法解析：${messageFromError(error)}`);
      }
    }
  }, []);

  const handleNativeState = useCallback(
    (nextState: RelayForegroundServiceState) => {
      const state = nextState.state;
      if (state === "connected") {
        nativeConnectedRef.current = true;
        stopPending("connect");
        setConnected(true);
        setStatus(nextState.status || "Connected");
        void consumeNativeMessages();
        if (clientTokenRef.current) {
          onConnectedRef.current();
        }
        return;
      }

      nativeConnectedRef.current = false;
      setConnected(false);
      if (state === "connecting") {
        setStatus(nextState.status || "Connecting");
        return;
      } else if (state === "reconnecting") {
        stopPending("connect");
        setStatus(nextState.status || "Reconnecting");
      } else if (state === "error") {
        stopPending("connect");
        setStatus(nextState.status || "WebSocket error");
      } else if (state === "stopped") {
        stopPending("connect");
        setStatus("Disconnected");
      }
      onDisconnectedRef.current();
      void consumeNativeMessages();
    },
    [consumeNativeMessages, setConnected, setStatus, stopPending],
  );

  // 原生服务即使在 JS 被系统挂起也会继续收包；消息先留在原生队列，回到前台
  // 或收到可用事件后再一次性归并到 React 状态仓库。
  useEffect(() => {
    if (!isNativeRelayAvailable()) {
      return undefined;
    }

    const unsubscribe = subscribeRelayForegroundService({
      onMessageAvailable: () => {
        void consumeNativeMessages();
      },
      onState: handleNativeState,
    });
    void getRelayForegroundServiceState().then(handleNativeState);
    void consumeNativeMessages();
    return unsubscribe;
  }, [consumeNativeMessages, handleNativeState]);

  useEffect(() => {
    return () => {
      if (heartbeatTimerRef.current) {
        clearInterval(heartbeatTimerRef.current);
        heartbeatTimerRef.current = null;
      }
      // Native service is intentionally not stopped here: its purpose is to keep
      // the Relay connection alive while the Activity/JS runtime is backgrounded.
      if (isNativeRelayAvailable()) {
        socketRef.current = null;
        return;
      }
      const socket = socketRef.current;
      if (socket) {
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

    if (isNativeRelayAvailable()) {
      let nativeSocket = socketRef.current;
      if (!nativeSocket) {
        nativeSocket = {
          get readyState() {
            return nativeConnectedRef.current ? WebSocket.OPEN : WebSocket.CONNECTING;
          },
          send(data: string) {
            if (!nativeConnectedRef.current) {
              throw new Error("Relay native socket is not connected");
            }
            void sendRelayForegroundServiceMessage(data).then((sent) => {
              if (!sent) {
                nativeConnectedRef.current = false;
                setConnected(false);
                setStatus("Relay send failed");
                onDisconnectedRef.current();
              }
            });
          },
          // Closing the JS adapter must not stop the process-wide foreground service.
          close() {
            nativeConnectedRef.current = false;
          },
        };
        socketRef.current = nativeSocket;
      }

      startPending("connect");
      setStatus("Connecting");
      void startRelayForegroundService({
        websocketURL: websocketURL(normalizedRelayURL),
        userID: userID.trim(),
        deviceID: deviceID.trim(),
        clientToken: clientToken.trim(),
      })
        .then((started) => {
          if (!started) {
            socketRef.current = null;
            nativeConnectedRef.current = false;
            stopPending("connect");
            setConnected(false);
            setStatus("Foreground service unavailable");
            addErrorMessage("无法启动 Android 后台连接服务，请重新构建包含原生 Service 的 APK。");
            onDisconnectedRef.current();
            return;
          }
          return getRelayForegroundServiceState().then(handleNativeState);
        })
        .catch((error) => {
          socketRef.current = null;
          nativeConnectedRef.current = false;
          stopPending("connect");
          setConnected(false);
          setStatus("WebSocket error");
          addErrorMessage(`Relay 连接失败：${messageFromError(error)}`);
          onDisconnectedRef.current();
        });
      return;
    }

    const previousSocket = socketRef.current;
    if (previousSocket) {
      if (previousSocket.readyState === WebSocket.OPEN) {
        // Socket 已经处于连接就绪状态，切勿粗暴断开重建
        return;
      }
      if (previousSocket.readyState === WebSocket.CONNECTING) {
        // 正在连接建立中，等待握手完成即可
        return;
      }
    }

    if (heartbeatTimerRef.current) {
      clearInterval(heartbeatTimerRef.current);
      heartbeatTimerRef.current = null;
    }
    if (previousSocket) {
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
      onDisconnectedRef.current();
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
      socket.close();
      socketRef.current = null;
      stopPending("connect");
      setConnected(false);
      setStatus("Connection timeout");
      addErrorMessage("Relay 连接超时，请检查地址、端口和服务器是否已启动。");
      onDisconnectedRef.current();
    }, connectTimeoutMs);

    const clearConnectTimeout = () => clearTimeout(timeoutID);

    socketRef.current = socket;
    setStatus("Connecting");

    socket.onopen = () => {
      clearConnectTimeout();
      if (socketRef.current !== socket) {
        return;
      }
      stopPending("connect");
      setConnected(true);
      setStatus("Connected");
      try {
        socket.send(JSON.stringify({
          type: "heartbeat",
          user_id: userID.trim(),
          device_id: deviceID.trim(),
          client_token: clientToken.trim(),
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
        try {
          socket.send(JSON.stringify({
            type: "heartbeat",
            user_id: userID.trim(),
            device_id: deviceID.trim(),
            client_token: clientToken.trim(),
            payload: { time: new Date().toISOString() },
          } satisfies RelayMessage));
        } catch (error) {
          console.warn("Relay heartbeat failed", error);
          socket.close();
        }
      }, heartbeatIntervalMs);
      onConnectedRef.current();
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
      onDisconnectedRef.current();
    };
    socket.onerror = () => {
      clearConnectTimeout();
      if (socketRef.current !== socket) {
        return;
      }
      stopPending("connect");
      setConnected(false);
      setStatus("WebSocket error");
      addErrorMessageRef.current("Relay 连接异常，请检查网络和服务器日志。");
    };
    socket.onmessage = (event) => {
      try {
        onMessageRef.current(JSON.parse(event.data) as RelayMessage);
      } catch (error) {
        addErrorMessageRef.current(`Relay 返回的数据无法解析：${messageFromError(error)}`);
      }
    };
  }, [
    addErrorMessage,
    clientToken,
    deviceID,
    handleNativeState,
    normalizedRelayURL,
    setConnected,
    setStatus,
    socketRef,
    startPending,
    stopPending,
    userID,
  ]);
}
