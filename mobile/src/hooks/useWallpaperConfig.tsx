import AsyncStorage from "@react-native-async-storage/async-storage";
import * as DocumentPicker from "expo-document-picker";
import { Directory, File, FileMode, Paths } from "expo-file-system";
import type { ReactNode } from "react";
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { Platform } from "react-native";

import {
  clampWallpaperBlur,
  clampWallpaperDim,
  defaultWallpaperSettings,
  detectWallpaperMedia,
  MAX_WALLPAPER_IMAGE_BYTES,
  MAX_WALLPAPER_VIDEO_BYTES,
  type WallpaperPresetId,
  type WallpaperResizeMode,
  type WallpaperSettings,
} from "../utils/wallpaperRegistry";

const WALLPAPER_STORAGE_KEY = "myai.wallpaper.v1";

type PickWallpaperFilter = "all" | "image" | "video";

export type PickWallpaperResult = {
  error?: string;
  ok: boolean;
};

type WallpaperContextValue = {
  busy: boolean;
  clearWallpaper: () => void;
  isWallpaperActive: boolean;
  loaded: boolean;
  pickAndSaveWallpaper: (filter?: PickWallpaperFilter) => Promise<PickWallpaperResult>;
  selectPreset: (presetId: WallpaperPresetId) => void;
  settings: WallpaperSettings;
  updateWallpaper: (
    patch: Partial<Pick<WallpaperSettings, "blurAmount" | "cardBackdrop" | "dimOpacity" | "resizeMode">>,
  ) => void;
};

const WallpaperContext = createContext<WallpaperContextValue>({
  busy: false,
  clearWallpaper: () => {},
  isWallpaperActive: false,
  loaded: true,
  pickAndSaveWallpaper: async () => ({ ok: false }),
  selectPreset: () => {},
  settings: defaultWallpaperSettings,
  updateWallpaper: () => {},
});

function readHeaderFromWebBlob(blob: Blob): Promise<Uint8Array> {
  return blob.slice(0, 16).arrayBuffer().then((buf) => new Uint8Array(buf));
}

function uint8ToBase64(bytes: Uint8Array): string {
  const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  let out = "";
  for (let i = 0; i < bytes.length; i += 3) {
    const a = bytes[i];
    const b = i + 1 < bytes.length ? bytes[i + 1] : 0;
    const c = i + 2 < bytes.length ? bytes[i + 2] : 0;
    const triple = (a << 16) | (b << 8) | c;
    out += chars[(triple >> 18) & 63];
    out += chars[(triple >> 12) & 63];
    out += i + 1 < bytes.length ? chars[(triple >> 6) & 63] : "=";
    out += i + 2 < bytes.length ? chars[triple & 63] : "=";
  }
  return out;
}

function deleteLocalFileUri(uri?: string) {
  if (!uri || Platform.OS === "web" || !uri.startsWith("file://")) return;
  try {
    const file = new File(uri);
    if (file.exists) {
      file.delete();
    }
  } catch {
    // 清理旧本地缓存失败时忽略，不影响当前设置切换
  }
}

