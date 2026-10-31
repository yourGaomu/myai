import { ActivityIndicator, Image, Pressable, StyleSheet, Text, View } from "react-native";
import { useEffect, useState } from "react";
import { Feather } from "@expo/vector-icons";
import { BlurView } from "expo-blur";
import { LinearGradient } from "expo-linear-gradient";

import type { SessionAgentMode, ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import type { AgentActivity } from "../../utils/agentActivity";
import { font, glass, palette, radius, shadow, space, weight } from "../../constants/theme";

type Props = {
  activity: AgentActivity;
  buttonFeedback: ButtonFeedback;
  connected: boolean;
  deviceID: string;
  isBusy: boolean;
  mode?: SessionAgentMode;
  modePending?: boolean;
  onOpenHistory?: () => void;
  onOpenMenu?: () => void;
  onQuickNew?: () => void;
  onToggleMode?: (nextMode: SessionAgentMode) => void;
  onToggleSettings: () => void;
  status: string;
  userID: string;
  viewMode: ViewMode;
};

export function AppHeader({
  activity,
  buttonFeedback,
  connected,
  deviceID,
  isBusy,
  mode = "chat",
  modePending = false,
  onOpenHistory,
  onOpenMenu,
  onToggleMode,
  onToggleSettings,
  viewMode,
}: Props) {
  const frames = activity === "tool" ? workingFrames : thinkingFrames;
  const animated = activity === "thinking" || activity === "tool" || activity === "permission" || activity === "uploading";
  const [frame, setFrame] = useState(0);

  useEffect(() => {
    setFrame(0);
    if (!animated) {
      return;
    }
    const timer = setInterval(() => setFrame((current) => (current + 1) % frames.length), 100);
    return () => clearInterval(timer);
  }, [animated, frames.length]);

  const activityText = activityLabel(activity);
  const statusText = activityText || (isBusy ? "处理中" : connected ? "在线" : "离线");

  const handleToggleMode = () => {
    if (onToggleMode && !modePending) {
      onToggleMode(mode === "chat" ? "plan" : "chat");
    }
  };

  return (
    <View style={styles.headerWrap}>
      <View style={styles.header}>
        {/* 左侧：菜单按钮，呼出侧边抽屉 */}
        <Pressable
          accessibilityLabel="打开导航菜单"
          onPress={onOpenMenu}
          style={({ pressed }) => buttonFeedback(styles.ghostButton, pressed)}
        >
          <Feather color={palette.ink2} name="menu" size={18} />
        </Pressable>

        {/* 中间：身份玻璃浮岛（头像 + 名字 + 设备徽章 + 状态行） */}
        <View style={styles.islandWrap}>
          <BlurView intensity={glass.intensity} style={styles.island} tint="dark">
            <View style={styles.avatarSlot}>
              <View style={styles.avatarHalo} />
              <LinearGradient
                colors={[palette.brand, palette.brandDark]}
                end={{ x: 1, y: 1 }}
                start={{ x: 0.15, y: 0 }}
                style={styles.avatarRing}
              >
                <Image
                  accessibilityLabel={activityText ? `MYAI ${activityText}` : "MYAI 头像"}
                  source={animated ? frames[frame] : require("../../../assets/avatar.png")}
                  style={[styles.avatarImage, animated && styles.motionImage]}
                />
              </LinearGradient>
              {connected ? <View style={styles.onlineDot} /> : null}
            </View>

            <View style={styles.identity}>
              <View style={styles.identityRow}>
                <Text style={styles.identityName}>MYAI</Text>
                {connected ? (
                  <View style={styles.deviceBadge}>
                    <Text numberOfLines={1} style={styles.deviceBadgeText}>
                      {deviceID.trim() || "pc-local"}
                    </Text>
                  </View>
                ) : null}
              </View>
              <View style={styles.statusRow}>
                {animated || isBusy ? (
                  <ActivityIndicator color={palette.brand} size="small" style={styles.inlineSpinner} />
                ) : (
                  <View
                    style={[
                      styles.statusDot,
                      connected ? styles.statusDotOnline : styles.statusDotOffline,
                    ]}
                  />
                )}
                <Text numberOfLines={1} style={styles.statusLabel}>
                  {statusText}
                </Text>
              </View>
            </View>
          </BlurView>

          {/* 模式切换：从浮岛里移出，独立成居中小分段控件 */}
          {connected ? (
            <View style={styles.modeSwitch}>
              <Pressable
                accessibilityLabel="切换到对话模式"
                disabled={modePending}
                onPress={() => mode !== "chat" && handleToggleMode()}
                style={({ pressed }) =>
                  buttonFeedback([styles.modeItem, mode === "chat" && styles.modeItemActive], pressed)
                }
              >
                <Text style={[styles.modeText, mode === "chat" && styles.modeTextActive]}>对话</Text>
              </Pressable>
              <Pressable
                accessibilityLabel="切换到规划模式"
                disabled={modePending}
                onPress={() => mode !== "plan" && handleToggleMode()}
                style={({ pressed }) =>
                  buttonFeedback([styles.modeItem, mode === "plan" && styles.modeItemActive], pressed)
                }
              >
                <Text style={[styles.modeText, mode === "plan" && styles.modeTextActive]}>规划</Text>
              </Pressable>
            </View>
          ) : null}
        </View>

        {/* 右侧：对话历史 + 设置 */}
        <View style={styles.rightActions}>
          {onOpenHistory ? (
            <Pressable
              accessibilityLabel="对话历史快捷跳转"
              onPress={onOpenHistory}
              style={({ pressed }) => buttonFeedback(styles.ghostButton, pressed)}
            >
              <Feather color={palette.ink2} name="clock" size={18} />
            </Pressable>
          ) : null}
          <Pressable
            accessibilityLabel="打开设置"
            onPress={onToggleSettings}
            style={({ pressed }) =>
              buttonFeedback(
                [styles.ghostButton, viewMode === "settings" && styles.ghostButtonActive],
                pressed,
              )
            }
          >
            <Feather
              color={viewMode === "settings" ? palette.brand : palette.ink2}
              name="sliders"
              size={18}
            />
          </Pressable>
        </View>
      </View>
    </View>
  );
}

const workingFrames = [
  require("../../../assets/loading/character-01.png"),
  require("../../../assets/loading/character-02.png"),
  require("../../../assets/loading/character-03.png"),
  require("../../../assets/loading/character-04.png"),
  require("../../../assets/loading/character-05.png"),
  require("../../../assets/loading/character-06.png"),
  require("../../../assets/loading/character-07.png"),
  require("../../../assets/loading/character-08.png"),
  require("../../../assets/loading/character-09.png"),
  require("../../../assets/loading/character-10.png"),
];

const thinkingFrames = [
  require("../../../assets/loading/thinking/character-01.png"),
  require("../../../assets/loading/thinking/character-02.png"),
  require("../../../assets/loading/thinking/character-03.png"),
  require("../../../assets/loading/thinking/character-04.png"),
  require("../../../assets/loading/thinking/character-05.png"),
  require("../../../assets/loading/thinking/character-06.png"),
  require("../../../assets/loading/thinking/character-07.png"),
  require("../../../assets/loading/thinking/character-08.png"),
  require("../../../assets/loading/thinking/character-09.png"),
  require("../../../assets/loading/thinking/character-10.png"),
];

function activityLabel(activity: AgentActivity) {
  switch (activity) {
    case "thinking": return "正在思考...";
    case "tool": return "调用工具...";
    case "permission": return "等待确认...";
    case "uploading": return "上传中...";
    default: return "";
  }
}

const styles = StyleSheet.create({
  headerWrap: {
    paddingBottom: space.xs,
    paddingHorizontal: space.sm,
    paddingTop: 2,
    zIndex: 30,
  },
  header: {
    alignItems: "flex-start",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  ghostButton: {
    alignItems: "center",
    backgroundColor: palette.glassInner,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    height: 36,
    justifyContent: "center",
    width: 36,
  },
  ghostButtonActive: {
    backgroundColor: palette.brandWash,
    borderColor: palette.brandWashStrong,
  },
  islandWrap: {
    alignItems: "center",
    flexShrink: 1,
  },
  island: {
    alignItems: "center",
    backgroundColor: palette.glassTint,
    borderColor: palette.glassEdge,
    borderRadius: radius.pill,
    borderWidth: 1,
    flexDirection: "row",
    gap: 9,
    overflow: "hidden",
    paddingBottom: 7,
    paddingLeft: 8,
    paddingRight: 14,
    paddingTop: 7,
    ...shadow.float,
  },
  avatarSlot: {
    height: 34,
    width: 34,
  },
  avatarHalo: {
    ...StyleSheet.absoluteFill,
    borderRadius: radius.pill,
    borderColor: palette.brandGlow,
    borderWidth: 1,
    margin: -4,
  },
  avatarRing: {
    alignItems: "center",
    borderRadius: 17,
    height: 34,
    justifyContent: "center",
    overflow: "hidden",
    width: 34,
  },
  avatarImage: {
    height: "100%",
    width: "100%",
  },
  motionImage: {
    transform: [{ scale: 1.05 }],
  },
  onlineDot: {
    backgroundColor: palette.ok,
    borderColor: palette.surface2,
    borderRadius: 5,
    borderWidth: 1,
    bottom: -1,
    height: 9,
    position: "absolute",
    right: -1,
    width: 9,
  },
  identity: {
    flexShrink: 1,
  },
  identityRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 5,
  },
  identityName: {
    color: palette.ink1,
    fontSize: font.label,
    fontWeight: weight.bold,
    letterSpacing: -0.2,
  },
  deviceBadge: {
    backgroundColor: palette.hairline,
    borderRadius: radius.xxs,
    maxWidth: 95,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  deviceBadgeText: {
    color: palette.ink3,
    fontSize: font.micro,
    fontWeight: weight.semibold,
  },
  statusRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 5,
    marginTop: 1,
  },
  inlineSpinner: {
    marginRight: 1,
    transform: [{ scale: 0.7 }],
  },
  statusDot: {
    borderRadius: 3,
    height: 6,
    width: 6,
  },
  statusDotOnline: {
    backgroundColor: palette.ok,
  },
  statusDotOffline: {
    backgroundColor: palette.err,
  },
  statusLabel: {
    color: palette.ink2,
    flexShrink: 1,
    fontSize: font.caption,
    fontWeight: weight.medium,
  },
  modeSwitch: {
    backgroundColor: palette.glassInner,
    borderRadius: radius.sm,
    flexDirection: "row",
    gap: 1,
    marginTop: space.md,
    padding: 2,
  },
  modeItem: {
    borderRadius: 7,
    paddingHorizontal: space.lg,
    paddingVertical: 3,
  },
  modeItemActive: {
    backgroundColor: palette.brandWashStrong,
  },
  modeText: {
    color: palette.ink3,
    fontSize: font.micro,
    fontWeight: weight.semibold,
  },
  modeTextActive: {
    color: palette.brand,
  },
  rightActions: {
    flexDirection: "row",
    gap: space.sm,
  },
});
