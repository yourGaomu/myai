import { ActivityIndicator, Image, Pressable, StyleSheet, Text, View } from "react-native";
import { useEffect, useState } from "react";

import type { SessionAgentMode, ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import type { AgentActivity } from "../../utils/agentActivity";

type Props = {
  activity: AgentActivity;
  buttonFeedback: ButtonFeedback;
  connected: boolean;
  deviceID: string;
  isBusy: boolean;
  mode?: SessionAgentMode;
  modePending?: boolean;
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
  onOpenMenu,
  onQuickNew,
  onToggleMode,
  onToggleSettings,
  status,
  userID,
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
    <View style={styles.header}>
      {/* 左侧：菜单按钮，呼出侧边抽屉（收纳设置与工作区，借鉴图四） */}
      <Pressable
        accessibilityLabel="打开导航菜单"
        onPress={onOpenMenu}
        style={({ pressed }) =>
          buttonFeedback(styles.actionButton, pressed)
        }
      >
        <Text style={styles.actionButtonText}>☰</Text>
      </Pressable>

      {/* 中上方：动作角色与状态浮岛（半透明玻璃拟态，借鉴图三 Muse） */}
      <View style={styles.characterIsland}>
        {/* 头像突出在上 */}
        <View style={styles.avatarWrap}>
          <Image
            accessibilityLabel={activityText ? `MYAI ${activityText}` : "MYAI 头像"}
            source={animated ? frames[frame] : require("../../../assets/icon.png")}
            style={[styles.avatarImage, animated && styles.motionImage]}
          />
        </View>

        {/* 角色名与设备徽章 */}
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

        {/* 状态与紧凑模式切换胶囊 (将原占地巨大的模式条缩进此处) */}
        <View style={styles.statusModeCapsule}>
          {animated || isBusy ? (
            <ActivityIndicator color="#12100e" size="small" style={styles.inlineSpinner} />
          ) : (
            <View
              style={[
                styles.statusDot,
                connected ? styles.statusDotOnline : styles.statusDotOffline,
              ]}
            />
          )}
          <Text style={styles.statusLabel}>{statusText}</Text>

          {/* 模式切换药丸 */}
          {connected ? (
            <>
              <Text style={styles.dotDivider}>·</Text>
              <Pressable
                accessibilityLabel="切换会话模式"
                disabled={modePending}
                onPress={handleToggleMode}
                style={({ pressed }) =>
                  buttonFeedback(
                    [
                      styles.modePill,
                      mode === "plan" && styles.modePillPlan,
                    ],
                    pressed,
                  )
                }
              >
                {modePending ? (
                  <ActivityIndicator color="#12100e" size="small" />
                ) : (
                  <>
                    <Text style={styles.modePillText}>
                      {mode === "plan" ? "🎯 规划模式" : "💬 对话模式"}
                    </Text>
                    <Text style={styles.modeDropdownArrow}>▾</Text>
                  </>
                )}
              </Pressable>
            </>
          ) : null}
        </View>
      </View>

      {/* 右侧：快速设置或新会话按钮 */}
      <View style={styles.rightActions}>
        <Pressable
          accessibilityLabel="打开设置"
          onPress={onToggleSettings}
          style={({ pressed }) =>
            buttonFeedback(
              [styles.actionButton, viewMode === "settings" && styles.actionButtonActive],
              pressed,
            )
          }
        >
          <Text style={styles.actionButtonText}>⚙</Text>
        </Pressable>
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
  header: {
    alignItems: "flex-start",
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 12,
    paddingTop: 4,
    zIndex: 30,
  },
  actionButton: {
    alignItems: "center",
    backgroundColor: "rgba(255, 253, 247, 0.9)",
    borderColor: "#12100e",
    borderRadius: 20,
    borderWidth: 2,
    elevation: 3,
    height: 40,
    justifyContent: "center",
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 1,
    shadowRadius: 0,
    width: 40,
  },
  actionButtonActive: {
    backgroundColor: "#ffd84f",
  },
  actionButtonText: {
    color: "#12100e",
    fontSize: 17,
    fontWeight: "900",
  },
  rightActions: {
    flexDirection: "row",
    gap: 6,
  },
  characterIsland: {
    alignItems: "center",
    backgroundColor: "rgba(255, 253, 247, 0.92)",
    borderColor: "#12100e",
    borderRadius: 24,
    borderWidth: 2,
    elevation: 3,
    flexShrink: 1,
    paddingBottom: 6,
    paddingHorizontal: 14,
    paddingTop: 4,
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  avatarWrap: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 18,
    borderWidth: 2,
    elevation: 2,
    height: 36,
    marginTop: -14,
    overflow: "hidden",
    shadowColor: "#12100e",
    shadowOffset: { width: 1, height: 1 },
    shadowOpacity: 1,
    shadowRadius: 0,
    width: 36,
  },
  avatarImage: {
    height: "100%",
    width: "100%",
  },
  motionImage: {
    transform: [{ scale: 1.05 }],
  },
  identityRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 5,
    marginTop: 2,
  },
  identityName: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
    letterSpacing: -0.2,
  },
  deviceBadge: {
    backgroundColor: "#f0ebe1",
    borderColor: "#12100e",
    borderRadius: 5,
    borderWidth: 1,
    maxWidth: 95,
    paddingHorizontal: 4,
    paddingVertical: 0.5,
  },
  deviceBadgeText: {
    color: "#12100e",
    fontSize: 9,
    fontWeight: "900",
  },
  statusModeCapsule: {
    alignItems: "center",
    backgroundColor: "#f5eee2",
    borderColor: "#d8cfbf",
    borderRadius: 999,
    borderWidth: 1,
    flexDirection: "row",
    gap: 4,
    marginTop: 4,
    paddingHorizontal: 7,
    paddingVertical: 2,
  },
  inlineSpinner: {
    marginRight: 2,
  },
  statusDot: {
    borderRadius: 4,
    height: 6,
    width: 6,
  },
  statusDotOnline: {
    backgroundColor: "#2e8b38",
  },
  statusDotOffline: {
    backgroundColor: "#ff7f68",
  },
  statusLabel: {
    color: "#12100e",
    fontSize: 10.5,
    fontWeight: "800",
  },
  dotDivider: {
    color: "#999083",
    fontSize: 10,
    fontWeight: "900",
  },
  modePill: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 1,
    flexDirection: "row",
    gap: 2,
    paddingHorizontal: 6,
    paddingVertical: 1,
  },
  modePillPlan: {
    backgroundColor: "#4fd7ee",
  },
  modePillText: {
    color: "#12100e",
    fontSize: 10,
    fontWeight: "900",
  },
  modeDropdownArrow: {
    color: "#12100e",
    fontSize: 8,
    fontWeight: "900",
    marginTop: -1,
  },
});
