import type { ImageSource } from "expo-image";

import type { AgentActivity } from "./agentActivity";

export type MotionSlot =
  | "startup"
  | "idle"
  | "thinking"
  | "working"
  | "subagent"
  | "attention"
  | "done";

export type ExtendedAgentActivity = AgentActivity | "subagent" | "done";

export type MotionCropPosition = "center" | "top" | "bottom";

export type CustomMotionSlotConfig = {
  cropPosition: MotionCropPosition;
  fileName: string;
  mimeType: string;
  offsetY: number;
  scale: number;
  size: number;
  updatedAt: string;
  uri: string;
};

export type MotionSettings = {
  customSlots: Partial<Record<MotionSlot, CustomMotionSlotConfig>>;
  motionEnabled: boolean;
  reduceMotion: boolean;
};

export type MotionSlotSpec = {
  description: string;
  fallbackOrder: MotionSlot[];
  key: MotionSlot;
  label: string;
  recommendedSize: string;
};

export const motionSlotSpecs: MotionSlotSpec[] = [
  {
    key: "startup",
    label: "启动加载动画",
    description: "App 打开过渡与初始化等待时展示的主角色动效",
    recommendedSize: "竖版透明 GIF/WebP · 建议 ≤ 8MB",
    fallbackOrder: ["startup", "working", "thinking", "idle"],
  },
  {
    key: "idle",
    label: "待机与主页形象",
    description: "新对话首页及顶栏已就绪静息状态的展示形象",
    recommendedSize: "透明背景 GIF/WebP/PNG · 建议 ≤ 5MB",
    fallbackOrder: ["idle", "done", "startup"],
  },
  {
    key: "thinking",
    label: "深度思考与推理",
    description: "模型处于推理（Reasoning）与规划生成阶段",
    recommendedSize: "建议透明背景循环 GIF · 建议 ≤ 6MB",
    fallbackOrder: ["thinking", "working", "startup"],
  },
  {
    key: "working",
    label: "工具调用与执行",
    description: "读取代码、运行终端命令、执行计划或上传资源中",
    recommendedSize: "建议透明背景循环 GIF · 建议 ≤ 6MB",
    fallbackOrder: ["working", "thinking", "startup"],
  },
  {
    key: "subagent",
    label: "子代理并行调度",
    description: "存在后台子代理并发检索或协同执行时展示",
    recommendedSize: "建议透明背景循环 GIF · 建议 ≤ 6MB",
    fallbackOrder: ["subagent", "working", "thinking"],
  },
  {
    key: "attention",
    label: "等待授权与提问确认",
    description: "出现危险工具授权或 AskUserQuestion 等待响应时",
    recommendedSize: "建议透明背景提示 GIF · 建议 ≤ 6MB",
    fallbackOrder: ["attention", "thinking", "idle"],
  },
  {
    key: "done",
    label: "完成与收尾反馈",
    description: "任务刚刚成功结束或离线收尾态",
    recommendedSize: "轻量静态或柔和动效 · 建议 ≤ 5MB",
    fallbackOrder: ["done", "idle", "working"],
  },
];

// 内置静态与透明 WebP 资产表（Expo Metro 通过静态 require 打包）
export const builtinMotionAssets = {
  startupFull: require("../../assets/motion/startup.webp") as ImageSource,
  workingFull: require("../../assets/motion/working.webp") as ImageSource,
  thinkingFull: require("../../assets/motion/thinking.webp") as ImageSource,
  workingAvatar: require("../../assets/motion/avatar-working.webp") as ImageSource,
  thinkingAvatar: require("../../assets/motion/avatar-thinking.webp") as ImageSource,
  staticHero: require("../../assets/hero-character.png") as ImageSource,
  staticAvatar: require("../../assets/avatar.png") as ImageSource,
} as const;

export function activityToMotionSlot(activity: ExtendedAgentActivity): MotionSlot {
  switch (activity) {
    case "thinking":
      return "thinking";
    case "tool":
    case "uploading":
      return "working";
    case "subagent":
      return "subagent";
    case "permission":
      return "attention";
    case "done":
      return "done";
    case "offline":
    case "idle":
    default:
      return "idle";
  }
}

export function activityStatusLabel(activity: ExtendedAgentActivity, statusText?: string): string {
  switch (activity) {
    case "thinking":
      return "深度思考";
    case "tool":
      return "执行工具";
    case "uploading":
      return "上传资源";
    case "subagent":
      return "并行调度";
    case "permission":
      return "等待确认";
    case "done":
      return "已完成";
    case "offline":
      return statusText || "离线";
    case "idle":
    default:
      return "已就绪";
  }
}

export function defaultBuiltinSource(
  slot: MotionSlot,
  variant: "full" | "avatar",
  prefersStatic: boolean,
): ImageSource | number {
  if (prefersStatic) {
    return variant === "avatar" ? builtinMotionAssets.staticAvatar : builtinMotionAssets.staticHero;
  }
  switch (slot) {
    case "startup":
      return variant === "avatar" ? builtinMotionAssets.workingAvatar : builtinMotionAssets.startupFull;
    case "thinking":
    case "attention":
      return variant === "avatar" ? builtinMotionAssets.thinkingAvatar : builtinMotionAssets.thinkingFull;
    case "working":
    case "subagent":
      return variant === "avatar" ? builtinMotionAssets.workingAvatar : builtinMotionAssets.workingFull;
    case "idle":
    case "done":
    default:
      return variant === "avatar" ? builtinMotionAssets.staticAvatar : builtinMotionAssets.staticHero;
  }
}

export type ResolvedMotionMedia = {
  cropPosition: MotionCropPosition;
  customConfig?: CustomMotionSlotConfig;
  isCustom: boolean;
  offsetY: number;
  scale: number;
  source: ImageSource | number | string;
};

export function resolveMotionMedia(
  slot: MotionSlot,
  settings: MotionSettings,
  variant: "full" | "avatar",
  systemReduceMotion: boolean,
): ResolvedMotionMedia {
  const prefersStatic = !settings.motionEnabled || settings.reduceMotion || systemReduceMotion;
  const spec = motionSlotSpecs.find((item) => item.key === slot) ?? motionSlotSpecs[1];

  // 先在精确槽位与继承链上查找用户自定义上传 GIF
  for (const candidate of spec.fallbackOrder) {
    const custom = settings.customSlots[candidate];
    if (custom?.uri) {
      return {
        cropPosition: custom.cropPosition,
        customConfig: custom,
        isCustom: true,
        offsetY: custom.offsetY,
        scale: custom.scale,
        source: custom.uri,
      };
    }
  }

  return {
    cropPosition: "center",
    isCustom: false,
    offsetY: 0,
    scale: 1,
    source: defaultBuiltinSource(slot, variant, prefersStatic),
  };
}
