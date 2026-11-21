import { useCallback, useEffect, useRef, type RefObject } from "react";
import * as DocumentPicker from "expo-document-picker";
import { File, Paths } from "expo-file-system";
import { Platform } from "react-native";

import { maxAttachedFiles } from "../constants/app";
import type { FileEntry, FileReadResultPayload, RelayMessage } from "../protocol";
import type { PendingAction, ViewMode } from "../types/app";
import type { ChatAttachment, UploadedAssetAttachment } from "../types/chat";
import { attachmentKey, inferMimeType, isImageAttachment, isWorkspaceFileAttachment, messageWithAttachedFiles, workspaceFileAttachment } from "../utils/attachments";
import { pickedImageURI } from "../utils/assetImagePreview";
import { AssetUploadCanceledError, AssetUploadDisconnectedError, assertAssetUploadScope, type RelayUploadScope } from "../utils/assetUpload";
import type { UploadMobileAsset } from "./useAssetUpload";
import { newRequestID } from "../utils/ids";
import { parentPathOf } from "../utils/paths";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;
const pickedPreviewLifetimeMs = 5 * 60_000;
const pickedPreviewMaxBytes = 64 * 1024 * 1024;
const pickedPreviewMaxEntries = 12;

function releasePickedURI(uri: string) {
  try {
    if (Platform.OS === "web") {
      if (uri.startsWith("blob:")) URL.revokeObjectURL(uri);
      return;
    }
    // DocumentPicker was called with copyToCacheDirectory. Never delete the
    // original document, only its one-level copy inside the picker's cache.
    const prefix = `${Paths.cache.uri.replace(/\/$/, "")}/DocumentPicker/`;
    if (!uri.startsWith(prefix)) return;
    const name = uri.slice(prefix.length);
    if (name && name !== "." && name !== ".." && /^[a-z\d_.-]+$/i.test(name)) new File(uri).delete();
  } catch { /* The picker or operating system may have removed the copy. */ }
}

