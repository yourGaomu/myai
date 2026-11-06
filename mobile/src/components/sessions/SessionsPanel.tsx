import { Fragment } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { SessionSummary } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { formatShortTime } from "../../utils/format";
import { palette, radius } from "../../constants/theme";
import { list } from "../common/listStyles";

type Props = {
  buttonFeedback: ButtonFeedback;
  clientToken: string;
  deletedSessions: SessionSummary[];
  onDeleteSession: (sessionID: string) => void;
  onNewSession: () => void;
  onRefreshDeletedSessions: () => void;
  onRestoreSession: (sessionID: string) => void;
  onSelectSession: (sessionID: string) => void;
  pendingSessions: boolean;
  sessionID: string;
  sessions: SessionSummary[];
  unreadSessionIDs?: Record<string, boolean>;
};

// 全部对话 + 回收站。侧栏只显示最近 30 个，这里是完整列表。
export function SessionsPanel(props: Props) {
  const { clientToken, deletedSessions, pendingSessions, sessions } = props;

  return (
    <View>
      <Pressable disabled={pendingSessions} onPress={props.onNewSession} style={({ pressed }) => [list.secondaryButton, pressed && list.pressed, pendingSessions && list.disabled]}>
        <Feather color={palette.ink1} name="plus" size={18} />
        <Text style={list.secondaryButtonText}>新对话</Text>
      </Pressable>

      <View style={styles.labelRow}>
        <Text style={list.sectionLabel}>全部对话 · {sessions.length}</Text>
        {pendingSessions ? <ActivityIndicator color={palette.ink3} size="small" /> : null}
      </View>
      {sessions.length === 0 ? <Text style={list.empty}>{clientToken ? "还没有对话" : "请先完成配对"}</Text> : null}
      <View style={list.bleed}>
        {sessions.map((session, index) => {
          const current = session.id === props.sessionID;
          return (
            <Fragment key={session.id}>
              {index > 0 ? <View style={styles.divider} /> : null}
              <View style={[styles.row, current && styles.rowCurrent]}>
                <Pressable disabled={pendingSessions} onPress={() => props.onSelectSession(session.id)} style={({ pressed }) => [styles.main, pressed && list.pressed]}>
                  {current ? <View style={styles.bar} /> : null}
                  <View style={list.body}>
                    <Text numberOfLines={1} style={[list.title, current && list.titleStrong]}>{session.title?.trim() || "未命名对话"}</Text>
                    <Text numberOfLines={1} style={list.sub}>{formatShortTime(session.updated_at || session.created_at) || "—"}</Text>
                  </View>
                  {props.unreadSessionIDs?.[session.id] ? <View style={styles.unread} /> : null}
                </Pressable>
                <Pressable accessibilityLabel="删除对话" disabled={pendingSessions} hitSlop={4} onPress={() => props.onDeleteSession(session.id)} style={({ pressed }) => [styles.trash, pressed && list.pressed]}>
                  <Feather color={palette.ink3} name="trash-2" size={17} />
                </Pressable>
              </View>
            </Fragment>
          );
        })}
      </View>

      <View style={styles.labelRow}>
        <Text style={list.sectionLabel}>回收站 · {deletedSessions.length}</Text>
        <Pressable disabled={pendingSessions || !clientToken} hitSlop={6} onPress={props.onRefreshDeletedSessions} style={[styles.refresh, (pendingSessions || !clientToken) && list.disabled]}>
          <Text style={list.textLink}>刷新</Text>
        </Pressable>
      </View>
      {deletedSessions.length === 0 ? <Text style={list.empty}>{clientToken ? "回收站是空的" : "请先完成配对"}</Text> : null}
      <View style={list.bleed}>
        {deletedSessions.map((session, index) => (
          <Fragment key={session.id}>
            {index > 0 ? <View style={styles.divider} /> : null}
            <View style={list.row}>
              <View style={list.body}>
                <Text numberOfLines={1} style={[list.title, styles.deleted]}>{session.title?.trim() || "未命名对话"}</Text>
                <Text numberOfLines={1} style={list.sub}>删除于 {formatShortTime(session.deleted_at || session.updated_at || session.created_at) || "—"}</Text>
              </View>
              <Pressable disabled={pendingSessions} hitSlop={6} onPress={() => props.onRestoreSession(session.id)} style={[list.textButton, pendingSessions && list.disabled]}>
                <Text style={list.textLink}>恢复</Text>
              </Pressable>
            </View>
          </Fragment>
        ))}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  labelRow: { alignItems: "center", flexDirection: "row", justifyContent: "space-between", marginTop: 6 },
  row: { alignItems: "center", flexDirection: "row" },
  rowCurrent: { backgroundColor: palette.surface1 },
  main: { alignItems: "center", flex: 1, flexDirection: "row", gap: 10, minHeight: 58, paddingLeft: 16, paddingRight: 4 },
  bar: { backgroundColor: palette.brand, borderRadius: radius.xxs, height: 18, marginLeft: -8, width: 3 },
  unread: { backgroundColor: palette.brand, borderRadius: radius.pill, height: 7, width: 7 },
  trash: { alignItems: "center", borderRadius: radius.pill, height: 44, justifyContent: "center", marginRight: 6, width: 44 },
  divider: { backgroundColor: palette.hairline, height: 1, marginLeft: 16 },
  refresh: { justifyContent: "center", minHeight: 32, paddingTop: 12 },
  deleted: { color: palette.ink2 },
});
