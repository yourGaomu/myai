import type {
  AssetUploadOperation,
  AssetUploadResultPayload,
  ErrorPayload,
  RelayMessage,
  UploadedAssetPayload,
} from "../protocol";
import { newRequestID } from "./ids";
import { normalizeUploadedAsset } from "./toolAssets";

export const assetUploadChunkSize = 262144;
export const assetUploadMaxFileBytes = 52428800;
// The Agent allows five minutes for its completion worker. Leave room for
// admission and for the result to travel back through the Relay.
export const assetUploadCompleteTimeoutMs = 330000;
const uploadRequestPrefix = "asset-upload-";
const maxCleanupEntries = 128;

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;
export type RelayUploadScope = Readonly<{ pairingID: string; connectionID: string }>;
export type UploadTransportErrorPayload = ErrorPayload & { transport_error?: boolean };

export type AssetUploadSource = {
  fileName: string;
  contentType: string;
  size: number;
  read: (offset: number, length: number) => Promise<Uint8Array>;
  close?: () => void;
};

type UploadTarget = { sessionID: string; scope: RelayUploadScope };
export type UploadAssetOptions = UploadTarget & {
  source: AssetUploadSource;
  signal?: AbortSignal;
};

type UploadContext = UploadTarget & {
  controller: AbortController;
  failure?: Error;
  uploadID?: string;
  cancelQueued: boolean;
  completed: boolean;
};

type PendingRequest = UploadTarget & {
  operation: AssetUploadOperation;
  uploadID?: string;
  resolve: (result: AssetUploadResultPayload) => void;
  reject: (error: Error) => void;
};

type Cleanup = {
  pairingID: string;
  sessionID: string;
  uploadID: string;
  lastAttempt?: string;
};

class UploadTimeoutError extends Error {
  constructor() {
    super("文件上传响应超时，请检查 Relay 和 Agent 连接后重试。");
  }
}

export class AssetUploadCanceledError extends Error {
  constructor() {
    super("已取消文件上传。");
    this.name = "AbortError";
  }
}

export class AssetUploadDisconnectedError extends Error {
  constructor() {
    super("Relay 连接已断开，文件上传已停止，请连接后重试。");
  }
}

export function assertAssetUploadScope(expected: RelayUploadScope, current: RelayUploadScope) {
  if (expected.pairingID !== current.pairingID) throw new AssetUploadCanceledError();
  if (!expected.pairingID || !expected.connectionID || expected.connectionID !== current.connectionID) {
    throw new AssetUploadDisconnectedError();
  }
}

