import type { DocumentPickerAsset } from "expo-document-picker";
import { File } from "expo-file-system";
import { fetch as expoFetch } from "expo/fetch";
import { Platform } from "react-native";

import type { UploadedAssetPayload } from "../protocol";
import { inferMimeType } from "./attachments";

type UploadMobileAssetOptions = {
  asset: DocumentPickerAsset;
  apiKey: string;
  baseURL: string;
  sessionID?: string;
};

export async function uploadMobileAsset({
  asset,
  apiKey,
  baseURL,
  sessionID,
}: UploadMobileAssetOptions): Promise<UploadedAssetPayload> {
  const normalizedBaseURL = normalizeBaseURL(baseURL);
  const normalizedAPIKey = apiKey.trim();
  if (!normalizedAPIKey) {
    throw new Error("请先在设置中填写独立的 Asset API Key，再上传文件（仅本次运行保存）。");
  }
  if (/[\r\n]/.test(normalizedAPIKey)) {
    throw new Error("Asset API Key 不能包含换行，请在设置中重新填写。");
  }

  const mimeType = inferMimeType(asset.name, asset.mimeType);
  const fileName = asset.name || "upload";

  const body = new FormData();
  if (Platform.OS === "web") {
    if (asset.file) {
      body.append("file", asset.file, fileName);
    } else {
      try {
        // Reading a picked URI is public/local and must never receive the API key.
        const res = await fetch(asset.uri, { credentials: "omit" });
        if (!res.ok) {
          throw new Error("file read failed");
        }
        body.append("file", await res.blob(), fileName);
      } catch {
        throw new Error("无法读取所选文件，请重新选择后上传。");
      }
    }
  } else {
    // expo/fetch supports redirect blocking on native platforms. Its multipart
    // encoder needs a Blob-compatible file, not React Native's { uri } object.
    const file = new File(asset.uri);
    body.append("file", {
      name: fileName,
      type: mimeType,
      bytes: () => file.bytes(),
    } as unknown as Blob);
  }
  body.append("title", fileName);
  if (sessionID) {
    body.append("scope", sessionID);
  }

  let response: Awaited<ReturnType<typeof expoFetch>>;
  try {
    response = await expoFetch(`${normalizedBaseURL}/api/assets`, {
      method: "POST",
      body,
      headers: { Authorization: `Bearer ${normalizedAPIKey}` },
      credentials: "omit",
      // The native Expo transport enforces this before any redirected request.
      // Require the final API URL, so credentials cannot reach another origin.
      redirect: "error",
    });
  } catch {
    // Never surface transport errors that may contain request headers or URLs.
    throw new Error("资源上传请求失败，请检查资源服务地址和网络，并填写无需重定向的最终服务地址。");
  }

  if (!response.ok) {
    throw new Error(uploadErrorMessage(response.status));
  }

  try {
    const data = (await response.json()) as UploadedAssetPayload;
    if (typeof data.code !== "string" || !data.code.trim() || typeof data.short_url !== "string" || !data.short_url.trim()) {
      throw new Error("invalid asset response");
    }
    // Only public metadata may enter chat attachments or relay payloads.
    return {
      code: data.code,
      short_url: data.short_url,
      bucket: data.bucket,
      object_key: data.object_key,
      file_name: data.file_name,
      content_type: data.content_type,
      size: data.size,
      expires_at: data.expires_at,
    };
  } catch {
    throw new Error("资源服务返回了无效的上传结果，请检查服务后重试。");
  }
}

function normalizeBaseURL(value: string) {
  const normalized = value.trim().replace(/\/+$/, "");
  if (!normalized) {
    throw new Error("asset service url is required");
  }
  let url: URL;
  try {
    url = new URL(normalized);
  } catch {
    throw new Error("资源服务地址无效，请填写完整的 http 或 https 地址。");
  }
  if (!["http:", "https:"].includes(url.protocol) || !url.host || url.username || url.password || url.search || url.hash) {
    throw new Error("资源服务地址仅支持不含凭据、查询参数和片段的 http 或 https 地址；请在独立字段填写 Asset API Key。");
  }
  return normalized;
}

function uploadErrorMessage(status: number) {
  if (status === 401 || status === 403) {
    return `资源服务鉴权失败（${status}），请检查设置中的 Asset API Key。`;
  }
  // Response bodies can echo Authorization; keep them out of chat/history.
  return `文件上传失败：资源服务返回 HTTP ${status}。`;
}
