import { newRequestID } from "./ids";

export type RelayPairing = {
  pairingID: string;
  relayURL: string;
  userID: string;
  deviceID: string;
  clientToken: string;
};

export function normalizeRelayUrl(raw: string): string {
  let value = raw.trim();
  if (!value || /[\s\u0000-\u001f\u007f\\?#]/.test(value)) return "";
  if (/^wss?:\/\//i.test(value)) {
    value = value.replace(/^ws/i, "http");
  } else if (!/^https?:\/\//i.test(value)) {
    if (/^[a-z][a-z\d+.-]*:\/\//i.test(value) || /^(?:https?|wss?):/i.test(value)) return "";
    const local = /^(?:localhost|127(?:\.\d{1,3}){3}|10(?:\.\d{1,3}){3}|192\.168(?:\.\d{1,3}){2}|172\.(?:1[6-9]|2\d|3[01])(?:\.\d{1,3}){2}|\[::1\])(?::\d+)?(?:\/|$)/i.test(value);
    value = `${local ? "http" : "https"}://${value}`;
  }
  const parts = /^(https?):\/\/([^/]+)(\/.*)?$/i.exec(value);
  if (!parts || parts[2].includes("@")) return "";
  const scheme = parts[1].toLowerCase();
  try {
    const url = new URL(`${scheme}://${parts[2]}${parts[3] || ""}`);
    const host = url.host.toLowerCase();
    if (!host || !url.hostname || (url.port && Number(url.port) === 0)) return "";
    const path = url.pathname.replace(/\/+$/, "").replace(/\/ws\/client$/i, "");
    return `${scheme}://${host}${path}`;
  } catch {
    return "";
  }
}

export function websocketURL(relayURL: string) {
  return relayURL ? relayURL.replace(/^http/, "ws") + "/ws/client" : "";
}

function field(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

export function restoreRelaySettings(raw: unknown): { relayURL: string; pairing: RelayPairing | null } {
  const data = raw && typeof raw === "object" ? raw as Record<string, unknown> : {};
  const relayURL = normalizeRelayUrl(field(data.relayURL));
  const userID = field(data.userID);
  const deviceID = field(data.deviceID);
  const clientToken = field(data.clientToken);
  // The old settings format did not have a pairing ID. Keep the original
  // issuer when migrating it and never transfer a token to the default URL.
  const pairing = relayURL && userID && deviceID && clientToken
    ? { pairingID: field(data.pairingID) || newRequestID(), relayURL, userID, deviceID, clientToken }
    : null;
  return { relayURL, pairing };
}

export function readPairingResponse(raw: unknown) {
  const data = raw && typeof raw === "object" ? raw as Record<string, unknown> : {};
  const userID = field(data.user_id);
  const deviceID = field(data.device_id);
  const clientToken = field(data.client_token);
  if (!userID || !deviceID || !clientToken) throw new Error("Relay 返回的配对信息不完整，请重新配对。");
  return { userID, deviceID, clientToken };
}
