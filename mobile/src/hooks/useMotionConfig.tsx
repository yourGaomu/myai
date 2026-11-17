import AsyncStorage from "@react-native-async-storage/async-storage";
import * as DocumentPicker from "expo-document-picker";
import { Directory, File, Paths } from "expo-file-system";
import type { ReactNode } from "react";
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { AccessibilityInfo, Platform } from "react-native";

import {
  resolveMotionMedia,
  type CustomMotionSlotConfig,
  type MotionCropPosition,
  type MotionSettings,
  type MotionSlot,
  type ResolvedMotionMedia,
} from "../utils/motionRegistry";

const MOTION_STORAGE_KEY = "myai.motion.v1";
const MAX_MOTION_FILE_BYTES = 8 * 1024 * 1024;

const defaultMotionSettings: MotionSettings = {
  customSlots: {},
  motionEnabled: true,
  reduceMotion: false,
};

type UploadMotionResult = {
  error?: string;
  ok: boolean;
};

type MotionContextValue = {
  busySlot: MotionSlot | null;
  loaded: boolean;
  pickAndSaveSlot: (slot: MotionSlot) => Promise<UploadMotionResult>;
  resetAllSlots: () => void;
  resetSlot: (slot: MotionSlot) => void;
  resolveMedia: (slot: MotionSlot, variant?: "full" | "avatar") => ResolvedMotionMedia;
  settings: MotionSettings;
  systemReduceMotion: boolean;
  updateMotionFlag: (patch: Partial<Pick<MotionSettings, "motionEnabled" | "reduceMotion">>) => void;
  updateSlotAdjustments: (
    slot: MotionSlot,
    patch: Partial<Pick<CustomMotionSlotConfig, "scale" | "offsetY" | "cropPosition">>,
  ) => void;
};

const MotionContext = createContext<MotionContextValue>({
  busySlot: null,
  loaded: true,
  pickAndSaveSlot: async () => ({ ok: false }),
  resetAllSlots: () => {},
  resetSlot: () => {},
  resolveMedia: (slot, variant = "full") => resolveMotionMedia(slot, defaultMotionSettings, variant, false),
  settings: defaultMotionSettings,
  systemReduceMotion: false,
  updateMotionFlag: () => {},
  updateSlotAdjustments: () => {},
});

function detectImageSignature(bytes: Uint8Array, fileName?: string, mimeType?: string): { ext: string; mime: string } | null {
  if (bytes.length >= 6) {
    const header = String.fromCharCode(bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5]);
    if (header === "GIF87a" || header === "GIF89a") {
      return { ext: ".gif", mime: "image/gif" };
    }
  }
  if (bytes.length >= 12) {
    const riff = String.fromCharCode(bytes[0], bytes[1], bytes[2], bytes[3]);
    const webp = String.fromCharCode(bytes[8], bytes[9], bytes[10], bytes[11]);
    if (riff === "RIFF" && webp === "WEBP") {
      return { ext: ".webp", mime: "image/webp" };
    }
  }
  if (bytes.length >= 4 && bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4e && bytes[3] === 0x47) {
    return { ext: ".png", mime: "image/png" };
  }
  const lower = (fileName || "").toLowerCase();
  if (mimeType === "image/gif" || lower.endsWith(".gif")) return { ext: ".gif", mime: "image/gif" };
  if (mimeType === "image/webp" || lower.endsWith(".webp")) return { ext: ".webp", mime: "image/webp" };
  if (mimeType === "image/png" || lower.endsWith(".png")) return { ext: ".png", mime: "image/png" };
  return null;
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

