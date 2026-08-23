import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";

import type { TokenUsage } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { tokenCount, usageHasValues, usageSummary } from "../../utils/tokenUsage";

type Props = {
  buttonFeedback: ButtonFeedback;
  connected: boolean;
  isBusy: boolean;
  lastUsage: TokenUsage | null;
  onSettingsPress: () => void;
};

export function TokenDock({ buttonFeedback, connected, isBusy, lastUsage, onSettingsPress }: Props) {
  return (
    <View style={styles.tokenDock}>
      <View style={styles.tokenDockHeader}>
        <View style={styles.tokenSummary}>
          <Text style={styles.tokenDockTitle}>{lastUsage ? usageSummary(lastUsage) : "令牌用量"}</Text>
          <Text style={styles.tokenDockMeta}>{lastUsage ? "当前回复用量" : "回复后显示"}</Text>
        </View>
        <View style={styles.tokenDockActions}>
          <Pressable onPress={onSettingsPress} style={({ pressed }) => buttonFeedback(styles.tokenSettingsButton, pressed)}>
            <Text style={styles.tokenSettingsText}>⚙</Text>
          </Pressable>
          <View style={[styles.statusPill, connected ? styles.statusPillOnline : styles.statusPillOffline]}>
            {isBusy ? (
              <ActivityIndicator color="#12100e" size="small" />
            ) : (
              <View style={styles.statusDot} />
            )}
            <Text style={styles.statusPillText}>{isBusy ? "处理中" : connected ? "在线" : "离线"}</Text>
          </View>
        </View>
      </View>
      {lastUsage && usageHasValues(lastUsage) ? (
        <View style={styles.tokenDockRow}>
          <Text style={styles.tokenDockMeta}>输入 {tokenCount(lastUsage.prompt_tokens)}</Text>
          <Text style={styles.tokenDockMeta}>输出 {tokenCount(lastUsage.completion_tokens)}</Text>
          <Text style={styles.tokenDockMeta}>思考 {tokenCount(lastUsage.reasoning_tokens)}</Text>
          <Text style={styles.tokenDockMeta}>缓存 {tokenCount(lastUsage.prompt_cached_tokens)}</Text>
        </View>
      ) : lastUsage ? (
        <Text style={styles.tokenDockMeta}>暂无令牌用量</Text>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  tokenDock: {
    backgroundColor: "#eee8dd",
    borderColor: "#d7cfc2",
    borderRadius: 12,
    borderWidth: 1,
    gap: 3,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  tokenDockHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
    justifyContent: "space-between",
  },
  tokenSummary: {
    flex: 1,
    minWidth: 0,
  },
  tokenDockActions: {
    alignItems: "center",
    flexDirection: "row",
    flexShrink: 0,
    gap: 5,
  },
  tokenSettingsButton: {
    alignItems: "center",
    backgroundColor: "#4fd7ee",
    borderColor: "#25231f",
    borderRadius: 6,
    borderWidth: 1,
    height: 28,
    justifyContent: "center",
    width: 30,
  },
  tokenSettingsText: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
  },
  tokenDockTitle: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  tokenDockRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  tokenDockMeta: {
    color: "#6c665f",
    fontSize: 11,
    fontWeight: "800",
  },
  statusPill: {
    alignItems: "center",
    borderColor: "#25231f",
    borderRadius: 999,
    borderWidth: 1,
    flexDirection: "row",
    gap: 4,
    minHeight: 28,
    paddingHorizontal: 7,
    paddingVertical: 3,
  },
  statusPillOnline: {
    backgroundColor: "#b9e9b0",
  },
  statusPillOffline: {
    backgroundColor: "#ff7f68",
  },
  statusDot: {
    backgroundColor: "#12100e",
    borderRadius: 3,
    height: 6,
    width: 6,
  },
  statusPillText: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
});
