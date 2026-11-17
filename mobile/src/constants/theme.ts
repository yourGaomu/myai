import { Platform } from "react-native";

// 设计 token 单一来源。静态导出保留默认值以兼容未迁移组件，动态主题请通过 useAppTheme() 获取。

export type ThemeMode = "dark" | "light" | "system";
export type AccentPreset = "cyan" | "blue" | "mint" | "amber";

export type AppearanceSettings = {
  accent: AccentPreset;
  highlightUserBubble: boolean;
  keepThinkingExpanded: boolean;
  readingFontSize: number;
  themeMode: ThemeMode;
};

export const defaultAppearanceSettings: AppearanceSettings = {
  accent: "cyan",
  highlightUserBubble: false,
  keepThinkingExpanded: false,
  readingFontSize: 16,
  themeMode: "dark",
};

export const accentOptions: Array<{
  description: string;
  key: AccentPreset;
  label: string;
  swatchDark: string;
  swatchLight: string;
}> = [
  { key: "cyan", label: "冰川青", description: "MyAI 默认发色灵感", swatchDark: "#3CCBE8", swatchLight: "#0891B2" },
  { key: "blue", label: "深空蓝", description: "冷静沉稳工程感", swatchDark: "#5B9BFF", swatchLight: "#2563EB" },
  { key: "mint", label: "薄荷绿", description: "轻盈低疲劳阅读", swatchDark: "#38D399", swatchLight: "#0D9488" },
  { key: "amber", label: "琥珀金", description: "暖调高对比聚焦", swatchDark: "#F2B84B", swatchLight: "#D97706" },
];

type AccentTokens = {
  brand: string;
  brandLight: string;
  brandDeep: string;
  brandDark: string;
  brandWash: string;
  brandWashStrong: string;
  brandGlow: string;
  inkInverse: string;
  cyan: string;
  cyanWash: string;
  ambientGlow: string;
};

const darkAccentMap: Record<AccentPreset, AccentTokens> = {
  cyan: {
    brand: "#3CCBE8",
    brandLight: "#67D6EE",
    brandDeep: "rgba(60,203,232,0.42)",
    brandDark: "#2AA9C4",
    brandWash: "rgba(60,203,232,0.14)",
    brandWashStrong: "rgba(60,203,232,0.20)",
    brandGlow: "rgba(60,203,232,0.35)",
    inkInverse: "#03232C",
    cyan: "#67D6EE",
    cyanWash: "rgba(60,203,232,0.14)",
    ambientGlow: "rgba(60,203,232,0.14)",
  },
  blue: {
    brand: "#5B9BFF",
    brandLight: "#84B6FF",
    brandDeep: "rgba(91,155,255,0.44)",
    brandDark: "#3B7BE3",
    brandWash: "rgba(91,155,255,0.15)",
    brandWashStrong: "rgba(91,155,255,0.22)",
    brandGlow: "rgba(91,155,255,0.34)",
    inkInverse: "#061D42",
    cyan: "#84B6FF",
    cyanWash: "rgba(91,155,255,0.15)",
    ambientGlow: "rgba(91,155,255,0.14)",
  },
  mint: {
    brand: "#38D399",
    brandLight: "#6AE2B6",
    brandDeep: "rgba(56,211,153,0.42)",
    brandDark: "#1FA977",
    brandWash: "rgba(56,211,153,0.14)",
    brandWashStrong: "rgba(56,211,153,0.20)",
    brandGlow: "rgba(56,211,153,0.32)",
    inkInverse: "#052B1E",
    cyan: "#6AE2B6",
    cyanWash: "rgba(56,211,153,0.14)",
    ambientGlow: "rgba(56,211,153,0.14)",
  },
  amber: {
    brand: "#F2B84B",
    brandLight: "#F7CF79",
    brandDeep: "rgba(242,184,75,0.45)",
    brandDark: "#CE9324",
    brandWash: "rgba(242,184,75,0.14)",
    brandWashStrong: "rgba(242,184,75,0.22)",
    brandGlow: "rgba(242,184,75,0.32)",
    inkInverse: "#2B1B03",
    cyan: "#F7CF79",
    cyanWash: "rgba(242,184,75,0.14)",
    ambientGlow: "rgba(242,184,75,0.14)",
  },
};

