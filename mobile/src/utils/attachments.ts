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
        `<uploaded_file name="${escapeAttribute(file.file_name)}" content_type="${escapeAttribute(file.content_type || "")}" size="${file.size || 0}" short_url="${escapeAttribute(file.short_url)}" code="${escapeAttribute(file.code)}" expires_at="${escapeAttribute(file.expires_at || "")}">`,
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

export function userMessageEcho(content: string, _files: ChatAttachment[]) {
  return content.trim();
}

// 历史消息只保存了发给模型的文本，因此从协议中的附件块恢复可展示的附件元数据。
export function parseAttachedFiles(content: string): ChatAttachment[] {
  const attachments: ChatAttachment[] = [];
  const uploadedPattern = /<uploaded_file\b([^>]*)>[\s\S]*?<\/uploaded_file>/gi;
  let uploadedMatch: RegExpExecArray | null;
  while ((uploadedMatch = uploadedPattern.exec(content))) {
    const attributes = parseAttributes(uploadedMatch[1]);
    const shortURL = attributes.short_url || "";
    if (!shortURL) {
      continue;
    }
    attachments.push({
      kind: "uploaded_asset",
      code: attributes.code || shortURL,
      short_url: shortURL,
      file_name: attributes.name || "已上传文件",
      content_type: attributes.content_type || undefined,
      size: numberAttribute(attributes.size),
      expires_at: attributes.expires_at || undefined,
    });
  }

  const workspacePattern = /<file\b([^>]*)>([\s\S]*?)<\/file>/gi;
  let workspaceMatch: RegExpExecArray | null;
  while ((workspaceMatch = workspacePattern.exec(content))) {
    const attributes = parseAttributes(workspaceMatch[1]);
    const path = attributes.path || "";
    if (!path) {
      continue;
    }
    const body = workspaceMatch[2].replace(/\n\[content truncated\]\s*$/i, "");
    attachments.push({
      kind: "workspace_file",
      path,
      name: path.split(/[\\/]/).pop() || path,
      language: attributes.language || "",
      content: body,
      size: numberAttribute(attributes.size) || 0,
      truncated: /\[content truncated\]/i.test(workspaceMatch[2]),
      binary: false,
    });
  }

  return attachments;
}

export function displayMessageText(content: string) {
  const value = content.trim();
  if (!value) {
    return "";
  }

  const markerIndex = value.indexOf("\n\nAttached files:");
  if (markerIndex >= 0) {
    return value.slice(0, markerIndex).trim();
  }

  return value
    .replace(/<uploaded_file\b[^>]*>[\s\S]*?<\/uploaded_file>/gi, "")
    .replace(/<file\b[^>]*>[\s\S]*?<\/file>/gi, "")
    .trim();
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
    return `${file.short_url} / ${formatAttachmentSize(file.size)} / 已上传`;
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
    return "大小未知";
  }
  return `${size} bytes`;
}

function parseAttributes(value: string) {
  const attributes: Record<string, string> = {};
  const pattern = /([a-zA-Z_][\w-]*)="([^"]*)"/g;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(value))) {
    attributes[match[1]] = decodeAttribute(match[2]);
  }
  return attributes;
}

function decodeAttribute(value: string) {
  return value
    .replace(/&quot;/g, '"')
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&amp;/g, "&");
}

function numberAttribute(value?: string) {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}
