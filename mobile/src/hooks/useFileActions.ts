import { useCallback, useEffect, useRef } from "react";
import * as DocumentPicker from "expo-document-picker";

import { maxAttachedFiles } from "../constants/app";
import type { FileEntry, FileReadResultPayload, RelayMessage } from "../protocol";
import type { PendingAction, ViewMode } from "../types/app";
import type { ChatAttachment, UploadedAssetAttachment } from "../types/chat";
import { attachmentKey, inferMimeType, isWorkspaceFileAttachment, messageWithAttachedFiles, workspaceFileAttachment } from "../utils/attachments";
import { AssetUploadCanceledError } from "../utils/assetUpload";
import type { UploadMobileAsset } from "./useAssetUpload";
import { newRequestID } from "../utils/ids";
import { parentPathOf } from "../utils/paths";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  addErrorMessage: (message: string) => void;
  uploadMobileAsset: UploadMobileAsset;
  attachedFiles: ChatAttachment[];
  changeDiffPath?: string;
  fileParent: string;
  filePreview: FileReadResultPayload | null;
  requestFiles: (path?: string) => boolean;
  selectedChange: string;
  sendEnvelope: SendEnvelope;
  sessionID: string;
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
  requestFiles,
  selectedChange,
  sendEnvelope,
  sessionID,
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

  const attachFilePreview = useCallback(() => {
    if (!filePreview) {
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
  }, [addErrorMessage, filePreview, setAttachedFiles, setViewMode]);

  const uploadControllerRef = useRef<AbortController | null>(null);
  useEffect(() => () => uploadControllerRef.current?.abort(), []);
  const cancelFileUpload = useCallback(() => uploadControllerRef.current?.abort(), []);

  const uploadLocalFile = useCallback(async (kind: "all" | "image" = "all") => {
    if (uploadControllerRef.current) return;
    const controller = new AbortController();
    uploadControllerRef.current = controller;
    startPending("upload");
    try {
      const result = await DocumentPicker.getDocumentAsync({
        copyToCacheDirectory: true,
        multiple: false,
        base64: false,
        type: kind === "image" ? ["image/*"] : ["image/*", "*/*"],
      });
      if (controller.signal.aborted || result.canceled || !result.assets?.[0]) return;
      const asset = result.assets[0];
      const uploaded = await uploadMobileAsset({ asset, sessionID, signal: controller.signal });
      if (controller.signal.aborted) return;
      const attachment: UploadedAssetAttachment = {
        ...uploaded,
        content_type: uploaded.content_type || inferMimeType(asset.name, asset.mimeType),
        file_name: uploaded.file_name || asset.name || "已上传文件",
        kind: "uploaded_asset",
        local_uri: asset.uri,
      };
      setAttachedFiles((current) => {
        if (current.some((file) => attachmentKey(file) === attachmentKey(attachment))) return current;
        if (current.length >= maxAttachedFiles) return [...current.slice(1), attachment];
        return [...current, attachment];
      });
      setViewMode("chat");
    } catch (error) {
      if (!(error instanceof AssetUploadCanceledError)) {
        addErrorMessage(error instanceof Error ? error.message : "文件上传失败，请稍后重试。");
      }
    } finally {
      uploadControllerRef.current = null;
      stopPending("upload");
    }
  }, [addErrorMessage, sessionID, setAttachedFiles, setViewMode, startPending, stopPending, uploadMobileAsset]);

  const removeAttachedFile = useCallback((key: string) => {
    setAttachedFiles((current) => current.filter((file) => attachmentKey(file) !== key));
  }, [setAttachedFiles]);

  const clearAttachedFiles = useCallback(() => {
    setAttachedFiles(() => []);
  }, [setAttachedFiles]);

  const sendMessageWithFiles = useCallback(
    (content: string, requestID: string) => {
      const sent = sendEnvelope("user_message", {
        request_id: requestID,
        payload: { content: messageWithAttachedFiles(content, attachedFiles) },
      });

      // 只有 Relay 确认接收后才清空编辑区；断线时保留正文和附件，方便用户修复连接后重试。
      if (sent) {
        setMessageInput("");
        clearAttachedFiles();
      }
      return sent;
    },
    [attachedFiles, clearAttachedFiles, sendEnvelope, setMessageInput],
  );

  return {
    cancelFileUpload,
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
