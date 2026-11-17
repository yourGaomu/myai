import { useState } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Switch, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";

import {
  accentOptions,
  font,
  radius,
  weight,
  type AccentPreset,
  type ThemeMode,
} from "../../../constants/theme";
import { useWallpaper } from "../../../hooks/useWallpaperConfig";
import { useAppTheme, useThemedStyles } from "../../../theme/ThemeProvider";
import {
  formatWallpaperSize,
  WALLPAPER_PRESETS,
  type WallpaperPresetId,
} from "../../../utils/wallpaperRegistry";
import { ChatWallpaperLayer } from "../../chat/ChatWallpaperLayer";

const themeModes: Array<{ icon: "moon" | "sun" | "smartphone"; key: ThemeMode; label: string }> = [
  { key: "dark", label: "深色", icon: "moon" },
  { key: "light", label: "浅色", icon: "sun" },
  { key: "system", label: "跟随系统", icon: "smartphone" },
];

const fontSizeChoices = [14, 15, 16, 17, 18];
const blurChoices = [
  { label: "无", value: 0 },
  { label: "轻柔", value: 16 },
  { label: "适中", value: 36 },
  { label: "深度", value: 64 },
];
const dimChoices = [
  { label: "通透", value: 0.2 },
  { label: "平衡", value: 0.42 },
  { label: "护眼", value: 0.6 },
];

