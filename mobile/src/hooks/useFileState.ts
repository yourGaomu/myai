import { useCallback, useState } from "react";

import type { FileEntry, FileReadResultPayload } from "../protocol";
import type { ChatAttachment } from "../types/chat";

type Draft = { attachedFiles: ChatAttachment[]; messageInput: string };
const emptyDraft: Draft = { attachedFiles: [], messageInput: "" };

// 文件浏览是工作区级状态；正文和待发送附件只属于当前配对与会话。
export function useFileState({ pairingID, sessionID }: { pairingID: string; sessionID: string }) {
  const [filePath, setFilePath] = useState(".");
  const [fileEntries, setFileEntries] = useState<FileEntry[]>([]);
  const [fileParent, setFileParent] = useState("");
  const [filePreview, setFilePreview] = useState<FileReadResultPayload | null>(null);
  const [drafts, setDrafts] = useState(() => new Map<string, Draft>());
  const draftKey = JSON.stringify([pairingID, sessionID]);
  const { attachedFiles, messageInput } = drafts.get(draftKey) || emptyDraft;

  // Callbacks close over the draft key. An upload started in another session can
  // finish into that draft without reading or replacing the visible composer.
  const updateDraft = useCallback((update: (draft: Draft) => Draft) => {
    setDrafts((current) => {
      const draft = current.get(draftKey) || emptyDraft;
      const updated = update(draft);
      if (updated.attachedFiles === draft.attachedFiles && updated.messageInput === draft.messageInput) return current;
      const next = new Map(current);
      if (updated.attachedFiles.length || updated.messageInput) next.set(draftKey, updated);
      else next.delete(draftKey);
      return next;
    });
  }, [draftKey]);
  const setAttachedFiles = useCallback((update: (files: ChatAttachment[]) => ChatAttachment[]) => {
    updateDraft((draft) => ({ ...draft, attachedFiles: update(draft.attachedFiles) }));
  }, [updateDraft]);
  const setMessageInput = useCallback((input: string) => {
    updateDraft((draft) => ({ ...draft, messageInput: input }));
  }, [updateDraft]);

  const clearFileEntries = useCallback(() => setFileEntries([]), []);

  return {
    attachedFiles,
    clearFileEntries,
    fileEntries,
    fileParent,
    filePath,
    filePreview,
    messageInput,
    setAttachedFiles,
    setFileEntries,
    setFileParent,
    setFilePath,
    setFilePreview,
    setMessageInput,
  };
}
