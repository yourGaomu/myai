import { useCallback, useState } from "react";

import type { FileEntry, FileReadResultPayload } from "../protocol";
import type { ChatAttachment } from "../types/chat";

// 保存文件浏览、预览和聊天附件状态，不包含任何远程请求逻辑。
export function useFileState() {
  const [filePath, setFilePath] = useState(".");
  const [fileEntries, setFileEntries] = useState<FileEntry[]>([]);
  const [fileParent, setFileParent] = useState("");
  const [filePreview, setFilePreview] = useState<FileReadResultPayload | null>(null);
  const [attachedFiles, setAttachedFiles] = useState<ChatAttachment[]>([]);
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
