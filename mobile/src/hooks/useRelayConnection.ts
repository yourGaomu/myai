import { useCallback, useEffect, useLayoutEffect, useRef, type RefObject } from "react";
import { AppState } from "react-native";

import {
  drainRelayForegroundServiceMessages,
  getRelayForegroundServiceState,
  hasRelayForegroundService,
  sendRelayForegroundServiceMessage,
  startRelayForegroundService,
  stopLegacyRelayForegroundService,
  subscribeRelayForegroundService,
  type RelayForegroundServiceState,
} from "../native/relayForegroundService";
import type { RelayMessage } from "../protocol";
import type { PendingAction } from "../types/app";
import type { RelayPairing } from "../utils/relayPairing";
import { startRelayWebSocket, type RelaySendHandle } from "../utils/relayWebSocket";

export type ConnectionScope = { pairingID: string; connectionID: string };

type Args = {
  pairing: RelayPairing;
  currentPairingRef: RefObject<RelayPairing | null>;
  relayScopeRef: RefObject<ConnectionScope>;
  socketRef: RefObject<RelaySendHandle | null>;
  onConnected: () => void;
  onDisconnected: () => void;
  onMessage: (message: RelayMessage) => void;
  setConnected: (connected: boolean) => void;
  setStatus: (status: string) => void;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

export function useRelayConnection({
  pairing, currentPairingRef, relayScopeRef, socketRef,
  onConnected, onDisconnected, onMessage, setConnected, setStatus, startPending, stopPending,
}: Args) {
  const callbacks = useRef({ onConnected, onDisconnected, onMessage });
  const connectRef = useRef<(() => void) | null>(null);
  useLayoutEffect(() => {
    callbacks.current = { onConnected, onDisconnected, onMessage };
  }, [onConnected, onDisconnected, onMessage]);

  useEffect(() => {
    const native = hasRelayForegroundService();
    let mounted = true;
    let active = AppState.currentState === "active";
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let retryAttempt = 0;
    let closeWebSocket: (() => void) | undefined;
    let authRejected = false;
    let nativeStartTimer: ReturnType<typeof setTimeout> | undefined;
    let nativeStatus: RelayForegroundServiceState["state"] | undefined;
    let nativeRevision = -1;
    let drainInProgress = false;
    let drainRequested = false;
    let legacyServiceRetired: Promise<boolean> | undefined;

    function isCurrent() {
      return mounted && currentPairingRef.current === pairing;
    }

    function clearRetry() {
      clearTimeout(retryTimer);
      retryTimer = undefined;
    }

    function clearStartTimer() {
      clearTimeout(nativeStartTimer);
      nativeStartTimer = undefined;
    }

    function disconnect(notify: boolean) {
      const previous = socketRef.current;
      socketRef.current = null;
      if (relayScopeRef.current.pairingID === pairing.pairingID) {
        relayScopeRef.current = { pairingID: pairing.pairingID, connectionID: "" };
      }
      if (previous && notify) {
        setConnected(false);
        callbacks.current.onDisconnected();
      }
    }

    function showOffline(
      state: "connecting" | "authenticating" | "reconnecting" | "stopped" | "auth_rejected" | "error",
      status: string,
    ) {
      disconnect(true);
      setStatus(status);
      if (state === "connecting" || state === "authenticating") startPending("connect");
      else stopPending("connect");
    }

    function install(handle: RelaySendHandle) {
      if (!isCurrent() || socketRef.current?.connectionID === handle.connectionID) return;
      if (socketRef.current) disconnect(true);
      socketRef.current = handle;
      relayScopeRef.current = { pairingID: pairing.pairingID, connectionID: handle.connectionID };
      retryAttempt = 0;
      clearRetry();
      setConnected(true);
      setStatus("Connected");
      stopPending("connect");
      callbacks.current.onConnected();
    }

    function dispatch(message: RelayMessage) {
      if (isCurrent() && socketRef.current && message && typeof message.type === "string" &&
        (!message.user_id || message.user_id === pairing.userID) &&
        (!message.device_id || message.device_id === pairing.deviceID)) {
        callbacks.current.onMessage(message);
      }
    }

    function scheduleRetry() {
      if (!isCurrent() || !active || retryTimer !== undefined || authRejected) return;
      const delay = Math.min(30_000, 500 * 2 ** retryAttempt);
      retryAttempt = Math.min(retryAttempt + 1, 6);
      retryTimer = setTimeout(() => {
        retryTimer = undefined;
        connect(false);
      }, delay);
    }

    async function drainNative() {
      drainRequested = true;
      if (drainInProgress || !isCurrent() || !socketRef.current) return;
      drainInProgress = true;
      try {
        while (drainRequested && isCurrent() && socketRef.current) {
          drainRequested = false;
          const handle: RelaySendHandle = socketRef.current;
          const messages = await drainRelayForegroundServiceMessages(pairing.pairingID, handle.connectionID);
          for (const raw of messages) {
            if (!isCurrent() || socketRef.current !== handle) break;
            let message: RelayMessage;
            try { message = JSON.parse(raw) as RelayMessage; } catch { continue; }
            dispatch(message);
          }
        }
      } catch {
        // Another wakeup or foreground transition can drain the current socket.
      } finally {
        drainInProgress = false;
      }
    }

    function applyNativeState(state: RelayForegroundServiceState | null) {
      if (!isCurrent() || !state || !Number.isSafeInteger(state.revision) || state.revision <= nativeRevision) return;
      nativeRevision = state.revision;
      if (state.pairingID !== pairing.pairingID) {
        if (nativeStatus) showOffline("stopped", "Disconnected");
        nativeStatus = undefined;
        return;
      }
      nativeStatus = state.state;
      clearStartTimer();
      authRejected = state.state === "auth_rejected";
      if (state.state === "connected" && state.connectionID) {
        const connectionID = state.connectionID;
        install({
          pairing,
          connectionID,
          send: (data) => sendRelayForegroundServiceMessage(pairing.pairingID, connectionID, data),
        });
        void drainNative();
      } else if (state.state !== "connected") {
        const status = state.status || "Disconnected";
        showOffline(state.state, state.error && state.error !== status ? `${status} · ${state.error}` : status);
      }
    }

    async function syncNative() {
      try {
        applyNativeState(await getRelayForegroundServiceState());
      } catch {
        // The current generation can still start and subscribe to state events.
      }
    }

    function connect(retry: boolean) {
      if (!isCurrent() || !active || (authRejected && !retry)) return;
      clearRetry();
      if (socketRef.current || closeWebSocket || nativeStartTimer !== undefined) return;
      if (native) {
        if (nativeStatus === "connecting" || nativeStatus === "authenticating") return;
        if ((nativeStatus === "reconnecting" || nativeStatus === "error") && !retry) return;
        authRejected = false;
        showOffline("connecting", "Connecting");
        nativeStartTimer = setTimeout(() => {
          nativeStartTimer = undefined;
          if (isCurrent()) {
            showOffline("reconnecting", "Reconnecting · Android 后台服务暂未就绪");
            void syncNative().then(() => connect(false));
          }
        }, 12_000);
        void startRelayForegroundService(pairing, retry).then((started) => {
          if (!started && isCurrent() && nativeStartTimer !== undefined) onStartFailure();
          else void syncNative();
        }, onStartFailure);
        return;
      }

      authRejected = false;
      showOffline("connecting", "Connecting");
      // An old native module cannot provide a scoped handle. Retire its
      // foreground connection before using React Native's WebSocket fallback.
      legacyServiceRetired ??= stopLegacyRelayForegroundService();
      const attempt = legacyServiceRetired;
      void attempt.then(() => {
        if (!isCurrent() || !active || closeWebSocket) return;
        try {
          closeWebSocket = startRelayWebSocket(pairing, {
            onAuthenticating: () => { if (isCurrent()) setStatus("Authenticating"); },
            onAuthenticated: install,
            onMessage: dispatch,
            onLost: ({ state, status }) => {
              closeWebSocket = undefined;
              if (!isCurrent()) return;
              authRejected = state === "auth_rejected";
              showOffline(state, status);
              scheduleRetry();
            },
          });
        } catch {
          onStartFailure();
        }
      }, () => {
        legacyServiceRetired = undefined;
        onStartFailure();
      });
    }

    function onStartFailure() {
      if (!isCurrent()) return;
      clearStartTimer();
      showOffline("reconnecting", "Reconnecting · 无法启动中继连接");
      scheduleRetry();
    }

    function onForeground() {
      if (!isCurrent()) return;
      if (native) {
        void syncNative().then(() => {
          connect(false);
          void drainNative();
        });
      } else {
        connect(false);
      }
    }

    const unsubscribe = native
      ? subscribeRelayForegroundService({ onState: applyNativeState, onMessageAvailable: () => { void drainNative(); } })
      : undefined;
    const appState = AppState.addEventListener("change", (state) => {
      const wasActive = active;
      active = state === "active";
      if (active && !wasActive) onForeground();
      else if (!active) clearRetry();
    });
    connectRef.current = () => connect(true);
    if (native || active) onForeground();
    return () => {
      mounted = false;
      connectRef.current = null;
      unsubscribe?.();
      appState.remove();
      clearRetry();
      clearStartTimer();
      closeWebSocket?.();
      disconnect(false);
      // Android keeps the scoped background connection across JS remounts.
    };
  }, [pairing, currentPairingRef, relayScopeRef, socketRef, setConnected, setStatus, startPending, stopPending]);

  return useCallback(() => connectRef.current?.(), []);
}
