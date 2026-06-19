import { useCallback } from "react";

import { maxAttachedFiles } from "../constants/app";
import type { FileEntry, FileReadResultPayload, RelayMessage } from "../protocol";
import type { PendingAction, ViewMode } from "../types/app";
import { messageWithAttachedFiles } from "../utils/attachments";
import { newRequestID } from "../utils/ids";
import { parentPathOf } from "../utils/paths";

type SendEnvelope = (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;

type Args = {
  addErrorMessage: (message: string) => void;
  attachedFiles: FileReadResultPayload[];
  changeDiffPath?: string;
  fileParent: string;
  filePreview: FileReadResultPayload | null;
  requestFiles: (path?: string) => boolean;
  selectedChange: string;
  sendEnvelope: SendEnvelope;
  setAttachedFiles: (updater: (current: FileReadResultPayload[]) => FileReadResultPayload[]) => void;
  setFilePreview: (preview: FileReadResultPayload | null) => void;
  setMessageInput: (input: string) => void;
  setViewMode: (mode: ViewMode) => void;
  startPending: (action: PendingAction) => void;
  stopPending: (action: PendingAction) => void;
};

export function useFileActions({
  addErrorMessage,
  attachedFiles,
  changeDiffPath,
  fileParent,
  filePreview,
  requestFiles,
  selectedChange,
  sendEnvelope,
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
      if (current.some((file) => file.path === filePreview.path)) {
        return current;
      }
      if (current.length >= maxAttachedFiles) {
        return [...current.slice(1), filePreview];
      }
      return [...current, filePreview];
    });
    setViewMode("chat");
  }, [addErrorMessage, filePreview, setAttachedFiles, setViewMode]);

  const removeAttachedFile = useCallback((path: string) => {
    setAttachedFiles((current) => current.filter((file) => file.path !== path));
  }, [setAttachedFiles]);

  const clearAttachedFiles = useCallback(() => {
    setAttachedFiles(() => []);
  }, [setAttachedFiles]);

  const sendMessageWithFiles = useCallback(
    (content: string, requestID: string) => {
      setMessageInput("");
      clearAttachedFiles();

      return sendEnvelope("user_message", {
        request_id: requestID,
        payload: { content: messageWithAttachedFiles(content, attachedFiles) },
      });
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
  };
}
