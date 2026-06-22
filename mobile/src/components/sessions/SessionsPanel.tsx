import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";

import { ButtonContent } from "../common/ButtonContent";
import type { SessionSummary } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { formatDateTime } from "../../utils/format";

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
}: Props) {
  return (
    <View style={[styles.panel, styles.sessionsPanel]}>
      <View style={styles.sessionsHeader}>
        <View>
          <Text style={styles.sessionsTitle}>全部会话</Text>
          <Text style={styles.pathText}>{sessions.length} session(s)</Text>
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
            <ActivityIndicator color="#12100e" size="small" />
            <Text style={styles.inlineLoadingText}>正在同步会话...</Text>
          </View>
        ) : null}
        {sessions.length === 0 ? (
          <Text style={styles.emptyText}>{clientToken ? "No sessions loaded" : "Pair first"}</Text>
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
                </View>
                <View style={styles.flex}>
                  <Text numberOfLines={1} style={styles.sessionListTitle}>{session.title || "New chat"}</Text>
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
            <Text style={styles.recycleTitle}>Recycle Bin</Text>
            <Text style={styles.pathText}>{deletedSessions.length} deleted session(s)</Text>
          </View>
          <Pressable
            disabled={pendingSessions || !clientToken}
            onPress={onRefreshDeletedSessions}
            style={({ pressed }) => buttonFeedback([styles.recycleButton, (pendingSessions || !clientToken) && styles.disabledButton], pressed)}
          >
            <ButtonContent loading={pendingSessions} text="Refresh" />
          </Pressable>
        </View>
        {deletedSessions.length === 0 ? (
          <Text style={styles.emptyText}>{clientToken ? "No deleted sessions" : "Pair first"}</Text>
        ) : (
          deletedSessions.map((session) => (
            <View key={session.id} style={styles.deletedItem}>
              <View style={styles.flex}>
                <Text numberOfLines={1} style={styles.deletedTitle}>{session.title || "New chat"}</Text>
                <Text numberOfLines={1} style={styles.sessionListMeta}>
                  Deleted {formatDateTime(session.deleted_at || session.updated_at || session.created_at)}
                </Text>
              </View>
              <Pressable
                disabled={pendingSessions}
                onPress={() => onRestoreSession(session.id)}
                style={({ pressed }) => buttonFeedback([styles.restoreButton, pendingSessions && styles.disabledButton], pressed)}
              >
                <Text style={styles.restoreButtonText}>Restore</Text>
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
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 4,
    elevation: 2,
    gap: 10,
    padding: 12,
    shadowColor: "#12100e",
    shadowOffset: { width: 4, height: 4 },
    shadowOpacity: 0.12,
    shadowRadius: 0,
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
    color: "#12100e",
    fontSize: 28,
    fontWeight: "900",
  },
  pathText: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "700",
    marginTop: 3,
  },
  floatingNewButton: {
    alignItems: "center",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
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
    backgroundColor: "#fff4cc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 10,
    paddingVertical: 9,
  },
  inlineLoadingText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  sessionListItem: {
    alignItems: "center",
    borderBottomColor: "#d8ccb9",
    borderBottomWidth: 1,
    flexDirection: "row",
    gap: 12,
    paddingVertical: 8,
  },
  sessionListItemActive: {
    backgroundColor: "#fff4cc",
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
    backgroundColor: "#f5f1e9",
    borderColor: "#4fd7ee",
    borderRadius: 34,
    borderWidth: 3,
    height: 54,
    justifyContent: "center",
    width: 54,
  },
  sessionAvatarText: {
    color: "#12100e",
    fontSize: 20,
    fontWeight: "900",
  },
  sessionListTitle: {
    color: "#12100e",
    fontSize: 18,
    fontWeight: "900",
  },
  sessionListMeta: {
    color: "#6c665f",
    fontSize: 12,
    marginTop: 4,
  },
  sessionListTime: {
    color: "#6c665f",
    fontSize: 12,
    maxWidth: 92,
    textAlign: "right",
  },
  deleteButton: {
    alignItems: "center",
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    justifyContent: "center",
    minHeight: 36,
    paddingHorizontal: 10,
  },
  deleteButtonText: {
    color: "#a3342f",
    fontSize: 12,
    fontWeight: "900",
  },
  recycleBox: {
    backgroundColor: "#f5f1e9",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
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
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  recycleButton: {
    alignItems: "center",
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    justifyContent: "center",
    minHeight: 36,
    paddingHorizontal: 10,
  },
  deletedItem: {
    alignItems: "center",
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    flexDirection: "row",
    gap: 8,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  deletedTitle: {
    color: "#12100e",
    fontSize: 14,
    fontWeight: "900",
  },
  restoreButton: {
    alignItems: "center",
    backgroundColor: "#b9e9b0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    justifyContent: "center",
    minHeight: 34,
    paddingHorizontal: 10,
  },
  restoreButtonText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
  emptyText: {
    color: "#6c665f",
  },
});
