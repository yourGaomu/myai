export type SharedAsset = {
  code?: string;
  contentType?: string;
  expiresAt?: string;
  fileName?: string;
  path?: string;
  shortURL: string;
  size?: number;
};

type RawSharedAsset = {
  code?: unknown;
  content_type?: unknown;
  expires_at?: unknown;
  file_name?: unknown;
  path?: unknown;
  short_url?: unknown;
  size?: unknown;
};

export function parseSharedAsset(toolName?: string, result?: string): SharedAsset | null {
  if (toolName !== "share_file" || !result) {
    return null;
  }

  let parsed: RawSharedAsset;
  try {
    parsed = JSON.parse(result) as RawSharedAsset;
  } catch {
    return null;
  }

  const shortURL = stringValue(parsed.short_url);
  if (!shortURL) {
    return null;
  }

  return {
    code: stringValue(parsed.code),
    contentType: stringValue(parsed.content_type),
    expiresAt: stringValue(parsed.expires_at),
    fileName: stringValue(parsed.file_name),
    path: stringValue(parsed.path),
    shortURL,
    size: numberValue(parsed.size),
  };
}

export function isPreviewableImageAsset(asset: SharedAsset) {
  const contentType = asset.contentType?.toLowerCase() || "";
  if (contentType.startsWith("image/") && contentType !== "image/svg+xml") {
    return true;
  }

  const sourceName = `${asset.fileName || ""} ${asset.path || ""}`.toLowerCase();
  return [".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp"].some((extension) => sourceName.endsWith(extension) || sourceName.includes(`${extension} `));
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function numberValue(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}
