import { useCallback } from "react";
import * as DocumentPicker from "expo-document-picker";

import { maxAttachedFiles } from "../constants/app";
import type { FileEntry, FileReadResultPayload, RelayMessage } from "../protocol";
import type { PendingAction, ViewMode } from "../types/app";
import type { ChatAttachment, UploadedAssetAttachment } from "../types/chat";
import { attachmentKey, isWorkspaceFileAttachment, messageWithAttachedFiles, workspaceFileAttachment } from "../utils/attachments";
import { uploadMobileAsset } from "../utils/assetUpload";
import { newRequestID } from "../utils/ids";
import { parentPathOf } from "../utils/paths";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  addErrorMessage: (message: string) => void;
  assetBaseURL: string;
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
  assetBaseURL,
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

  const uploadLocalFile = useCallback(async () => {
    // 手机文件先上传 Asset 服务，聊天消息只携带短链接元数据，避免大文件经过 WebSocket。
    if (!assetBaseURL.trim()) {
      addErrorMessage("请先在设置中填写资源服务地址，再上传手机文件。");
      return;
    }

    startPending("upload");
    try {
      const result = await DocumentPicker.getDocumentAsync({
        copyToCacheDirectory: true,
        multiple: false,
        base64: false,
      });
      if (result.canceled || !result.assets?.[0]) {
        return;
      }

      const asset = result.assets[0];
      const uploaded = await uploadMobileAsset({
        asset,
        baseURL: assetBaseURL,
        sessionID,
      });
      const attachment: UploadedAssetAttachment = {
        ...uploaded,
        content_type: uploaded.content_type || asset.mimeType || "application/octet-stream",
        file_name: uploaded.file_name || asset.name || "已上传文件",
        kind: "uploaded_asset",
        local_uri: asset.uri,
      };
      setAttachedFiles((current) => {
        if (current.some((file) => attachmentKey(file) === attachmentKey(attachment))) {
          return current;
        }
        if (current.length >= maxAttachedFiles) {
          return [...current.slice(1), attachment];
        }
        return [...current, attachment];
      });
      setViewMode("chat");
    } catch (error) {
      addErrorMessage(uploadFailureMessage(error));
    } finally {
      stopPending("upload");
    }
  }, [addErrorMessage, assetBaseURL, sessionID, setAttachedFiles, setViewMode, startPending, stopPending]);

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

function uploadFailureMessage(error: unknown) {
  const raw = error instanceof Error ? error.message : String(error || "");
  const normalized = raw.toLowerCase();

  if (normalized.includes("network request failed") || normalized.includes("failed to fetch")) {
    return "无法连接资源服务，请检查资源服务地址、服务器端口和手机网络。";
  }
  if (normalized.includes("asset service url is required")) {
    return "请先在设置中填写资源服务地址。";
  }
  if (normalized.includes("request timed out") || normalized.includes("timeout")) {
    return "文件上传超时，请检查网络后重试。";
  }
  if (normalized.includes("upload failed") || normalized.includes("asset upload failed")) {
    return `文件上传失败：${raw.replace(/^asset upload failed:\s*/i, "")}`;
  }
  return raw ? `文件上传失败：${raw}` : "文件上传失败，请稍后重试。";
}
