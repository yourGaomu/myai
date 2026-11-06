import { Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import { font, palette, radius, weight } from "../../constants/theme";
import { HeaderIconButton } from "./ScreenHeader";

type Tone = "ok" | "busy" | "wait" | "off";

type Props = {
  busy: boolean;
  connected: boolean;
  deviceID: string;
  onNewSession: () => void;
  onOpenMenu: () => void;
  onOpenSession: () => void;
  planMode: boolean;
  scrolled: boolean;
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

// 对话页顶栏：左侧栏、中间会话标题（点开会话设置）、右侧新对话。副标题显示连接和执行状态。
export function AppHeader({ busy, connected, deviceID, onNewSession, onOpenMenu, onOpenSession, planMode, scrolled, status, title, topInset, waiting }: Props) {
  const device = deviceID.trim() || "电脑";
  const base = describeConnection(status, connected, deviceID);
  const { label, tone } = !connected
    ? base
    : waiting
      ? { label: "等待你确认", tone: "wait" as Tone }
      : busy
        ? { label: `正在执行 · ${device}`, tone: "busy" as Tone }
        : planMode
          ? { label: `规划模式 · ${device}`, tone: "ok" as Tone }
          : base;

  return (
    <View style={[styles.bar, { paddingTop: topInset }, scrolled && styles.barScrolled]}>
      <View style={styles.row}>
        <HeaderIconButton accessibilityLabel="打开侧栏" icon="menu" onPress={onOpenMenu} />
        <Pressable
          accessibilityLabel="会话设置"
          accessibilityRole="button"
          onPress={onOpenSession}
          style={({ pressed }) => [styles.titleButton, pressed && styles.pressed]}
        >
          <View style={styles.titleRow}>
            <Text numberOfLines={1} style={styles.title}>
              {title}
            </Text>
            <Feather color={palette.ink3} name="chevron-down" size={14} />
          </View>
          <View style={styles.subRow}>
            <View style={[styles.halo, tone === "busy" && styles.haloBusy, tone === "wait" && styles.haloWait]}>
              <View style={[styles.dot, styles[tone]]} />
            </View>
            <Text numberOfLines={1} style={styles.sub}>
              {label}
            </Text>
          </View>
        </Pressable>
        <HeaderIconButton accessibilityLabel="新对话" icon="edit" onPress={onNewSession} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {
    backgroundColor: palette.canvas,
    borderBottomColor: palette.transparent,
    borderBottomWidth: 1,
    paddingHorizontal: 6,
    zIndex: 30,
  },
  barScrolled: {
    borderBottomColor: palette.hairline,
  },
  row: {
    alignItems: "center",
    flexDirection: "row",
    gap: 4,
    minHeight: 52,
  },
  titleButton: {
    alignItems: "center",
    borderRadius: radius.md - 2,
    flex: 1,
    minWidth: 0,
    paddingHorizontal: 10,
    paddingVertical: 3,
  },
  pressed: {
    backgroundColor: palette.pressed,
  },
  titleRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 3,
    maxWidth: "100%",
  },
  title: {
    color: palette.ink1,
    flexShrink: 1,
    fontSize: font.bodyLg,
    fontWeight: weight.semibold,
  },
  subRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: 4,
    maxWidth: "100%",
  },
  halo: {
    alignItems: "center",
    borderRadius: radius.pill,
    height: 13,
    justifyContent: "center",
    width: 13,
  },
  haloBusy: { backgroundColor: palette.brandWash },
  haloWait: { backgroundColor: palette.warnWash },
  dot: { borderRadius: radius.pill, height: 7, width: 7 },
  ok: { backgroundColor: palette.ok },
  busy: { backgroundColor: palette.brand },
  wait: { backgroundColor: palette.warn },
  off: { backgroundColor: palette.err },
  sub: {
    color: palette.ink2,
    flexShrink: 1,
    fontSize: font.caption - 0.5,
  },
});
