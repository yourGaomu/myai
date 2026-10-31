import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import type { TokenUsage } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { tokenCount, usageHasValues, usageSummary } from "../../utils/tokenUsage";
import { font, palette, radius, weight } from "../../constants/theme";

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
            <Feather color={palette.ink2} name="sliders" size={14} />
          </Pressable>
          <View style={[styles.statusPill, connected ? styles.statusPillOnline : styles.statusPillOffline]}>
            {isBusy ? (
              <ActivityIndicator color={palette.ink1} size="small" />
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
    backgroundColor: palette.surface1,
    borderColor: palette.hairline,
    borderRadius: radius.md,
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
    backgroundColor: palette.cyanWash,
    borderColor: palette.hairlineStrong,
    borderRadius: radius.sm,
    borderWidth: 1,
    height: 28,
    justifyContent: "center",
    width: 30,
  },
  tokenSettingsText: {
    color: palette.ink1,
    fontSize: font.body,
    fontWeight: weight.bold,
  },
  tokenDockTitle: {
    color: palette.ink1,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  tokenDockRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  tokenDockMeta: {
    color: palette.ink2,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  statusPill: {
    alignItems: "center",
    borderColor: palette.hairlineStrong,
    borderRadius: radius.pill,
    borderWidth: 1,
    flexDirection: "row",
    gap: 4,
    minHeight: 28,
    paddingHorizontal: 7,
    paddingVertical: 3,
  },
  statusPillOnline: {
    backgroundColor: palette.okWash,
  },
  statusPillOffline: {
    backgroundColor: palette.errWash,
  },
  statusDot: {
    backgroundColor: palette.surface3,
    borderRadius: radius.xxs,
    height: 6,
    width: 6,
  },
  statusPillText: {
    color: palette.ink1,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
});
