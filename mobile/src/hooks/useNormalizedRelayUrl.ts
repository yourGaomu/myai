import { useMemo } from "react";

export function normalizeRelayUrl(relayURL: string) {
  let value = relayURL.trim().replace(/\/+$/, "");
  if (!value) {
    return value;
  }

  // Accept the forms users commonly paste into settings. Pairing is HTTP,
  // while the WebSocket helper derives ws/wss from this normalized origin.
  if (/^wss?:\/\//i.test(value)) {
    value = value.replace(/^ws/i, "http");
  } else if (!/^https?:\/\//i.test(value)) {
    const isLocal = /^(localhost|127(?:\.\d{1,3}){3}|10(?:\.\d{1,3}){3}|192\.168(?:\.\d{1,3}){2}|172\.(?:1[6-9]|2\d|3[01])(?:\.\d{1,3}){2})(?::\d+)?$/i.test(value);
    value = `${isLocal ? "http" : "https"}://${value}`;
  }

  value = value.replace(/^(https?):\/\//i, (_match, scheme: string) => `${scheme.toLowerCase()}://`);
  // Avoid generating /ws/client/ws/client when a WebSocket URL was pasted.
  return value.replace(/\/ws\/client$/i, "");
}

export function useNormalizedRelayUrl(relayURL: string) {
  return useMemo(() => normalizeRelayUrl(relayURL), [relayURL]);
}
