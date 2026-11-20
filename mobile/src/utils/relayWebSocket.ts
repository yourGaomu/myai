import type { RelayMessage } from "../protocol";
import { newRequestID } from "./ids";
import { websocketURL, type RelayPairing } from "./relayPairing";

const heartbeatIntervalMs = 25_000;
const responseTimeoutMs = 10_000;
const maxEarlyMessages = 256;
const maxEarlyCharacters = 8 * 1024 * 1024;

export type RelaySendHandle = {
  pairing: RelayPairing;
  connectionID: string;
  send: (data: string) => void | Promise<boolean>;
};

export type RelaySocketLoss = { state: "reconnecting" | "auth_rejected"; status: string };

function heartbeatOutcome(message: RelayMessage, expectedID: string) {
  if (!expectedID || message.request_id !== expectedID) return undefined;
  const payload = message.payload as { received?: string; error_code?: string; message?: string } | undefined;
  if (message.type === "heartbeat" && payload?.received === "heartbeat") return "authenticated";
  if (message.type !== "error") return undefined;
  if (payload?.error_code === "client_auth_rejected") return "rejected";
  if (payload?.error_code === "client_auth_unavailable" || payload?.message === "client token is invalid or expired") {
    return "unavailable";
  }
  return undefined;
}

export function startRelayWebSocket(
  pairing: RelayPairing,
  callbacks: {
    onAuthenticating: () => void;
    onAuthenticated: (handle: RelaySendHandle) => void;
    onMessage: (message: RelayMessage) => void;
    onLost: (loss: RelaySocketLoss) => void;
  },
) {
  const socket = new WebSocket(websocketURL(pairing.relayURL));
  const connectionID = newRequestID();
  let closed = false;
  let authenticated = false;
  let heartbeatID = "";
  let deadline: ReturnType<typeof setTimeout> | undefined;
  let heartbeatTimer: ReturnType<typeof setTimeout> | undefined;
  const earlyMessages: RelayMessage[] = [];
  let earlyCharacters = 0;

  function close() {
    if (closed) return;
    closed = true;
    clearTimeout(deadline);
    clearTimeout(heartbeatTimer);
    earlyMessages.length = 0;
    earlyCharacters = 0;
    // Keep onopen so React Native's Android fallback can close a socket that
    // finishes its native handshake after close() was first called.
    socket.onclose = null;
    socket.onerror = null;
    socket.onmessage = null;
    try { socket.close(); } catch { /* A late onopen will close the socket. */ }
  }

  function lose(status: string, state: RelaySocketLoss["state"] = "reconnecting") {
    if (closed) return;
    close();
    callbacks.onLost({ state, status });
  }

  function sendHeartbeat() {
    if (closed) return;
    heartbeatID = newRequestID();
    const heartbeat: RelayMessage = {
      type: "heartbeat",
      request_id: heartbeatID,
      user_id: pairing.userID,
      device_id: pairing.deviceID,
      client_token: pairing.clientToken,
      payload: { time: new Date().toISOString() },
    };
    try {
      socket.send(JSON.stringify(heartbeat));
      deadline = setTimeout(() => lose("Reconnecting · 中继认证回执超时"), responseTimeoutMs);
    } catch {
      lose("Reconnecting · 心跳发送失败");
    }
  }

  socket.onopen = () => {
    if (closed) {
      socket.close();
      return;
    }
    clearTimeout(deadline);
    callbacks.onAuthenticating();
    sendHeartbeat();
  };
  socket.onclose = () => lose("Reconnecting · 中继连接已断开");
  socket.onerror = () => lose("Reconnecting · 中继连接失败");
  socket.onmessage = (event) => {
    if (closed || typeof event.data !== "string") return;
    let message: RelayMessage;
    try {
      message = JSON.parse(event.data) as RelayMessage;
    } catch {
      return;
    }
    if (!message || typeof message.type !== "string" ||
      (message.user_id && message.user_id !== pairing.userID) ||
      (message.device_id && message.device_id !== pairing.deviceID)) return;

    const outcome = heartbeatOutcome(message, heartbeatID);
    if (outcome && message.user_id === pairing.userID && message.device_id === pairing.deviceID) {
      clearTimeout(deadline);
      heartbeatID = "";
      if (outcome === "rejected") {
        lose("配对已失效，请重新配对", "auth_rejected");
        return;
      }
      if (outcome === "unavailable") {
        lose("Reconnecting · 中继暂时无法验证配对");
        return;
      }
      heartbeatTimer = setTimeout(sendHeartbeat, heartbeatIntervalMs);
      if (!authenticated) {
        authenticated = true;
        callbacks.onAuthenticated({
          pairing,
          connectionID,
          send(data) {
            if (closed || socket.readyState !== WebSocket.OPEN) throw new Error("Relay connection is closed");
            socket.send(data);
          },
        });
        for (const pending of earlyMessages) {
          if (closed) break;
          callbacks.onMessage(pending);
        }
        earlyMessages.length = 0;
        earlyCharacters = 0;
      }
      return;
    }
    if (message.type === "heartbeat") return;
    delete message.client_token;
    if (authenticated) {
      callbacks.onMessage(message);
    } else if (earlyMessages.length < maxEarlyMessages && earlyCharacters + event.data.length <= maxEarlyCharacters) {
      // The relay can replay pending interactions before emitting the ACK.
      earlyMessages.push(message);
      earlyCharacters += event.data.length;
    } else {
      lose("Reconnecting · 中继待恢复消息过多");
    }
  };
  deadline = setTimeout(() => lose("Reconnecting · 中继连接超时"), responseTimeoutMs);

  return close;
}