export function AppearanceSettings() {
  const theme = useAppTheme();
  const styles = useThemedStyles(makeStyles);
  const wallpaper = useWallpaper();
  const [wallpaperFeedback, setWallpaperFeedback] = useState<{ error?: boolean; text: string } | null>(null);
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

  const wp = wallpaper.settings;
  const activePreset = WALLPAPER_PRESETS.find((item) => item.id === wp.presetId);
  const wallpaperStatusLabel =
    wp.kind === "none"
      ? "默认纯色画布"
      : wp.kind === "preset"
        ? `预设 · ${activePreset?.label || "渐变"}`
        : `${wp.kind === "video" ? "视频壁纸" : "图片/动图"} · ${wp.fileName || "本地媒体"}${wp.fileSize ? ` (${formatWallpaperSize(wp.fileSize)})` : ""}`;

  const handlePickWallpaper = async (filter: "video" | "image") => {
    setWallpaperFeedback(null);
    const result = await wallpaper.pickAndSaveWallpaper(filter);
    if (result.ok) {
      setWallpaperFeedback({ text: filter === "video" ? "已启用 MP4/视频对话壁纸。" : "已启用图片/动图对话壁纸。" });
    } else if (result.error) {
      setWallpaperFeedback({ error: true, text: result.error });
    }
  };

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

      {/* 对话背景与动态壁纸 */}
      <View style={styles.card}>
        <View style={styles.cardHeaderRow}>
          <View style={styles.cardTitleCol}>
            <Text style={styles.cardTitle}>对话背景壁纸</Text>
            <Text style={styles.cardSub}>支持本地 .mp4 视频、GIF/WebP 动图、高清静图或氛围渐变</Text>
          </View>
          {wallpaper.isWallpaperActive ? (
            <Pressable
              onPress={() => {
                setWallpaperFeedback(null);
                wallpaper.clearWallpaper();
              }}
              style={({ pressed }) => [styles.clearChip, pressed && styles.pressed]}
            >
              <Text style={styles.clearChipText}>清除</Text>
            </Pressable>
          ) : null}
        </View>

        <View style={styles.wallpaperPreviewBox}>
          <ChatWallpaperLayer active />
          <View style={styles.wallpaperPreviewInner}>
            <View style={styles.previewUserBubble}>
              <Text style={styles.previewBubbleSmallText}>Wallpaper 视频背景测试</Text>
            </View>
            <View
              style={[
                styles.wallpaperPreviewReply,
                wallpaper.isWallpaperActive && wp.cardBackdrop && styles.wallpaperPreviewReplyGlass,
              ]}
            >
              <Text style={styles.wallpaperPreviewReplyText}>
                回复正文支持磨砂底衬与遮罩调节，在动态视频上方依然保持清晰可读。
              </Text>
            </View>
            <View style={styles.wallpaperBadge}>
              <View style={[styles.wallpaperBadgeDot, { backgroundColor: wallpaper.isWallpaperActive ? palette.brand : palette.ink4 }]} />
              <Text numberOfLines={1} style={styles.wallpaperBadgeText}>{wallpaperStatusLabel}</Text>
            </View>
          </View>
        </View>

        <View style={styles.presetGrid}>
          <Pressable
            accessibilityRole="button"
            accessibilityState={{ selected: wp.kind === "none" }}
            onPress={() => {
              setWallpaperFeedback(null);
              wallpaper.clearWallpaper();
            }}
            style={({ pressed }) => [styles.presetChip, wp.kind === "none" && styles.presetChipActive, pressed && styles.pressed]}
          >
            <View style={[styles.presetSwatch, { backgroundColor: palette.canvas, borderColor: palette.hairlineStrong }]} />
            <Text style={[styles.presetLabel, wp.kind === "none" && styles.presetLabelActive]}>默认纯色</Text>
          </Pressable>
          {WALLPAPER_PRESETS.map((preset) => {
            const active = wp.kind === "preset" && wp.presetId === preset.id;
            return (
              <Pressable
                accessibilityRole="button"
                accessibilityState={{ selected: active }}
                key={preset.id}
                onPress={() => {
                  setWallpaperFeedback(null);
                  wallpaper.selectPreset(preset.id as WallpaperPresetId);
                }}
                style={({ pressed }) => [styles.presetChip, active && styles.presetChipActive, pressed && styles.pressed]}
              >
                <View style={[styles.presetSwatch, { backgroundColor: isDark ? preset.swatchDark : preset.swatchLight }]} />
                <Text style={[styles.presetLabel, active && styles.presetLabelActive]}>{preset.label}</Text>
              </Pressable>
            );
          })}
        </View>

        <View style={styles.uploadRow}>
          <Pressable
            disabled={wallpaper.busy}
            onPress={() => void handlePickWallpaper("video")}
            style={({ pressed }) => [styles.uploadBtnPrimary, wallpaper.busy && styles.disabledBtn, pressed && styles.pressed]}
          >
            {wallpaper.busy ? (
              <ActivityIndicator color={palette.inkInverse} size="small" />
            ) : (
              <Feather color={palette.inkInverse} name="video" size={15} />
            )}
            <Text style={styles.uploadBtnPrimaryText}>导入 MP4 视频</Text>
          </Pressable>
          <Pressable
            disabled={wallpaper.busy}
            onPress={() => void handlePickWallpaper("image")}
            style={({ pressed }) => [styles.uploadBtnSecondary, wallpaper.busy && styles.disabledBtn, pressed && styles.pressed]}
          >
            <Feather color={palette.ink1} name="image" size={15} />
            <Text style={styles.uploadBtnSecondaryText}>导入图片 / GIF</Text>
          </Pressable>
        </View>

        <Text style={styles.wallpaperHintText}>
          本地导入 MP4/MOV/WebM（上限 50 MB）或 PNG/JPG/GIF/WebP（上限 20 MB）；视频默认静音循环且不打断后台音乐。
        </Text>

        {wallpaperFeedback ? (
          <View style={[styles.feedbackBox, wallpaperFeedback.error ? styles.feedbackErr : styles.feedbackOk]}>
            <Text style={[styles.feedbackText, wallpaperFeedback.error ? styles.feedbackErrText : styles.feedbackOkText]}>
              {wallpaperFeedback.text}
            </Text>
          </View>
        ) : null}

        {wallpaper.isWallpaperActive ? (
          <View style={styles.tuneSection}>
            <View style={styles.divider} />

            <View style={styles.tuneBlock}>
              <View style={styles.tuneHeaderRow}>
                <Text style={styles.tuneLabel}>护眼遮罩浓度</Text>
                <View style={styles.stepRow}>
                  <Pressable
                    onPress={() => wallpaper.updateWallpaper({ dimOpacity: wp.dimOpacity - 0.08 })}
                    style={({ pressed }) => [styles.stepBtn, pressed && styles.pressed]}
                  >
                    <Feather color={palette.ink1} name="minus" size={13} />
                  </Pressable>
                  <Text style={styles.tuneValue}>{Math.round(wp.dimOpacity * 100)}%</Text>
                  <Pressable
                    onPress={() => wallpaper.updateWallpaper({ dimOpacity: wp.dimOpacity + 0.08 })}
                    style={({ pressed }) => [styles.stepBtn, pressed && styles.pressed]}
                  >
                    <Feather color={palette.ink1} name="plus" size={13} />
                  </Pressable>
                </View>
              </View>
              <View style={styles.choiceRow}>
                {dimChoices.map((item) => {
                  const active = Math.abs(wp.dimOpacity - item.value) < 0.05;
                  return (
                    <Pressable
                      key={item.label}
                      onPress={() => wallpaper.updateWallpaper({ dimOpacity: item.value })}
                      style={({ pressed }) => [styles.choiceChip, active && styles.choiceChipActive, pressed && styles.pressed]}
                    >
                      <Text style={[styles.choiceText, active && styles.choiceTextActive]}>{item.label}</Text>
                    </Pressable>
                  );
                })}
              </View>
            </View>

            <View style={styles.tuneBlock}>
              <View style={styles.tuneHeaderRow}>
                <Text style={styles.tuneLabel}>背景高斯模糊</Text>
                <Text style={styles.tuneValue}>{wp.blurAmount === 0 ? "关闭" : `强度 ${wp.blurAmount}`}</Text>
              </View>
              <View style={styles.choiceRow}>
                {blurChoices.map((item) => {
                  const active =
                    item.value === 0 ? wp.blurAmount === 0 : Math.abs(wp.blurAmount - item.value) <= 10;
                  return (
                    <Pressable
                      key={item.label}
                      onPress={() => wallpaper.updateWallpaper({ blurAmount: item.value })}
                      style={({ pressed }) => [styles.choiceChip, active && styles.choiceChipActive, pressed && styles.pressed]}
                    >
                      <Text style={[styles.choiceText, active && styles.choiceTextActive]}>{item.label}</Text>
                    </Pressable>
                  );
                })}
              </View>
            </View>

            {wp.kind === "image" || wp.kind === "video" ? (
              <View style={styles.tuneBlock}>
                <View style={styles.tuneHeaderRow}>
                  <Text style={styles.tuneLabel}>画面填充方式</Text>
                </View>
                <View style={styles.choiceRow}>
                  {(
                    [
                      { key: "cover", label: "铺满裁切" },
                      { key: "contain", label: "完整居中" },
                    ] as const
                  ).map((item) => {
                    const active = wp.resizeMode === item.key;
                    return (
                      <Pressable
                        key={item.key}
                        onPress={() => wallpaper.updateWallpaper({ resizeMode: item.key })}
                        style={({ pressed }) => [styles.choiceChip, active && styles.choiceChipActive, pressed && styles.pressed]}
                      >
                        <Text style={[styles.choiceText, active && styles.choiceTextActive]}>{item.label}</Text>
                      </Pressable>
                    );
                  })}
                </View>
              </View>
            ) : null}

            <View style={styles.switchRow}>
              <View style={styles.switchTextCol}>
                <Text style={styles.switchTitle}>回复文字阅读底衬</Text>
                <Text style={styles.switchSub}>为 Agent 长回复叠加轻透磨砂衬底，避免动态背景花屏干扰阅读</Text>
              </View>
              <Switch
                onValueChange={(value) => wallpaper.updateWallpaper({ cardBackdrop: value })}
                thumbColor={wp.cardBackdrop ? palette.brand : palette.ink3}
                trackColor={{ false: palette.surface3, true: palette.brandWashStrong }}
                value={wp.cardBackdrop}
              />
            </View>
          </View>
        ) : null}
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
    cardHeaderRow: {
      alignItems: "flex-start",
      flexDirection: "row",
      gap: 10,
      justifyContent: "space-between",
    },
    cardTitleCol: {
      flex: 1,
      gap: 6,
    },
    clearChip: {
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      paddingHorizontal: 10,
      paddingVertical: 5,
    },
    clearChipText: {
      color: palette.ink2,
      fontSize: font.caption,
      fontWeight: weight.semibold,
    },
    wallpaperPreviewBox: {
      backgroundColor: palette.canvas,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.md,
      borderWidth: 1,
      height: 168,
      overflow: "hidden",
      position: "relative",
    },
    wallpaperPreviewInner: {
      flex: 1,
      gap: 8,
      justifyContent: "space-between",
      padding: 12,
    },
    previewBubbleSmallText: {
      color: palette.ink1,
      fontSize: font.label,
      lineHeight: 19,
    },
    wallpaperPreviewReply: {
      alignSelf: "stretch",
      borderRadius: radius.md,
      paddingHorizontal: 8,
      paddingVertical: 6,
    },
    wallpaperPreviewReplyGlass: {
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
      borderWidth: 1,
    },
    wallpaperPreviewReplyText: {
      color: palette.ink1,
      fontSize: font.caption,
      lineHeight: 18,
    },
    wallpaperBadge: {
      alignItems: "center",
      alignSelf: "flex-start",
      backgroundColor: palette.floatingGlass,
      borderColor: palette.floatingGlassBorder,
      borderRadius: radius.pill,
      borderWidth: 1,
      flexDirection: "row",
      gap: 6,
      maxWidth: "100%",
      paddingHorizontal: 10,
      paddingVertical: 4,
    },
    wallpaperBadgeDot: {
      borderRadius: radius.pill,
      height: 6,
      width: 6,
    },
    wallpaperBadgeText: {
      color: palette.ink2,
      fontSize: 11,
      fontWeight: weight.medium,
    },
    presetGrid: {
      flexDirection: "row",
      flexWrap: "wrap",
      gap: 8,
    },
    presetChip: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      flexBasis: "48%",
      flexDirection: "row",
      flexGrow: 1,
      gap: 8,
      minHeight: 40,
      paddingHorizontal: 10,
      paddingVertical: 7,
    },
    presetChipActive: {
      backgroundColor: palette.brandWash,
      borderColor: palette.brandDeep,
    },
    presetSwatch: {
      borderColor: "transparent",
      borderRadius: radius.pill,
      borderWidth: 1,
      height: 16,
      width: 16,
    },
    presetLabel: {
      color: palette.ink2,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    presetLabelActive: {
      color: palette.ink1,
    },
    uploadRow: {
      flexDirection: "row",
      gap: 8,
    },
    uploadBtnPrimary: {
      alignItems: "center",
      backgroundColor: palette.brand,
      borderRadius: radius.md,
      flex: 1,
      flexDirection: "row",
      gap: 7,
      justifyContent: "center",
      minHeight: 42,
      paddingHorizontal: 12,
    },
    uploadBtnPrimaryText: {
      color: palette.inkInverse,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    uploadBtnSecondary: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairlineStrong,
      borderRadius: radius.md,
      borderWidth: 1,
      flex: 1,
      flexDirection: "row",
      gap: 7,
      justifyContent: "center",
      minHeight: 42,
      paddingHorizontal: 12,
    },
    uploadBtnSecondaryText: {
      color: palette.ink1,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    disabledBtn: {
      opacity: 0.55,
    },
    wallpaperHintText: {
      color: palette.ink3,
      fontSize: 11.5,
      lineHeight: 17,
    },
    feedbackBox: {
      borderRadius: radius.sm,
      borderWidth: 1,
      paddingHorizontal: 10,
      paddingVertical: 8,
    },
    feedbackOk: {
      backgroundColor: palette.okWash,
      borderColor: palette.okBorder,
    },
    feedbackErr: {
      backgroundColor: palette.errWash,
      borderColor: palette.errBorder,
    },
    feedbackText: {
      fontSize: font.caption,
      fontWeight: weight.medium,
      lineHeight: 17,
    },
    feedbackOkText: {
      color: palette.ok,
    },
    feedbackErrText: {
      color: palette.err,
    },
    tuneSection: {
      gap: 12,
    },
    tuneBlock: {
      gap: 8,
    },
    tuneHeaderRow: {
      alignItems: "center",
      flexDirection: "row",
      justifyContent: "space-between",
    },
    tuneLabel: {
      color: palette.ink1,
      fontSize: font.label,
      fontWeight: weight.semibold,
    },
    tuneValue: {
      color: palette.ink2,
      fontSize: font.caption,
      fontWeight: weight.semibold,
      minWidth: 42,
      textAlign: "center",
    },
    stepRow: {
      alignItems: "center",
      flexDirection: "row",
      gap: 6,
    },
    stepBtn: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.pill,
      borderWidth: 1,
      height: 28,
      justifyContent: "center",
      width: 28,
    },
    choiceRow: {
      flexDirection: "row",
      gap: 8,
    },
    choiceChip: {
      alignItems: "center",
      backgroundColor: palette.surface2,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      flex: 1,
      height: 34,
      justifyContent: "center",
      paddingHorizontal: 8,
    },
    choiceChipActive: {
      backgroundColor: palette.brandWashStrong,
      borderColor: palette.brand,
    },
    choiceText: {
      color: palette.ink2,
      fontSize: font.caption,
      fontWeight: weight.semibold,
    },
    choiceTextActive: {
      color: palette.brandLight,
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