const lightAccentMap: Record<AccentPreset, AccentTokens> = {
  cyan: {
    brand: "#0891B2",
    brandLight: "#0E7490",
    brandDeep: "rgba(8,145,178,0.34)",
    brandDark: "#155E75",
    brandWash: "rgba(8,145,178,0.10)",
    brandWashStrong: "rgba(8,145,178,0.16)",
    brandGlow: "rgba(8,145,178,0.22)",
    inkInverse: "#FFFFFF",
    cyan: "#0E7490",
    cyanWash: "rgba(8,145,178,0.10)",
    ambientGlow: "rgba(8,145,178,0.10)",
  },
  blue: {
    brand: "#2563EB",
    brandLight: "#1D4ED8",
    brandDeep: "rgba(37,99,235,0.34)",
    brandDark: "#1E40AF",
    brandWash: "rgba(37,99,235,0.10)",
    brandWashStrong: "rgba(37,99,235,0.16)",
    brandGlow: "rgba(37,99,235,0.22)",
    inkInverse: "#FFFFFF",
    cyan: "#1D4ED8",
    cyanWash: "rgba(37,99,235,0.10)",
    ambientGlow: "rgba(37,99,235,0.10)",
  },
  mint: {
    brand: "#0D9488",
    brandLight: "#0F766E",
    brandDeep: "rgba(13,148,136,0.34)",
    brandDark: "#115E59",
    brandWash: "rgba(13,148,136,0.10)",
    brandWashStrong: "rgba(13,148,136,0.16)",
    brandGlow: "rgba(13,148,136,0.22)",
    inkInverse: "#FFFFFF",
    cyan: "#0F766E",
    cyanWash: "rgba(13,148,136,0.10)",
    ambientGlow: "rgba(13,148,136,0.10)",
  },
  amber: {
    brand: "#D97706",
    brandLight: "#B45309",
    brandDeep: "rgba(217,119,6,0.36)",
    brandDark: "#92400E",
    brandWash: "rgba(217,119,6,0.11)",
    brandWashStrong: "rgba(217,119,6,0.17)",
    brandGlow: "rgba(217,119,6,0.22)",
    inkInverse: "#FFFFFF",
    cyan: "#B45309",
    cyanWash: "rgba(217,119,6,0.11)",
    ambientGlow: "rgba(217,119,6,0.10)",
  },
};

const darkBasePalette = {
  canvas: "#0F1114",
  surface1: "#171A1F",
  surface2: "#1A1D23",
  surface3: "#23272E",
  surface4: "#383D47",
  bubble: "#23272E",
  well: "#23272E",

  hairline: "rgba(255,255,255,0.075)",
  hairlineStrong: "rgba(255,255,255,0.14)",
  hairlineSoft: "rgba(255,255,255,0.05)",
  pressed: "rgba(255,255,255,0.06)",

  ink1: "#ECEEF1",
  ink2: "#A3A9B4",
  ink3: "#868D99",
  ink4: "#5E6573",

  brandCode: "#23272E",

  ok: "#5CD69B",
  okWash: "rgba(92,214,155,0.12)",
  okBorder: "rgba(92,214,155,0.30)",
  warn: "#F2C94C",
  warnWash: "rgba(242,201,76,0.13)",
  err: "#FF8A75",
  errWash: "rgba(255,138,117,0.12)",
  errBorder: "rgba(255,138,117,0.32)",
  info: "#8FB8FF",
  infoWash: "rgba(143,184,255,0.12)",

  diffAdd: "#5CD69B",
  diffAddBg: "rgba(92,214,155,0.10)",
  diffDel: "#FF8A75",
  diffDelBg: "rgba(255,138,117,0.10)",

  glassTint: "#1A1D23",
  glassTintStrong: "#1A1D23",
  glassTintSoft: "#171A1F",
  glassEdge: "rgba(255,255,255,0.075)",
  glassInner: "rgba(255,255,255,0.05)",
  floatingGlass: "rgba(23,26,31,0.82)",
  floatingGlassBorder: "rgba(255,255,255,0.11)",

  scrim: "rgba(0,0,0,0.55)",
  scrimHeavy: "rgba(4,4,6,0.92)",
  transparent: "transparent",
  recessed: "#23272E",
};

