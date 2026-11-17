import { Pressable, StyleSheet, Switch, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import {
  accentOptions,
  font,
  radius,
  weight,
  type AccentPreset,
  type ThemeMode,
} from "../../../constants/theme";
import { useAppTheme, useThemedStyles } from "../../../theme/ThemeProvider";

const themeModes: Array<{ icon: "moon" | "sun" | "smartphone"; key: ThemeMode; label: string }> = [
  { key: "dark", label: "深色", icon: "moon" },
  { key: "light", label: "浅色", icon: "sun" },
  { key: "system", label: "跟随系统", icon: "smartphone" },
];

const fontSizeChoices = [14, 15, 16, 17, 18];

export function AppearanceSettings() {
  const theme = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const {
    accent,
    highlightUserBubble,
    isDark,
    keepThinkingExpanded,
    palette,
    readingFontSize,
    resetAppearance,
    themeMode,
    updateAppearance,
  } = theme;

  return (
    <View style={styles.root}>
      {/* 主题模式 */}
      <View style={styles.card}>
        <Text style={styles.cardTitle}>色彩外观</Text>
        <Text style={styles.cardSub}>切换石墨深色、浅色高亮画布或自动跟随系统主题</Text>
        <View style={styles.segmentedRow}>
          {themeModes.map((item) => {
            const active = themeMode === item.key;
            return (
              <Pressable
                accessibilityRole="button"
                accessibilityState={{ selected: active }}
                key={item.key}
                onPress={() => updateAppearance({ themeMode: item.key })}
                style={({ pressed }) => [styles.modeBtn, active && styles.modeBtnActive, pressed && styles.pressed]}
              >
                <Feather color={active ? palette.inkInverse : palette.ink2} name={item.icon} size={15} />
                <Text style={[styles.modeBtnText, active && styles.modeBtnTextActive]}>{item.label}</Text>
              </Pressable>
            );
          })}
        </View>
      </View>

      {/* 主题强调色 */}
      <View style={styles.card}>
        <Text style={styles.cardTitle}>品牌强调色</Text>
        <Text style={styles.cardSub}>控制发送按钮、悬浮状态光晕、进度条与选中态主色调</Text>
        <View style={styles.accentGrid}>
          {accentOptions.map((item) => {
            const active = accent === item.key;
            const swatch = isDark ? item.swatchDark : item.swatchLight;
            return (
              <Pressable
                accessibilityRole="button"
                accessibilityState={{ selected: active }}
                key={item.key}
                onPress={() => updateAppearance({ accent: item.key as AccentPreset })}
                style={({ pressed }) => [styles.accentCard, active && styles.accentCardActive, pressed && styles.pressed]}
              >
                <View style={[styles.swatchCircle, { backgroundColor: swatch }]}>
                  {active ? <Feather color={palette.inkInverse} name="check" size={13} /> : null}
                </View>
                <View style={styles.accentMeta}>
                  <Text style={styles.accentLabel}>{item.label}</Text>
                  <Text numberOfLines={1} style={styles.accentDesc}>{item.description}</Text>
                </View>
              </Pressable>
            );
          })}
        </View>
      </View>

      {/* 排版与对话展示 */}
      <View style={styles.card}>
        <Text style={styles.cardTitle}>正文字号与对话细节</Text>
        <Text style={styles.cardSub}>当前对话正文基准：{readingFontSize} px</Text>
        <View style={styles.fontRow}>
          {fontSizeChoices.map((size) => {
            const active = readingFontSize === size;
            return (
              <Pressable
                accessibilityRole="button"
                accessibilityState={{ selected: active }}
                key={size}
                onPress={() => updateAppearance({ readingFontSize: size })}
                style={({ pressed }) => [styles.fontChip, active && styles.fontChipActive, pressed && styles.pressed]}
              >
                <Text style={[styles.fontChipText, active && styles.fontChipTextActive]}>{size}</Text>
              </Pressable>
            );
          })}
        </View>

        <View style={styles.previewBubbleWrap}>
          <View style={styles.previewUserBubble}>
            <Text style={[styles.previewBubbleText, { fontSize: readingFontSize }]}>帮我排查 Agent 重连间隔并整理验证步骤</Text>
          </View>
          <Text style={[styles.previewAssistantText, { fontSize: readingFontSize }]}>
            已定位退避逻辑，以下是具体修复方案与变更摘要。
          </Text>
        </View>

        <View style={styles.divider} />

        <View style={styles.switchRow}>
          <View style={styles.switchTextCol}>
            <Text style={styles.switchTitle}>主题色用户气泡</Text>
            <Text style={styles.switchSub}>关闭为中性灰底，开启后以品牌强调色淡底凸显我的发言</Text>
          </View>
          <Switch
            onValueChange={(value) => updateAppearance({ highlightUserBubble: value })}
            thumbColor={highlightUserBubble ? palette.brand : palette.ink3}
            trackColor={{ false: palette.surface3, true: palette.brandWashStrong }}
            value={highlightUserBubble}
          />
        </View>

        <View style={styles.divider} />

        <View style={styles.switchRow}>
          <View style={styles.switchTextCol}>
            <Text style={styles.switchTitle}>思考过程默认保持展开</Text>
            <Text style={styles.switchSub}>开启后推理完成不自动折叠，便于随时回看思路</Text>
          </View>
          <Switch
            onValueChange={(value) => updateAppearance({ keepThinkingExpanded: value })}
            thumbColor={keepThinkingExpanded ? palette.brand : palette.ink3}
            trackColor={{ false: palette.surface3, true: palette.brandWashStrong }}
            value={keepThinkingExpanded}
          />
        </View>
      </View>

      <Pressable onPress={resetAppearance} style={({ pressed }) => [styles.resetBtn, pressed && styles.pressed]}>
        <Feather color={palette.ink2} name="rotate-ccw" size={15} />
        <Text style={styles.resetText}>恢复默认外观设置</Text>
      </Pressable>
    </View>
  );
}

const makeStyles = ({ palette }: ReturnType<typeof useAppTheme>) =>
  StyleSheet.create({
    root: {
      gap: 14,
      paddingBottom: 18,
    },
    card: {
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.lg,
      borderWidth: 1,
      gap: 12,
      padding: 15,
    },
    cardTitle: {
      color: palette.ink1,
      fontSize: font.bodyLg,
      fontWeight: weight.semibold,
    },
    cardSub: {
      color: palette.ink2,
      fontSize: font.caption,
      marginTop: -6,
    },
    segmentedRow: {
      flexDirection: "row",
      gap: 8,
    },
    modeBtn: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      flex: 1,
      flexDirection: "row",
      gap: 6,
      justifyContent: "center",
      minHeight: 40,
      paddingHorizontal: 10,
    },
    modeBtnActive: {
      backgroundColor: palette.brand,
      borderColor: palette.brand,
    },
    modeBtnText: {
      color: palette.ink2,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    modeBtnTextActive: {
      color: palette.inkInverse,
    },
    accentGrid: {
      flexDirection: "row",
      flexWrap: "wrap",
      gap: 8,
    },
    accentCard: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      flexBasis: "48%",
      flexDirection: "row",
      flexGrow: 1,
      gap: 10,
      padding: 10,
    },
    accentCardActive: {
      backgroundColor: palette.brandWash,
      borderColor: palette.brandDeep,
    },
    swatchCircle: {
      alignItems: "center",
      borderRadius: radius.pill,
      height: 26,
      justifyContent: "center",
      width: 26,
    },
    accentMeta: {
      flex: 1,
      minWidth: 0,
    },
    accentLabel: {
      color: palette.ink1,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    accentDesc: {
      color: palette.ink3,
      fontSize: 11,
      marginTop: 1,
    },
    fontRow: {
      flexDirection: "row",
      gap: 8,
    },
    fontChip: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      flex: 1,
      height: 36,
      justifyContent: "center",
    },
    fontChipActive: {
      backgroundColor: palette.brandWashStrong,
      borderColor: palette.brand,
    },
    fontChipText: {
      color: palette.ink2,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    fontChipTextActive: {
      color: palette.brandLight,
    },
    previewBubbleWrap: {
      backgroundColor: palette.canvas,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      gap: 8,
      padding: 12,
    },
    previewUserBubble: {
      alignSelf: "flex-end",
      backgroundColor: palette.userBubbleBg,
      borderColor: palette.userBubbleBorder,
      borderRadius: radius.xl,
      borderWidth: 1,
      maxWidth: "90%",
      paddingHorizontal: 14,
      paddingVertical: 8,
    },
    previewBubbleText: {
      color: palette.ink1,
      lineHeight: 22,
    },
    previewAssistantText: {
      color: palette.ink1,
      lineHeight: 22,
    },
    divider: {
      backgroundColor: palette.hairline,
      height: 1,
    },
    switchRow: {
      alignItems: "center",
      flexDirection: "row",
      gap: 12,
    },
    switchTextCol: {
      flex: 1,
      gap: 2,
    },
    switchTitle: {
      color: palette.ink1,
      fontSize: font.body,
      fontWeight: weight.semibold,
    },
    switchSub: {
      color: palette.ink2,
      fontSize: font.caption,
      lineHeight: 17,
    },
    resetBtn: {
      alignItems: "center",
      backgroundColor: palette.surface1,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      flexDirection: "row",
      gap: 8,
      justifyContent: "center",
      minHeight: 44,
    },
    resetText: {
      color: palette.ink2,
      fontSize: font.body,
      fontWeight: weight.medium,
    },
    pressed: {
      opacity: 0.78,
    },
  });
