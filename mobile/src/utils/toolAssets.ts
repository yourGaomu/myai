import type { AssetSummary, UploadedAssetPayload } from "../protocol";

export type SharedAsset = {
  assetID: string;
  businessID?: string;
  checksum?: string;
  code?: string;
  contentType?: string;
  expiresAt?: string;
  fileName?: string;
  maxVisits?: number;
  path?: string;
  shortURL?: string;
  size?: number;
};

export function assetSummaryToSharedAsset(summary: AssetSummary): SharedAsset {
  return {
    assetID: summary.asset_id || summary.id,
    businessID: summary.business_id,
    checksum: summary.checksum,
    code: summary.code,
    contentType: summary.content_type,
    expiresAt: summary.expires_at,
    fileName: summary.file_name,
    maxVisits: summary.max_visits,
    path: summary.path,
    shortURL: assetPublicURL(summary.short_url),
    size: summary.size,
  };
}

// Copy only the canonical public metadata fields. Local URIs, backend storage
// paths, credentials and response extras never become uploaded attachments.
export function normalizeUploadedAsset(value: unknown): UploadedAssetPayload | null {
  if (!isRecord(value)) return null;
  const assetID = stringValue(value.asset_id);
  const fileName = stringValue(value.file_name);
  const size = numberValue(value.size);
  if (!assetID || !fileName || size === undefined || !Number.isSafeInteger(size) || size < 0) return null;
  return {
    asset_id: assetID,
    business_id: stringValue(value.business_id),
    file_name: fileName,
    content_type: stringValue(value.content_type),
    size,
    checksum: stringValue(value.checksum),
    short_url: assetPublicURL(value.short_url),
    code: stringValue(value.code),
    expires_at: stringValue(value.expires_at),
    max_visits: numberValue(value.max_visits),
  };
}

const shortLinksTools = new Set([
  "upload_file", "get_asset_info", "create_asset_shortlink", "get_asset_download_url",
]);

function isAssetTool(toolName?: string) {
  if (toolName === "share_file") return true;
  if (!toolName) return false;
  // Native MCP tools may expose their original name or a server-qualified name.
  if (shortLinksTools.has(toolName)) return true;
  return [...shortLinksTools].some((name) =>
    toolName === `short_links.${name}` || toolName === `short_links__${name}` ||
    toolName === `mcp__short_links__${name}` || toolName === `mcp_short_links_${name}`,
  );
}

export function parseSharedAssets(toolName?: string, result?: string): SharedAsset[] {
  if (!isAssetTool(toolName) || !result) return [];
  let parsed: unknown;
  try { parsed = JSON.parse(result); } catch { return []; }
  if (!isRecord(parsed)) return [];
  const values = Array.isArray(parsed.files) ? parsed.files : Array.isArray(parsed.assets) ? parsed.assets : [parsed];
  return values.flatMap((value) => {
    const asset = normalizeUploadedAsset(value);
    if (!asset) return [];
    return [{
      assetID: asset.asset_id,
      businessID: asset.business_id,
      checksum: asset.checksum,
      code: asset.code,
      contentType: asset.content_type,
      expiresAt: asset.expires_at,
      fileName: asset.file_name,
      maxVisits: asset.max_visits,
      path: isRecord(value) ? stringValue(value.path) : undefined,
      shortURL: asset.short_url,
      size: asset.size,
    }];
  });
}

export function parseSharedAsset(toolName?: string, result?: string): SharedAsset | null {
  return parseSharedAssets(toolName, result)[0] || null;
}

export function isPreviewableImageAsset(asset: SharedAsset) {
  const contentType = asset.contentType?.toLowerCase() || "";
  if (contentType.startsWith("image/") && contentType !== "image/svg+xml") return true;
  const sourceName = `${asset.fileName || ""} ${asset.path || ""}`.toLowerCase();
  return [".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp"].some((extension) => sourceName.endsWith(extension) || sourceName.includes(`${extension} `));
}

export function assetPublicURL(value: unknown): string | undefined {
  const text = stringValue(value);
  if (!text) return undefined;
  try {
    const url = new URL(text);
    return ["http:", "https:"].includes(url.protocol) && !url.username && !url.password ? text : undefined;
  } catch { return undefined; }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value.trim() || undefined : undefined;
}

function numberValue(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}
