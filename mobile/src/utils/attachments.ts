import type { FileReadResultPayload } from "../protocol";
import type { ChatAttachment, UploadedAssetAttachment, WorkspaceFileAttachment } from "../types/chat";

const maxAttachedFileChars = 12000;

export function workspaceFileAttachment(file: FileReadResultPayload): WorkspaceFileAttachment {
  return {
    ...file,
    kind: "workspace_file",
  };
}

export function uploadedAssetAttachment(asset: UploadedAssetAttachment): UploadedAssetAttachment {
  return asset;
}

export function messageWithAttachedFiles(content: string, files: ChatAttachment[]) {
  if (files.length === 0) {
    return content;
  }

  const fileBlocks = files.map((file) => {
    if (isUploadedAssetAttachment(file)) {
      return [
        `<uploaded_file name="${escapeAttribute(file.file_name)}" content_type="${escapeAttribute(file.content_type || "")}" size="${file.size || 0}" short_url="${escapeAttribute(file.short_url)}" code="${escapeAttribute(file.code)}">`,
        "The user uploaded this file from mobile. Use the short_url to inspect or download it when needed.",
        "</uploaded_file>",
      ].join("\n");
    }

    const body = truncateText(file.content || "", maxAttachedFileChars);
    return [
      `<file path="${escapeAttribute(file.path)}" language="${escapeAttribute(file.language)}" size="${file.size}">`,
      body,
      file.truncated || (file.content || "").length > maxAttachedFileChars ? "\n[content truncated]" : "",
      "</file>",
    ].join("\n");
  });

  const prompt = content || "Please read the attached file content and tell me what you see.";
  return `${prompt}\n\nAttached files:\n${fileBlocks.join("\n\n")}`;
}

export function userMessageEcho(content: string, files: ChatAttachment[]) {
  const text = content || "Sent attached file context";
  if (files.length === 0) {
    return text;
  }
  const names = files.map((file) => `@${attachmentTitle(file)}`).join("\n");
  return `${text}\n\n${names}`;
}

export function attachmentKey(file: ChatAttachment) {
  if (isUploadedAssetAttachment(file)) {
    return `asset:${file.code || file.short_url}`;
  }
  return `workspace:${file.path}`;
}

export function attachmentTitle(file: ChatAttachment) {
  return isUploadedAssetAttachment(file) ? file.file_name : file.name;
}

export function attachmentMeta(file: ChatAttachment) {
  if (isUploadedAssetAttachment(file)) {
    return `${file.short_url} / ${formatAttachmentSize(file.size)} / uploaded`;
  }
  return `${file.path} / ${formatAttachmentSize(file.size)}${file.truncated ? " / truncated" : ""}`;
}

export function isWorkspaceFileAttachment(file: ChatAttachment): file is WorkspaceFileAttachment {
  return file.kind === "workspace_file";
}

export function isUploadedAssetAttachment(file: ChatAttachment): file is UploadedAssetAttachment {
  return file.kind === "uploaded_asset";
}

function truncateText(text: string, maxChars: number) {
  if (text.length <= maxChars) {
    return text;
  }
  return text.slice(0, maxChars);
}

function escapeAttribute(value: string) {
  return value.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function formatAttachmentSize(size?: number) {
  if (!size || size <= 0) {
    return "unknown size";
  }
  return `${size} bytes`;
}
