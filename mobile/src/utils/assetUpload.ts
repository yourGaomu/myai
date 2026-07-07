import type { DocumentPickerAsset } from "expo-document-picker";

import type { UploadedAssetPayload } from "../protocol";

type UploadMobileAssetOptions = {
  asset: DocumentPickerAsset;
  baseURL: string;
  sessionID?: string;
};

export async function uploadMobileAsset({
  asset,
  baseURL,
  sessionID,
}: UploadMobileAssetOptions): Promise<UploadedAssetPayload> {
  const normalizedBaseURL = normalizeBaseURL(baseURL);
  if (!normalizedBaseURL) {
    throw new Error("Asset service URL is required");
  }

  const body = new FormData();
  if (asset.file) {
    body.append("file", asset.file);
  } else {
    body.append("file", {
      uri: asset.uri,
      name: asset.name || "upload",
      type: asset.mimeType || "application/octet-stream",
    } as unknown as Blob);
  }
  body.append("title", asset.name || "mobile upload");
  if (sessionID) {
    body.append("scope", sessionID);
  }

  const response = await fetch(`${normalizedBaseURL}/api/assets`, {
    method: "POST",
    body,
  });

  if (!response.ok) {
    throw new Error(await uploadErrorMessage(response));
  }

  return (await response.json()) as UploadedAssetPayload;
}

function normalizeBaseURL(value: string) {
  return value.trim().replace(/\/+$/, "");
}

async function uploadErrorMessage(response: Response) {
  const fallback = `Upload failed: ${response.status} ${response.statusText}`;
  try {
    const data = (await response.json()) as { error?: string; message?: string };
    return data.error || data.message || fallback;
  } catch {
    try {
      const text = await response.text();
      return text || fallback;
    } catch {
      return fallback;
    }
  }
}
