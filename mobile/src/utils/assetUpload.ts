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
const uploadRequestPrefix = "asset-upload-";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

export type AssetUploadSource = {
  fileName: string;
  contentType: string;
  size: number;
  read: (offset: number, length: number) => Promise<Uint8Array>;
  close?: () => void;
};

export type UploadAssetOptions = {
  source: AssetUploadSource;
  sessionID: string;
  signal?: AbortSignal;
};

type UploadContext = {
  controller: AbortController;
  sessionID: string;
  uploadID?: string;
  cancelSent: boolean;
  completed: boolean;
};

type PendingRequest = {
  operation: AssetUploadOperation;
  uploadID?: string;
  context?: UploadContext;
  resolve: (result: AssetUploadResultPayload) => void;
  reject: (error: Error) => void;
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

// A successful socket.send only queues a message, including on Android. Every
// operation waits for its own correlated Agent result before advancing offsets.
export function createAssetUploadClient({
  sendEnvelope,
  isConnected,
  requestTimeoutMs = 30000,
}: {
  sendEnvelope: SendEnvelope;
  isConnected: () => boolean;
  requestTimeoutMs?: number;
}) {
  const pending = new Map<string, PendingRequest>();
  const active = new Set<UploadContext>();
  const beginRequests = new Map<string, UploadContext>();
  const canceledUploads = new Map<string, string>();

  function request(
    operation: AssetUploadOperation,
    payload: object,
    sessionID: string,
    context?: UploadContext,
  ): Promise<AssetUploadResultPayload> {
    const signal = context?.controller.signal;
    if (signal?.aborted) return Promise.reject(abortReason(signal));
    const requestID = `${uploadRequestPrefix}${newRequestID()}`;
    return new Promise((resolve, reject) => {
      const cleanup = () => {
        clearTimeout(timer);
        signal?.removeEventListener("abort", onAbort);
        pending.delete(requestID);
      };
      const fail = (error: Error) => { cleanup(); reject(error); };
      const onAbort = () => fail(abortReason(signal!));
      const timer = setTimeout(() => fail(new UploadTimeoutError()), requestTimeoutMs);
      pending.set(requestID, {
        operation,
        uploadID: "upload_id" in payload ? String(payload.upload_id) : undefined,
        context,
        resolve: (result) => { cleanup(); resolve(result); },
        reject: fail,
      });
      if (operation === "begin" && context) {
        beginRequests.set(requestID, context);
        // Keep bounded tombstones for begin replies arriving after a timeout or
        // cancellation: their newly learned upload ID must still be canceled.
        if (beginRequests.size > 128) beginRequests.delete(beginRequests.keys().next().value!);
      }
      signal?.addEventListener("abort", onAbort, { once: true });
      try {
        if (!sendEnvelope(`asset_upload_${operation}`, { request_id: requestID, session_id: sessionID, payload })) {
          fail(new Error("Relay 连接不可用，文件上传未完成。"));
        }
      } catch {
        fail(new Error("Relay 连接不可用，文件上传未完成。"));
      }
    });
  }

  function cancelRemote(context: UploadContext) {
    if (!context.uploadID || context.completed || context.cancelSent) return;
    context.cancelSent = true;
    canceledUploads.set(context.uploadID, context.sessionID);
    flushCancellations();
  }

  function flushCancellations() {
    if (!isConnected()) return;
    for (const [uploadID, sessionID] of canceledUploads) {
      canceledUploads.delete(uploadID);
      void request("cancel", { upload_id: uploadID }, sessionID).catch(() => {
        // Retain failed cleanup for the next paired connection, without retrying
        // begin/complete or creating another backend asset.
        canceledUploads.set(uploadID, sessionID);
      });
    }
  }

  function handleMessage(message: RelayMessage): boolean {
    const requestID = message.request_id || "";
    const waiting = pending.get(requestID);
    if (message.type === "error") {
      const payload = (message.payload || {}) as ErrorPayload;
      if (!waiting && !requestID.startsWith(uploadRequestPrefix) && !payload.request_type?.startsWith("asset_upload_")) return false;
      waiting?.reject(new Error(payload.message || "Agent 文件上传失败。"));
      return true;
    }
    if (message.type !== "asset_upload_result") return false;
    const result = message.payload as AssetUploadResultPayload | undefined;
    const beginContext = beginRequests.get(requestID);
    if (beginContext && result?.operation === "begin" && typeof result.upload_id === "string" && result.upload_id) {
      beginRequests.delete(requestID);
      beginContext.uploadID = result.upload_id;
      if (beginContext.controller.signal.aborted) cancelRemote(beginContext);
    }
    if (!waiting) return true;
    if (!result || result.operation !== waiting.operation || !result.upload_id ||
        (waiting.uploadID && result.upload_id !== waiting.uploadID) ||
        result.chunk_size !== assetUploadChunkSize || result.max_file_bytes !== assetUploadMaxFileBytes ||
        !Number.isSafeInteger(result.offset) || result.offset < 0 || result.offset > assetUploadMaxFileBytes) {
      waiting.reject(new Error("Agent 返回了无效的文件上传结果。"));
      return true;
    }
    waiting.resolve(result);
    return true;
  }

  async function upload({ source, sessionID, signal }: UploadAssetOptions): Promise<UploadedAssetPayload> {
    const context: UploadContext = { controller: new AbortController(), sessionID, cancelSent: false, completed: false };
    const onAbort = () => {
      context.controller.abort(new AssetUploadCanceledError());
      cancelRemote(context);
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    if (signal?.aborted) onAbort();
    active.add(context);
    const checkCanceled = () => {
      if (context.controller.signal.aborted) throw abortReason(context.controller.signal);
    };
    const uploadedAsset = (result: AssetUploadResultPayload) => {
      if (!result.asset) return undefined;
      const asset = normalizeUploadedAsset(result.asset);
      if (!asset || asset.size !== source.size) throw new Error("Agent 返回了无效的文件元数据。");
      context.completed = true;
      return asset;
    };
    try {
      checkCanceled();
      if (!Number.isSafeInteger(source.size) || source.size <= 0) throw new Error("文件为空或大小无效，请选择非空文件。");
      if (source.size > assetUploadMaxFileBytes) throw new Error("文件超过 50 MiB 上限，请选择较小的文件。");
      if (!isConnected()) throw new Error("当前未连接 Relay，请先完成配对并连接。");
      const beginPayload = {
        file_name: source.fileName,
        content_type: source.contentType,
        size: source.size,
        idempotency_key: newRequestID(),
      };
      let result: AssetUploadResultPayload;
      try {
        result = await request("begin", beginPayload, sessionID, context);
      } catch (error) {
        if (!(error instanceof UploadTimeoutError)) throw error;
        // The same key recovers an uncertain begin; each envelope still gets a
        // new request ID. Never restart an upload with another create key.
        result = await request("begin", beginPayload, sessionID, context);
      }
      context.uploadID = result.upload_id;
      const existingAsset = uploadedAsset(result);
      if (existingAsset) return existingAsset;
      let offset = result.offset;
      if (offset > source.size) throw new Error("Agent 返回了无效的上传偏移。");
      while (offset < source.size) {
        checkCanceled();
        const length = Math.min(assetUploadChunkSize, source.size - offset);
        const bytes = await source.read(offset, length);
        checkCanceled();
        if (bytes.length !== length) throw new Error("读取文件分块失败，请重新选择文件。");
        const chunkPayload = { upload_id: context.uploadID, offset, data: encodeBase64(bytes) };
        const expectedOffset = offset + length;
        try {
          result = await request("chunk", chunkPayload, sessionID, context);
        } catch (error) {
          if (!(error instanceof UploadTimeoutError)) throw error;
          result = await request("status", { upload_id: context.uploadID }, sessionID, context);
          const recoveredAsset = uploadedAsset(result);
          if (recoveredAsset) return recoveredAsset;
          if (result.offset === offset) result = await request("chunk", chunkPayload, sessionID, context);
        }
        if (result.offset !== expectedOffset) throw new Error("Agent 返回了不一致的上传偏移。");
        offset = result.offset;
      }
      try {
        result = await request("complete", { upload_id: context.uploadID }, sessionID, context);
      } catch (error) {
        if (!(error instanceof UploadTimeoutError)) throw error;
        result = await request("status", { upload_id: context.uploadID }, sessionID, context);
        const recoveredAsset = uploadedAsset(result);
        if (recoveredAsset) return recoveredAsset;
        if (result.offset !== source.size) throw new Error("Agent 尚未收到完整文件。");
        result = await request("complete", { upload_id: context.uploadID }, sessionID, context);
      }
      if (result.offset !== source.size) throw new Error("Agent 尚未收到完整文件。");
      const asset = uploadedAsset(result);
      if (!asset) throw new Error("Agent 未返回已上传文件的元数据。");
      return asset;
    } catch (error) {
      context.controller.abort(error);
      cancelRemote(context);
      throw error;
    } finally {
      active.delete(context);
      signal?.removeEventListener("abort", onAbort);
      try { source.close?.(); } catch { /* The upload outcome takes precedence. */ }
    }
  }

  function disconnect() {
    const error = new Error("Relay 连接已断开，文件上传已停止，请重新上传。");
    for (const context of active) {
      context.controller.abort(error);
      cancelRemote(context);
    }
    for (const request of [...pending.values()]) request.reject(error);
  }

  return { upload, handleMessage, disconnect, reconnect: flushCancellations };
}

function abortReason(signal: AbortSignal): Error {
  return signal.reason instanceof Error ? signal.reason : new AssetUploadCanceledError();
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
