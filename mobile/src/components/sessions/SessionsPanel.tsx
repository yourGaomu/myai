import { Fragment, useMemo, useState } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, TextInput, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { SessionSummary } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { formatShortTime } from "../../utils/format";
import { groupSessionsByDate } from "../../utils/session";
import { font, radius, weight } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { ConfirmModal } from "../common/ConfirmModal";
import { useListStyles } from "../common/listStyles";

type Props = {
  buttonFeedback: ButtonFeedback;
  canRenameSessions: boolean;
  clientToken: string;
  deletedSessions: SessionSummary[];
  mode?: "active" | "trash";
  onDeleteSession: (sessionID: string) => void;
  onNewSession: () => void;
  onRefreshDeletedSessions: () => void;
  onRenameSession: (sessionID: string) => void;
  onRestoreSession: (sessionID: string) => void;
  onSelectSession: (sessionID: string) => void;
  pendingSessions: boolean;
  sessionID: string;
  sessions: SessionSummary[];
  unreadSessionIDs?: Record<string, boolean>;
};

// 全部对话（支持日期分组、搜索、二次确认删除）与独立回收站视图（一键还原）。
export function SessionsPanel(props: Props) {
  const { palette } = useAppTheme();
  const list = useListStyles();
  const styles = useThemedStyles(makeStyles);
  const { clientToken, deletedSessions, mode = "active", pendingSessions, sessions } = props;

  const [query, setQuery] = useState("");
  const [confirmDeleteSession, setConfirmDeleteSession] = useState<SessionSummary | null>(null);

  const liveSessions = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    return sessions.filter((s) => !s.deleted && (!keyword || (s.title || "").toLowerCase().includes(keyword)));
  }, [query, sessions]);

  const groupedLive = useMemo(() => groupSessionsByDate(liveSessions), [liveSessions]);

  if (mode === "trash") {
    return (
      <View>
        <View style={styles.labelRow}>
          <Text style={list.sectionLabel}>已移入回收站 · {deletedSessions.length}</Text>
          <Pressable
            disabled={pendingSessions || !clientToken}
            hitSlop={6}
            onPress={props.onRefreshDeletedSessions}
            style={[styles.refresh, (pendingSessions || !clientToken) && list.disabled]}
          >
            <Text style={list.textLink}>刷新</Text>
          </Pressable>
        </View>
        {deletedSessions.length === 0 ? (
          <View style={styles.emptyBox}>
            <Feather color={palette.ink4} name="trash-2" size={28} />
            <Text style={list.empty}>{clientToken ? "回收站是空的，删除的对话会暂存在这里" : "请先完成配对"}</Text>
          </View>
        ) : (
          <View style={list.bleed}>
            {deletedSessions.map((session, index) => (
              <Fragment key={session.id}>
                {index > 0 ? <View style={styles.divider} /> : null}
                <View style={list.row}>
                  <View style={list.body}>
                    <Text numberOfLines={1} style={[list.title, styles.deleted]}>
                      {session.title?.trim() || "未命名对话"}
                    </Text>
                    <Text numberOfLines={1} style={list.sub}>
                      删除于 {formatShortTime(session.deleted_at || session.updated_at || session.created_at) || "—"}
                    </Text>
                  </View>
                  <Pressable
                    disabled={pendingSessions}
                    hitSlop={6}
                    onPress={() => props.onRestoreSession(session.id)}
                    style={({ pressed }) => [styles.restoreBtn, pressed && list.pressed, pendingSessions && list.disabled]}
                  >
                    <Feather color={palette.brandLight} name="rotate-ccw" size={14} />
                    <Text style={list.textLink}>恢复</Text>
                  </Pressable>
                </View>
              </Fragment>
            ))}
          </View>
        )}
      </View>
    );
  }

  return (
    <View>
      <Pressable
        disabled={pendingSessions}
        onPress={props.onNewSession}
        style={({ pressed }) => [list.secondaryButton, pressed && list.pressed, pendingSessions && list.disabled]}
      >
        <Feather color={palette.ink1} name="plus" size={18} />
        <Text style={list.secondaryButtonText}>新对话</Text>
      </Pressable>

      <View style={styles.search}>
        <Feather color={palette.ink3} name="search" size={16} />
        <TextInput
          onChangeText={setQuery}
          placeholder="搜索全部对话标题…"
          placeholderTextColor={palette.ink4}
          style={styles.searchInput}
          value={query}
        />
        {query ? (
          <Pressable hitSlop={6} onPress={() => setQuery("")}>
            <Feather color={palette.ink3} name="x" size={14} />
          </Pressable>
        ) : null}
      </View>

      <View style={styles.labelRow}>
        <Text style={list.sectionLabel}>全部对话 · {liveSessions.length}</Text>
        {pendingSessions ? <ActivityIndicator color={palette.ink3} size="small" /> : null}
      </View>

      {liveSessions.length === 0 ? (
        <Text style={list.empty}>{clientToken ? (query ? "没有匹配的对话" : "还没有对话") : "请先完成配对"}</Text>
      ) : null}

      {groupedLive.map((group) => (
        <Fragment key={group.label}>
          <Text style={styles.groupHeader}>{group.label}</Text>
          <View style={list.bleed}>
            {group.sessions.map((session, index) => {
              const current = session.id === props.sessionID;
              return (
                <Fragment key={session.id}>
                  {index > 0 ? <View style={styles.divider} /> : null}
                  <View style={[styles.row, current && styles.rowCurrent]}>
                    <Pressable
                      disabled={pendingSessions}
                      onPress={() => props.onSelectSession(session.id)}
                      style={({ pressed }) => [styles.main, pressed && list.pressed]}
                    >
                      {current ? <View style={styles.bar} /> : null}
                      <View style={list.body}>
                        <Text numberOfLines={1} style={[list.title, current && list.titleStrong]}>
                          {session.title?.trim() || "未命名对话"}
                        </Text>
                        <Text numberOfLines={1} style={list.sub}>
                          {formatShortTime(session.updated_at || session.created_at) || "—"}
                        </Text>
                      </View>
                      {props.unreadSessionIDs?.[session.id] ? <View style={styles.unread} /> : null}
                    </Pressable>
                    <Pressable
                      accessibilityLabel="重命名对话"
                      accessibilityRole="button"
                      disabled={!props.canRenameSessions}
                      onPress={() => props.onRenameSession(session.id)}
                      style={({ pressed }) => [styles.rowAction, pressed && list.pressed, !props.canRenameSessions && list.disabled]}
                    >
                      <Feather color={palette.ink3} name="edit-2" size={17} />
                    </Pressable>
                    <Pressable
                      accessibilityLabel="删除对话"
                      disabled={pendingSessions}
                      hitSlop={4}
                      onPress={() => setConfirmDeleteSession(session)}
                      style={({ pressed }) => [styles.rowAction, styles.trash, pressed && list.pressed]}
                    >
                      <Feather color={palette.ink3} name="trash-2" size={17} />
                    </Pressable>
                  </View>
                </Fragment>
              );
            })}
          </View>
        </Fragment>
      ))}

      <ConfirmModal
        confirmLabel="移入回收站"
        danger
        icon="trash-2"
        message={`确定删除「${confirmDeleteSession?.title?.trim() || "未命名对话"}」吗？之后仍可在侧栏「回收站」中恢复。`}
        onCancel={() => setConfirmDeleteSession(null)}
        onConfirm={() => {
          if (confirmDeleteSession) {
            props.onDeleteSession(confirmDeleteSession.id);
          }
          setConfirmDeleteSession(null);
        }}
        title="移入回收站"
        visible={Boolean(confirmDeleteSession)}
      />
    </View>
  );
}

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    search: {
      alignItems: "center",
      backgroundColor: palette.well,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      flexDirection: "row",
      gap: 8,
      height: 40,
      marginTop: 12,
      paddingHorizontal: 12,
    },
    searchInput: { color: palette.ink1, flex: 1, fontSize: font.label, padding: 0 },
    labelRow: { alignItems: "center", flexDirection: "row", justifyContent: "space-between", marginTop: 4 },
    groupHeader: {
      color: palette.ink3,
      fontSize: font.caption,
      fontWeight: weight.semibold,
      marginBottom: 4,
      marginTop: 10,
    },
    row: { alignItems: "center", flexDirection: "row" },
    rowCurrent: { backgroundColor: palette.surface1 },
    main: { alignItems: "center", flex: 1, flexDirection: "row", gap: 10, minHeight: 58, paddingLeft: 16, paddingRight: 4 },
    bar: { backgroundColor: palette.brand, borderRadius: radius.xxs, height: 18, marginLeft: -8, width: 3 },
    unread: { backgroundColor: palette.brand, borderRadius: radius.pill, height: 7, width: 7 },
    rowAction: { alignItems: "center", borderRadius: radius.pill, height: 44, justifyContent: "center", width: 44 },
    trash: { marginRight: 6 },
    divider: { backgroundColor: palette.hairline, height: 1, marginLeft: 16 },
    refresh: { justifyContent: "center", minHeight: 32, paddingTop: 12 },
    deleted: { color: palette.ink2 },
    emptyBox: { alignItems: "center", gap: 8, paddingTop: 40 },
    restoreBtn: {
      alignItems: "center",
      backgroundColor: palette.brandWash,
      borderColor: palette.brandDeep,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 5,
      minHeight: 32,
      paddingHorizontal: 11,
    },
  });
