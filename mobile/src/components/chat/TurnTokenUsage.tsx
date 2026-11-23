import { useEffect, useRef } from "react";
import { Animated, Platform, StyleSheet, Text, View } from "react-native";
import { LinearGradient } from "expo-linear-gradient";

import { font, radius, weight } from "../../constants/theme";
import { useMotion } from "../../hooks/useMotionConfig";
import type { TokenUsage } from "../../protocol";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { formatCacheRate, promptCacheLabel, promptCacheRate, turnUsageCounts } from "../../utils/tokenUsage";

export function CacheUsageMeter({ usage, session = false }: { usage?: TokenUsage; session?: boolean }) {
  const { palette } = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const rate = promptCacheRate(usage);
  const label = session ? "会话累计缓存命中率" : "缓存命中率";
  const percent = rate === undefined ? "—" : formatCacheRate(rate);
  const note = rate === undefined
    ? promptCacheLabel(usage)
    : session
      ? `命中 ${usage?.prompt_cached_tokens ?? 0} / ${usage?.prompt_tokens} 输入 Token`
      : `命中 ${usage?.prompt_cached_tokens ?? 0} Token`;
  const hint = rate !== undefined
    ? "按已记录的输入 Token 计算"
    : usage?.prompt_cache_reported && !usage.prompt_cache_incomplete
      ? "输入 Token 缺失或计数异常，无法计算比例"
      : undefined;

  return (
    <View accessible accessibilityLabel={[`${label} ${rate === undefined ? "不可用" : percent}`, note, hint].filter(Boolean).join("，")} style={styles.cache}>
      <View style={styles.cacheRow}>
        <Text style={styles.label}>{label}</Text>
        <Text style={[styles.rate, rate !== undefined && rate > 0 && styles.rateHit]}>{percent}</Text>
      </View>
      {rate !== undefined ? (
        <View style={styles.track}>
          <LinearGradient
            colors={[palette.brand, palette.brandLight]}
            end={{ x: 1, y: 0 }}
            start={{ x: 0, y: 0 }}
            style={{ height: "100%", width: `${rate * 100}%` }}
          />
        </View>
      ) : null}
      <Text style={styles.note}>{hint ? `${note} · ${hint}` : note}</Text>
    </View>
  );
}

export function TurnTokenUsage({ id, running, show, usage }: { id: string; running: boolean; show: boolean; usage?: TokenUsage }) {
  const styles = useThemedStyles(makeStyles);
  const { loaded, settings, systemReduceMotion } = useMotion();
  const motionEnabled = loaded && settings.motionEnabled && !settings.reduceMotion && !systemReduceMotion;
  const accent = useRef(new Animated.Value(1)).current;
  const previous = useRef({ id, running });

  useEffect(() => {
    const wasRunning = previous.current.id === id && previous.current.running;
    previous.current = { id, running };
    accent.setValue(1);
    if (!motionEnabled || !wasRunning || running || !show) return;
    accent.setValue(0);
    const animation = Animated.timing(accent, { duration: 180, toValue: 1, useNativeDriver: Platform.OS !== "web" });
    animation.start();
    return () => animation.stop();
  }, [accent, id, motionEnabled, running, show]);

  if (running || !show) return null;
  const { input, output, total } = turnUsageCounts(usage);
  return (
    <View style={styles.footer}>
      <View style={styles.counts}>
        <View style={styles.total}>
          <Animated.View pointerEvents="none" style={[styles.totalAccent, { opacity: accent }]} />
          <Text style={styles.totalText}>合计 {total === undefined ? "未上报" : `${total} tokens`}</Text>
        </View>
        <Text style={styles.count}>输入 {input ?? "未上报"}</Text>
        <Text style={styles.count}>输出 {output ?? "未上报"}</Text>
      </View>
      <CacheUsageMeter usage={usage} />
    </View>
  );
}

const makeStyles = ({ palette }: AppTheme) => StyleSheet.create({
  footer: { backgroundColor: palette.well, borderColor: palette.hairline, borderRadius: radius.md, borderWidth: 1, gap: 8, padding: 10 },
  counts: { alignItems: "baseline", columnGap: 12, flexDirection: "row", flexWrap: "wrap", rowGap: 4 },
  total: { borderRadius: radius.pill, flexShrink: 1, overflow: "hidden", paddingHorizontal: 8, paddingVertical: 3 },
  totalAccent: { backgroundColor: palette.brandWashStrong, bottom: 0, left: 0, position: "absolute", right: 0, top: 0 },
  totalText: { color: palette.ink1, fontSize: font.label, fontVariant: ["tabular-nums"], fontWeight: weight.semibold },
  count: { color: palette.ink2, flexShrink: 1, fontSize: font.caption, fontVariant: ["tabular-nums"] },
  cache: { gap: 5 },
  cacheRow: { alignItems: "baseline", columnGap: 10, flexDirection: "row", flexWrap: "wrap", justifyContent: "space-between" },
  label: { color: palette.ink2, fontSize: font.caption, fontWeight: weight.semibold },
  rate: { color: palette.ink2, fontSize: font.caption, fontVariant: ["tabular-nums"], fontWeight: weight.semibold },
  rateHit: { color: palette.brandLight },
  track: { backgroundColor: palette.surface3, borderRadius: radius.pill, height: 4, overflow: "hidden" },
  note: { color: palette.ink3, fontSize: font.caption, fontVariant: ["tabular-nums"] },
});
