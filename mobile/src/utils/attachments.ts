import type { FileReadResultPayload } from "../protocol";

const maxAttachedFileChars = 12000;

export function messageWithAttachedFiles(content: string, files: FileReadResultPayload[]) {
  if (files.length === 0) {
    return content;
  }

  const fileBlocks = files.map((file) => {
    const body = truncateText(file.content || "", maxAttachedFileChars);
    return [
      `<file path="${file.path}" language="${file.language}" size="${file.size}">`,
      body,
      file.truncated || (file.content || "").length > maxAttachedFileChars ? "\n[content truncated]" : "",
      "</file>",
    ].join("\n");
  });

  const prompt = content || "Please read the attached file content and tell me what you see.";
  return `${prompt}\n\nAttached files:\n${fileBlocks.join("\n\n")}`;
}

export function userMessageEcho(content: string, files: FileReadResultPayload[]) {
  const text = content || "Sent attached file context";
  if (files.length === 0) {
    return text;
  }
  const names = files.map((file) => `@${file.path}`).join("\n");
  return `${text}\n\n${names}`;
}

function truncateText(text: string, maxChars: number) {
  if (text.length <= maxChars) {
    return text;
  }
  return text.slice(0, maxChars);
}