export function WallpaperProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<WallpaperSettings>(defaultWallpaperSettings);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let active = true;
    AsyncStorage.getItem(WALLPAPER_STORAGE_KEY)
      .then((raw) => {
        if (!active || !raw) return;
        try {
          const parsed = JSON.parse(raw) as Partial<WallpaperSettings>;
          let nextKind =
            parsed.kind === "none" || parsed.kind === "preset" || parsed.kind === "image" || parsed.kind === "video"
              ? parsed.kind
              : defaultWallpaperSettings.kind;
          let nextUri = typeof parsed.localUri === "string" ? parsed.localUri : undefined;

          if ((nextKind === "image" || nextKind === "video") && nextUri) {
            if (Platform.OS !== "web" && nextUri.startsWith("file://")) {
              try {
                if (!new File(nextUri).exists) {
                  nextKind = "none";
                  nextUri = undefined;
                }
              } catch {
                nextKind = "none";
                nextUri = undefined;
              }
            } else if (Platform.OS === "web" && nextUri.startsWith("blob:")) {
              // 浏览器刷新后旧 blob URL 会失效
              nextKind = "none";
              nextUri = undefined;
            }
          } else if (nextKind === "image" || nextKind === "video") {
            nextKind = "none";
          }

          setSettings({
            blurAmount:
              typeof parsed.blurAmount === "number"
                ? clampWallpaperBlur(parsed.blurAmount)
                : defaultWallpaperSettings.blurAmount,
            cardBackdrop:
              typeof parsed.cardBackdrop === "boolean"
                ? parsed.cardBackdrop
                : defaultWallpaperSettings.cardBackdrop,
            dimOpacity:
              typeof parsed.dimOpacity === "number"
                ? clampWallpaperDim(parsed.dimOpacity)
                : defaultWallpaperSettings.dimOpacity,
            fileName: nextUri ? parsed.fileName : undefined,
            fileSize: nextUri ? parsed.fileSize : undefined,
            kind: nextKind,
            localUri: nextUri,
            mimeType: nextUri ? parsed.mimeType : undefined,
            presetId:
              parsed.presetId === "aurora" || parsed.presetId === "abyss" || parsed.presetId === "amber-mist"
                ? parsed.presetId
                : defaultWallpaperSettings.presetId,
            resizeMode: (parsed.resizeMode === "contain" ? "contain" : "cover") as WallpaperResizeMode,
          });
        } catch {
          // ignore corrupted storage
        }
      })
      .finally(() => {
        if (active) setLoaded(true);
      });

    return () => {
      active = false;
    };
  }, []);

  const persist = useCallback((next: WallpaperSettings) => {
    void AsyncStorage.setItem(WALLPAPER_STORAGE_KEY, JSON.stringify(next));
  }, []);

  const updateWallpaper = useCallback(
    (patch: Partial<Pick<WallpaperSettings, "blurAmount" | "cardBackdrop" | "dimOpacity" | "resizeMode">>) => {
      setSettings((prev) => {
        const next: WallpaperSettings = {
          ...prev,
          ...patch,
          blurAmount: typeof patch.blurAmount === "number" ? clampWallpaperBlur(patch.blurAmount) : prev.blurAmount,
          dimOpacity: typeof patch.dimOpacity === "number" ? clampWallpaperDim(patch.dimOpacity) : prev.dimOpacity,
        };
        persist(next);
        return next;
      });
    },
    [persist],
  );

  const selectPreset = useCallback(
    (presetId: WallpaperPresetId) => {
      setSettings((prev) => {
        if (prev.localUri) {
          deleteLocalFileUri(prev.localUri);
        }
        const next: WallpaperSettings = {
          ...prev,
          fileName: undefined,
          fileSize: undefined,
          kind: "preset",
          localUri: undefined,
          mimeType: undefined,
          presetId,
        };
        persist(next);
        return next;
      });
    },
    [persist],
  );

  const clearWallpaper = useCallback(() => {
    setSettings((prev) => {
      if (prev.localUri) {
        deleteLocalFileUri(prev.localUri);
      }
      const next: WallpaperSettings = {
        ...prev,
        fileName: undefined,
        fileSize: undefined,
        kind: "none",
        localUri: undefined,
        mimeType: undefined,
      };
      persist(next);
      return next;
    });
  }, [persist]);

  const pickAndSaveWallpaper = useCallback(
    async (filter: PickWallpaperFilter = "all"): Promise<PickWallpaperResult> => {
      setBusy(true);
      try {
        const pickerType =
          filter === "video"
            ? ["video/mp4", "video/quicktime", "video/webm", "video/*"]
            : filter === "image"
              ? ["image/png", "image/jpeg", "image/webp", "image/gif", "image/*"]
              : ["video/mp4", "video/quicktime", "video/*", "image/*"];

        const picked = await DocumentPicker.getDocumentAsync({
          copyToCacheDirectory: true,
          multiple: false,
          type: pickerType,
        });

        if (picked.canceled || !picked.assets?.[0]) {
          return { ok: false };
        }

        const asset = picked.assets[0];
        let headerBytes: Uint8Array<ArrayBufferLike> = new Uint8Array(0);
        let fileSize = typeof asset.size === "number" ? asset.size : 0;

        if (Platform.OS === "web") {
          if (asset.file) {
            fileSize = asset.file.size;
            headerBytes = await readHeaderFromWebBlob(asset.file);
          } else {
            const resp = await fetch(asset.uri);
            const blob = await resp.blob();
            fileSize = blob.size;
            headerBytes = await readHeaderFromWebBlob(blob);
          }
        } else {
          const sourceFile = new File(asset.uri);
          if (!fileSize && sourceFile.size) {
            fileSize = sourceFile.size;
          }
          try {
            const handle = sourceFile.open(FileMode.ReadOnly);
            try {
              headerBytes = handle.readBytes(16);
              if (!fileSize && handle.size) {
                fileSize = handle.size;
              }
            } finally {
              handle.close();
            }
          } catch {
            // 部分 content URI 若不允许随机 FileHandle 读取，下面允许结合 MIME 与扩展名降级识别
          }
        }

        if (fileSize === 0) {
          return { ok: false, error: "所选文件为空，请重新选择。" };
        }

        const detected = detectWallpaperMedia(headerBytes, asset.name, asset.mimeType);
        if (!detected) {
          return {
            ok: false,
            error: "暂不支持该文件格式，请选择 MP4 / MOV / WebM 视频或 PNG / JPG / GIF / WebP 图片。",
          };
        }

        if (detected.mediaKind === "video" && fileSize > MAX_WALLPAPER_VIDEO_BYTES) {
          return {
            ok: false,
            error: "视频壁纸超过 50 MB 上限，建议压缩时长或分辨率后再导入。",
          };
        }
        if (detected.mediaKind === "image" && fileSize > MAX_WALLPAPER_IMAGE_BYTES) {
          return {
            ok: false,
            error: "图片或动图超过 20 MB 上限，建议适度压缩后再导入。",
          };
        }

        let storedUri = "";
        if (Platform.OS === "web") {
          if (detected.mediaKind === "image" && fileSize <= 4 * 1024 * 1024) {
            const raw = asset.file
              ? new Uint8Array(await asset.file.arrayBuffer())
              : new Uint8Array(await (await fetch(asset.uri)).arrayBuffer());
            storedUri = `data:${detected.mime};base64,${uint8ToBase64(raw)}`;
          } else {
            storedUri = asset.file && typeof URL !== "undefined" ? URL.createObjectURL(asset.file) : asset.uri;
          }
        } else {
          const dir = new Directory(Paths.document, "custom-wallpaper");
          dir.create({ idempotent: true, intermediates: true });
          const destFile = new File(dir, `wallpaper-${Date.now()}${detected.ext}`);
          const sourceFile = new File(asset.uri);
          await sourceFile.copy(destFile, { overwrite: true });
          storedUri = destFile.uri;
        }

        setSettings((prev) => {
          if (prev.localUri && prev.localUri !== storedUri) {
            deleteLocalFileUri(prev.localUri);
          }
          const next: WallpaperSettings = {
            ...prev,
            fileName: asset.name || `wallpaper${detected.ext}`,
            fileSize,
            kind: detected.mediaKind,
            localUri: storedUri,
            mimeType: detected.mime,
          };
          persist(next);
          return next;
        });

        return { ok: true };
      } catch (err) {
        return {
          ok: false,
          error: err instanceof Error ? err.message : "导入壁纸失败，请重试。",
        };
      } finally {
        setBusy(false);
      }
    },
    [persist],
  );

  const isWallpaperActive = useMemo(() => {
    if (settings.kind === "none") return false;
    if (settings.kind === "preset") return true;
    return Boolean(settings.localUri);
  }, [settings.kind, settings.localUri]);

  const value = useMemo<WallpaperContextValue>(
    () => ({
      busy,
      clearWallpaper,
      isWallpaperActive,
      loaded,
      pickAndSaveWallpaper,
      selectPreset,
      settings,
      updateWallpaper,
    }),
    [busy, clearWallpaper, isWallpaperActive, loaded, pickAndSaveWallpaper, selectPreset, settings, updateWallpaper],
  );

  return <WallpaperContext.Provider value={value}>{children}</WallpaperContext.Provider>;
}

export function useWallpaper(): WallpaperContextValue {
  return useContext(WallpaperContext);
}
