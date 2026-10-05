import type { DocumentPickerAsset } from "expo-document-picker";
import { Platform } from "react-native";

import type { UploadedAssetPayload } from "../protocol";
import { inferMimeType } from "./attachments";

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
    throw new Error("asset service url is required");
  }

  const mimeType = inferMimeType(asset.name, asset.mimeType);
  const fileName = asset.name || "upload";

  const body = new FormData();
  if (Platform.OS === "web") {
    if (asset.file) {
      body.append("file", asset.file);
    } else {
      try {
        const res = await fetch(asset.uri);
        const blob = await res.blob();
        body.append("file", blob, fileName);
      } catch {
        body.append("file", {
          uri: asset.uri,
          name: fileName,
          type: mimeType,
        } as any);
      }
    }
  } else {
    // React Native Native (iOS / Android) FormData file object contract
    body.append("file", {
      uri: asset.uri,
      name: fileName,
      type: mimeType,
    } as any);
  }
  body.append("title", fileName);
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
  const fallback = `asset upload failed: ${response.status} ${response.statusText}`;
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
