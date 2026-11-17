import { useEffect, useRef } from "react";
import { Animated, Easing, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import type { AgentActivity } from "../../utils/agentActivity";
import { activityStatusLabel, activityToMotionSlot } from "../../utils/motionRegistry";
import { CharacterAvatar } from "../common/CharacterAvatar";
import { HeaderIconButton } from "./ScreenHeader";

type Tone = "ok" | "busy" | "wait" | "off";

type Props = {
  activity: AgentActivity;
  busy: boolean;
  connected: boolean;
  deviceID: string;
  emptyHome: boolean;
  nearHeader: boolean;
  onNewSession: () => void;
  onOpenMenu: () => void;
  onOpenSession: () => void;
  planMode: boolean;
  status: string;
  title: string;
  topInset: number;
  waiting: boolean;
};

// 把 Relay 连接状态翻译成一句人话。
export function describeConnection(status: string, connected: boolean, deviceID: string): { label: string; tone: Tone } {
  const device = deviceID.trim() || "电脑";
  if (connected) return { label: `${device} · 在线`, tone: "ok" };
  if (/pairing/i.test(status)) return { label: "正在配对…", tone: "wait" };
  if (/reconnect/i.test(status)) return { label: `重连中 · ${device}`, tone: "wait" };
  if (/connecting|restoring/i.test(status)) return { label: `正在连接 ${device}…`, tone: "wait" };
  return { label: `${device} · 离线`, tone: "off" };
}

// 对话页悬浮顶栏：透明穿透手势；空首页显示简洁在线胶囊，有对话时中间显示悬浮角色状态岛，
// 聊天内容滚动接近顶部时自动平滑半透明化 (opacity: 0.26) 以免遮挡消息阅读。
export function AppHeader({
  activity,
  busy,
  connected,
  deviceID,
  emptyHome,
  nearHeader,
  onNewSession,
  onOpenMenu,
  onOpenSession,
  planMode,
  status,
  title,
  topInset,
  waiting,
}: Props) {
  const theme = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const { palette } = theme;

  const base = describeConnection(status, connected, deviceID);
  const { label, tone } = !connected
    ? base
    : waiting
      ? { label: "待确认", tone: "wait" as Tone }
      : busy
        ? { label: activityStatusLabel(activity), tone: "busy" as Tone }
        : planMode
          ? { label: "规划模式", tone: "ok" as Tone }
          : { label: "已就绪", tone: "ok" as Tone };

  const islandOpacity = useRef(new Animated.Value(nearHeader && !emptyHome ? 0.26 : 1)).current;

  useEffect(() => {
    const targetOpacity = nearHeader && !emptyHome ? 0.26 : 1;
    Animated.timing(islandOpacity, {
      duration: 210,
      easing: Easing.out(Easing.cubic),
      toValue: targetOpacity,
      useNativeDriver: true,
    }).start();
  }, [emptyHome, islandOpacity, nearHeader]);

  const dotColor =
    tone === "off"
      ? palette.err
      : tone === "wait"
        ? palette.warn
        : tone === "busy"
          ? activity === "subagent"
            ? palette.info
            : palette.brandLight
          : palette.ok;

  const motionSlot = activityToMotionSlot(activity);

  return (
    <View pointerEvents="box-none" style={[styles.root, { paddingTop: topInset + 4 }]}>
      <View pointerEvents="box-none" style={styles.row}>
        <HeaderIconButton accessibilityLabel="打开侧栏" glassy icon="menu" onPress={onOpenMenu} />

        <Animated.View pointerEvents="box-none" style={[styles.centerWrap, { opacity: islandOpacity }]}>
          {emptyHome ? (
            <Pressable
              accessibilityLabel="会话与设备设置"
              accessibilityRole="button"
              onPress={onOpenSession}
              style={({ pressed }) => [styles.homePill, pressed && styles.pressed]}
            >
              <View style={[styles.dot, { backgroundColor: dotColor }]} />
              <Text numberOfLines={1} style={styles.homePillText}>
                {connected ? `${deviceID.trim() || "本机"} · 在线` : base.label}
              </Text>
              <Feather color={palette.ink3} name="chevron-down" size={13} />
            </Pressable>
          ) : (
            <Pressable
              accessibilityLabel={`${title}，点击管理会话`}
              accessibilityRole="button"
              onPress={onOpenSession}
              style={({ pressed }) => [styles.heroIsland, pressed && styles.pressed]}
            >
              <View style={styles.avatarContainer}>
                <CharacterAvatar
                  accessibilityLabel={label}
                  height={52}
                  size={52}
                  slot={motionSlot}
                  variant="full"
                />
              </View>
              <View style={styles.statusPill}>
                <View style={[styles.dot, { backgroundColor: dotColor }]} />
                <Text numberOfLines={1} style={styles.statusText}>
                  {label}
                </Text>
                <Feather color={palette.ink3} name="chevron-down" size={11} />
              </View>
            </Pressable>
          )}
        </Animated.View>

        <View pointerEvents="box-none" style={styles.rightCluster}>
          <HeaderIconButton accessibilityLabel="新对话" glassy icon="edit" onPress={onNewSession} />
        </View>
      </View>
    </View>
  );
}

const makeStyles = ({ palette, shadow }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    root: {
      backgroundColor: "transparent",
      paddingBottom: 4,
      paddingHorizontal: 12,
    },
    row: {
      alignItems: "flex-start",
      flexDirection: "row",
      justifyContent: "space-between",
      minHeight: 54,
    },
    centerWrap: {
      alignItems: "center",
      flex: 1,
      justifyContent: "center",
      paddingHorizontal: 8,
    },
    rightCluster: {
      alignItems: "center",
      flexDirection: "row",
      gap: 6,
    },
    homePill: {
      alignItems: "center",
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 6,
      marginTop: 4,
      maxWidth: 210,
      minHeight: 34,
      paddingHorizontal: 12,
      ...shadow.card,
    },
    homePillText: {
      color: palette.ink1,
      fontSize: font.caption,
      fontWeight: weight.semibold,
    },
    heroIsland: {
      alignItems: "center",
      justifyContent: "center",
    },
    avatarContainer: {
      alignItems: "center",
      height: 50,
      justifyContent: "center",
      width: 52,
    },
    statusPill: {
      alignItems: "center",
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 5,
      marginTop: -8,
      maxWidth: 148,
      paddingHorizontal: 10,
      paddingVertical: 3,
      ...shadow.card,
    },
    statusText: {
      color: palette.ink1,
      fontSize: 11,
      fontWeight: weight.semibold,
    },
    dot: {
      borderRadius: radius.pill,
      height: 7,
      width: 7,
    },
    pressed: {
      opacity: 0.78,
    },
  });
