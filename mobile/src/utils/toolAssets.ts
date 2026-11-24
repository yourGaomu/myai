import type { AssetSummary, UploadedAssetPayload } from "../protocol";
import { isAssetID } from "./assetImagePreview";

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
    businessID: stringValue(summary.business_id),
    checksum: stringValue(summary.checksum),
    code: stringValue(summary.code),
    contentType: stringValue(summary.content_type),
    expiresAt: stringValue(summary.expires_at),
    fileName: stringValue(summary.file_name),
    maxVisits: numberValue(summary.max_visits),
    path: stringValue(summary.path),
    shortURL: assetPublicURL(summary.short_url),
    size: numberValue(summary.size),
  };
}

export function isAssistantAssetTool(toolName?: string) {
  return toolName === "attach_image" || toolName === "generate_image" || toolName === "share_file";
}

// Whitelist metadata from authenticated completions and the session asset list.
// The database record ID is distinct from the previewable asset ID.
export function assetSummariesForSession(value: unknown, sessionID: string): AssetSummary[] {
  if (!sessionID || !Array.isArray(value)) return [];
  return value.flatMap((item) => {
    if (!isRecord(item) || (item.session_id !== undefined && item.session_id !== sessionID)) return [];
    const id = stringValue(item.id);
    const assetID = isAssetID(item.asset_id) ? item.asset_id : undefined;
    if (!id && !assetID) return [];
    return [{
      id: id || assetID!, asset_id: assetID, session_id: sessionID,
      request_id: stringValue(item.request_id), tool_call_id: stringValue(item.tool_call_id), tool_name: stringValue(item.tool_name),
      business_id: stringValue(item.business_id), checksum: stringValue(item.checksum),
      file_name: stringValue(item.file_name), content_type: stringValue(item.content_type), path: stringValue(item.path),
      size: numberValue(item.size), short_url: assetPublicURL(item.short_url), code: stringValue(item.code),
      expires_at: stringValue(item.expires_at), max_visits: numberValue(item.max_visits), created_at: stringValue(item.created_at),
    }];
  });
}

// Prefer defined metadata from the newer source without discarding an existing
// short link when a completion only reports file identity.
export function mergeSharedAssets(...groups: (readonly SharedAsset[])[]): SharedAsset[] {
  const assets = new Map<string, SharedAsset>();
  for (const group of groups) {
    for (const asset of group) {
      const previous = assets.get(asset.assetID);
      assets.set(asset.assetID, previous ? mergeAssetFields(previous, asset) : asset);
    }
  }
  return [...assets.values()];
}

export function mergeAssetSummaries(...groups: (readonly AssetSummary[])[]): AssetSummary[] {
  const canonicalIDs = new Map<string, string>();
  for (const group of groups) {
    for (const asset of group) if (asset.asset_id) canonicalIDs.set(asset.id, asset.asset_id);
  }
  const assets = new Map<string, AssetSummary>();
  for (const group of groups) {
    for (const asset of group) {
      const key = asset.asset_id || canonicalIDs.get(asset.id) || asset.id;
      const previous = assets.get(key);
      assets.set(key, previous ? mergeAssetFields(previous, asset) : asset);
    }
  }
  return [...assets.values()];
}

function mergeAssetFields<T extends object>(previous: T, next: T): T {
  const merged = { ...previous };
  for (const key in next) {
    if (next[key] !== undefined) merged[key] = next[key];
  }
  return merged;
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
  if (isAssistantAssetTool(toolName)) return true;
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

export function isRelayPreviewImageAsset(asset: SharedAsset) {
  const contentType = asset.contentType?.toLowerCase().split(";", 1)[0].trim();
  if (contentType && ["image/jpeg", "image/jpg", "image/pjpeg", "image/png", "image/x-png", "image/gif", "image/webp"].includes(contentType)) return true;
  if (contentType && contentType !== "application/octet-stream" && contentType !== "image/*") return false;
  return /\.(?:jpe?g|png|gif|webp)$/i.test(asset.fileName || asset.path || "");
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