const lightBasePalette = {
  canvas: "#F5F7FA",
  surface1: "#FFFFFF",
  surface2: "#EEF2F6",
  surface3: "#E3E9F0",
  surface4: "#CED8E5",
  bubble: "#E8EEF5",
  well: "#EAEFF5",

  hairline: "rgba(15,23,42,0.08)",
  hairlineStrong: "rgba(15,23,42,0.16)",
  hairlineSoft: "rgba(15,23,42,0.05)",
  pressed: "rgba(15,23,42,0.05)",

  ink1: "#11161D",
  ink2: "#4B5565",
  ink3: "#687486",
  ink4: "#94A3B8",

  brandCode: "#E6ECF3",

  ok: "#139A5C",
  okWash: "rgba(19,154,92,0.10)",
  okBorder: "rgba(19,154,92,0.28)",
  warn: "#C4840D",
  warnWash: "rgba(196,132,13,0.12)",
  err: "#D94B38",
  errWash: "rgba(217,75,56,0.10)",
  errBorder: "rgba(217,75,56,0.28)",
  info: "#2563EB",
  infoWash: "rgba(37,99,235,0.10)",

  diffAdd: "#139A5C",
  diffAddBg: "rgba(19,154,92,0.10)",
  diffDel: "#D94B38",
  diffDelBg: "rgba(217,75,56,0.10)",

  glassTint: "#FFFFFF",
  glassTintStrong: "#FFFFFF",
  glassTintSoft: "#F8FAFC",
  glassEdge: "rgba(15,23,42,0.08)",
  glassInner: "rgba(255,255,255,0.75)",
  floatingGlass: "rgba(255,255,255,0.88)",
  floatingGlassBorder: "rgba(15,23,42,0.10)",

  scrim: "rgba(15,23,42,0.42)",
  scrimHeavy: "rgba(15,23,42,0.86)",
  transparent: "transparent",
  recessed: "#E8EEF4",
};

export type ThemePalette = typeof darkBasePalette & AccentTokens & {
  userBubbleBg: string;
  userBubbleBorder: string;
};

export function buildPalette(isDark: boolean, accent: AccentPreset, highlightUserBubble: boolean): ThemePalette {
  const base = isDark ? darkBasePalette : lightBasePalette;
  const accentTokens = (isDark ? darkAccentMap : lightAccentMap)[accent] ?? (isDark ? darkAccentMap : lightAccentMap).cyan;
  return {
    ...base,
    ...accentTokens,
    userBubbleBg: highlightUserBubble ? accentTokens.brandWashStrong : base.bubble,
    userBubbleBorder: highlightUserBubble ? accentTokens.brandDeep : base.transparent,
  };
}

export const palette: ThemePalette = buildPalette(true, "cyan", false);

// 圆角
export const radius = {
  xxs: 4,
  xs: 6,
  sm: 10,
  md: 14,
  lg: 16,
  xl: 20,
  xxl: 24,
  sheet: 26,
  pill: 999,
} as const;

// 间距（4 的倍数为主）
export const space = {
  xs: 4,
  sm: 6,
  md: 8,
  lg: 12,
  xl: 16,
  xxl: 20,
  xxxl: 26,
} as const;

export function buildFont(readingSize = 16) {
  const delta = Math.max(-2, Math.min(2, readingSize - 16));
  return {
    micro: 11,
    caption: 12.5 + delta * 0.25,
    label: 13.5 + delta * 0.5,
    body: 15 + delta * 0.75,
    bodyLg: 16 + delta * 0.75,
    reading: 16 + delta,
    title: 17,
    display: 22,
    large: 28,
  };
}

export type ThemeFont = ReturnType<typeof buildFont>;

export const font = buildFont(16);

export const leading = {
  tight: 1.25,
  normal: 1.45,
  relaxed: 1.68,
} as const;

export const weight = {
  regular: "400",
  medium: "500",
  semibold: "600",
  bold: "700",
  heavy: "900",
} as const;

export const fontFamily = {
  mono: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }) as string,
} as const;

export function buildShadow(isDark: boolean, brandColor: string) {
  return {
    float: {
      shadowColor: "#000000",
      shadowOffset: { width: 0, height: 12 },
      shadowOpacity: isDark ? 0.45 : 0.14,
      shadowRadius: isDark ? 24 : 20,
      elevation: isDark ? 12 : 8,
    },
    card: {
      shadowColor: "#000000",
      shadowOffset: { width: 0, height: 2 },
      shadowOpacity: isDark ? 0.22 : 0.08,
      shadowRadius: 6,
      elevation: 2,
    },
    brandGlow: {
      shadowColor: brandColor,
      shadowOffset: { width: 0, height: 10 },
      shadowOpacity: isDark ? 0.3 : 0.2,
      shadowRadius: 22,
      elevation: 0,
    },
    none: {
      shadowColor: "transparent",
      shadowOffset: { width: 0, height: 0 },
      shadowOpacity: 0,
      shadowRadius: 0,
      elevation: 0,
    },
  };
}

export type ThemeShadow = ReturnType<typeof buildShadow>;

export const shadow = buildShadow(true, palette.brand);

export const glass = {
  intensity: 24,
  tint: palette.glassTint,
  edge: palette.glassEdge,
} as const;

/** @deprecated 已废弃，请使用 useAppTheme() 或 palette / radius / font / weight / shadow */
export const darkTheme = new Proxy({} as Record<string, never>, {
  get(_t, prop) {
    throw new Error(
      `darkTheme.${String(prop)} 已废弃。请改用 useAppTheme() 或 palette / radius / font / weight / shadow。`,
    );
  },
});
