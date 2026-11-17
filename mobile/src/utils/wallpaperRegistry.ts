export type WallpaperKind = "none" | "preset" | "image" | "video";
export type WallpaperResizeMode = "cover" | "contain";
export type WallpaperPresetId = "aurora" | "abyss" | "amber-mist";

export type WallpaperPreset = {
  darkColors: readonly [string, string, ...string[]];
  description: string;
  id: WallpaperPresetId;
  label: string;
  lightColors: readonly [string, string, ...string[]];
  locations?: readonly [number, number, ...number[]];
  swatchDark: string;
  swatchLight: string;
};

export const WALLPAPER_PRESETS: WallpaperPreset[] = [
  {
    id: "aurora",
    label: "极光微光",
    description: "冰川青与深空靛紫柔和交织",
    swatchDark: "#154759",
    swatchLight: "#BAE6FD",
    darkColors: ["#0B1720", "#102C3A", "#152136", "#0D1017"],
    lightColors: ["#E6F7FB", "#EDF4FF", "#F4F7FB", "#FFFFFF"],
    locations: [0, 0.35, 0.72, 1],
  },
  {
    id: "abyss",
    label: "深海夜幕",
    description: "低对比度冷静蓝灰纵向渐隐",
    swatchDark: "#1B2A4A",
    swatchLight: "#C7D2FE",
    darkColors: ["#0C1322", "#14223B", "#101827", "#0B0E14"],
    lightColors: ["#E8EEFF", "#F0F4FF", "#F6F8FC", "#FFFFFF"],
    locations: [0, 0.38, 0.75, 1],
  },
  {
    id: "amber-mist",
    label: "晨曦琥珀",
    description: "暖金余晖与柔雾底色",
    swatchDark: "#4B3219",
    swatchLight: "#FDE68A",
    darkColors: ["#1F1610", "#2F2016", "#1C1821", "#0F1115"],
    lightColors: ["#FFF3DF", "#FDF6EB", "#F9F7F5", "#FFFFFF"],
    locations: [0, 0.32, 0.7, 1],
  },
];

export type WallpaperSettings = {
  blurAmount: number;
  cardBackdrop: boolean;
  dimOpacity: number;
  fileName?: string;
  fileSize?: number;
  kind: WallpaperKind;
  localUri?: string;
  mimeType?: string;
  presetId: WallpaperPresetId;
  resizeMode: WallpaperResizeMode;
};

export const defaultWallpaperSettings: WallpaperSettings = {
  blurAmount: 0,
  cardBackdrop: true,
  dimOpacity: 0.42,
  kind: "none",
  presetId: "aurora",
  resizeMode: "cover",
};

export const MAX_WALLPAPER_IMAGE_BYTES = 20 * 1024 * 1024;
export const MAX_WALLPAPER_VIDEO_BYTES = 50 * 1024 * 1024;

export type DetectedWallpaperMedia = {
  ext: string;
  mediaKind: "image" | "video";
  mime: string;
};

export function clampWallpaperDim(value: number): number {
  if (!Number.isFinite(value)) return defaultWallpaperSettings.dimOpacity;
  return Math.max(0, Math.min(0.85, Number(value.toFixed(2))));
}

export function clampWallpaperBlur(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(80, Math.round(value)));
}

export function getWallpaperPreset(presetId?: string): WallpaperPreset {
  return WALLPAPER_PRESETS.find((item) => item.id === presetId) ?? WALLPAPER_PRESETS[0];
}

export function formatWallpaperSize(bytes?: number): string {
  if (!bytes || bytes <= 0) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function detectWallpaperMedia(
  header: Uint8Array,
  fileName?: string,
  mimeType?: string,
): DetectedWallpaperMedia | null {
  if (header.length >= 6) {
    const gifHeader = String.fromCharCode(header[0], header[1], header[2], header[3], header[4], header[5]);
    if (gifHeader === "GIF87a" || gifHeader === "GIF89a") {
      return { ext: ".gif", mediaKind: "image", mime: "image/gif" };
    }
  }

  if (header.length >= 12) {
    const riff = String.fromCharCode(header[0], header[1], header[2], header[3]);
    const webp = String.fromCharCode(header[8], header[9], header[10], header[11]);
    if (riff === "RIFF" && webp === "WEBP") {
      return { ext: ".webp", mediaKind: "image", mime: "image/webp" };
    }

    const ftyp = String.fromCharCode(header[4], header[5], header[6], header[7]);
    if (ftyp === "ftyp") {
      const brand = String.fromCharCode(header[8], header[9], header[10], header[11]);
      if (brand === "qt  ") {
        return { ext: ".mov", mediaKind: "video", mime: "video/quicktime" };
      }
      return { ext: ".mp4", mediaKind: "video", mime: "video/mp4" };
    }
  }

  if (header.length >= 4 && header[0] === 0x89 && header[1] === 0x50 && header[2] === 0x4e && header[3] === 0x47) {
    return { ext: ".png", mediaKind: "image", mime: "image/png" };
  }

  if (header.length >= 3 && header[0] === 0xff && header[1] === 0xd8 && header[2] === 0xff) {
    return { ext: ".jpg", mediaKind: "image", mime: "image/jpeg" };
  }

  if (header.length >= 4 && header[0] === 0x1a && header[1] === 0x45 && header[2] === 0xdf && header[3] === 0xa3) {
    return { ext: ".webm", mediaKind: "video", mime: "video/webm" };
  }

  const lowerName = (fileName || "").toLowerCase();
  const lowerMime = (mimeType || "").toLowerCase();

  if (lowerMime === "video/quicktime" || lowerName.endsWith(".mov")) {
    return { ext: ".mov", mediaKind: "video", mime: "video/quicktime" };
  }
  if (lowerMime === "video/webm" || lowerName.endsWith(".webm")) {
    return { ext: ".webm", mediaKind: "video", mime: "video/webm" };
  }
  if (lowerMime.startsWith("video/") || lowerName.endsWith(".mp4") || lowerName.endsWith(".m4v")) {
    return { ext: ".mp4", mediaKind: "video", mime: "video/mp4" };
  }

  if (lowerMime === "image/gif" || lowerName.endsWith(".gif")) {
    return { ext: ".gif", mediaKind: "image", mime: "image/gif" };
  }
  if (lowerMime === "image/webp" || lowerName.endsWith(".webp")) {
    return { ext: ".webp", mediaKind: "image", mime: "image/webp" };
  }
  if (lowerMime === "image/png" || lowerName.endsWith(".png")) {
    return { ext: ".png", mediaKind: "image", mime: "image/png" };
  }
  if (lowerMime === "image/jpeg" || lowerName.endsWith(".jpg") || lowerName.endsWith(".jpeg")) {
    return { ext: ".jpg", mediaKind: "image", mime: "image/jpeg" };
  }
  if (lowerMime.startsWith("image/")) {
    return { ext: ".png", mediaKind: "image", mime: lowerMime };
  }

  return null;
}
