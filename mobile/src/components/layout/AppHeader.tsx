import { ActivityIndicator, Image, Pressable, StyleSheet, Text, View } from "react-native";
import { useEffect, useState } from "react";

import type { ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import type { AgentActivity } from "../../utils/agentActivity";

type Props = {
  activity: AgentActivity;
  buttonFeedback: ButtonFeedback;
  connected: boolean;
  deviceID: string;
  isBusy: boolean;
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
  const connectionText = connected ? `手机控制电脑 / ${userID.trim() || "本地用户"} / ${deviceID.trim() || "本地设备"}` : disconnectedLabel(status);
  const statusText = activityText || (isBusy ? "处理中" : connected ? "在线" : "离线");

  return (
    <View style={styles.header}>
      <View style={styles.brand}>
        <View style={styles.brandMark}>
          <Image
            accessibilityLabel={activityText ? `MYAI ${activityText}` : "MYAI 图标"}
            source={animated ? frames[frame] : require("../../../assets/icon.png")}
            style={[styles.brandImage, animated && styles.motionImage]}
          />
        </View>
        <View style={styles.headerText}>
          <View style={styles.titleRow}>
            <Text style={styles.title}>MYAI</Text>
            {connected ? (
              <View style={styles.devicePill}>
                <Text numberOfLines={1} style={styles.devicePillText}>
                  {deviceID.trim() || "pc-local"}
                </Text>
              </View>
            ) : null}
          </View>
          <View style={styles.contextRow}>
            <Text numberOfLines={1} style={styles.subtitle}>
              {activityText || (connected ? `📁 ${userID.trim() || "local"} · 电脑已连接` : disconnectedLabel(status))}
            </Text>
          </View>
        </View>
      </View>
      <View style={styles.headerActions}>
        <Pressable
          onPress={onToggleSettings}
          style={({ pressed }) => buttonFeedback([styles.ghostButton, viewMode === "settings" && styles.ghostButtonActive], pressed)}
        >
          <Text style={styles.ghostButtonText}>⚙</Text>
        </Pressable>
        <View style={[styles.statusPill, connected ? styles.statusPillOnline : styles.statusPillOffline]}>
          {animated || isBusy ? (
            <ActivityIndicator color="#12100e" size="small" />
          ) : (
            <View style={[styles.statusDot, connected && styles.statusDotOnline]} />
          )}
          <Text style={styles.statusPillText}>{statusText}</Text>
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
    case "tool": return "正在调用工具...";
    case "permission": return "等待操作确认...";
    case "uploading": return "正在上传文件...";
    default: return "";
  }
}

function disconnectedLabel(status: string) {
  switch (status) {
    case "Connecting": return "正在连接 Relay...";
    case "Pairing": return "正在配对...";
    case "Connection timeout": return "Relay 连接超时";
    case "WebSocket error": return "Relay 连接异常";
    default: return "未连接 Relay";
  }
}

const styles = StyleSheet.create({
  header: {
    alignItems: "center",
    backgroundColor: "#f4f5f7",
    borderBottomColor: "#25231f",
    borderBottomWidth: 2,
    flexDirection: "row",
    gap: 8,
    minHeight: 60,
    paddingBottom: 11,
    paddingTop: 7,
  },
  brand: {
    alignItems: "center",
    flex: 1,
    flexDirection: "row",
    gap: 10,
    minWidth: 0,
  },
  brandMark: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 12,
    borderWidth: 2,
    height: 42,
    justifyContent: "center",
    overflow: "hidden",
    width: 42,
  },
  brandImage: {
    height: 40,
    width: 40,
  },
  motionImage: {
    height: 50,
    resizeMode: "contain",
    width: 24,
  },
  headerText: {
    flex: 1,
    minWidth: 0,
  },
  headerActions: {
    alignItems: "center",
    flexDirection: "row",
    flexShrink: 0,
    gap: 8,
  },
  titleRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 6,
  },
  title: {
    color: "#12100e",
    fontSize: 17,
    fontWeight: "900",
    letterSpacing: -0.2,
    lineHeight: 22,
  },
  devicePill: {
    backgroundColor: "#fffdf7",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 1.5,
    maxWidth: 90,
    paddingHorizontal: 5,
    paddingVertical: 1,
  },
  devicePillText: {
    color: "#12100e",
    fontSize: 9.5,
    fontWeight: "900",
  },
  contextRow: {
    marginTop: 1,
  },
  subtitle: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "700",
  },
  ghostButton: {
    alignItems: "center",
    backgroundColor: "#4fd7ee",
    borderColor: "#12100e",
    borderRadius: 10,
    borderWidth: 2,
    elevation: 2,
    height: 34,
    justifyContent: "center",
    minWidth: 34,
    paddingHorizontal: 8,
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 1.5 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  ghostButtonActive: {
    backgroundColor: "#ffd84f",
  },
  ghostButtonText: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  statusPill: {
    alignItems: "center",
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 2,
    elevation: 2,
    flexDirection: "row",
    gap: 5,
    minHeight: 34,
    paddingHorizontal: 9,
    paddingVertical: 4,
    shadowColor: "#12100e",
    shadowOffset: { width: 0, height: 1.5 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  statusPillOnline: {
    backgroundColor: "#b9e9b0",
  },
  statusPillOffline: {
    backgroundColor: "#ff7f68",
  },
  statusDot: {
    backgroundColor: "#12100e",
    borderRadius: 4,
    height: 7,
    width: 7,
  },
  statusDotOnline: {
    backgroundColor: "#0e4e16",
  },
  statusPillText: {
    color: "#12100e",
    fontSize: 11.5,
    fontWeight: "900",
  },
});
