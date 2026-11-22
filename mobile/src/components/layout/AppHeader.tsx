import { useEffect, useRef } from "react";
import { Animated, Easing, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../theme/ThemeProvider";
import type { AgentActivity } from "../../utils/agentActivity";
import { activityStatusLabel, activityToMotionSlot } from "../../utils/motionRegistry";
import { CharacterAvatar } from "../common/CharacterAvatar";
import { PulseDot } from "../common/MotionEffects";
import { Spinner } from "../common/Spinner";
import { HeaderIconButton } from "./ScreenHeader";

type Tone = "ok" | "busy" | "wait" | "off";

type Props = {
  activity: AgentActivity;
  busy: boolean;
  connected: boolean;
  contextUsage?: string;
  deviceID: string;
  emptyHome: boolean;
  nearHeader: boolean;
  onNewSession: () => void;
  onOpenContext: () => void;
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
  if (/配对已失效/.test(status)) return { label: "配对已失效", tone: "off" };
  if (/authenticating|正在认证|验证配对/i.test(status) && !/重连|reconnect/i.test(status)) {
    return { label: "正在验证配对…", tone: "wait" };
  }
  if (/pairing/i.test(status)) return { label: "正在配对…", tone: "wait" };
  if (/reconnect|重连/i.test(status)) return { label: `重连中 · ${device}`, tone: "wait" };
  if (/connecting|restoring|正在连接/i.test(status)) return { label: `正在连接 ${device}…`, tone: "wait" };
  return { label: `${device} · 离线`, tone: "off" };
}

// 对话页悬浮顶栏：透明穿透手势；空首页显示简洁在线胶囊，有对话时中间显示悬浮角色状态岛，
// 聊天内容滚动接近顶部时自动平滑半透明化 (opacity: 0.26) 以免遮挡消息阅读。
export function AppHeader({
  activity,
  busy,
  connected,
  contextUsage,
  deviceID,
  emptyHome,
  nearHeader,
  onNewSession,
  onOpenContext,
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
              <PulseDot color={dotColor} pulse={tone !== "off"} size={7} />
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
                {busy ? (
                  <View pointerEvents="none" style={styles.avatarRing}>
                    <Spinner color={palette.brandLight} size={50} />
                  </View>
                ) : null}
              </View>
              <View style={[styles.statusPill, busy && styles.statusPillBusy]}>
                <PulseDot color={dotColor} pulse={tone !== "off"} size={7} />
                <Text numberOfLines={1} style={styles.statusText}>
                  {label}
                </Text>
                <Feather color={palette.ink3} name="chevron-down" size={11} />
              </View>
            </Pressable>
          )}
        </Animated.View>

        <View pointerEvents="box-none" style={styles.rightCluster}>
          {contextUsage !== undefined ? (
            <Pressable
              accessibilityHint="打开上下文详情"
              accessibilityLabel={`上下文用量 ${contextUsage}`}
              accessibilityRole="button"
              onPress={onOpenContext}
              style={({ pressed }) => [styles.contextPill, pressed && styles.pressed]}
            >
              <Feather color={palette.ink2} name="pie-chart" size={13} />
              <Text adjustsFontSizeToFit minimumFontScale={0.8} numberOfLines={1} style={styles.contextText}>{contextUsage}</Text>
            </Pressable>
          ) : null}
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
      minWidth: 0,
      paddingHorizontal: 8,
    },
    rightCluster: {
      alignItems: "center",
      flexDirection: "row",
      gap: 6,
    },
    contextPill: {
      alignItems: "center",
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 5,
      height: 42,
      justifyContent: "center",
      maxWidth: 124,
      paddingHorizontal: 9,
      ...shadow.card,
    },
    contextText: { color: palette.ink1, flexShrink: 1, fontSize: 11, fontWeight: weight.semibold },
    homePill: {
      alignItems: "center",
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 7,
      marginTop: 4,
      maxWidth: "100%",
      minWidth: 0,
      minHeight: 34,
      paddingHorizontal: 12,
      ...shadow.card,
    },
    homePillText: {
      color: palette.ink1,
      flexShrink: 1,
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
      position: "relative",
      width: 52,
    },
    avatarRing: {
      alignItems: "center",
      bottom: 0,
      justifyContent: "center",
      left: 0,
      opacity: 0.72,
      position: "absolute",
      right: 0,
      top: 0,
    },
    statusPill: {
      alignItems: "center",
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 6,
      marginTop: -8,
      maxWidth: 148,
      paddingHorizontal: 10,
      paddingVertical: 3,
      ...shadow.card,
    },
    statusPillBusy: {
      borderColor: palette.brandDeep,
    },
    statusText: {
      color: palette.ink1,
      fontSize: 11,
      fontWeight: weight.semibold,
    },
    pressed: {
      opacity: 0.78,
      transform: [{ scale: 0.96 }],
    },
  });
