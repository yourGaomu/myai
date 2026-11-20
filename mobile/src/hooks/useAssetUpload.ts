import { useCallback, useEffect, useRef, type RefObject } from "react";
import type { DocumentPickerAsset } from "expo-document-picker";
import { File, FileMode } from "expo-file-system";
import { Platform } from "react-native";

import type { RelayMessage, UploadedAssetPayload } from "../protocol";
import { inferMimeType } from "../utils/attachments";
import {
  AssetUploadCanceledError,
  assetUploadMaxFileBytes,
  assertAssetUploadScope,
  createAssetUploadClient,
  type AssetUploadSource,
  type RelayUploadScope,
} from "../utils/assetUpload";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;
export type UploadMobileAsset = (options: {
  asset: DocumentPickerAsset;
  scope: RelayUploadScope;
  sessionID: string;
  signal?: AbortSignal;
}) => Promise<UploadedAssetPayload>;

export function useAssetUpload({ sendEnvelope, relayScopeRef }: {
  sendEnvelope: SendEnvelope;
  relayScopeRef: RefObject<RelayUploadScope>;
}) {
  const senderRef = useRef(sendEnvelope);
  senderRef.current = sendEnvelope;
  const clientRef = useRef<ReturnType<typeof createAssetUploadClient> | null>(null);
  if (!clientRef.current) {
    clientRef.current = createAssetUploadClient({
      sendEnvelope: (type, overrides) => senderRef.current(type, overrides),
      getScope: () => relayScopeRef.current,
    });
  }
  const client = clientRef.current;
  useEffect(() => () => client.disconnect(), [client]);

  const uploadMobileAsset: UploadMobileAsset = useCallback(async ({ asset, scope, sessionID, signal }) => {
    if (signal?.aborted) throw new AssetUploadCanceledError();
    assertAssetUploadScope(scope, relayScopeRef.current);
    if (asset.size != null && (!Number.isSafeInteger(asset.size) || asset.size <= 0)) {
      throw new Error("文件为空或大小无效，请选择非空文件。");
    }
    if (asset.size != null && asset.size > assetUploadMaxFileBytes) {
      throw new Error("文件超过 50 MiB 上限，请选择较小的文件。");
    }
    const metadata = {
      fileName: asset.name || "upload",
      contentType: inferMimeType(asset.name, asset.mimeType),
    };
    let source: AssetUploadSource;
    if (Platform.OS === "web") {
      const file = asset.file;
      if (!file) throw new Error("无法读取所选文件，请重新选择后上传。");
      source = {
        ...metadata,
        size: file.size,
        read: async (offset, length) => {
          try { return new Uint8Array(await file.slice(offset, offset + length).arrayBuffer()); }
          catch { throw new Error("无法读取所选文件，请重新选择后上传。"); }
        },
      };
    } else {
      // Expo's File.slice() calls bytesSync() on the entire file. Use its native
      // read-only handle instead so a 50 MiB upload retains only one 256 KiB chunk.
      try {
        const file = new File(asset.uri);
        if (file.size > assetUploadMaxFileBytes) throw new Error("文件超过 50 MiB 上限，请选择较小的文件。");
        const handle = file.open(FileMode.ReadOnly);
        source = {
          ...metadata,
          size: file.size,
          read: async (offset, length) => {
            try { handle.offset = offset; return handle.readBytes(length); }
            catch { throw new Error("无法读取所选文件，请重新选择后上传。"); }
          },
          close: () => handle.close(),
        };
      } catch (error) {
        if (error instanceof Error && error.message.includes("50 MiB")) throw error;
        throw new Error("无法读取所选文件，请重新选择后上传。");
      }
    }
    return client.upload({ source, scope, sessionID, signal });
  }, [client, relayScopeRef]);

  return {
    uploadMobileAsset,
    handleAssetUploadMessage: client.handleMessage,
    disconnectAssetUploads: client.disconnect,
    reconnectAssetUploads: client.reconnect,
  };
}
