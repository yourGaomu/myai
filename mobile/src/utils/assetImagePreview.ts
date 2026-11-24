import type { AssetImagePreviewPayload, AssetImagePreviewResultPayload, ErrorPayload, RelayMessage } from "../protocol";
import type { RelayScope } from "../types/app";
import { newRequestID } from "./ids";

const requestType = "asset_image_preview";
const requestPrefix = "asset-image-preview-";
export const maxPreviewBytes = 2 * 1024 * 1024;
const maxPreviewDimension = 2048;
const maxQueuedPreviews = 32;

export type AssetImagePreview = { uri: string; width: number; height: number };
export type AssetImagePreviewError = "offline" | "timeout" | "unsupported" | "unavailable" | "invalid" | "busy";
export type AssetImagePreviewResult =
  | { status: "ready"; image: AssetImagePreview }
  | { status: "error"; reason: AssetImagePreviewError };

type Target = Readonly<RelayScope> & { sessionID: string; assetID: string };
type Listener = (result: AssetImagePreviewResult) => void;
type Entry = {
  key: string;
  target: Target;
  listeners: Set<Listener>;
  requestID?: string;
  timer?: ReturnType<typeof setTimeout>;
};
type CachedPreview = {
  target: Target;
  image: AssetImagePreview;
  expiresAt: number;
  timer: ReturnType<typeof setTimeout>;
};

export function isAssetID(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= 128 && !/[^A-Za-z0-9_-]/.test(value);
}

// This input comes only from the local document picker. In particular, neither
// a remote URL nor a data URL can become a candidate for the local image.
export function pickedImageURI(uri?: string) {
  return uri && /^(?:file:\/\/\/(?![\\/])|content:\/\/|blob:)/i.test(uri) ? uri : undefined;
}

function keyFor(target: Target) {
  return JSON.stringify([target.pairingID, target.connectionID, target.sessionID, target.assetID]);
}

function sameConnection(left: Readonly<RelayScope>, right: Readonly<RelayScope>) {
  return left.pairingID === right.pairingID && left.connectionID === right.connectionID;
}

export function decodeAssetImagePreview(payload: unknown): AssetImagePreview | null {
  const result = payload as Partial<AssetImagePreviewResultPayload> | null | undefined;
  if (!result) return null;
  const mime = result.content_type;
  if (mime !== "image/jpeg" && mime !== "image/png" && mime !== "image/gif" && mime !== "image/webp") return null;
  const { data, width, height } = result;
  if (typeof width !== "number" || !Number.isInteger(width) || width < 1 || width > maxPreviewDimension ||
      typeof height !== "number" || !Number.isInteger(height) || height < 1 || height > maxPreviewDimension) return null;
  if (typeof data !== "string" || data.length > Math.ceil(maxPreviewBytes / 3) * 4 || data.length % 4 || /[^A-Za-z0-9+/=]/.test(data)) return null;
  const padding = data.endsWith("==") ? 2 : data.endsWith("=") ? 1 : 0;
  const firstPadding = data.indexOf("=");
  if ((firstPadding >= 0 && firstPadding !== data.length - padding) || data.length / 4 * 3 - padding > maxPreviewBytes) return null;
  const prefix = data.slice(0, 16);
  const signatures: Record<string, string[]> = {
    "image/jpeg": ["/9j/"],
    "image/png": ["iVBORw0KGgo"],
    "image/gif": ["R0lGODdh", "R0lGODlh"],
  };
  if (mime === "image/webp") {
    const bytes = decodeBase64Prefix(data, 12);
    if (!bytes || bytes.length < 12 || String.fromCharCode(...bytes.slice(0,4)) !== "RIFF" || String.fromCharCode(...bytes.slice(8,12)) !== "WEBP") return null;
  } else if (!signatures[mime].some((signature) => prefix.startsWith(signature))) return null;
  return { uri: `data:${mime};base64,${data}`, width, height };
}

