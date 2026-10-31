// 设计 token 单一来源。所有组件禁止出现颜色字面量，必须引用这里的值。
// 命名约定：语义名，不含具体颜色（不要出现 yellow/black 之类的名字，否则换主题时没法用）。

export const palette = {
  // —— 中性色阶（深色画布，由深到浅）
  canvas: "#08080A", // 最底层背景
  surface1: "#101014", // 一级容器（面板、列表底）
  surface2: "#17171B", // 二级容器（卡片、代码块）
  surface3: "#1C1C22", // 三级容器（浮起的元素）
  surface4: "#232329", // 输入框内层、按下态

  // —— 描边（深色底下必须用透明度，不能用实色）
  hairline: "rgba(255,255,255,0.07)", // 默认分隔线 / 1px 边框
  hairlineStrong: "rgba(255,255,255,0.12)", // 需要强调的边框
  hairlineSoft: "rgba(255,255,255,0.045)", // 极弱分隔

  // —— 文字（三级层次，实测对比度见文档 2.3）
  ink1: "#F2F0EC", // 主文字（正文、标题）
  ink2: "#9C978D", // 次文字（说明、副标题）
  ink3: "#8A8479", // 弱文字（元信息、时间戳）
  ink4: "#6B6660", // 极弱（占位符、禁用态）。禁止用于正文
  inkInverse: "#241F14", // 放在亮色块上的文字（品牌黄底上）

  // —— 品牌色（暖黄，唯一强调色）
  brand: "#F6D45C",
  brandLight: "#F9DC72", // 渐变亮端
  brandDeep: "#E9B93F", // 渐变暗端（按钮、头像）
  brandDark: "#D9A62F", // 头像渐变暗端
  brandWash: "rgba(246,212,92,0.10)", // 淡底（选中标签背景）
  brandWashStrong: "rgba(246,212,92,0.16)",
  brandCode: "rgba(246,212,92,0.11)", // 行内代码底
  brandGlow: "rgba(246,212,92,0.55)", // 光晕

  // —— 状态色（低饱和，深色底上不刺眼）
  ok: "#4ADE80",
  okWash: "rgba(74,222,128,0.13)",
  okBorder: "rgba(74,222,128,0.3)",
  warn: "#F0CE68",
  warnWash: "rgba(240,206,104,0.13)",
  err: "#E08D7D",
  errWash: "rgba(224,141,125,0.13)",
  errBorder: "rgba(224,141,125,0.32)",
  info: "#7FD1A8",
  infoWash: "rgba(127,209,168,0.12)",
  cyan: "#5BC8DE",
  cyanWash: "rgba(91,200,222,0.12)",

  // —— diff 专用（深色底红绿，比通用状态色更亮，保证代码可读）
  diffAdd: "#8FD8B4",
  diffAddBg: "rgba(127,209,168,0.09)",
  diffDel: "#E08D7D",
  diffDelBg: "rgba(224,141,125,0.10)",

  // —— 玻璃拟态（BlurView fallback 与 tint 用）
  glassTint: "rgba(24,24,29,0.72)",
  glassTintStrong: "rgba(28,28,34,0.82)",
  glassTintSoft: "rgba(22,22,27,0.80)",
  glassEdge: "rgba(255,255,255,0.09)", // 玻璃上边缘高光
  glassInner: "rgba(255,255,255,0.05)", // 内描边高光

  // —— 遮罩
  scrim: "rgba(4,4,6,0.72)", // 抽屉/弹窗背后的遮罩
  scrimHeavy: "rgba(4,4,6,0.92)", // 全屏图片查看

  // —— 纯色（仅限必须的场合，优先用上面语义色）
  transparent: "transparent",

  // —— 下凹区域（工具结果体、代码块内部、diff 区域）
  recessed: "rgba(0,0,0,0.22)",

  // —— 顶部环境光（MobileScreenShell 背景光晕）
  ambientGlow: "rgba(246,212,92,0.07)",
} as const;

// 圆角
export const radius = {
  xxs: 4, // 极小圆角（用于小徽章）
  xs: 6,
  sm: 9,
  md: 12,
  lg: 16,
  xl: 20,
  pill: 999,
} as const;

// 间距（4 的倍数）
export const space = {
  xs: 4,
  sm: 6,
  md: 8,
  lg: 12,
  xl: 16,
  xxl: 20,
  xxxl: 26,
} as const;

// 字号（21 档 → 7 档）
export const font = {
  micro: 10, // 徽章、极小标签
  caption: 11.5, // 元信息、时间戳
  label: 12.5, // 按钮文字、列表副标题
  body: 14, // 正文
  bodyLg: 15, // 强调正文
  title: 17, // 区块标题
  display: 22, // 页面主标题
} as const;

// 行高倍率（正文必须 >= 字号的 1.6 倍，保证中文可读）
export const leading = {
  tight: 1.25,
  normal: 1.45,
  relaxed: 1.68,
} as const;

// 字重（RN 只接受 100 的整数倍字符串）
export const weight = {
  regular: "400",
  medium: "500",
  semibold: "600",
  bold: "700",
  heavy: "900", // 仅限：主标题、徽章数字。正文禁止使用
} as const;

// 阴影：深色主题下层次靠"表面色差 + 1px 极淡边框"，不靠强阴影。
// 只在浮起元素（浮岛、输入栏、标签栏、Modal）使用，且必须是柔和的。
export const shadow = {
  float: {
    shadowColor: "#000000",
    shadowOffset: { width: 0, height: 10 },
    shadowOpacity: 0.45,
    shadowRadius: 18,
    elevation: 12,
  },
  card: {
    shadowColor: "#000000",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.28,
    shadowRadius: 6,
    elevation: 3,
  },
  // Android 不支持彩色阴影，elevation 给 0，靠边框与渐变本身表现
  brandGlow: {
    shadowColor: palette.brand,
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.5,
    shadowRadius: 10,
    elevation: 0,
  },
  none: {
    shadowColor: "transparent",
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0,
    shadowRadius: 0,
    elevation: 0,
  },
} as const;

// 玻璃拟态参数：Android 上 BlurView 效果弱，用高不透明度实色近似。
export const glass = {
  intensity: 24,
  tint: palette.glassTint,
  edge: palette.glassEdge,
} as const;

// 兼容旧命名：原 theme.ts 的 darkTheme 已废弃。这里导出会抛错的占位，
// 让任何遗漏的 import 在运行期立刻暴露，而不是静默显示错误颜色。
/** @deprecated 已废弃，请使用 palette / radius / font / weight / shadow */
export const darkTheme = new Proxy({} as Record<string, never>, {
  get(_t, prop) {
    throw new Error(
      `darkTheme.${String(prop)} 已废弃。请改用 palette / radius / font / weight / shadow。`,
    );
  },
});

// 将来支持浅色主题时，在这里定义 lightPalette，并按同样结构提供 lightRadius 等。
// 组件侧目前直接从本文件解构导入；将来换主题时再统一替换为 context。
