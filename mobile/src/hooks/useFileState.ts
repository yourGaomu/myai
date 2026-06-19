import { useCallback, useState } from "react";

import type { FileEntry, FileReadResultPayload } from "../protocol";

export function useFileState() {
  const [filePath, setFilePath] = useState(".");
  const [fileEntries, setFileEntries] = useState<FileEntry[]>([]);
  const [fileParent, setFileParent] = useState("");
  const [filePreview, setFilePreview] = useState<FileReadResultPayload | null>(null);
  const [attachedFiles, setAttachedFiles] = useState<FileReadResultPayload[]>([]);
  const [messageInput, setMessageInput] = useState("");

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
