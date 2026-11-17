import AsyncStorage from "@react-native-async-storage/async-storage";
import { StatusBar } from "expo-status-bar";
import type { ReactNode } from "react";
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { useColorScheme } from "react-native";

import {
  buildFont,
  buildPalette,
  buildShadow,
  defaultAppearanceSettings,
  type AppearanceSettings,
  type ThemeFont,
  type ThemePalette,
  type ThemeShadow,
} from "../constants/theme";
import { useAndroidNavigationBar } from "../hooks/useAndroidNavigationBar";

const APPEARANCE_STORAGE_KEY = "myai.appearance.v1";

export type AppTheme = AppearanceSettings & {
  font: ThemeFont;
  isDark: boolean;
  palette: ThemePalette;
  shadow: ThemeShadow;
};

export type ThemeContextValue = AppTheme & {
  loaded: boolean;
  resetAppearance: () => void;
  updateAppearance: (patch: Partial<AppearanceSettings>) => void;
};

const fallbackTheme: AppTheme = {
  ...defaultAppearanceSettings,
  font: buildFont(defaultAppearanceSettings.readingFontSize),
  isDark: true,
  palette: buildPalette(true, defaultAppearanceSettings.accent, defaultAppearanceSettings.highlightUserBubble),
  shadow: buildShadow(true, "#3CCBE8"),
};

const ThemeContext = createContext<ThemeContextValue>({
  ...fallbackTheme,
  loaded: true,
  resetAppearance: () => {},
  updateAppearance: () => {},
});

export function ThemeProvider({ children }: { children: ReactNode }) {
  const systemScheme = useColorScheme();
  const [settings, setSettings] = useState<AppearanceSettings>(defaultAppearanceSettings);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let active = true;
    AsyncStorage.getItem(APPEARANCE_STORAGE_KEY)
      .then((raw) => {
        if (!active || !raw) {
          return;
        }
        try {
          const parsed = JSON.parse(raw) as Partial<AppearanceSettings>;
          setSettings((prev) => ({
            accent:
              parsed.accent === "cyan" || parsed.accent === "blue" || parsed.accent === "mint" || parsed.accent === "amber"
                ? parsed.accent
                : prev.accent,
            highlightUserBubble: typeof parsed.highlightUserBubble === "boolean" ? parsed.highlightUserBubble : prev.highlightUserBubble,
            keepThinkingExpanded: typeof parsed.keepThinkingExpanded === "boolean" ? parsed.keepThinkingExpanded : prev.keepThinkingExpanded,
            readingFontSize:
              typeof parsed.readingFontSize === "number" && Number.isFinite(parsed.readingFontSize)
                ? Math.max(14, Math.min(18, Math.round(parsed.readingFontSize)))
                : prev.readingFontSize,
            themeMode:
              parsed.themeMode === "dark" || parsed.themeMode === "light" || parsed.themeMode === "system"
                ? parsed.themeMode
                : prev.themeMode,
          }));
        } catch {
          // ignore corrupted appearance cache
        }
      })
      .finally(() => {
        if (active) {
          setLoaded(true);
        }
      });
    return () => {
      active = false;
    };
  }, []);

  const persist = useCallback((next: AppearanceSettings) => {
    void AsyncStorage.setItem(APPEARANCE_STORAGE_KEY, JSON.stringify(next));
  }, []);

  const updateAppearance = useCallback(
    (patch: Partial<AppearanceSettings>) => {
      setSettings((prev) => {
        const next: AppearanceSettings = {
          ...prev,
          ...patch,
          readingFontSize:
            typeof patch.readingFontSize === "number"
              ? Math.max(14, Math.min(18, Math.round(patch.readingFontSize)))
              : prev.readingFontSize,
        };
        persist(next);
        return next;
      });
    },
    [persist],
  );

  const resetAppearance = useCallback(() => {
    setSettings(defaultAppearanceSettings);
    persist(defaultAppearanceSettings);
  }, [persist]);

  const isDark = settings.themeMode === "system" ? systemScheme !== "light" : settings.themeMode === "dark";

  useAndroidNavigationBar(isDark);

  const value = useMemo<ThemeContextValue>(() => {
    const nextPalette = buildPalette(isDark, settings.accent, settings.highlightUserBubble);
    return {
      ...settings,
      font: buildFont(settings.readingFontSize),
      isDark,
      loaded,
      palette: nextPalette,
      resetAppearance,
      shadow: buildShadow(isDark, nextPalette.brand),
      updateAppearance,
    };
  }, [isDark, loaded, resetAppearance, settings, updateAppearance]);

  return (
    <ThemeContext.Provider value={value}>
      <StatusBar style={isDark ? "light" : "dark"} />
      {children}
    </ThemeContext.Provider>
  );
}

export function useAppTheme(): ThemeContextValue {
  return useContext(ThemeContext);
}

export function useThemedStyles<T>(factory: (theme: ThemeContextValue) => T): T {
  const theme = useAppTheme();
  return useMemo(() => factory(theme), [factory, theme]);
}