type Args = {
  addErrorMessage: (message: string) => void;
  uploadMobileAsset: UploadMobileAsset;
  attachedFiles: ChatAttachment[];
  changeDiffPath?: string;
  fileParent: string;
  filePreview: FileReadResultPayload | null;
  pairingID: string;
  relayScopeRef: RefObject<RelayUploadScope>;
  requestFiles: (path?: string) => boolean;
  selectedChange: string;
  sendEnvelope: SendEnvelope;
  sessionID: string;
  sessionIDRef: RefObject<string>;
  setAttachedFiles: (updater: (current: ChatAttachment[]) => ChatAttachment[]) => void;
  setFilePreview: (preview: FileReadResultPayload | null) => void;
  setMessageInput: (input: string) => void;
  setViewMode: (mode: ViewMode) => void;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

// 统一处理远程 workspace 文件与手机本地上传文件，两者最终都转换成聊天附件。
export function useFileActions({
  addErrorMessage,
  uploadMobileAsset,
  attachedFiles,
  changeDiffPath,
  fileParent,
  filePreview,
  pairingID,
  relayScopeRef,
  requestFiles,
  selectedChange,
  sendEnvelope,
  sessionID,
  sessionIDRef,
  setAttachedFiles,
  setFilePreview,
  setMessageInput,
  setViewMode,
  startPending,
  stopPending,
}: Args) {
  const readFilePath = useCallback(
    (path: string) => {
      startPending("files");
      if (!sendEnvelope("file_read", {
        request_id: newRequestID(),
        payload: { path },
      })) {
        stopPending("files");
      }
    },
    [sendEnvelope, startPending, stopPending],
  );

  const openFileEntry = useCallback(
    (entry: FileEntry) => {
      if (entry.type === "dir") {
        setFilePreview(null);
        requestFiles(entry.path);
        return;
      }
      readFilePath(entry.path);
    },
    [readFilePath, requestFiles, setFilePreview],
  );

  const openSelectedChangeFile = useCallback(() => {
    const path = changeDiffPath || selectedChange;
    if (!path) {
      return;
    }

    requestFiles(parentPathOf(path));
    readFilePath(path);
  }, [changeDiffPath, readFilePath, requestFiles, selectedChange]);

  const goToParent = useCallback(() => {
    if (!fileParent) {
      return;
    }
    setFilePreview(null);
    requestFiles(fileParent);
  }, [fileParent, requestFiles, setFilePreview]);

  const refreshCurrentFiles = useCallback(() => {
    requestFiles();
  }, [requestFiles]);

  const isCurrentDraft = useCallback(() => (
    relayScopeRef.current.pairingID === pairingID && sessionIDRef.current === sessionID
  ), [pairingID, relayScopeRef, sessionID, sessionIDRef]);

  const attachFilePreview = useCallback(() => {
    if (!filePreview || !isCurrentDraft()) {
      return;
    }
    if (filePreview.binary) {
      addErrorMessage("Binary files cannot be attached to chat yet");
      return;
    }
    if (!filePreview.content) {
      addErrorMessage("File content is empty");
      return;
    }

    setAttachedFiles((current) => {
      if (current.some((file) => isWorkspaceFileAttachment(file) && file.path === filePreview.path)) {
        return current;
      }
      const attachment = workspaceFileAttachment(filePreview);
      if (current.length >= maxAttachedFiles) {
        return [...current.slice(1), attachment];
      }
      return [...current, attachment];
    });
    setViewMode("chat");
  }, [addErrorMessage, filePreview, isCurrentDraft, setAttachedFiles, setViewMode]);

  const uploadRef = useRef<{ controller: AbortController; onError: (message: string) => void } | null>(null);
  const pickedPreviews = useRef(new Map<string, { size: number; timer: ReturnType<typeof setTimeout> }>());
  const releasePickedPreview = useCallback((uri: string) => {
    const previews = pickedPreviews.current;
    const preview = previews.get(uri);
    if (!preview) return;
    previews.delete(uri);
    clearTimeout(preview.timer);
    releasePickedURI(uri);
  }, []);
  const retainPickedPreview = useCallback((uri: string, size: number) => {
    const previews = pickedPreviews.current;
    clearTimeout(previews.get(uri)?.timer);
    previews.delete(uri);
    const timer = setTimeout(() => releasePickedPreview(uri), pickedPreviewLifetimeMs);
    previews.set(uri, { size, timer });
    let bytes = [...previews.values()].reduce((total, preview) => total + preview.size, 0);
    for (const [key, preview] of previews) {
      if (previews.size <= pickedPreviewMaxEntries && bytes <= pickedPreviewMaxBytes) break;
      releasePickedPreview(key);
      bytes -= preview.size;
    }
  }, [releasePickedPreview]);
  useEffect(() => () => {
    const upload = uploadRef.current;
    uploadRef.current = null;
    upload?.controller.abort();
    for (const uri of pickedPreviews.current.keys()) releasePickedPreview(uri);
  }, [releasePickedPreview]);
  const cancelFileUpload = useCallback(() => {
    const upload = uploadRef.current;
    if (!upload) return;
    uploadRef.current = null;
    upload.controller.abort();
    stopPending("upload");
  }, [stopPending]);
  const disconnectFileUpload = useCallback(() => {
    const upload = uploadRef.current;
    if (!upload) return;
    cancelFileUpload();
    if (relayScopeRef.current.pairingID === pairingID) {
      upload.onError(new AssetUploadDisconnectedError().message);
    }
  }, [cancelFileUpload, pairingID, relayScopeRef]);

  const uploadLocalFile = useCallback(async (kind: "all" | "image" = "all") => {
    if (!isCurrentDraft() || uploadRef.current) return;
    if (!sessionID) {
      addErrorMessage("请先加载会话再上传文件。");
      return;
    }
    const scope = { ...relayScopeRef.current };
    if (!scope.connectionID) {
      addErrorMessage("当前未连接 Relay，请先连接再上传文件。");
      return;
    }
    const controller = new AbortController();
    const upload = { controller, onError: addErrorMessage };
    uploadRef.current = upload;
    startPending("upload");
    let pickedURI: string | undefined;
    let keepPickedPreview = false;
    try {
      const result = await DocumentPicker.getDocumentAsync({
        copyToCacheDirectory: true,
        multiple: false,
        base64: false,
        type: kind === "image" ? ["image/*"] : ["image/*", "*/*"],
      });
      const asset = result.assets?.[0];
      pickedURI = asset?.uri;
      if (!asset || uploadRef.current !== upload || controller.signal.aborted) return;
      assertAssetUploadScope(scope, relayScopeRef.current);
      const uploaded = await uploadMobileAsset({ asset, scope, sessionID, signal: controller.signal });
      if (uploadRef.current !== upload || controller.signal.aborted) return;
      assertAssetUploadScope(scope, relayScopeRef.current);
      const attachment: UploadedAssetAttachment = {
        ...uploaded,
        content_type: uploaded.content_type || inferMimeType(asset.name, asset.mimeType),
        file_name: uploaded.file_name || asset.name || "已上传文件",
        kind: "uploaded_asset",
      };
      const localURI = isImageAttachment(attachment) ? pickedImageURI(pickedURI) : undefined;
      if (localURI) {
        retainPickedPreview(localURI, uploaded.size);
        attachment.local_uri = localURI;
        keepPickedPreview = true;
      }
      // The setter belongs to the originating draft. Never move the visible
      // screen or attach to whichever session happens to be open now.
      setAttachedFiles((current) => {
        if (current.some((file) => attachmentKey(file) === attachmentKey(attachment))) return current;
        if (current.length >= maxAttachedFiles) return [...current.slice(1), attachment];
        return [...current, attachment];
      });
    } catch (error) {
      if (uploadRef.current === upload && relayScopeRef.current.pairingID === scope.pairingID && !(error instanceof AssetUploadCanceledError)) {
        const failure = relayScopeRef.current.connectionID === scope.connectionID ? error : new AssetUploadDisconnectedError();
        addErrorMessage(failure instanceof Error ? failure.message : "文件上传失败，请稍后重试。");
      }
    } finally {
      if (pickedURI && !keepPickedPreview) releasePickedURI(pickedURI);
      if (uploadRef.current === upload) {
        uploadRef.current = null;
        stopPending("upload");
      }
    }
  }, [addErrorMessage, isCurrentDraft, relayScopeRef, retainPickedPreview, sessionID, setAttachedFiles, startPending, stopPending, uploadMobileAsset]);

  const removeAttachedFile = useCallback((key: string) => {
    setAttachedFiles((current) => current.filter((file) => attachmentKey(file) !== key));
  }, [setAttachedFiles]);

  const clearAttachedFiles = useCallback(() => {
    setAttachedFiles(() => []);
  }, [setAttachedFiles]);

  const sendMessageWithFiles = useCallback(
    (content: string, requestID: string) => {
      if (!isCurrentDraft()) return false;
      const sent = sendEnvelope("user_message", {
        request_id: requestID,
        session_id: sessionID,
        payload: { content: messageWithAttachedFiles(content, attachedFiles) },
      });

      // 发送入队后只清空原会话的草稿；本地发送失败时保留正文和附件。
      if (sent) {
        setMessageInput("");
        clearAttachedFiles();
      }
      return sent;
    },
    [attachedFiles, clearAttachedFiles, isCurrentDraft, sendEnvelope, sessionID, setMessageInput],
  );

  return {
    cancelFileUpload,
    disconnectFileUpload,
    attachFilePreview,
    clearAttachedFiles,
    goToParent,
    openFileEntry,
    openSelectedChangeFile,
    refreshCurrentFiles,
    readFilePath,
    removeAttachedFile,
    sendMessageWithFiles,
    uploadLocalFile,
  };
}
