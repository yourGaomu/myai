import { Platform } from "react-native";

// 设计 token 单一来源。所有组件禁止出现颜色字面量，必须引用这里的值。
// 命名约定：语义名，不含具体颜色（不要出现 yellow/black 之类的名字，否则换主题时没法用）。
// 当前主题：石墨深色画布 + 取自角色发色的青色强调。强调色只用于主操作、选中态和链接。

export const palette = {
  // —— 中性色阶（深色画布，由深到浅）
  canvas: "#0F1114", // 最底层背景
  surface1: "#171A1F", // 一级容器（卡片、列表分组）
  surface2: "#1A1D23", // 二级容器（底部弹层、侧栏、浮起的卡片）
  surface3: "#23272E", // 三级容器（凹槽、用户气泡、次要按钮）
  surface4: "#383D47", // 分段控件选中块、按下态
  bubble: "#23272E", // 用户消息气泡
  well: "#23272E", // 凹槽：分段控件底、代码块、输入框内层

  // —— 描边（深色底下必须用透明度，不能用实色）
  hairline: "rgba(255,255,255,0.075)", // 默认分隔线 / 1px 边框
  hairlineStrong: "rgba(255,255,255,0.14)", // 需要强调的边框（输入框外框）
  hairlineSoft: "rgba(255,255,255,0.05)", // 极弱分隔
  pressed: "rgba(255,255,255,0.06)", // 行按下反馈

  // —— 文字（四级层次）
  ink1: "#ECEEF1", // 主文字（正文、标题）
  ink2: "#A3A9B4", // 次文字（说明、副标题）
  ink3: "#868D99", // 弱文字（元信息、时间戳）
  ink4: "#5E6573", // 极弱（占位符、禁用态）。禁止用于正文
  inkInverse: "#03232C", // 放在强调色填充上的文字

  // —— 品牌色（青色，唯一强调色）
  brand: "#3CCBE8", // 填充：发送按钮、主按钮、进度
  brandLight: "#67D6EE", // 文字用强调色：链接、选中文字、运行中状态
  brandDeep: "rgba(60,203,232,0.42)", // 强调描边
  brandDark: "#2AA9C4", // 头像光圈暗端
  brandWash: "rgba(60,203,232,0.14)", // 淡底（选中标签背景）
  brandWashStrong: "rgba(60,203,232,0.20)",
  brandCode: "#23272E", // 行内代码底
  brandGlow: "rgba(60,203,232,0.35)", // 光晕

  // —— 状态色（取自角色配饰：黄色发珠、珊瑚色星星）
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
  // 运行中 / 进行中，与强调色同色系
  cyan: "#67D6EE",
  cyanWash: "rgba(60,203,232,0.14)",

  // —— diff 专用：只给底色和 +/− 号上色，正文保持主文字色
  diffAdd: "#5CD69B",
  diffAddBg: "rgba(92,214,155,0.10)",
  diffDel: "#FF8A75",
  diffDelBg: "rgba(255,138,117,0.10)",

  // —— 浮层底色（旧名保留：原为毛玻璃，现统一为不透明面）
  glassTint: "#1A1D23",
  glassTintStrong: "#1A1D23",
  glassTintSoft: "#171A1F",
  glassEdge: "rgba(255,255,255,0.075)",
  glassInner: "rgba(255,255,255,0.05)",

  // —— 遮罩
  scrim: "rgba(0,0,0,0.55)", // 抽屉/弹窗背后的遮罩
  scrimHeavy: "rgba(4,4,6,0.92)", // 全屏图片查看

  transparent: "transparent",

  // —— 下凹区域（工具结果体、代码块内部）
  recessed: "#23272E",

  // —— 角色插画背后的柔光
  ambientGlow: "rgba(60,203,232,0.14)",
} as const;

// 圆角
export const radius = {
  xxs: 4,
  xs: 6,
  sm: 10,
  md: 14,
  lg: 16,
  xl: 20,
  xxl: 24, // 输入框外框
  sheet: 26, // 底部弹层、侧栏
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

// 字号：正文 16 起步，元信息不低于 12
export const font = {
  micro: 11,
  caption: 12.5,
  label: 13.5,
  body: 15, // 列表行、按钮
  bodyLg: 16,
  reading: 16, // 对话正文
  title: 17, // 顶栏标题、弹层标题
  display: 22, // 空状态问候
  large: 28, // 设置等大标题页
} as const;

// 行高倍率（中文正文 >= 1.6）
export const leading = {
  tight: 1.25,
  normal: 1.45,
  relaxed: 1.68,
} as const;

// 字重：常规 400、强调 500、标题 600。700 只用于数字和极少数强调
export const weight = {
  regular: "400",
  medium: "500",
  semibold: "600",
  bold: "700",
  heavy: "900", // 仅限：主标题、徽章数字。正文禁止使用
} as const;

export const fontFamily = {
  mono: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }) as string,
} as const;

// 阴影：深色主题下层次靠表面色差，阴影只给浮起元素（输入框、弹层、侧栏）。
export const shadow = {
  float: {
    shadowColor: "#000000",
    shadowOffset: { width: 0, height: 12 },
    shadowOpacity: 0.45,
    shadowRadius: 24,
    elevation: 12,
  },
  card: {
    shadowColor: "#000000",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.22,
    shadowRadius: 6,
    elevation: 2,
  },
  // Android 不支持彩色阴影，elevation 给 0
  brandGlow: {
    shadowColor: palette.brand,
    shadowOffset: { width: 0, height: 10 },
    shadowOpacity: 0.3,
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
} as const;

// 旧的毛玻璃参数，保留导出以兼容未迁移的组件。
export const glass = {
  intensity: 24,
  tint: palette.glassTint,
  edge: palette.glassEdge,
} as const;

/** @deprecated 已废弃，请使用 palette / radius / font / weight / shadow */
export const darkTheme = new Proxy({} as Record<string, never>, {
  get(_t, prop) {
    throw new Error(
      `darkTheme.${String(prop)} 已废弃。请改用 palette / radius / font / weight / shadow。`,
    );
  },
});