function decodeBase64Prefix(value: string, maxBytes: number): number[] | null {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  const result: number[] = [];
  for (let index = 0; index + 3 < value.length && result.length < maxBytes; index += 4) {
    const a=alphabet.indexOf(value[index]),b=alphabet.indexOf(value[index+1]);
    const c=value[index+2]==="="?0:alphabet.indexOf(value[index+2]);
    const d=value[index+3]==="="?0:alphabet.indexOf(value[index+3]);
    if(a<0||b<0||c<0||d<0)return null;
    result.push((a<<2)|(b>>4));if(value[index+2]!=="="&&result.length<maxBytes)result.push(((b&15)<<4)|(c>>2));if(value[index+3]!=="="&&result.length<maxBytes)result.push(((c&3)<<6)|d);
  }
  return result;
}

export function createAssetImagePreviewLoader({
  getScope,
  isSessionCurrent,
  sendEnvelope,
  requestTimeoutMs = 22_000,
  cacheTTL = 90_000,
  cacheLimit = 16,
  concurrency = 2,
}: {
  getScope: () => Readonly<RelayScope>;
  isSessionCurrent: (sessionID: string) => boolean;
  sendEnvelope: (type: typeof requestType, overrides: Partial<RelayMessage<AssetImagePreviewPayload>>) => boolean;
  requestTimeoutMs?: number;
  cacheTTL?: number;
  cacheLimit?: number;
  concurrency?: number;
}) {
  const cache = new Map<string, CachedPreview>();
  const entries = new Map<string, Entry>();
  const pending = new Map<string, Entry>();
  const queue = new Set<Entry>();

  function current(target: Target) {
    return sameConnection(target, getScope()) && isSessionCurrent(target.sessionID);
  }

  function removeCached(key: string) {
    const item = cache.get(key);
    if (item) clearTimeout(item.timer);
    cache.delete(key);
  }

  function remember(entry: Entry, image: AssetImagePreview) {
    removeCached(entry.key);
    cache.set(entry.key, {
      target: entry.target, image, expiresAt: Date.now() + cacheTTL,
      timer: setTimeout(() => removeCached(entry.key), cacheTTL),
    });
    while (cache.size > cacheLimit) removeCached(cache.keys().next().value!);
  }

  function remove(entry: Entry) {
    queue.delete(entry);
    if (entries.get(entry.key) === entry) entries.delete(entry.key);
    if (entry.requestID) pending.delete(entry.requestID);
    clearTimeout(entry.timer);
  }

  function notify(entry: Entry, result: AssetImagePreviewResult) {
    for (const listener of entry.listeners) listener(result);
    entry.listeners.clear();
  }

  function finish(entry: Entry, result?: AssetImagePreviewResult) {
    const owned = entries.get(entry.key) === entry && current(entry.target);
    remove(entry);
    if (owned && result) {
      if (result.status === "ready") remember(entry, result.image);
      notify(entry, result);
    } else {
      entry.listeners.clear();
    }
    pump();
  }

  function pump() {
    while (pending.size < concurrency && queue.size) {
      const entry = queue.values().next().value!;
      queue.delete(entry);
      if (!current(entry.target) || !entry.listeners.size) {
        remove(entry);
        notify(entry, { status: "error", reason: "offline" });
        continue;
      }
      const { sessionID, assetID } = entry.target;
      const requestID = requestPrefix + newRequestID();
      entry.requestID = requestID;
      entry.timer = setTimeout(() => finish(entry, { status: "error", reason: "timeout" }), requestTimeoutMs);
      pending.set(requestID, entry);
      let sent = false;
      try {
        sent = sendEnvelope(requestType, { session_id: sessionID, request_id: requestID, payload: { asset_id: assetID } });
      } catch { /* Treat a failed enqueue like a disconnect. */ }
      if (!sent && pending.get(requestID) === entry) {
        finish(entry, { status: "error", reason: "offline" });
      }
    }
  }

  function prune() {
    const scope = getScope();
    for (const [key, item] of cache) if (!current(item.target)) removeCached(key);
    for (const entry of entries.values()) {
      if (current(entry.target)) continue;
      if (entry.requestID && sameConnection(entry.target, scope)) {
        // Keep the in-flight slot until it drains. Rapid session changes must
        // not stack more work on the same Relay connection.
        entries.delete(entry.key);
      } else {
        remove(entry);
      }
      notify(entry, { status: "error", reason: "offline" });
    }
    for (const entry of pending.values()) if (!sameConnection(entry.target, scope)) remove(entry);
    pump();
  }

  function subscribe(sessionID: string, assetID: string, listener: Listener) {
    prune();
    const target: Target = { ...getScope(), sessionID, assetID };
    if (!isAssetID(assetID)) {
      listener({ status: "error", reason: "invalid" });
      return () => {};
    }
    if (!sessionID || !target.pairingID || !target.connectionID || !current(target)) {
      listener({ status: "error", reason: "offline" });
      return () => {};
    }
    const key = keyFor(target);
    const item = cache.get(key);
    if (item && item.expiresAt > Date.now()) {
      cache.delete(key);
      cache.set(key, item);
      listener({ status: "ready", image: item.image });
      return () => {};
    }
    removeCached(key);
    let entry = entries.get(key);
    if (!entry) {
      if (queue.size >= maxQueuedPreviews) {
        listener({ status: "error", reason: "busy" });
        return () => {};
      }
      entry = { key, target, listeners: new Set() };
      entries.set(key, entry);
      queue.add(entry);
    }
    const subscription = entry;
    subscription.listeners.add(listener);
    pump();
    return () => {
      subscription.listeners.delete(listener);
      if (!subscription.listeners.size && !subscription.requestID) remove(subscription);
    };
  }

  function handleMessage(message: RelayMessage) {
    const requestID = typeof message.request_id === "string" ? message.request_id : "";
    const error = message.type === "error" ? message.payload as (ErrorPayload & { transport_error?: boolean; asset_id?: string }) | undefined : undefined;
    if (message.type !== "asset_image_preview_result" &&
        !(message.type === "error" && (requestID.startsWith(requestPrefix) || error?.request_type === requestType))) return false;
    const entry = pending.get(requestID);
    if (!entry || !sameConnection(entry.target, getScope()) || message.session_id !== entry.target.sessionID) return true;
    if (message.type === "error") {
      if ((error?.request_type && error.request_type !== requestType) ||
          (error?.asset_id !== undefined && error.asset_id !== entry.target.assetID)) return true;
      const unsupported = typeof error?.message === "string" && /(?:unsupported|unknown)(?:\s+\w+){0,2}\s+(?:message|request|type)/i.test(error.message);
      finish(entry, { status: "error", reason: error?.transport_error ? "offline" : unsupported ? "unsupported" : "unavailable" });
    } else {
      const result = message.payload as Partial<AssetImagePreviewResultPayload> | undefined;
      if (result?.asset_id !== entry.target.assetID) return true;
      const image = entries.get(entry.key) === entry && current(entry.target) ? decodeAssetImagePreview(result) : null;
      finish(entry, image ? { status: "ready", image } : { status: "error", reason: "invalid" });
    }
    return true;
  }

  function discard(sessionID: string, assetID: string, image: AssetImagePreview) {
    const key = keyFor({ ...getScope(), sessionID, assetID });
    if (cache.get(key)?.image === image) removeCached(key);
  }

  function clear() {
    for (const key of cache.keys()) removeCached(key);
    for (const entry of [...entries.values(), ...pending.values()]) {
      remove(entry);
      entry.listeners.clear();
    }
  }

  return { clear, discard, handleMessage, prune, subscribe };
}

export type AssetImagePreviewLoader = ReturnType<typeof createAssetImagePreviewLoader>;
