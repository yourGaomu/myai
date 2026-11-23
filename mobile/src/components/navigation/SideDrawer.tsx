import { useEffect, useMemo, useRef, useState, type ComponentProps } from "react";
import { Animated, Easing, Modal, Platform, Pressable, ScrollView, Text, TextInput, View, useWindowDimensions } from "react-native";
import { Feather } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import type { SessionSummary } from "../../protocol";
import type { ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import { groupSessionsByDate } from "../../utils/session";
import { useAppTheme } from "../../theme/ThemeProvider";
import { CharacterAvatar } from "../common/CharacterAvatar";
import { PulseDot } from "../common/MotionEffects";
import { Spinner } from "../common/Spinner";
import { useDrawerStyles } from "./sideDrawerStyles";

type Props = {
  activeSessionID?: string;
  busySessionID?: string;
  buttonFeedback: ButtonFeedback;
  changesCount: number;
  connected: boolean;
  deletedCount?: number;
  deviceID: string;
  onClose: () => void;
  onNewSession?: () => void;
  onOpenSettings: () => void;
  onSelectSession?: (sessionID: string) => void;
  onSelectViewMode: (mode: ViewMode) => void;
  sessions: SessionSummary[];
  unreadSessionIDs?: Record<string, boolean>;
  userID: string;
  viewMode: ViewMode;
  visible: boolean;
};

type FeatherName = ComponentProps<typeof Feather>["name"];
const maxListed = 30;

// 全局侧栏：新对话、工作区页面入口（含独立回收站）、按日期分组会话、设置。
export function SideDrawer(props: Props) {
  const { palette } = useAppTheme();
  const styles = useDrawerStyles();
  const { onClose, sessions, visible } = props;
  const insets = useSafeAreaInsets();
  const { width } = useWindowDimensions();
  const drawerWidth = Math.min(width * 0.84, 320);
  const slide = useRef(new Animated.Value(-drawerWidth)).current;
  const fade = useRef(new Animated.Value(0)).current;
  const [mounted, setMounted] = useState(visible);
  const [query, setQuery] = useState("");

  useEffect(() => {
    if (visible) {
      setMounted(true);
      Animated.parallel([
        Animated.timing(slide, { duration: 260, easing: Easing.out(Easing.cubic), toValue: 0, useNativeDriver: true }),
        Animated.timing(fade, { duration: 220, toValue: 1, useNativeDriver: true }),
      ]).start();
      return;
    }
    Animated.parallel([
      Animated.timing(slide, { duration: 200, easing: Easing.in(Easing.quad), toValue: -drawerWidth, useNativeDriver: true }),
      Animated.timing(fade, { duration: 200, toValue: 0, useNativeDriver: true }),
    ]).start(({ finished }) => {
      if (finished) {
        setMounted(false);
        setQuery("");
      }
    });
  }, [drawerWidth, fade, slide, visible]);

  const groups = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    const live = sessions.filter((s) => !s.deleted && (!keyword || (s.title || "").toLowerCase().includes(keyword)));
    let budget = maxListed;
    return groupSessionsByDate(live)
      .map((group) => {
        const taken = group.sessions.slice(0, Math.max(0, budget));
        budget -= taken.length;
        return { ...group, sessions: taken };
      })
      .filter((group) => group.sessions.length > 0);
  }, [query, sessions]);

  if (!mounted) {
    return null;
  }

  const go = (mode: ViewMode) => {
    props.onSelectViewMode(mode);
    onClose();
  };
  const displayName = props.userID.trim() && props.userID.trim() !== "local" ? props.userID.trim() : "MyAI";
  const device = props.deviceID.trim() || "电脑";
  const total = sessions.filter((s) => !s.deleted).length;

  return (
    <Modal animationType="none" navigationBarTranslucent onRequestClose={onClose} statusBarTranslucent transparent visible>
      <View style={styles.root}>
        <Animated.View style={[styles.backdrop, { opacity: fade }]}>
          <Pressable accessibilityLabel="关闭侧栏" onPress={onClose} style={{ flex: 1 }} />
        </Animated.View>
        <Animated.View
          style={[
            styles.sheet,
            {
              paddingBottom: Math.max(insets.bottom, Platform.OS === "android" ? 20 : 12) + 6,
              paddingTop: Math.max(insets.top, 24) + 12,
              transform: [{ translateX: slide }],
              width: drawerWidth,
            },
          ]}
        >
          <View style={styles.profile}>
            <CharacterAvatar circular size={42} slot={props.busySessionID ? "working" : "idle"} variant="avatar" />
            <View style={styles.profileText}>
              <Text numberOfLines={1} style={styles.name}>{displayName}</Text>
              <View style={styles.statusRow}>
                <PulseDot color={props.connected ? palette.ok : palette.err} pulse={props.connected} size={7} />
                <Text numberOfLines={1} style={styles.statusText}>{device} · {props.connected ? "在线" : "离线"}</Text>
              </View>
            </View>
          </View>

          <View style={styles.search}>
            <Feather color={palette.ink3} name="search" size={17} />
            <TextInput
              onChangeText={setQuery}
              placeholder="搜索对话"
              placeholderTextColor={palette.ink3}
              style={styles.searchInput}
              value={query}
            />
            {query ? (
              <Pressable accessibilityLabel="清空搜索" onPress={() => setQuery("")} style={styles.clearBtn}>
                <Feather color={palette.ink3} name="x" size={14} />
              </Pressable>
            ) : null}
          </View>

          <NavRow icon="edit" label="新对话" onPress={() => { props.onNewSession?.(); onClose(); }} />
          <NavRow active={props.viewMode === "sessions"} badge={total} icon="message-square" label="全部对话" onPress={() => go("sessions")} />
          <NavRow active={props.viewMode === "files"} icon="folder" label="文件" onPress={() => go("files")} />
          <NavRow active={props.viewMode === "changes" || props.viewMode === "changeDetail"} badge={props.changesCount} icon="git-pull-request" label="变更" onPress={() => go("changes")} />
          <NavRow active={props.viewMode === "knowledge"} icon="book-open" label="知识库与记忆" onPress={() => go("knowledge")} />

          <ScrollView keyboardShouldPersistTaps="handled" showsVerticalScrollIndicator={false} style={styles.list}>
            {groups.length === 0 ? <Text style={styles.empty}>{query ? "没有匹配的对话" : "还没有对话"}</Text> : null}
            {groups.map((group) => (
              <View key={group.label}>
                <Text style={styles.groupLabel}>{group.label}</Text>
                {group.sessions.map((session) => {
                  const current = session.id === props.activeSessionID;
                  return (
                    <Pressable
                      key={session.id}
                      onPress={() => { props.onSelectSession?.(session.id); onClose(); }}
                      style={({ pressed }) => [styles.session, current && styles.sessionCurrent, pressed && styles.pressed]}
                    >
                      <Text numberOfLines={1} style={[styles.sessionText, current && styles.sessionTextCurrent]}>
                        {session.title?.trim() || "未命名对话"}
                      </Text>
                      {session.id === props.busySessionID ? <Spinner size={13} /> : props.unreadSessionIDs?.[session.id] ? <View style={styles.unread} /> : null}
                    </Pressable>
                  );
                })}
              </View>
            ))}
          </ScrollView>

          <View style={styles.footer}>
            <View style={styles.footerItem}>
              <NavRow active={props.viewMode === "settings"} icon="settings" label="设置" onPress={() => { props.onOpenSettings(); onClose(); }} />
            </View>
            <View style={styles.footerItem}>
              <NavRow active={props.viewMode === "trash"} badge={props.deletedCount} icon="trash-2" label="回收站" onPress={() => go("trash")} />
            </View>
          </View>
        </Animated.View>
      </View>
    </Modal>
  );
}

function NavRow({ active, badge, icon, label, onPress }: { active?: boolean; badge?: number; icon: FeatherName; label: string; onPress: () => void }) {
  const { palette } = useAppTheme();
  const styles = useDrawerStyles();
  return (
    <Pressable accessibilityRole="button" onPress={onPress} style={({ pressed }) => [styles.nav, active && styles.navActive, pressed && styles.pressed]}>
      {active ? <View style={styles.navActiveBar} /> : null}
      <Feather color={active ? palette.brandLight : palette.ink2} name={icon} size={19} />
      <Text numberOfLines={1} style={styles.navText}>{label}</Text>
      {badge ? (
        <View style={styles.badge}>
          <Text style={styles.badgeText}>{badge > 99 ? "99+" : badge}</Text>
        </View>
      ) : null}
    </Pressable>
  );
}