export function MotionProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<MotionSettings>(defaultMotionSettings);
  const [systemReduceMotion, setSystemReduceMotion] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [busySlot, setBusySlot] = useState<MotionSlot | null>(null);

  useEffect(() => {
    void AccessibilityInfo.isReduceMotionEnabled().then(setSystemReduceMotion);
    const sub = AccessibilityInfo.addEventListener("reduceMotionChanged", setSystemReduceMotion);
    return () => sub.remove();
  }, []);

  useEffect(() => {
    let active = true;
    AsyncStorage.getItem(MOTION_STORAGE_KEY)
      .then((raw) => {
        if (!active || !raw) {
          return;
        }
        try {
          const parsed = JSON.parse(raw) as Partial<MotionSettings>;
          const nextSlots: Partial<Record<MotionSlot, CustomMotionSlotConfig>> = {};
          if (parsed.customSlots) {
            for (const [key, item] of Object.entries(parsed.customSlots)) {
              if (!item?.uri) continue;
              // 原生端验证沙盒文件依然存在，防止手动清缓存后吊死
              if (Platform.OS !== "web" && item.uri.startsWith("file://")) {
                try {
                  if (!new File(item.uri).exists) continue;
                } catch {
                  continue;
                }
              }
              nextSlots[key as MotionSlot] = item;
            }
          }
          setSettings({
            customSlots: nextSlots,
            motionEnabled: typeof parsed.motionEnabled === "boolean" ? parsed.motionEnabled : true,
            reduceMotion: typeof parsed.reduceMotion === "boolean" ? parsed.reduceMotion : false,
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

  const persist = useCallback((next: MotionSettings) => {
    void AsyncStorage.setItem(MOTION_STORAGE_KEY, JSON.stringify(next));
  }, []);

  const updateMotionFlag = useCallback(
    (patch: Partial<Pick<MotionSettings, "motionEnabled" | "reduceMotion">>) => {
      setSettings((prev) => {
        const next = { ...prev, ...patch };
        persist(next);
        return next;
      });
    },
    [persist],
  );

  const updateSlotAdjustments = useCallback(
    (
      slot: MotionSlot,
      patch: Partial<Pick<CustomMotionSlotConfig, "scale" | "offsetY" | "cropPosition">>,
    ) => {
      setSettings((prev) => {
        const existing = prev.customSlots[slot];
        if (!existing) return prev;
        const next: MotionSettings = {
          ...prev,
          customSlots: {
            ...prev.customSlots,
            [slot]: {
              ...existing,
              ...patch,
              cropPosition: (patch.cropPosition ?? existing.cropPosition) as MotionCropPosition,
              offsetY: typeof patch.offsetY === "number" ? Math.max(-16, Math.min(16, patch.offsetY)) : existing.offsetY,
              scale: typeof patch.scale === "number" ? Math.max(0.75, Math.min(1.6, Number(patch.scale.toFixed(2)))) : existing.scale,
            },
          },
        };
        persist(next);
        return next;
      });
    },
    [persist],
  );

  const deleteLocalUri = useCallback((uri?: string) => {
    if (!uri || Platform.OS === "web" || !uri.startsWith("file://")) return;
    try {
      const f = new File(uri);
      if (f.exists) {
        f.delete();
      }
    } catch {
      // ignore cleanup error
    }
  }, []);

  const resetSlot = useCallback(
    (slot: MotionSlot) => {
      setSettings((prev) => {
        const current = prev.customSlots[slot];
        if (!current) return prev;
        deleteLocalUri(current.uri);
        const nextSlots = { ...prev.customSlots };
        delete nextSlots[slot];
        const next = { ...prev, customSlots: nextSlots };
        persist(next);
        return next;
      });
    },
    [deleteLocalUri, persist],
  );

  const resetAllSlots = useCallback(() => {
    setSettings((prev) => {
      for (const item of Object.values(prev.customSlots)) {
        deleteLocalUri(item?.uri);
      }
      const next = { ...prev, customSlots: {} };
      persist(next);
      return next;
    });
  }, [deleteLocalUri, persist]);

  const pickAndSaveSlot = useCallback(
    async (slot: MotionSlot): Promise<UploadMotionResult> => {
      setBusySlot(slot);
      try {
        const picked = await DocumentPicker.getDocumentAsync({
          copyToCacheDirectory: true,
          multiple: false,
          type: ["image/gif", "image/webp", "image/png", "image/*"],
        });
        if (picked.canceled || !picked.assets?.[0]) {
          return { ok: false };
        }
        const asset = picked.assets[0];

        let bytes: Uint8Array;
        if (Platform.OS === "web") {
          if (asset.file) {
            bytes = new Uint8Array(await asset.file.arrayBuffer());
          } else {
            const resp = await fetch(asset.uri);
            bytes = new Uint8Array(await resp.arrayBuffer());
          }
        } else {
          bytes = await new File(asset.uri).bytes();
        }

        if (bytes.byteLength === 0) {
          return { ok: false, error: "所选文件为空，请重新选择。" };
        }
        if (bytes.byteLength > MAX_MOTION_FILE_BYTES) {
          return { ok: false, error: "动图文件超过 8MB 上限，建议压缩后再上传以保证手机端流畅度。" };
        }

        const detected = detectImageSignature(bytes, asset.name, asset.mimeType);
        if (!detected) {
          return { ok: false, error: "仅支持 GIF、Animated WebP 或透明 PNG 图片。" };
        }

        let storedUri = "";
        if (Platform.OS === "web") {
          storedUri = `data:${detected.mime};base64,${uint8ToBase64(bytes)}`;
        } else {
          const motionDir = new Directory(Paths.document, "custom-motion");
          motionDir.create({ idempotent: true, intermediates: true });
          const destFile = new File(motionDir, `${slot}-${Date.now()}${detected.ext}`);
          destFile.create({ overwrite: true });
          destFile.write(bytes);
          storedUri = destFile.uri;
        }

        setSettings((prev) => {
          const old = prev.customSlots[slot];
          if (old?.uri && old.uri !== storedUri) {
            deleteLocalUri(old.uri);
          }
          const next: MotionSettings = {
            ...prev,
            customSlots: {
              ...prev.customSlots,
              [slot]: {
                cropPosition: old?.cropPosition ?? "center",
                fileName: asset.name || `${slot}${detected.ext}`,
                mimeType: detected.mime,
                offsetY: old?.offsetY ?? 0,
                scale: old?.scale ?? 1,
                size: bytes.byteLength,
                updatedAt: new Date().toISOString(),
                uri: storedUri,
              },
            },
          };
          persist(next);
          return next;
        });

        return { ok: true };
      } catch (err) {
        return {
          error: err instanceof Error ? err.message : "保存自定义动图失败，请重试。",
          ok: false,
        };
      } finally {
        setBusySlot(null);
      }
    },
    [deleteLocalUri, persist],
  );

  const resolveMedia = useCallback(
    (slot: MotionSlot, variant: "full" | "avatar" = "full") =>
      resolveMotionMedia(slot, settings, variant, systemReduceMotion),
    [settings, systemReduceMotion],
  );

  const value = useMemo<MotionContextValue>(
    () => ({
      busySlot,
      loaded,
      pickAndSaveSlot,
      resetAllSlots,
      resetSlot,
      resolveMedia,
      settings,
      systemReduceMotion,
      updateMotionFlag,
      updateSlotAdjustments,
    }),
    [
      busySlot,
      loaded,
      pickAndSaveSlot,
      resetAllSlots,
      resetSlot,
      resolveMedia,
      settings,
      systemReduceMotion,
      updateMotionFlag,
      updateSlotAdjustments,
    ],
  );

  return <MotionContext.Provider value={value}>{children}</MotionContext.Provider>;
}

export function useMotion(): MotionContextValue {
  return useContext(MotionContext);
}