// A successful socket.send only queues a message, including on Android. Every
// operation waits for its own correlated Agent result before advancing offsets.
export function createAssetUploadClient({
  sendEnvelope,
  getScope,
  requestTimeoutMs = 30000,
  completeTimeoutMs = assetUploadCompleteTimeoutMs,
}: {
  sendEnvelope: SendEnvelope;
  getScope: () => RelayUploadScope;
  requestTimeoutMs?: number;
  completeTimeoutMs?: number;
}) {
  const pending = new Map<string, PendingRequest>();
  const active = new Set<UploadContext>();
  const beginRequests = new Map<string, UploadContext>();
  const cancellations = new Map<string, Cleanup>();

  function checkTarget(target: UploadTarget, context?: UploadContext) {
    if (context?.failure) throw context.failure;
    assertAssetUploadScope(target.scope, getScope());
  }

  function request(
    operation: AssetUploadOperation,
    payload: object,
    target: UploadTarget,
    context?: UploadContext,
  ): Promise<AssetUploadResultPayload> {
    checkTarget(target, context);
    const { scope, sessionID } = target;
    const signal = context?.controller.signal;
    const requestID = `${uploadRequestPrefix}${newRequestID()}`;
    return new Promise((resolve, reject) => {
      const cleanup = () => {
        clearTimeout(timer);
        signal?.removeEventListener("abort", onAbort);
        pending.delete(requestID);
      };
      const fail = (error: Error) => { cleanup(); reject(error); };
      const onAbort = () => fail(context?.failure || new AssetUploadCanceledError());
      const timer = setTimeout(() => fail(new UploadTimeoutError()), operation === "complete" ? completeTimeoutMs : requestTimeoutMs);
      pending.set(requestID, {
        operation, scope, sessionID,
        uploadID: "upload_id" in payload ? String(payload.upload_id) : undefined,
        resolve: (result) => { cleanup(); resolve(result); },
        reject: fail,
      });
      if (operation === "begin" && context) {
        // A timed-out or canceled begin can still deliver a newly allocated
        // upload ID. Keep a bounded, pairing-scoped cleanup tombstone.
        beginRequests.set(requestID, context);
        if (beginRequests.size > maxCleanupEntries) beginRequests.delete(beginRequests.keys().next().value!);
      }
      signal?.addEventListener("abort", onAbort, { once: true });
      try {
        if (!sendEnvelope(`asset_upload_${operation}`, { request_id: requestID, session_id: sessionID, payload })) {
          fail(new AssetUploadDisconnectedError());
        }
      } catch {
        fail(new AssetUploadDisconnectedError());
      }
    });
  }

  function queueCancellation(context: UploadContext) {
    const { uploadID, sessionID, scope: { pairingID } } = context;
    if (!uploadID || context.completed || context.cancelQueued || pairingID !== getScope().pairingID) return;
    context.cancelQueued = true;
    cancellations.set(JSON.stringify([pairingID, sessionID, uploadID]), { pairingID, sessionID, uploadID });
    if (cancellations.size > maxCleanupEntries) cancellations.delete(cancellations.keys().next().value!);
  }

  function flushCancellations() {
    const scope = { ...getScope() };
    for (const [key, cleanup] of cancellations) {
      if (cleanup.pairingID !== scope.pairingID) {
        cancellations.delete(key);
        continue;
      }
      if (!scope.connectionID || cleanup.lastAttempt === scope.connectionID) continue;
      // Cleanup may use a *new connection*, but only for the same pairing.
      // Recheck before every send, not merely when the loop started.
      if (scope.connectionID !== getScope().connectionID || scope.pairingID !== getScope().pairingID) return;
      cleanup.lastAttempt = scope.connectionID;
      void request("cancel", { upload_id: cleanup.uploadID }, { scope, sessionID: cleanup.sessionID }).then(() => {
        if (cancellations.get(key) === cleanup) cancellations.delete(key);
      }, () => { /* best effort; retry at most once on the next connection */ });
    }
  }

  function handleMessage(message: RelayMessage): boolean {
    const requestID = message.request_id || "";
    const waiting = pending.get(requestID);
    const current = getScope();
    const waitingIsCurrent = waiting && current.pairingID === waiting.scope.pairingID &&
      current.connectionID === waiting.scope.connectionID && matchesSession(waiting, message);
    if (message.type === "error") {
      const payload = (message.payload || {}) as UploadTransportErrorPayload;
      if (!waiting && !requestID.startsWith(uploadRequestPrefix) && !payload.request_type?.startsWith("asset_upload_")) return false;
      if (waitingIsCurrent && (!payload.request_type || payload.request_type === `asset_upload_${waiting.operation}`)) {
        waiting.reject(payload.transport_error ? new AssetUploadDisconnectedError() : new Error(payload.message || "Agent 文件上传失败。"));
        beginRequests.delete(requestID);
      }
      return true;
    }
    if (message.type !== "asset_upload_result") return false;
    const result = message.payload as AssetUploadResultPayload | undefined;
    const valid = result && typeof result.upload_id === "string" && result.upload_id &&
      result.chunk_size === assetUploadChunkSize && result.max_file_bytes === assetUploadMaxFileBytes &&
      Number.isSafeInteger(result.offset) && result.offset >= 0 && result.offset <= assetUploadMaxFileBytes;
    const begin = beginRequests.get(requestID);
    if (begin && valid && result.operation === "begin" && matchesSession(begin, message)) {
      beginRequests.delete(requestID);
      if (current.pairingID === begin.scope.pairingID && (!begin.uploadID || begin.uploadID === result.upload_id)) {
        begin.uploadID = result.upload_id;
        // The Relay can deliver a delayed begin to a replacement peer for the
        // same pairing. Learn its ID for cleanup, never resume that operation.
        if (begin.failure || current.connectionID !== begin.scope.connectionID) {
          queueCancellation(begin);
          flushCancellations();
        }
      }
    }
    if (!waitingIsCurrent) return true;
    if (!valid || result.operation !== waiting.operation || (waiting.uploadID && result.upload_id !== waiting.uploadID) ||
        (begin?.uploadID && begin.uploadID !== result.upload_id)) {
      waiting.reject(new Error("Agent 返回了无效的文件上传结果。"));
    } else {
      waiting.resolve(result);
    }
    return true;
  }

  async function upload({ source, sessionID, signal, scope }: UploadAssetOptions): Promise<UploadedAssetPayload> {
    const context: UploadContext = {
      controller: new AbortController(), scope: { ...scope }, sessionID, cancelQueued: false, completed: false,
    };
    const onAbort = () => {
      abortUpload(context, new AssetUploadCanceledError());
      queueCancellation(context);
      flushCancellations();
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    if (signal?.aborted) onAbort();
    active.add(context);
    const check = () => checkTarget(context, context);
    const ask = (operation: AssetUploadOperation, payload: object) => request(operation, payload, context, context);
    const uploadedAsset = (result: AssetUploadResultPayload) => {
      check();
      if (!result.asset) return undefined;
      const asset = normalizeUploadedAsset(result.asset);
      if (!asset || asset.size !== source.size || result.offset !== source.size) {
        throw new Error("Agent 返回了无效的文件元数据。");
      }
      context.completed = true;
      return asset;
    };
    try {
      check();
      if (!sessionID) throw new Error("请先加载会话再上传文件。");
      if (!Number.isSafeInteger(source.size) || source.size <= 0) throw new Error("文件为空或大小无效，请选择非空文件。");
      if (source.size > assetUploadMaxFileBytes) throw new Error("文件超过 50 MiB 上限，请选择较小的文件。");
      const beginPayload = {
        file_name: source.fileName,
        content_type: source.contentType,
        size: source.size,
        idempotency_key: newRequestID(),
      };
      let result: AssetUploadResultPayload;
      try {
        result = await ask("begin", beginPayload);
      } catch (error) {
        if (!(error instanceof UploadTimeoutError)) throw error;
        // Retry with the same create key, not a second backend upload.
        result = await ask("begin", beginPayload);
      }
      check();
      const uploadID = result.upload_id;
      const uploadRef = { upload_id: uploadID };
      const existingAsset = uploadedAsset(result);
      if (existingAsset) return existingAsset;
      let offset = result.offset;
      if (offset > source.size) throw new Error("Agent 返回了无效的上传偏移。");
      while (offset < source.size) {
        check();
        const length = Math.min(assetUploadChunkSize, source.size - offset);
        const bytes = await source.read(offset, length);
        check();
        if (bytes.length !== length) throw new Error("读取文件分块失败，请重新选择文件。");
        const chunkPayload = { ...uploadRef, offset, data: encodeBase64(bytes) };
        const expectedOffset = offset + length;
        try {
          result = await ask("chunk", chunkPayload);
        } catch (error) {
          if (!(error instanceof UploadTimeoutError)) throw error;
          result = await ask("status", uploadRef);
          const recoveredAsset = uploadedAsset(result);
          if (recoveredAsset) return recoveredAsset;
          if (result.offset === offset) result = await ask("chunk", chunkPayload);
        }
        check();
        if (result.offset !== expectedOffset) throw new Error("Agent 返回了不一致的上传偏移。");
        offset = result.offset;
      }
      try {
        result = await ask("complete", uploadRef);
      } catch (error) {
        if (!(error instanceof UploadTimeoutError)) throw error;
        // One status read and, if necessary, one same-ID rejoin. The server
        // shares an existing worker/receipt or retries using its existing key.
        result = await ask("status", uploadRef);
        const recoveredAsset = uploadedAsset(result);
        if (recoveredAsset) return recoveredAsset;
        if (result.offset !== source.size) throw new Error("Agent 尚未收到完整文件。");
        result = await ask("complete", uploadRef);
      }
      const asset = uploadedAsset(result);
      if (!asset) throw new Error("Agent 未返回已上传文件的元数据。");
      return asset;
    } catch (error) {
      // An abort/epoch change can race even with a rejected awaited reply.
      let failure = error instanceof Error ? error : new Error("文件上传失败。");
      try { check(); } catch (fenced) { if (fenced instanceof Error) failure = fenced; }
      abortUpload(context, failure);
      queueCancellation(context);
      flushCancellations();
      throw failure;
    } finally {
      active.delete(context);
      signal?.removeEventListener("abort", onAbort);
      try { source.close?.(); } catch { /* The upload outcome takes precedence. */ }
    }
  }

  function disconnect() {
    const error = new AssetUploadDisconnectedError();
    for (const context of active) {
      abortUpload(context, error);
      queueCancellation(context);
    }
    for (const waiting of [...pending.values()]) waiting.reject(error);
  }

  return { upload, handleMessage, disconnect, reconnect: flushCancellations };
}

function matchesSession(target: UploadTarget, message: RelayMessage) {
  return !message.session_id || message.session_id === target.sessionID;
}

function abortUpload(context: UploadContext, error: Error) {
  if (context.failure) return;
  // Keep the reason separately: not all React Native AbortSignal shims keep
  // the argument to abort(), and disconnect is not a user cancellation.
  context.failure = error;
  context.controller.abort();
}

function encodeBase64(bytes: Uint8Array): string {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  const encoded: string[] = [];
  for (let index = 0; index < bytes.length; index += 3) {
    const a = bytes[index];
    const b = bytes[index + 1];
    const c = bytes[index + 2];
    encoded.push(alphabet[a >> 2], alphabet[((a & 3) << 4) | ((b || 0) >> 4)],
      index + 1 < bytes.length ? alphabet[((b & 15) << 2) | ((c || 0) >> 6)] : "=",
      index + 2 < bytes.length ? alphabet[c & 63] : "=");
  }
  return encoded.join("");
}
