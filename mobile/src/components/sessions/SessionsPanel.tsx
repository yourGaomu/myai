import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { SessionSummary } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { formatDateTime } from "../../utils/format";
import { font, palette, radius, shadow, weight } from "../../constants/theme";

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

export function SessionsPanel({
  buttonFeedback,
  clientToken,
  deletedSessions,
  onDeleteSession,
  onNewSession,
  onRefreshDeletedSessions,
  onRestoreSession,
  onSelectSession,
  pendingSessions,
  sessionID,
  sessions,
  unreadSessionIDs,
}: Props) {
  return (
    <View style={[styles.panel, styles.sessionsPanel]}>
      <View style={styles.sessionsHeader}>
        <View>
          <Text style={styles.sessionsTitle}>全部会话</Text>
          <Text style={styles.pathText}>{sessions.length} 个会话</Text>
        </View>
        <Pressable
          disabled={pendingSessions}
          onPress={onNewSession}
          style={({ pressed }) => buttonFeedback([styles.floatingNewButton, pendingSessions && styles.disabledButton], pressed)}
        >
          <ButtonContent loading={pendingSessions} text={pendingSessions ? "处理中" : "+"} />
        </Pressable>
      </View>

      <View style={styles.sessionList}>
        {pendingSessions ? (
          <View style={styles.inlineLoading}>
            <ActivityIndicator color={palette.ink1} size="small" />
            <Text style={styles.inlineLoadingText}>正在同步会话...</Text>
          </View>
        ) : null}
        {sessions.length === 0 ? (
          <Text style={styles.emptyText}>{clientToken ? "还没有加载会话" : "请先完成配对"}</Text>
        ) : (
          sessions.map((session) => (
            <View
              key={session.id}
              style={[styles.sessionListItem, session.id === sessionID && styles.sessionListItemActive]}
            >
              <Pressable
                disabled={pendingSessions}
                onPress={() => onSelectSession(session.id)}
                style={({ pressed }) => buttonFeedback([styles.sessionMain, pendingSessions && styles.disabledButton], pressed)}
              >
                <View style={styles.sessionAvatar}>
                  <Text style={styles.sessionAvatarText}>…</Text>
                  {unreadSessionIDs?.[session.id] ? <View style={styles.unreadDot} /> : null}
                </View>
                <View style={styles.flex}>
                  <Text numberOfLines={1} style={styles.sessionListTitle}>{session.title || "新对话"}</Text>
                  <Text numberOfLines={1} style={styles.sessionListMeta}>
                    {session.model || "model"} / {session.permission_mode || "permission"}
                  </Text>
                </View>
                <Text numberOfLines={2} style={styles.sessionListTime}>{formatDateTime(session.updated_at || session.created_at)}</Text>
              </Pressable>
              <Pressable
                disabled={pendingSessions}
                onPress={() => onDeleteSession(session.id)}
                style={({ pressed }) => buttonFeedback([styles.deleteButton, pendingSessions && styles.disabledButton], pressed)}
              >
                <Text style={styles.deleteButtonText}>删除</Text>
              </Pressable>
            </View>
          ))
        )}
      </View>

      <View style={styles.recycleBox}>
        <View style={styles.recycleHeader}>
          <View>
            <Text style={styles.recycleTitle}>回收站</Text>
            <Text style={styles.pathText}>{deletedSessions.length} 个已删除会话</Text>
          </View>
          <Pressable
            disabled={pendingSessions || !clientToken}
            onPress={onRefreshDeletedSessions}
            style={({ pressed }) => buttonFeedback([styles.recycleButton, (pendingSessions || !clientToken) && styles.disabledButton], pressed)}
          >
            <ButtonContent color={palette.ink1} loading={pendingSessions} text="刷新" />
          </Pressable>
        </View>
        {deletedSessions.length === 0 ? (
          <Text style={styles.emptyText}>{clientToken ? "没有已删除会话" : "请先完成配对"}</Text>
        ) : (
          deletedSessions.map((session) => (
            <View key={session.id} style={styles.deletedItem}>
              <View style={styles.flex}>
                <Text numberOfLines={1} style={styles.deletedTitle}>{session.title || "新对话"}</Text>
                <Text numberOfLines={1} style={styles.sessionListMeta}>
                  删除于 {formatDateTime(session.deleted_at || session.updated_at || session.created_at)}
                </Text>
              </View>
              <Pressable
                disabled={pendingSessions}
                onPress={() => onRestoreSession(session.id)}
                style={({ pressed }) => buttonFeedback([styles.restoreButton, pendingSessions && styles.disabledButton], pressed)}
              >
                <Text style={styles.restoreButtonText}>恢复</Text>
              </Pressable>
            </View>
          ))
        )}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  panel: {
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.lg,
    borderWidth: 1,
    gap: 10,
    padding: 12,
    ...shadow.card,
  },
  sessionsPanel: {
    minHeight: 420,
  },
  sessionsHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 12,
    justifyContent: "space-between",
  },
  sessionsTitle: {
    color: palette.ink1,
    fontSize: font.display,
    fontWeight: weight.heavy,
  },
  pathText: {
    color: palette.ink2,
    fontSize: font.label,
    fontWeight: weight.bold,
    marginTop: 3,
  },
  floatingNewButton: {
    alignItems: "center",
    backgroundColor: palette.brand,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    height: 48,
    justifyContent: "center",
    minWidth: 58,
    paddingHorizontal: 12,
  },
  disabledButton: {
    opacity: 0.45,
  },
  sessionList: {
    gap: 4,
  },
  inlineLoading: {
    alignItems: "center",
    backgroundColor: palette.brandWash,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 10,
    paddingVertical: 9,
  },
  inlineLoadingText: {
    color: palette.ink1,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  sessionListItem: {
    alignItems: "center",
    borderBottomColor: palette.hairline,
    borderBottomWidth: 1,
    flexDirection: "row",
    gap: 12,
    paddingVertical: 8,
  },
  sessionListItemActive: {
    backgroundColor: palette.brandWash,
  },
  sessionMain: {
    alignItems: "center",
    flex: 1,
    flexDirection: "row",
    gap: 12,
    minWidth: 0,
    paddingVertical: 6,
  },
  sessionAvatar: {
    alignItems: "center",
    backgroundColor: palette.surface2,
    borderColor: palette.cyanWash,
    borderRadius: radius.pill,
    borderWidth: 1,
    height: 54,
    justifyContent: "center",
    width: 54,
  },
  sessionAvatarText: {
    color: palette.ink1,
    fontSize: font.display,
    fontWeight: weight.bold,
  },
  sessionListTitle: {
    color: palette.ink1,
    fontSize: font.title,
    fontWeight: weight.bold,
  },
  sessionListMeta: {
    color: palette.ink2,
    fontSize: font.label,
    marginTop: 4,
  },
  sessionListTime: {
    color: palette.ink2,
    fontSize: font.label,
    maxWidth: 92,
    textAlign: "right",
  },
  deleteButton: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 36,
    paddingHorizontal: 10,
  },
  deleteButtonText: {
    color: palette.err,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  recycleBox: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    gap: 8,
    marginTop: 8,
    padding: 10,
  },
  recycleHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  recycleTitle: {
    color: palette.ink1,
    fontSize: font.title,
    fontWeight: weight.bold,
  },
  recycleButton: {
    alignItems: "center",
    backgroundColor: palette.brandWash,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 36,
    paddingHorizontal: 10,
  },
  deletedItem: {
    alignItems: "center",
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  deletedTitle: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  restoreButton: {
    alignItems: "center",
    backgroundColor: palette.okWash,
    borderColor: palette.hairline,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 34,
    paddingHorizontal: 10,
  },
  restoreButtonText: {
    color: palette.ink1,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
  emptyText: {
    color: palette.ink2,
  },
  unreadDot: {
    backgroundColor: palette.errWash,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.xs,
    borderWidth: 1,
    height: 10,
    position: "absolute",
    right: -2,
    top: -2,
    width: 10,
  },
});
